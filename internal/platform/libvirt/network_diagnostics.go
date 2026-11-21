package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) NetworkDiagnostics(ctx context.Context, name string) (domain.NetworkDiagnostics, error) {
	snapshot, err := c.networkDiagnosticsSnapshot(ctx, name)
	if err != nil {
		return domain.NetworkDiagnostics{}, err
	}
	probeContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	record := snapshot.network
	result := domain.NetworkDiagnostics{
		Network: name, CheckedAt: time.Now().UTC(), Active: record.Active, Bridge: record.Bridge,
		MTU: record.MTU, DNSHostCount: len(record.DNSHosts),
		IPv4Forward: readForwarding("/proc/sys/net/ipv4/ip_forward"),
		IPv6Forward: readForwarding("/proc/sys/net/ipv6/conf/all/forwarding"), NATIPv6: record.NATIPv6,
		IPv6Configured: hasIPv6(record),
		LeaseCount:     snapshot.leaseCount, LibvirtVersion: snapshot.libvirtVersion,
		RuntimeDrift: len(snapshot.driftDetails) > 0, DriftDetails: snapshot.driftDetails,
	}
	dnsmasqRunning := dnsmasqRunningForNetwork(record.Name)
	result.DnsmasqVersion = commandVersion(probeContext, "dnsmasq", "--version")

	result.Checks = append(result.Checks, checkNetworkActive(record))
	result.Checks = append(result.Checks, checkNetworkCompatibility(result.LibvirtVersion, result.DnsmasqVersion))
	result.Checks = append(result.Checks, checkNetworkBridge(record, &result))
	result.Checks = append(result.Checks, checkNetworkMTU(record, result.BridgeMTU))
	result.Checks = append(result.Checks, checkNetworkForwarding(record, result.IPv4Forward, result.IPv6Forward))
	result.Checks = append(result.Checks, checkNetworkIPv6DHCP(record, &result))
	result.Checks = append(result.Checks, checkNetworkDHCPv6Service(record, dnsmasqRunning, &result))
	result.Checks = append(result.Checks, checkNetworkRAService(record, dnsmasqRunning, &result))

	if record.ForwardMode == "nat" {
		portForwards := inspectPortForwardFirewall(probeContext, record.Name)
		result.PortForwardRules = portForwards.count
		result.PortForwardReady = portForwards.ready
		result.PortForwardError = portForwards.error
		result.Checks = append(result.Checks, checkNetworkPortForwards(record, portForwards))
	}

	if record.ForwardMode == "nat" && record.NATIPv6 {
		result.Checks = append(result.Checks, checkNetworkNAT66(record, result.IPv6Forward))
		firewall := inspectNetworkFirewall(probeContext, record)
		result.FirewallBackend = firewall.backend
		result.NAT66Firewall = firewall.nat66Rule
		result.Checks = append(result.Checks, checkNetworkNAT66Firewall(record, firewall))
	}
	if record.DNSDomain != "" || len(record.DNSServers) > 0 || hasIPv6(record) {
		result.DNSService, result.DNSResolution = probeNetworkDNS(probeContext, record, dnsmasqRunning)
		result.Checks = append(result.Checks, checkNetworkDNS(record, result.DNSService, result.DNSResolution))
	}
	if address, ok := ipv6Address(record); ok {
		result.GatewayReachable = pingAddress(probeContext, address.Address)
		result.Checks = append(result.Checks, checkNetworkIPv6Gateway(record, result.GatewayReachable))
		if record.ForwardMode == "nat" {
			result.OutboundReachable = pingAddress(probeContext, "2606:4700:4700::1111")
			result.Checks = append(result.Checks, checkNetworkIPv6Outbound(record, result.OutboundReachable))
		}
	}
	result.Checks = append(result.Checks, checkNetworkRuntimeDrift(record, snapshot.driftDetails, snapshot.driftError))
	for _, warning := range record.Warnings {
		result.Checks = append(result.Checks, domain.NetworkCheck{ID: "warning", Status: "warning", Title: "配置警告", Detail: warning})
	}
	return result, nil
}

func checkNetworkPortForwards(network domain.VirtualNetwork, inspection portForwardInspection) domain.NetworkCheck {
	if inspection.error != "" {
		return domain.NetworkCheck{ID: "port-forward-firewall", Status: "warning", Title: "入站端口转发", Detail: "无法读取受控 nftables 规则：" + inspection.error + "。请确认 nft 可用且服务具有 CAP_NET_ADMIN。"}
	}
	if inspection.count > 0 {
		return domain.NetworkCheck{ID: "port-forward-firewall", Status: "ok", Title: "入站端口转发", Detail: fmt.Sprintf("受控 nftables 表已加载 %d 条 NAT 规则。", inspection.count)}
	}
	return domain.NetworkCheck{ID: "port-forward-firewall", Status: "ok", Title: "入站端口转发", Detail: "当前网络没有启用的入站端口转发规则。"}
}

func waitForNetworkIPv6Ready(ctx context.Context, network domain.VirtualNetwork) {
	address, ok := ipv6Address(network)
	if !network.Active || !ok {
		return
	}
	deadline := time.Now().Add(6 * time.Second)
	for {
		ready := bridgeHasAddress(network.Bridge, address.Address)
		if ready && network.ForwardMode == "nat" && network.NATIPv6 {
			firewall := inspectNetworkFirewall(ctx, network)
			ready = firewall.error != "" || firewall.nat66Rule
		}
		if ready {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}

type networkDiagnosticsSnapshot struct {
	network        domain.VirtualNetwork
	leaseCount     int
	libvirtVersion string
	driftDetails   []string
	driftError     string
}

func (c *Client) networkDiagnosticsSnapshot(ctx context.Context, name string) (networkDiagnosticsSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return networkDiagnosticsSnapshot{}, err
	}
	network, err := c.conn.LookupNetworkByName(name)
	if errors.Is(err, libvirt.ERR_NO_NETWORK) {
		return networkDiagnosticsSnapshot{}, domain.ErrNotFound
	}
	if err != nil {
		return networkDiagnosticsSnapshot{}, fmt.Errorf("lookup network for diagnostics: %w", err)
	}
	defer network.Free()
	record, err := networkFromLibvirt(network)
	if err != nil {
		return networkDiagnosticsSnapshot{}, err
	}
	result := networkDiagnosticsSnapshot{network: record, libvirtVersion: formatLibvirtVersion(c.conn)}
	if record.Active && record.ForwardMode != "bridge" {
		if leases, leaseErr := network.GetDHCPLeases(); leaseErr == nil {
			result.leaseCount = len(leases)
		}
	}
	result.driftDetails, result.driftError = networkRuntimeDrift(network, record)
	return result, nil
}

func checkNetworkActive(network domain.VirtualNetwork) domain.NetworkCheck {
	if network.Active {
		return domain.NetworkCheck{ID: "active", Status: "ok", Title: "网络状态", Detail: "libvirt 网络处于活跃状态。"}
	}
	return domain.NetworkCheck{ID: "active", Status: "warning", Title: "网络状态", Detail: "网络尚未启动，DHCPv6、RA 和静态租约不会生效。"}
}

func checkNetworkCompatibility(libvirtVersion, dnsmasqVersion string) domain.NetworkCheck {
	const minimumLibvirt = "8.0.0"
	const minimumDnsmasq = "2.85"
	libvirtValue, libvirtOK := parseVersion(libvirtVersion)
	dnsmasqValue, dnsmasqOK := parseVersion(dnsmasqVersion)
	if !libvirtOK || !dnsmasqOK {
		return domain.NetworkCheck{ID: "compatibility", Status: "warning", Title: "IPv6 兼容性", Detail: "无法完整识别 libvirt 或 dnsmasq 版本；商用部署应确保 libvirt >= 8.0.0、dnsmasq >= 2.85，并通过发布兼容矩阵。"}
	}
	if compareVersion(libvirtValue, [3]int{8, 0, 0}) < 0 || compareVersion(dnsmasqValue, [3]int{2, 85, 0}) < 0 {
		return domain.NetworkCheck{ID: "compatibility", Status: "error", Title: "IPv6 兼容性", Detail: fmt.Sprintf("libvirt %s、dnsmasq %s 低于受支持基线 libvirt %s、dnsmasq %s；请升级后再启用双栈/NAT66。", libvirtVersion, dnsmasqVersion, minimumLibvirt, minimumDnsmasq)}
	}
	return domain.NetworkCheck{ID: "compatibility", Status: "ok", Title: "IPv6 兼容性", Detail: fmt.Sprintf("libvirt %s、dnsmasq %s 满足受支持基线 libvirt %s、dnsmasq %s。", libvirtVersion, dnsmasqVersion, minimumLibvirt, minimumDnsmasq)}
}

func checkNetworkBridge(network domain.VirtualNetwork, result *domain.NetworkDiagnostics) domain.NetworkCheck {
	if network.Bridge == "" {
		return domain.NetworkCheck{ID: "bridge", Status: "error", Title: "桥接口", Detail: "网络定义中没有桥接口名称。"}
	}
	iface, err := net.InterfaceByName(network.Bridge)
	if err != nil {
		return domain.NetworkCheck{ID: "bridge", Status: "error", Title: "桥接口", Detail: "无法找到桥接口 " + network.Bridge + "：" + err.Error()}
	}
	result.BridgeReady = true
	result.BridgeMTU = iface.MTU
	addresses, _ := iface.Addrs()
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, address.String())
	}
	return domain.NetworkCheck{ID: "bridge", Status: "ok", Title: "桥接口", Detail: fmt.Sprintf("桥接口 %s 已就绪，地址：%s。", network.Bridge, defaultDiagnosticValue(strings.Join(values, ", "), "无（等待 RA/DHCPv6）"))}
}

func checkNetworkMTU(network domain.VirtualNetwork, bridgeMTU int) domain.NetworkCheck {
	if !network.Active {
		detail := "网络未激活；"
		if network.MTU > 0 {
			detail += fmt.Sprintf("持久配置 MTU %d，启动后校验桥接口。", network.MTU)
		} else {
			detail += "未固定 MTU，启动后使用 libvirt/上游默认值。"
		}
		return domain.NetworkCheck{ID: "mtu", Status: "ok", Title: "网络 MTU", Detail: detail}
	}
	if bridgeMTU <= 0 {
		return domain.NetworkCheck{ID: "mtu", Status: "warning", Title: "网络 MTU", Detail: "无法读取桥接口 MTU；请确认桥接口仍存在且诊断进程有权限读取网络接口。"}
	}
	if network.MTU > 0 && bridgeMTU != network.MTU {
		return domain.NetworkCheck{ID: "mtu", Status: "error", Title: "网络 MTU", Detail: fmt.Sprintf("持久配置 MTU %d，桥接口实际 MTU %d；重启网络或检查上游配置。", network.MTU, bridgeMTU)}
	}
	if network.MTU > 0 {
		return domain.NetworkCheck{ID: "mtu", Status: "ok", Title: "网络 MTU", Detail: fmt.Sprintf("桥接口实际 MTU %d，与网络配置一致。", bridgeMTU)}
	}
	return domain.NetworkCheck{ID: "mtu", Status: "ok", Title: "网络 MTU", Detail: fmt.Sprintf("未固定 MTU，桥接口使用 %d。", bridgeMTU)}
}

func checkNetworkForwarding(network domain.VirtualNetwork, ipv4, ipv6 bool) domain.NetworkCheck {
	if network.ForwardMode != "nat" && network.ForwardMode != "isolated" {
		return domain.NetworkCheck{ID: "forwarding", Status: "ok", Title: "转发策略", Detail: "外部桥接网络使用上游转发策略。"}
	}
	if network.ForwardMode == "isolated" {
		return domain.NetworkCheck{ID: "forwarding", Status: "ok", Title: "转发策略", Detail: "隔离网络不执行宿主转发。"}
	}
	if hasIPv4(network) && !ipv4 {
		return domain.NetworkCheck{ID: "forwarding", Status: "error", Title: "IPv4 转发", Detail: "NAT 网络需要宿主机 IPv4 转发，但 net.ipv4.ip_forward=0。"}
	}
	if hasIPv6(network) && !ipv6 {
		return domain.NetworkCheck{ID: "forwarding", Status: "error", Title: "IPv6 转发", Detail: "NAT 网络需要宿主机 IPv6 转发，但 net.ipv6.conf.all.forwarding=0。"}
	}
	return domain.NetworkCheck{ID: "forwarding", Status: "ok", Title: "转发策略", Detail: "宿主机转发开关满足当前网络模式。"}
}

func checkNetworkIPv6DHCP(network domain.VirtualNetwork, result *domain.NetworkDiagnostics) domain.NetworkCheck {
	address, ok := ipv6Address(network)
	if !ok {
		return domain.NetworkCheck{ID: "dhcpv6", Status: "ok", Title: "IPv6 地址分配", Detail: "网络未配置 IPv6。"}
	}
	if address.IPv6Mode != "dhcpv6" {
		return domain.NetworkCheck{ID: "dhcpv6", Status: "ok", Title: "IPv6 地址分配", Detail: "使用 RA/SLAAC，不提供有状态 DHCPv6。"}
	}
	result.DHCPv6Range = address.DHCPStart != "" && address.DHCPEnd != ""
	if !result.DHCPv6Range {
		return domain.NetworkCheck{ID: "dhcpv6", Status: "error", Title: "DHCPv6 地址池", Detail: "DHCPv6 模式缺少可用的地址范围。"}
	}
	return domain.NetworkCheck{ID: "dhcpv6", Status: "ok", Title: "DHCPv6 地址池", Detail: fmt.Sprintf("DHCPv6 范围 %s - %s，当前租约 %d 条。", address.DHCPStart, address.DHCPEnd, result.LeaseCount)}
}

func checkNetworkDHCPv6Service(network domain.VirtualNetwork, running bool, result *domain.NetworkDiagnostics) domain.NetworkCheck {
	address, ok := ipv6Address(network)
	if !ok || address.IPv6Mode != "dhcpv6" {
		return domain.NetworkCheck{ID: "dhcpv6-service", Status: "ok", Title: "DHCPv6 服务", Detail: "当前网络不使用有状态 DHCPv6。"}
	}
	if !network.Active {
		return domain.NetworkCheck{ID: "dhcpv6-service", Status: "warning", Title: "DHCPv6 服务", Detail: "网络未激活，DHCPv6 服务不会启动。"}
	}
	result.DHCPv6Service = running
	if !running {
		return domain.NetworkCheck{ID: "dhcpv6-service", Status: "error", Title: "DHCPv6 服务", Detail: "未找到该网络的 dnsmasq/DHCPv6 进程；请重启 libvirt 网络并检查 dnsmasq 日志。"}
	}
	return domain.NetworkCheck{ID: "dhcpv6-service", Status: "ok", Title: "DHCPv6 服务", Detail: "已发现该网络的 dnsmasq 进程，DHCPv6 服务处于运行状态。"}
}

func checkNetworkRAService(network domain.VirtualNetwork, running bool, result *domain.NetworkDiagnostics) domain.NetworkCheck {
	if !hasIPv6(network) {
		return domain.NetworkCheck{ID: "ra", Status: "ok", Title: "RA 服务", Detail: "网络未配置 IPv6。"}
	}
	if !network.Active {
		return domain.NetworkCheck{ID: "ra", Status: "warning", Title: "RA 服务", Detail: "网络未激活，RA 不会发布。"}
	}
	result.RAService = running
	if !running {
		return domain.NetworkCheck{ID: "ra", Status: "error", Title: "RA 服务", Detail: "未找到该网络的 dnsmasq/RA 进程；SLAAC 客户端无法获取前缀和默认路由。"}
	}
	return domain.NetworkCheck{ID: "ra", Status: "ok", Title: "RA 服务", Detail: "已发现该网络的 dnsmasq 进程，RA/SLAAC 服务处于运行状态。"}
}

func checkNetworkNAT66(network domain.VirtualNetwork, ipv6Forward bool) domain.NetworkCheck {
	status := "ok"
	detail := "libvirt 已启用 IPv6 NAT，guest 流量可由宿主机转换后转发。"
	if !ipv6Forward {
		status = "error"
		detail = "NAT66 已启用，但宿主机 net.ipv6.conf.all.forwarding=0；请启用 IPv6 转发后重启网络。"
	}
	return domain.NetworkCheck{ID: "nat66", Status: status, Title: "IPv6 NAT 转发", Detail: detail}
}

func checkNetworkNAT66Firewall(network domain.VirtualNetwork, firewall firewallInspection) domain.NetworkCheck {
	if firewall.error != "" {
		return domain.NetworkCheck{ID: "nat66-firewall", Status: "warning", Title: "NAT66 防火墙规则", Detail: "无法读取宿主防火墙规则：" + firewall.error + "。请授予服务 CAP_NET_ADMIN，或从宿主机运行 IPv6 兼容性检查。"}
	}
	if firewall.nat66Rule {
		return domain.NetworkCheck{ID: "nat66-firewall", Status: "ok", Title: "NAT66 防火墙规则", Detail: fmt.Sprintf("%s 已包含 %s 的 IPv6 masquerade 规则。", defaultDiagnosticValue(firewall.backend, "宿主防火墙"), firewall.prefix)}
	}
	return domain.NetworkCheck{ID: "nat66-firewall", Status: "error", Title: "NAT66 防火墙规则", Detail: fmt.Sprintf("%s 中未找到 %s 的 IPv6 masquerade 规则；IPv6 客户端可能无法出网。", defaultDiagnosticValue(firewall.backend, "宿主防火墙"), defaultDiagnosticValue(firewall.prefix, "当前 IPv6 前缀"))}
}

func checkNetworkDNS(network domain.VirtualNetwork, service, resolution bool) domain.NetworkCheck {
	if !service {
		return domain.NetworkCheck{ID: "dns", Status: "warning", Title: "IPv6 DNS 服务", Detail: "未发现可用的网络 DNS 服务；请确认 dnsmasq 运行且 DNS domain/forwarder 配置有效。"}
	}
	detail := fmt.Sprintf("DNS 域 %s，上游服务器 %s；网络 DNS 服务可访问。", defaultDiagnosticValue(network.DNSDomain, "未设置"), defaultDiagnosticValue(strings.Join(network.DNSServers, ", "), "使用宿主机默认"))
	if !resolution {
		if !bridgeConnected(network.Bridge) {
			return domain.NetworkCheck{ID: "dns", Status: "warning", Title: "IPv6 DNS 服务", Detail: detail + " 网络当前没有连接端口，待客户机接入后再完成本地查询探测。"}
		}
		return domain.NetworkCheck{ID: "dns", Status: "warning", Title: "IPv6 DNS 服务", Detail: detail + " 但测试查询未获得响应，请检查防火墙和上游 DNS。"}
	}
	return domain.NetworkCheck{ID: "dns", Status: "ok", Title: "IPv6 DNS 服务", Detail: detail + " 测试查询已获得 DNS 响应。"}
}

func checkNetworkIPv6Gateway(network domain.VirtualNetwork, reachable bool) domain.NetworkCheck {
	address, _ := ipv6Address(network)
	if !network.Active {
		return domain.NetworkCheck{ID: "gateway", Status: "warning", Title: "IPv6 网关", Detail: "网络未激活，未执行网关探测。"}
	}
	if !reachable {
		if !bridgeConnected(network.Bridge) {
			return domain.NetworkCheck{ID: "gateway", Status: "warning", Title: "IPv6 网关", Detail: fmt.Sprintf("网络当前没有连接端口，IPv6 网关 %s 将在客户机接入后完成邻居发现。", address.Address)}
		}
		return domain.NetworkCheck{ID: "gateway", Status: "error", Title: "IPv6 网关", Detail: fmt.Sprintf("无法从宿主机 ping 通 IPv6 网关 %s，请检查桥接口和 RA 地址。", address.Address)}
	}
	return domain.NetworkCheck{ID: "gateway", Status: "ok", Title: "IPv6 网关", Detail: fmt.Sprintf("宿主机可访问 IPv6 网关 %s。", address.Address)}
}

func checkNetworkIPv6Outbound(network domain.VirtualNetwork, reachable bool) domain.NetworkCheck {
	if !network.Active {
		return domain.NetworkCheck{ID: "outbound", Status: "warning", Title: "宿主机 IPv6 出网", Detail: "网络未激活，未执行出网探测。"}
	}
	if !reachable {
		return domain.NetworkCheck{ID: "outbound", Status: "warning", Title: "宿主机 IPv6 出网", Detail: "宿主机无法访问公共 IPv6 测试地址；可能是上游网络不支持 IPv6、DNS/路由被阻断或防火墙未放行。NAT66 入站不在当前产品范围。"}
	}
	return domain.NetworkCheck{ID: "outbound", Status: "ok", Title: "宿主机 IPv6 出网", Detail: "宿主机可访问公共 IPv6 测试地址；NAT66 仅承诺客户机出站访问，不提供入站端口映射。"}
}

func checkNetworkRuntimeDrift(network domain.VirtualNetwork, details []string, driftError string) domain.NetworkCheck {
	if !network.Active || !network.Persistent {
		return domain.NetworkCheck{ID: "drift", Status: "ok", Title: "运行态配置漂移", Detail: "网络未同时处于活跃和持久状态，不适用漂移检查。"}
	}
	if driftError != "" {
		return domain.NetworkCheck{ID: "drift", Status: "warning", Title: "运行态配置漂移", Detail: "无法比较持久配置与运行态配置：" + driftError}
	}
	if len(details) > 0 {
		return domain.NetworkCheck{ID: "drift", Status: "warning", Title: "运行态配置漂移", Detail: "持久配置与运行态不一致：" + strings.Join(details, "；") + "。请停止并重新启动网络以应用持久配置。"}
	}
	return domain.NetworkCheck{ID: "drift", Status: "ok", Title: "运行态配置漂移", Detail: "持久配置与 libvirt 运行态一致。"}
}

type firewallInspection struct {
	backend   string
	prefix    string
	nat66Rule bool
	error     string
}

func inspectNetworkFirewall(ctx context.Context, network domain.VirtualNetwork) firewallInspection {
	address, ok := ipv6Address(network)
	result := firewallInspection{}
	if !ok {
		return result
	}
	prefix, err := netip.ParseAddr(address.Address)
	if err != nil {
		return result
	}
	result.prefix = netip.PrefixFrom(prefix, address.Prefix).Masked().String()
	if path, err := exec.LookPath("nft"); err == nil {
		output, commandErr := commandOutput(ctx, path, "list", "ruleset")
		result.backend = "nftables"
		if commandErr == nil {
			result.nat66Rule = containsNAT66Rule(output, result.prefix, false)
			return result
		}
		result.error = commandErr.Error()
	}
	if path, err := exec.LookPath("ip6tables"); err == nil {
		output, commandErr := commandOutput(ctx, path, "-t", "nat", "-S")
		result.backend = "ip6tables"
		if commandErr != nil {
			if result.error == "" {
				result.error = commandErr.Error()
			}
			return result
		}
		result.error = ""
		result.nat66Rule = containsNAT66Rule(output, result.prefix, true)
		return result
	}
	if result.error == "" {
		result.error = "未找到 nftables 或 ip6tables"
	}
	return result
}

func containsNAT66Rule(rules, prefix string, legacy bool) bool {
	for _, line := range strings.Split(rules, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if lower == "" || !strings.Contains(lower, prefix) {
			continue
		}
		if legacy {
			if strings.Contains(lower, "masquerade") && strings.Contains(lower, "-s "+prefix) {
				return true
			}
			continue
		}
		if strings.Contains(lower, "masquerade") && strings.Contains(lower, "ip6 saddr "+prefix) {
			return true
		}
	}
	return false
}

func bridgeHasAddress(name, address string) bool {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return false
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return false
	}
	target, err := netip.ParseAddr(address)
	if err != nil {
		return false
	}
	for _, value := range addresses {
		prefix, err := netip.ParsePrefix(value.String())
		if err == nil && prefix.Addr() == target {
			return true
		}
	}
	return false
}

func bridgeConnected(name string) bool {
	iface, err := net.InterfaceByName(name)
	return err == nil && iface.Flags&net.FlagRunning != 0
}

func probeNetworkDNS(ctx context.Context, network domain.VirtualNetwork, dnsmasqRunning bool) (bool, bool) {
	if !dnsmasqRunning {
		return false, false
	}
	servers := make([]string, 0, 2)
	if address, ok := ipv6Address(network); ok {
		servers = append(servers, address.Address)
	}
	for _, address := range network.Addresses {
		if networkAddressFamily(address) == "ipv4" {
			servers = append(servers, address.Address)
		}
	}
	if len(servers) == 0 {
		return true, false
	}
	for _, server := range servers {
		resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: 1500 * time.Millisecond}
			return dialer.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
		}}
		lookupContext, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, err := resolver.LookupTXT(lookupContext, "virtmgr-diagnostics.invalid")
		cancel()
		if err == nil || isDNSNotFound(err) {
			return true, true
		}
	}
	return true, false
}

func isDNSNotFound(err error) bool {
	var dnsError *net.DNSError
	return errors.As(err, &dnsError) && dnsError.IsNotFound
}

func pingAddress(ctx context.Context, address string) bool {
	path, err := exec.LookPath("ping")
	if err != nil {
		return false
	}
	args := []string{"-c", "1", "-W", "2"}
	if strings.Contains(address, ":") {
		args = append([]string{"-6"}, args...)
	}
	args = append(args, address)
	_, err = commandOutput(ctx, path, args...)
	return err == nil
}

func dnsmasqRunningForNetwork(name string) bool {
	target := "/var/lib/libvirt/dnsmasq/" + name + ".conf"
	processes, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, process := range processes {
		if !process.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(process.Name()); err != nil {
			continue
		}
		commandLine, err := os.ReadFile("/proc/" + process.Name() + "/cmdline")
		if err != nil {
			continue
		}
		for _, argument := range strings.Split(string(commandLine), "\x00") {
			if strings.Contains(argument, target) {
				return true
			}
		}
	}
	return false
}

func formatLibvirtVersion(connection *libvirt.Connect) string {
	value, err := connection.GetLibVersion()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d", value/1_000_000, (value/1_000)%1_000, value%1_000)
}

func commandVersion(ctx context.Context, name string, args ...string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	output, err := commandOutput(ctx, path, args...)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	return line
}

func parseVersion(value string) ([3]int, bool) {
	for _, field := range strings.Fields(value) {
		field = strings.TrimSpace(strings.TrimPrefix(field, "v"))
		parts := strings.Split(field, ".")
		if len(parts) < 2 {
			continue
		}
		var result [3]int
		valid := true
		for index := 0; index < len(parts) && index < 3; index++ {
			digits := parts[index]
			end := 0
			for end < len(digits) && digits[end] >= '0' && digits[end] <= '9' {
				end++
			}
			if end == 0 {
				valid = false
				break
			}
			number, err := strconv.Atoi(digits[:end])
			if err != nil {
				valid = false
				break
			}
			result[index] = number
		}
		if valid {
			return result, true
		}
	}
	return [3]int{}, false
}

func compareVersion(left, right [3]int) int {
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}

func commandOutput(ctx context.Context, path string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, path, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return "", errors.New(message)
	}
	return string(output), nil
}

func networkRuntimeDrift(network *libvirt.Network, active domain.VirtualNetwork) ([]string, string) {
	if !active.Active || !active.Persistent {
		return nil, ""
	}
	activeXML, err := network.GetXMLDesc(0)
	if err != nil {
		return nil, err.Error()
	}
	persistentXML, err := network.GetXMLDesc(libvirt.NETWORK_XML_INACTIVE)
	if err != nil {
		return nil, err.Error()
	}
	activeConfig, err := parseManagedNetworkConfig(activeXML)
	if err != nil {
		return nil, err.Error()
	}
	persistentConfig, err := parseManagedNetworkConfig(persistentXML)
	if err != nil {
		return nil, err.Error()
	}
	return compareManagedNetworkConfig(activeConfig, persistentConfig), ""
}

type managedNetworkConfig struct {
	Bridge     string   `json:"bridge"`
	Forward    string   `json:"forward"`
	NATIPv6    bool     `json:"natIpv6"`
	MTU        int      `json:"mtu"`
	DNSDomain  string   `json:"dnsDomain"`
	DNSServers []string `json:"dnsServers"`
	DNSHosts   []string `json:"dnsHosts"`
	Addresses  []string `json:"addresses"`
}

func parseManagedNetworkConfig(xmlText string) (managedNetworkConfig, error) {
	var parsed networkXML
	if err := xml.Unmarshal([]byte(xmlText), &parsed); err != nil {
		return managedNetworkConfig{}, err
	}
	result := managedNetworkConfig{
		Bridge: parsed.Bridge.Name, Forward: parsed.Forward.Mode,
		NATIPv6: parsed.Forward.NAT != nil && parsed.Forward.NAT.IPv6 == "yes",
		MTU:     networkMTU(parsed),
	}
	if parsed.Domain != nil {
		result.DNSDomain = strings.ToLower(parsed.Domain.Name)
	}
	if parsed.DNS != nil {
		for _, forwarder := range parsed.DNS.Forwarders {
			result.DNSServers = append(result.DNSServers, forwarder.Addr)
		}
		for _, host := range parsed.DNS.Hosts {
			names := append([]string(nil), host.Hostnames...)
			sort.Strings(names)
			result.DNSHosts = append(result.DNSHosts, host.IP+"|"+strings.Join(names, ","))
		}
	}
	sort.Strings(result.DNSServers)
	sort.Strings(result.DNSHosts)
	for _, item := range parsed.IPs {
		family := item.Family
		if family == "" {
			family = "ipv4"
		}
		prefix := item.Prefix
		if prefix == 0 && family == "ipv4" {
			prefix = prefixFromNetmask(item.Netmask)
		}
		line := fmt.Sprintf("%s|%s|%d", family, item.Address, prefix)
		if item.DHCP != nil {
			line += "|dhcp"
			if item.DHCP.Range != nil {
				line += "|" + item.DHCP.Range.Start + "-" + item.DHCP.Range.End
			}
			hosts := make([]string, 0, len(item.DHCP.Hosts))
			for _, host := range item.DHCP.Hosts {
				hosts = append(hosts, host.MAC+"|"+host.ID+"|"+host.IP+"|"+host.Name)
			}
			sort.Strings(hosts)
			line += "|" + strings.Join(hosts, ",")
		}
		result.Addresses = append(result.Addresses, line)
	}
	sort.Strings(result.Addresses)
	return result, nil
}

func compareManagedNetworkConfig(active, persistent managedNetworkConfig) []string {
	details := make([]string, 0)
	if active.Bridge != persistent.Bridge {
		details = append(details, fmt.Sprintf("桥接口运行态 %s、持久态 %s", active.Bridge, persistent.Bridge))
	}
	if active.Forward != persistent.Forward {
		details = append(details, fmt.Sprintf("转发模式运行态 %s、持久态 %s", active.Forward, persistent.Forward))
	}
	if active.NATIPv6 != persistent.NATIPv6 {
		details = append(details, "NAT66 开关不一致")
	}
	if active.MTU != persistent.MTU {
		details = append(details, fmt.Sprintf("网络 MTU 运行态 %d、持久态 %d", active.MTU, persistent.MTU))
	}
	if active.DNSDomain != persistent.DNSDomain {
		details = append(details, fmt.Sprintf("DNS 域运行态 %s、持久态 %s", active.DNSDomain, persistent.DNSDomain))
	}
	if strings.Join(active.DNSServers, ",") != strings.Join(persistent.DNSServers, ",") {
		details = append(details, "DNS 上游服务器不一致")
	}
	if strings.Join(active.DNSHosts, ",") != strings.Join(persistent.DNSHosts, ",") {
		details = append(details, "静态 DNS A/AAAA 记录不一致")
	}
	if strings.Join(active.Addresses, ",") != strings.Join(persistent.Addresses, ",") {
		details = append(details, "IPv4/IPv6 地址、DHCP 地址池或静态主机不一致")
	}
	return details
}

func networkWarnings(network domain.VirtualNetwork) []string {
	warnings := make([]string, 0)
	if hasIPv6(network) && network.ForwardMode == "nat" && !network.NATIPv6 {
		warnings = append(warnings, "NAT 网络已配置 IPv6，但未启用 IPv6 NAT；IPv6 客户端可能无法访问外部网络。")
	}
	if network.Active && network.NATIPv6 && !readForwarding("/proc/sys/net/ipv6/conf/all/forwarding") {
		warnings = append(warnings, "IPv6 NAT 需要宿主机 IPv6 转发，但当前 net.ipv6.conf.all.forwarding=0。")
	}
	if address, ok := ipv6Address(network); ok && address.IPv6Mode == "dhcpv6" && (address.DHCPStart == "" || address.DHCPEnd == "") {
		warnings = append(warnings, "DHCPv6 模式缺少地址范围，IPv6 静态主机和有状态客户端无法分配地址。")
	}
	if hasIPv6(network) && network.MTU > 0 && network.MTU < 1280 {
		warnings = append(warnings, "IPv6 网络 MTU 小于 1280，不符合 IPv6 最小链路 MTU。")
	}
	return warnings
}

func hasIPv4(network domain.VirtualNetwork) bool {
	for _, address := range network.Addresses {
		if networkAddressFamily(address) == "ipv4" {
			return true
		}
	}
	return false
}

func hasIPv6(network domain.VirtualNetwork) bool {
	_, ok := ipv6Address(network)
	return ok
}

func ipv6Address(network domain.VirtualNetwork) (domain.NetworkAddress, bool) {
	for _, address := range network.Addresses {
		if networkAddressFamily(address) == "ipv6" {
			return address, true
		}
	}
	return domain.NetworkAddress{}, false
}

func networkAddressFamily(address domain.NetworkAddress) string {
	if strings.EqualFold(strings.TrimSpace(address.Family), "ipv6") {
		return "ipv6"
	}
	return "ipv4"
}

func readForwarding(path string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(data)) == "1"
}

func defaultDiagnosticValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
