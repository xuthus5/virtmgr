package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

type networkXML struct {
	Forward struct {
		Mode string         `xml:"mode,attr"`
		NAT  *networkNATXML `xml:"nat"`
	} `xml:"forward"`
	Domain *networkDomainXML `xml:"domain"`
	DNS    *networkDNSXML    `xml:"dns"`
	Bridge struct {
		Name string `xml:"name,attr"`
	} `xml:"bridge"`
	IPs []networkIPXML `xml:"ip"`
}

type networkIPXML struct {
	Family  string          `xml:"family,attr"`
	Address string          `xml:"address,attr"`
	Netmask string          `xml:"netmask,attr"`
	Prefix  int             `xml:"prefix,attr"`
	DHCP    *networkDHCPXML `xml:"dhcp"`
}

type networkDHCPXML struct {
	Range *networkRangeXML     `xml:"range"`
	Hosts []networkDHCPHostXML `xml:"host"`
}

type networkDHCPHostXML struct {
	XMLName xml.Name `xml:"host"`
	MAC     string   `xml:"mac,attr,omitempty"`
	ID      string   `xml:"id,attr,omitempty"`
	IP      string   `xml:"ip,attr"`
	Name    string   `xml:"name,attr,omitempty"`
}

type networkDefinitionXML struct {
	XMLName xml.Name                 `xml:"network"`
	Name    string                   `xml:"name"`
	Forward *networkForwardXML       `xml:"forward,omitempty"`
	Domain  *networkDomainXML        `xml:"domain,omitempty"`
	DNS     *networkDNSXML           `xml:"dns,omitempty"`
	Bridge  *networkBridgeXML        `xml:"bridge,omitempty"`
	IPs     []networkIPDefinitionXML `xml:"ip,omitempty"`
}

type networkForwardXML struct {
	Mode string         `xml:"mode,attr"`
	NAT  *networkNATXML `xml:"nat,omitempty"`
}

type networkNATXML struct {
	IPv6 string `xml:"ipv6,attr,omitempty"`
}

type networkDomainXML struct {
	Name string `xml:"name,attr"`
}

type networkDNSXML struct {
	Forwarders []networkDNSForwarderXML `xml:"forwarder"`
}

type networkDNSForwarderXML struct {
	Addr string `xml:"addr,attr"`
}

type networkBridgeXML struct {
	Name string `xml:"name,attr"`
}

type interfaceXML struct {
	Type string `xml:"type,attr"`
}

type networkIPDefinitionXML struct {
	Family  string                    `xml:"family,attr,omitempty"`
	Address string                    `xml:"address,attr"`
	Prefix  int                       `xml:"prefix,attr"`
	DHCP    *networkDHCPDefinitionXML `xml:"dhcp,omitempty"`
}

type networkDHCPDefinitionXML struct {
	Range networkRangeXML `xml:"range"`
}

type networkRangeXML struct {
	Start string `xml:"start,attr"`
	End   string `xml:"end,attr"`
}

func (c *Client) ListNetworks(ctx context.Context) ([]domain.VirtualNetwork, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return nil, err
	}
	networks, err := c.conn.ListAllNetworks(libvirt.CONNECT_LIST_NETWORKS_ACTIVE | libvirt.CONNECT_LIST_NETWORKS_INACTIVE)
	if err != nil {
		return nil, fmt.Errorf("list libvirt networks: %w", err)
	}
	result := make([]domain.VirtualNetwork, 0, len(networks))
	for i := range networks {
		network := &networks[i]
		item, err := networkFromLibvirt(network)
		_ = network.Free()
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (c *Client) ListHostBridges(ctx context.Context) ([]domain.HostBridge, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return nil, err
	}
	interfaces, err := c.conn.ListAllInterfaces(libvirt.CONNECT_LIST_INTERFACES_ACTIVE)
	if err != nil {
		return nil, fmt.Errorf("list host interfaces: %w", err)
	}
	used := map[string]bool{}
	networks, err := c.conn.ListAllNetworks(libvirt.CONNECT_LIST_NETWORKS_ACTIVE | libvirt.CONNECT_LIST_NETWORKS_INACTIVE)
	if err != nil {
		return nil, fmt.Errorf("list networks while reading host bridges: %w", err)
	}
	for i := range networks {
		network := &networks[i]
		xmlText, xmlErr := network.GetXMLDesc(0)
		_ = network.Free()
		if xmlErr != nil {
			continue
		}
		var parsed networkXML
		if xml.Unmarshal([]byte(xmlText), &parsed) == nil && parsed.Bridge.Name != "" {
			used[parsed.Bridge.Name] = true
		}
	}
	result := make([]domain.HostBridge, 0)
	for i := range interfaces {
		item := &interfaces[i]
		name, nameErr := item.GetName()
		xmlText, xmlErr := item.GetXMLDesc(0)
		_ = item.Free()
		if nameErr != nil || xmlErr != nil {
			continue
		}
		var parsed interfaceXML
		if xml.Unmarshal([]byte(xmlText), &parsed) != nil || parsed.Type != "bridge" || used[name] {
			continue
		}
		result = append(result, domain.HostBridge{Name: name})
	}
	return result, nil
}

func (c *Client) ListDHCPLeases(ctx context.Context, networkName string) ([]domain.DHCPLease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return nil, err
	}
	network, err := c.conn.LookupNetworkByName(networkName)
	if errors.Is(err, libvirt.ERR_NO_NETWORK) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lookup libvirt network %s: %w", networkName, err)
	}
	defer func() { _ = network.Free() }()
	active, err := network.IsActive()
	if err != nil {
		return nil, fmt.Errorf("read network %s state: %w", networkName, err)
	}
	if !active {
		return []domain.DHCPLease{}, nil
	}
	leases, err := network.GetDHCPLeases()
	if err != nil {
		return nil, fmt.Errorf("list DHCP leases for network %s: %w", networkName, err)
	}
	result := make([]domain.DHCPLease, 0, len(leases))
	for _, lease := range leases {
		result = append(result, dhcpLeaseFromLibvirt(lease))
	}
	return result, nil
}

func dhcpLeaseFromLibvirt(lease libvirt.NetworkDHCPLease) domain.DHCPLease {
	family := "unknown"
	switch lease.Type {
	case libvirt.IP_ADDR_TYPE_IPV4:
		family = "ipv4"
	case libvirt.IP_ADDR_TYPE_IPV6:
		family = "ipv6"
	}
	var expiresAt *time.Time
	if lease.ExpiryTime.Unix() > 0 {
		value := lease.ExpiryTime.UTC()
		expiresAt = &value
	}
	return domain.DHCPLease{
		Interface: lease.Iface, ExpiresAt: expiresAt, Family: family, MAC: lease.Mac,
		IPAddress: lease.IPaddr, Prefix: lease.Prefix, Hostname: lease.Hostname,
		ClientID: lease.Clientid, IAID: lease.Iaid,
	}
}

func (c *Client) CreateNetwork(ctx context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VirtualNetwork{}, err
	}
	if existing, err := c.conn.LookupNetworkByName(definition.Name); err == nil {
		_ = existing.Free()
		return domain.VirtualNetwork{}, domain.ErrConflict
	} else if !errors.Is(err, libvirt.ERR_NO_NETWORK) {
		return domain.VirtualNetwork{}, fmt.Errorf("lookup network before create: %w", err)
	}
	if definition.Mode == "bridge" {
		if err := c.validateHostBridgeLocked(definition.Bridge, ""); err != nil {
			return domain.VirtualNetwork{}, err
		}
	}
	xmlText, err := networkXMLFromDefinition(definition)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	network, err := c.conn.NetworkDefineXML(xmlText)
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("define libvirt network: %w", err)
	}
	defer network.Free()
	rollback := func() {
		if definition.Start {
			_ = network.Destroy()
		}
		_ = network.Undefine()
	}
	if definition.Autostart {
		if err := network.SetAutostart(true); err != nil {
			rollback()
			return domain.VirtualNetwork{}, fmt.Errorf("set network autostart: %w", err)
		}
	}
	if definition.Start {
		if err := network.Create(); err != nil {
			rollback()
			return domain.VirtualNetwork{}, fmt.Errorf("start new network: %w", err)
		}
	}
	result, err := networkFromLibvirt(network)
	if err != nil {
		rollback()
		return domain.VirtualNetwork{}, err
	}
	waitForNetworkIPv6Ready(ctx, result)
	return result, nil
}

func (c *Client) validateHostBridgeLocked(bridge, currentNetwork string) error {
	if bridge == "" {
		return fmt.Errorf("%w: host bridge name is required", domain.ErrInvalidInput)
	}
	hostInterface, err := c.conn.LookupInterfaceByName(bridge)
	if errors.Is(err, libvirt.ERR_NO_INTERFACE) {
		return fmt.Errorf("%w: host bridge %s does not exist", domain.ErrInvalidInput, bridge)
	}
	if err != nil {
		return fmt.Errorf("lookup host bridge %s: %w", bridge, err)
	}
	defer func() { _ = hostInterface.Free() }()
	xmlText, err := hostInterface.GetXMLDesc(0)
	if err != nil {
		return fmt.Errorf("read host bridge %s: %w", bridge, err)
	}
	var parsed interfaceXML
	if err := xml.Unmarshal([]byte(xmlText), &parsed); err != nil {
		return fmt.Errorf("decode host bridge %s: %w", bridge, err)
	}
	if parsed.Type != "bridge" {
		return fmt.Errorf("%w: host interface %s is not a bridge", domain.ErrInvalidInput, bridge)
	}
	networks, err := c.conn.ListAllNetworks(libvirt.CONNECT_LIST_NETWORKS_ACTIVE | libvirt.CONNECT_LIST_NETWORKS_INACTIVE)
	if err != nil {
		return fmt.Errorf("list networks while validating bridge %s: %w", bridge, err)
	}
	for i := range networks {
		network := &networks[i]
		name, nameErr := network.GetName()
		xmlText, xmlErr := network.GetXMLDesc(0)
		_ = network.Free()
		if nameErr != nil || xmlErr != nil || name == currentNetwork {
			continue
		}
		var existing networkXML
		if xml.Unmarshal([]byte(xmlText), &existing) != nil {
			continue
		}
		if existing.Bridge.Name == bridge {
			return fmt.Errorf("%w: host bridge %s is already used by network %s", domain.ErrConflict, bridge, name)
		}
	}
	return nil
}

func (c *Client) PerformNetworkAction(ctx context.Context, name string, action domain.VirtualNetworkAction) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	network, err := c.conn.LookupNetworkByName(name)
	if errors.Is(err, libvirt.ERR_NO_NETWORK) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lookup network: %w", err)
	}
	defer network.Free()
	active, err := network.IsActive()
	if err != nil {
		return fmt.Errorf("read network state: %w", err)
	}
	switch action {
	case domain.VirtualNetworkActionStart:
		if active {
			return domain.ErrConflict
		}
		if err := network.Create(); err != nil {
			return fmt.Errorf("start network: %w", err)
		}
		if record, recordErr := networkFromLibvirt(network); recordErr == nil {
			waitForNetworkIPv6Ready(ctx, record)
		}
	case domain.VirtualNetworkActionStop:
		if !active {
			return domain.ErrConflict
		}
		if err := network.Destroy(); err != nil {
			return fmt.Errorf("stop network: %w", err)
		}
	default:
		return domain.ErrInvalidInput
	}
	return nil
}

func (c *Client) DeleteNetwork(ctx context.Context, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	network, err := c.conn.LookupNetworkByName(name)
	if errors.Is(err, libvirt.ERR_NO_NETWORK) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lookup network: %w", err)
	}
	defer network.Free()
	active, err := network.IsActive()
	if err != nil {
		return fmt.Errorf("read network state: %w", err)
	}
	if active {
		return domain.ErrConflict
	}
	if err := network.Undefine(); err != nil {
		return fmt.Errorf("delete network: %w", err)
	}
	return nil
}

func networkXMLFromDefinition(definition domain.VirtualNetworkDefinition) (string, error) {
	item := networkDefinitionXML{Name: definition.Name}
	switch definition.Mode {
	case "nat":
		item.Forward = &networkForwardXML{Mode: "nat"}
		if definition.IPv6Address != "" && definition.NATIPv6 {
			item.Forward.NAT = &networkNATXML{IPv6: "yes"}
		}
	case "isolated":
	case "bridge":
		if definition.Bridge == "" {
			return "", fmt.Errorf("%w: host bridge name is required", domain.ErrInvalidInput)
		}
		item.Forward = &networkForwardXML{Mode: "bridge"}
		item.Bridge = &networkBridgeXML{Name: definition.Bridge}
	default:
		return "", fmt.Errorf("%w: unsupported network mode", domain.ErrInvalidInput)
	}
	if definition.Mode != "bridge" && definition.IPv4Address != "" {
		ipv4 := networkIPDefinitionXML{Address: definition.IPv4Address, Prefix: definition.Prefix}
		if definition.DHCPStart != "" && definition.DHCPEnd != "" {
			ipv4.DHCP = &networkDHCPDefinitionXML{Range: networkRangeXML{Start: definition.DHCPStart, End: definition.DHCPEnd}}
		}
		item.IPs = append(item.IPs, ipv4)
	}
	if definition.Mode != "bridge" && definition.IPv6Address != "" {
		ipv6 := networkIPDefinitionXML{Family: "ipv6", Address: definition.IPv6Address, Prefix: definition.IPv6Prefix}
		if definition.IPv6Mode == "dhcpv6" && definition.IPv6DHCPStart != "" && definition.IPv6DHCPEnd != "" {
			ipv6.DHCP = &networkDHCPDefinitionXML{Range: networkRangeXML{Start: definition.IPv6DHCPStart, End: definition.IPv6DHCPEnd}}
		}
		item.IPs = append(item.IPs, ipv6)
	}
	if definition.DNSDomain != "" {
		item.Domain = &networkDomainXML{Name: definition.DNSDomain}
	}
	if len(definition.DNSServers) > 0 {
		item.DNS = &networkDNSXML{Forwarders: make([]networkDNSForwarderXML, 0, len(definition.DNSServers))}
		for _, server := range definition.DNSServers {
			item.DNS.Forwarders = append(item.DNS.Forwarders, networkDNSForwarderXML{Addr: server})
		}
	}
	body, err := xml.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("encode network xml: %w", err)
	}
	return xml.Header + string(body), nil
}

func networkFromLibvirt(network *libvirt.Network) (domain.VirtualNetwork, error) {
	name, err := network.GetName()
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("read network name: %w", err)
	}
	uuid, _ := network.GetUUIDString()
	active, _ := network.IsActive()
	persistent, _ := network.IsPersistent()
	autostart, _ := network.GetAutostart()
	xmlText, err := network.GetXMLDesc(0)
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("read network %s xml: %w", name, err)
	}
	var parsed networkXML
	if err := xml.Unmarshal([]byte(xmlText), &parsed); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("decode network %s xml: %w", name, err)
	}
	addresses := make([]domain.NetworkAddress, 0, len(parsed.IPs))
	for _, item := range parsed.IPs {
		family := item.Family
		if family == "" {
			family = "ipv4"
		}
		prefix := item.Prefix
		if prefix == 0 && family == "ipv4" {
			prefix = prefixFromNetmask(item.Netmask)
		}
		address := domain.NetworkAddress{
			Family: family, Address: item.Address, Prefix: prefix,
		}
		if family == "ipv6" {
			address.IPv6Mode = "slaac"
		}
		if item.DHCP != nil {
			address.DHCPEnabled = true
			if family == "ipv6" {
				address.IPv6Mode = "dhcpv6"
			}
			if item.DHCP.Range != nil {
				address.DHCPStart = item.DHCP.Range.Start
				address.DHCPEnd = item.DHCP.Range.End
			}
			for _, host := range item.DHCP.Hosts {
				address.Hosts = append(address.Hosts, domain.NetworkDHCPHost{MAC: host.MAC, ID: host.ID, IP: host.IP, Name: host.Name})
			}
		}
		addresses = append(addresses, address)
	}
	natIPv6 := parsed.Forward.Mode == "nat" && parsed.Forward.NAT != nil && parsed.Forward.NAT.IPv6 == "yes"
	dnsServers := make([]string, 0)
	if parsed.DNS != nil {
		for _, forwarder := range parsed.DNS.Forwarders {
			if strings.TrimSpace(forwarder.Addr) != "" {
				dnsServers = append(dnsServers, forwarder.Addr)
			}
		}
	}
	dnsDomain := ""
	if parsed.Domain != nil {
		dnsDomain = parsed.Domain.Name
	}
	result := domain.VirtualNetwork{
		Name: name, UUID: uuid, Bridge: parsed.Bridge.Name, Active: active,
		Persistent: persistent, Autostart: autostart, ForwardMode: parsed.Forward.Mode,
		NATIPv6: natIPv6, DNSDomain: dnsDomain, DNSServers: dnsServers, Addresses: addresses,
	}
	result.Warnings = networkWarnings(result)
	return result, nil
}

func prefixFromNetmask(mask string) int {
	ip := net.ParseIP(mask)
	if ip == nil {
		return 0
	}
	ones, bits := net.IPMask(ip.To4()).Size()
	if bits == 0 {
		return 0
	}
	return ones
}
