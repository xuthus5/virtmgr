package libvirtbackend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestRealLibvirtReadOnly(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	host, err := client.Host(ctx)
	if err != nil || host.CPUs == 0 || host.MemoryBytes == 0 {
		t.Fatalf("Host() = %+v, %v", host, err)
	}
	hostMetrics, err := client.HostMetrics(ctx)
	if err != nil || hostMetrics.CPUCapacity == 0 || hostMetrics.MemoryTotalBytes == 0 {
		t.Fatalf("HostMetrics() = %+v, %v", hostMetrics, err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		devices, err := client.ListHostDevices(ctx)
		if err != nil {
			t.Fatalf("ListHostDevices() attempt %d error = %v", attempt+1, err)
		}
		for _, device := range devices {
			if device.Name == "" || device.Type == "" {
				t.Fatalf("invalid host device: %+v", device)
			}
		}
	}
	vms, err := client.ListVMs(ctx)
	if err != nil {
		t.Fatalf("ListVMs() error = %v", err)
	}
	for _, vm := range vms {
		if vm.UUID == "" || vm.Name == "" {
			t.Fatalf("invalid vm: %+v", vm)
		}
	}
	if len(vms) > 0 {
		vm, err := client.GetVM(ctx, vms[0].UUID)
		if err != nil {
			t.Fatalf("GetVM() error = %v", err)
		}
		if vm.UUID != vms[0].UUID || vm.Name != vms[0].Name {
			t.Fatalf("GetVM() = %+v, want UUID %s", vm, vms[0].UUID)
		}
		vmMetrics, err := client.VMMetrics(ctx, vm.UUID)
		if err != nil || vmMetrics.MemoryTotalBytes == 0 {
			t.Fatalf("VMMetrics() = %+v, %v", vmMetrics, err)
		}
	}
	pools, err := client.ListStoragePools(ctx)
	if err != nil {
		t.Fatalf("ListStoragePools() error = %v", err)
	}
	for _, pool := range pools {
		if pool.Name == "" {
			t.Fatalf("invalid storage pool: %+v", pool)
		}
		if pool.State == "active" {
			volumes, err := client.ListStorageVolumes(ctx, pool.Name)
			if err != nil {
				t.Fatalf("ListStorageVolumes(%s) error = %v", pool.Name, err)
			}
			for _, volume := range volumes {
				if volume.Name == "" {
					t.Fatalf("invalid volume in pool %s: %+v", pool.Name, volume)
				}
			}
			break
		}
	}
	networks, err := client.ListNetworks(ctx)
	if err != nil {
		t.Fatalf("ListNetworks() error = %v", err)
	}
	for _, network := range networks {
		if network.Name == "" {
			t.Fatalf("invalid network: %+v", network)
		}
		if network.Active {
			leases, err := client.ListDHCPLeases(ctx, network.Name)
			if err != nil {
				t.Fatalf("ListDHCPLeases(%s) error = %v", network.Name, err)
			}
			for _, lease := range leases {
				if lease.IPAddress == "" || lease.Family == "" {
					t.Fatalf("invalid DHCP lease in network %s: %+v", network.Name, lease)
				}
			}
		}
	}
}

func TestRealLibvirtCloudInitCreate(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolName := "virtmgr-it-" + suffix
	poolDir := t.TempDir()
	pool, err := client.CreateStoragePool(ctx, domain.StoragePoolCreateSpec{
		Name: poolName, Type: "dir", TargetPath: poolDir, Start: true,
	})
	if err != nil {
		t.Fatalf("CreateStoragePool() error = %v", err)
	}
	defer func() {
		_ = client.PerformStoragePoolAction(ctx, pool.Name, domain.StoragePoolActionStop)
		_ = client.DeleteStoragePool(ctx, pool.Name)
	}()
	vm, err := client.CreateVM(ctx, domain.VMCreateSpec{
		Name: "virtmgr-cloudinit-it-" + suffix, VCPUs: 1, MemoryMiB: 128,
		DiskPath: filepath.Join(poolDir, "disk.qcow2"), DiskFormat: "qcow2", Network: "default",
		Firmware: domain.VMFirmwareBIOS,
		CloudInit: &domain.CloudInitSpec{
			UserData: "#cloud-config\nruncmd:\n  - echo integration-test\n", Pool: pool.Name,
		},
	})
	if err != nil {
		t.Fatalf("CreateVM() error = %v", err)
	}
	defer func() { _ = client.DeleteVM(ctx, vm.UUID) }()
	if !vm.CloudInit {
		t.Fatalf("created VM cloudInit = false: %+v", vm)
	}
	xmlResult, err := client.GetVMXML(ctx, vm.UUID, domain.VMXMLFormatConfig)
	if err != nil {
		t.Fatalf("GetVMXML() error = %v", err)
	}
	if !strings.Contains(xmlResult.XML, "<serial>cloud-init</serial>") || !strings.Contains(xmlResult.XML, "-cidata.iso") {
		t.Fatalf("domain XML missing cloud-init seed: %s", xmlResult.XML)
	}
	volumes, err := client.ListStorageVolumes(ctx, pool.Name)
	if err != nil {
		t.Fatalf("ListStorageVolumes() error = %v", err)
	}
	foundSeed := false
	for _, volume := range volumes {
		if volume.Name == cloudInitSeedVolumeName(vm.Name) {
			foundSeed = true
			break
		}
	}
	if !foundSeed {
		t.Fatalf("cloud-init seed volume not found: %+v", volumes)
	}
}

func TestRealLibvirtUpdateNetwork(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	name := "virtmgr-net-update-it-" + suffix
	created, err := client.CreateNetwork(ctx, domain.VirtualNetworkDefinition{
		Name: name, Mode: "isolated", IPv4Address: "10.211.0.1", Prefix: 24,
		DHCPStart: "10.211.0.2", DHCPEnd: "10.211.0.254",
	})
	if err != nil {
		t.Fatalf("CreateNetwork() error = %v", err)
	}
	defer func() {
		_ = client.PerformNetworkAction(ctx, created.Name, domain.VirtualNetworkActionStop)
		_ = client.DeleteNetwork(ctx, created.Name)
	}()
	updated, err := client.UpdateNetwork(ctx, domain.VirtualNetworkDefinition{
		Name: name, Mode: "nat", IPv4Address: "10.212.0.1", Prefix: 24,
		DHCPStart: "10.212.0.20", DHCPEnd: "10.212.0.120", Autostart: true,
	})
	if err != nil {
		t.Fatalf("UpdateNetwork() error = %v", err)
	}
	if updated.Name != name || updated.Active || !updated.Autostart || updated.ForwardMode != "nat" || len(updated.Addresses) == 0 {
		t.Fatalf("updated network = %+v", updated)
	}
	address := updated.Addresses[0]
	if address.Address != "10.212.0.1" || address.Prefix != 24 || address.DHCPStart != "10.212.0.20" || address.DHCPEnd != "10.212.0.120" {
		t.Fatalf("updated address = %+v", address)
	}
}

func TestRealLibvirtActiveNetworkDHCPHost(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	name := "virtmgr-dhcp-it-" + suffix
	created, err := client.CreateNetwork(ctx, domain.VirtualNetworkDefinition{
		Name: name, Mode: "isolated", IPv4Address: "10.213.0.1", Prefix: 24,
		DHCPStart: "10.213.0.2", DHCPEnd: "10.213.0.254", Start: true,
	})
	if err != nil {
		t.Fatalf("CreateNetwork() error = %v", err)
	}
	defer func() {
		_ = client.PerformNetworkAction(ctx, created.Name, domain.VirtualNetworkActionStop)
		_ = client.DeleteNetwork(ctx, created.Name)
	}()
	host := domain.NetworkDHCPHost{MAC: "52:54:00:de:ad:01", IP: "10.213.0.50", Name: "probe-host"}
	updated, err := client.AddNetworkDHCPHost(ctx, created.Name, host)
	if err != nil {
		t.Fatalf("AddNetworkDHCPHost() error = %v", err)
	}
	if !updated.Active || len(updated.Addresses) == 0 || !containsDHCPHost(updated.Addresses[0].Hosts, host) {
		t.Fatalf("updated network = %+v", updated)
	}
	updated, err = client.DeleteNetworkDHCPHost(ctx, created.Name, host)
	if err != nil {
		t.Fatalf("DeleteNetworkDHCPHost() error = %v", err)
	}
	if !updated.Active || len(updated.Addresses) == 0 || containsDHCPHost(updated.Addresses[0].Hosts, host) {
		t.Fatalf("network after DHCP host delete = %+v", updated)
	}
}

func TestRealLibvirtIPv6DualStackNetwork(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	ipv4 := fmt.Sprintf("10.253.%d", suffix%250+1)
	ipv6 := fmt.Sprintf("fd00:253:%x", uint64(suffix)&0xffff)
	name := fmt.Sprintf("virtmgr-it-ipv6-%d", suffix)
	definition := domain.VirtualNetworkDefinition{
		Name: name, Mode: "nat", IPv4Address: ipv4 + ".1", Prefix: 24,
		DHCPStart: ipv4 + ".2", DHCPEnd: ipv4 + ".254",
		IPv6Address: ipv6 + "::1", IPv6Prefix: 64, IPv6Mode: "dhcpv6",
		IPv6DHCPStart: ipv6 + "::2", IPv6DHCPEnd: ipv6 + "::ff", NATIPv6: true,
		MTU: 1500, MTUPresent: true,
		DNSDomain: "it.example.com", DNSServers: []string{"1.1.1.1", "2606:4700:4700::1111"},
		Start: true,
	}
	created, err := client.CreateNetwork(ctx, definition)
	if err != nil {
		t.Fatalf("CreateNetwork() error = %v", err)
	}
	defer func() {
		_ = client.PerformNetworkAction(ctx, created.Name, domain.VirtualNetworkActionStop)
		_ = client.DeleteNetwork(ctx, created.Name)
	}()
	if !created.Active || created.ForwardMode != "nat" || !created.NATIPv6 || created.MTU != 1500 || created.DNSDomain != "it.example.com" || len(created.DNSServers) != 2 {
		t.Fatalf("created IPv6 network = %+v", created)
	}
	if len(created.Addresses) != 2 {
		t.Fatalf("created IPv6 network addresses = %+v", created.Addresses)
	}
	var ipv6Address *domain.NetworkAddress
	for index := range created.Addresses {
		if created.Addresses[index].Family == "ipv6" {
			ipv6Address = &created.Addresses[index]
		}
	}
	if ipv6Address == nil || ipv6Address.IPv6Mode != "dhcpv6" || ipv6Address.DHCPStart == "" || ipv6Address.DHCPEnd == "" {
		t.Fatalf("created DHCPv6 address = %+v", ipv6Address)
	}
	dnsHost := domain.NetworkDNSHost{IP: ipv6 + "::60", Hostnames: []string{"it-dns.it.example.com", "it-dns"}}
	updated, err := client.AddNetworkDNSHost(ctx, created.Name, dnsHost)
	if err != nil {
		t.Fatalf("AddNetworkDNSHost() error = %v", err)
	}
	if !containsDNSHost(updated.DNSHosts, dnsHost) {
		t.Fatalf("IPv6 DNS host not found: %+v", updated.DNSHosts)
	}

	host := domain.NetworkDHCPHost{ID: "00:03:00:01:52:54:00:ab:cd:ef", IP: ipv6 + "::50", Name: "it-ipv6-host"}
	updated, err = client.AddNetworkDHCPHost(ctx, created.Name, host)
	if err != nil {
		t.Fatalf("AddNetworkDHCPHost() error = %v", err)
	}
	if !containsDHCPHost(staticHostsOf(updated), host) {
		t.Fatalf("IPv6 static host not found: %+v", updated.Addresses)
	}
	secondHost := domain.NetworkDHCPHost{ID: "00:03:00:01:52:54:00:ab:cd:f0", IP: ipv6 + "::51", Name: "it-ipv6-host-2"}
	updated, err = client.AddNetworkDHCPHost(ctx, created.Name, secondHost)
	if err != nil {
		t.Fatalf("AddNetworkDHCPHost(second) error = %v", err)
	}
	if !containsDHCPHost(staticHostsOf(updated), host) || !containsDHCPHost(staticHostsOf(updated), secondHost) {
		t.Fatalf("multiple IPv6 static hosts not found: %+v", updated.Addresses)
	}
	if err := client.PerformNetworkAction(ctx, created.Name, domain.VirtualNetworkActionStop); err != nil {
		t.Fatalf("stop network before reservation restart: %v", err)
	}
	if err := client.PerformNetworkAction(ctx, created.Name, domain.VirtualNetworkActionStart); err != nil {
		t.Fatalf("restart network with reservations: %v", err)
	}
	networks, err := client.ListNetworks(ctx)
	if err != nil {
		t.Fatalf("ListNetworks() after restart error = %v", err)
	}
	var restarted *domain.VirtualNetwork
	for index := range networks {
		if networks[index].Name == created.Name {
			restarted = &networks[index]
			break
		}
	}
	if restarted == nil || !containsDHCPHost(staticHostsOf(*restarted), host) || !containsDHCPHost(staticHostsOf(*restarted), secondHost) || !containsDNSHost(restarted.DNSHosts, dnsHost) {
		t.Fatalf("IPv6 reservations did not survive restart: %+v", restarted)
	}
	updated, err = client.DeleteNetworkDHCPHost(ctx, created.Name, host)
	if err != nil {
		t.Fatalf("DeleteNetworkDHCPHost() error = %v", err)
	}
	if containsDHCPHost(staticHostsOf(updated), host) {
		t.Fatalf("IPv6 static host still present: %+v", updated.Addresses)
	}
	updated, err = client.DeleteNetworkDHCPHost(ctx, created.Name, secondHost)
	if err != nil {
		t.Fatalf("DeleteNetworkDHCPHost(second) error = %v", err)
	}
	if containsDHCPHost(staticHostsOf(updated), secondHost) {
		t.Fatalf("second IPv6 static host still present: %+v", updated.Addresses)
	}
	updated, err = client.DeleteNetworkDNSHost(ctx, created.Name, dnsHost)
	if err != nil {
		t.Fatalf("DeleteNetworkDNSHost() error = %v", err)
	}
	if containsDNSHost(updated.DNSHosts, dnsHost) {
		t.Fatalf("IPv6 DNS host still present: %+v", updated.DNSHosts)
	}

	diagnostics, err := client.NetworkDiagnostics(ctx, created.Name)
	if err != nil {
		t.Fatalf("NetworkDiagnostics() error = %v", err)
	}
	if !diagnostics.Active || !diagnostics.BridgeReady || !diagnostics.NATIPv6 || !diagnostics.DHCPv6Range || diagnostics.MTU != 1500 || len(diagnostics.Checks) == 0 {
		t.Fatalf("IPv6 diagnostics = %+v", diagnostics)
	}
	if diagnostics.RuntimeDrift || len(diagnostics.DriftDetails) != 0 {
		t.Fatalf("IPv6 diagnostics unexpectedly report runtime drift: %+v", diagnostics)
	}
	if diagnostics.LibvirtVersion == "" || diagnostics.DnsmasqVersion == "" {
		t.Fatalf("IPv6 diagnostics did not record runtime versions: %+v", diagnostics)
	}
	if !diagnostics.DHCPv6Service || !diagnostics.RAService || !diagnostics.DNSService || !diagnostics.BridgeReady {
		t.Fatalf("IPv6 services were not detected: %+v", diagnostics)
	}
	if os.Geteuid() == 0 {
		if _, err := exec.LookPath("nft"); err == nil {
			if !diagnostics.NAT66Firewall || diagnostics.FirewallBackend != "nftables" {
				t.Fatalf("NAT66 nftables rule was not detected: %+v", diagnostics)
			}
		} else if _, err := exec.LookPath("ip6tables"); err == nil {
			if !diagnostics.NAT66Firewall || diagnostics.FirewallBackend != "ip6tables" {
				t.Fatalf("NAT66 ip6tables rule was not detected: %+v", diagnostics)
			}
		}
	}
}

func TestRealLibvirtIPv6OnlySLAACNetwork(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	ipv6 := fmt.Sprintf("fd00:254:%x", uint64(suffix)&0xffff)
	name := fmt.Sprintf("virtmgr-it-slaac-%d", suffix)
	created, err := client.CreateNetwork(ctx, domain.VirtualNetworkDefinition{
		Name: name, Mode: "isolated", IPv6Address: ipv6 + "::1", IPv6Prefix: 64, IPv6Mode: "slaac", Start: true,
	})
	if err != nil {
		t.Fatalf("CreateNetwork() error = %v", err)
	}
	defer func() {
		_ = client.PerformNetworkAction(ctx, created.Name, domain.VirtualNetworkActionStop)
		_ = client.DeleteNetwork(ctx, created.Name)
	}()
	if !created.Active || len(created.Addresses) != 1 || created.Addresses[0].Family != "ipv6" || created.Addresses[0].IPv6Mode != "slaac" {
		t.Fatalf("created SLAAC network = %+v", created)
	}
	if created.Addresses[0].DHCPStart != "" || created.Addresses[0].DHCPEnd != "" {
		t.Fatalf("SLAAC network unexpectedly has DHCPv6 range: %+v", created.Addresses[0])
	}
	diagnostics, err := client.NetworkDiagnostics(ctx, created.Name)
	if err != nil {
		t.Fatalf("NetworkDiagnostics() error = %v", err)
	}
	if !diagnostics.Active || !diagnostics.BridgeReady || diagnostics.DHCPv6Range {
		t.Fatalf("SLAAC diagnostics = %+v", diagnostics)
	}
}

func TestRealLibvirtIPv6DHCPv6NAT66DataPlane(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	if os.Geteuid() != 0 {
		t.Skip("root is required to create a network namespace and veth pair")
	}
	if _, err := exec.LookPath("dhcpcd"); err != nil {
		t.Skip("dhcpcd is required for the IPv6 data-plane test")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	ipv4 := fmt.Sprintf("10.252.%d", suffix%250+1)
	ipv6 := fmt.Sprintf("fd00:262:%x", uint64(suffix)&0xffff)
	name := fmt.Sprintf("virtmgr-it-nat66-%d", suffix)
	created, err := client.CreateNetwork(ctx, domain.VirtualNetworkDefinition{
		Name: name, Mode: "nat", IPv4Address: ipv4 + ".1", Prefix: 24,
		DHCPStart: ipv4 + ".2", DHCPEnd: ipv4 + ".254",
		IPv6Address: ipv6 + "::1", IPv6Prefix: 64, IPv6Mode: "dhcpv6",
		IPv6DHCPStart: ipv6 + "::2", IPv6DHCPEnd: ipv6 + "::ff", NATIPv6: true,
		Start: true,
	})
	if err != nil {
		t.Fatalf("CreateNetwork() error = %v", err)
	}
	defer func() {
		_ = client.PerformNetworkAction(ctx, created.Name, domain.VirtualNetworkActionStop)
		_ = client.DeleteNetwork(ctx, created.Name)
	}()
	if created.Bridge == "" {
		t.Fatalf("created network has no bridge: %+v", created)
	}

	tag := fmt.Sprintf("%06d", suffix%1_000_000)
	namespace := "vmns" + tag
	hostVeth := "vmh" + tag
	namespaceVeth := "vmn" + tag
	cleanup := func() {
		_ = exec.Command("ip", "netns", "del", namespace).Run()
		_ = exec.Command("ip", "link", "del", hostVeth).Run()
	}
	defer cleanup()
	runNetworkCommand(t, "ip", "netns", "add", namespace)
	runNetworkCommand(t, "ip", "link", "add", hostVeth, "type", "veth", "peer", "name", namespaceVeth)
	runNetworkCommand(t, "ip", "link", "set", hostVeth, "master", created.Bridge)
	runNetworkCommand(t, "ip", "link", "set", hostVeth, "up")
	runNetworkCommand(t, "ip", "link", "set", namespaceVeth, "netns", namespace)
	runNetworkCommand(t, "ip", "-n", namespace, "link", "set", namespaceVeth, "up")
	runNetworkCommand(t, "ip", "-n", namespace, "link", "set", "lo", "up")
	runNetworkCommand(t, "ip", "netns", "exec", namespace, "dhcpcd", "-6", "-1", "-t", "20", namespaceVeth)

	addresses := runNetworkCommand(t, "ip", "-n", namespace, "-6", "-o", "addr", "show", "dev", namespaceVeth)
	if !strings.Contains(addresses, ipv6+"::") {
		t.Fatalf("namespace did not obtain an address from %s/64: %s", ipv6, addresses)
	}
	runNetworkCommand(t, "ip", "netns", "exec", namespace, "ping", "-6", "-c", "1", "-W", "3", ipv6+"::1")
	if hostIPv6InternetAvailable() {
		runNetworkCommand(t, "ip", "netns", "exec", namespace, "ping", "-6", "-c", "1", "-W", "5", "2606:4700:4700::1111")
	}

	leases, err := client.ListDHCPLeases(ctx, created.Name)
	if err != nil {
		t.Fatalf("ListDHCPLeases() error = %v", err)
	}
	foundIPv6Lease := false
	for _, lease := range leases {
		if lease.Family == "ipv6" && strings.HasPrefix(lease.IPAddress, ipv6+"::") {
			foundIPv6Lease = true
		}
	}
	if !foundIPv6Lease {
		t.Fatalf("IPv6 lease not found: %+v", leases)
	}
}

func TestRealLibvirtIPv6SLAACDataPlane(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	if os.Geteuid() != 0 {
		t.Skip("root is required to create a network namespace and veth pair")
	}
	if _, err := exec.LookPath("dhcpcd"); err != nil {
		t.Skip("dhcpcd is required for the IPv6 data-plane test")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	ipv6 := fmt.Sprintf("fd00:263:%x", uint64(suffix)&0xffff)
	name := fmt.Sprintf("virtmgr-it-slaac-plane-%d", suffix)
	created, err := client.CreateNetwork(ctx, domain.VirtualNetworkDefinition{
		Name: name, Mode: "isolated", IPv6Address: ipv6 + "::1", IPv6Prefix: 64, IPv6Mode: "slaac", Start: true,
	})
	if err != nil {
		t.Fatalf("CreateNetwork() error = %v", err)
	}
	defer func() {
		_ = client.PerformNetworkAction(ctx, created.Name, domain.VirtualNetworkActionStop)
		_ = client.DeleteNetwork(ctx, created.Name)
	}()

	tag := fmt.Sprintf("%06d", suffix%1_000_000)
	namespace := "vmns" + tag
	hostVeth := "vmh" + tag
	namespaceVeth := "vmn" + tag
	defer func() {
		_ = exec.Command("ip", "netns", "del", namespace).Run()
		_ = exec.Command("ip", "link", "del", hostVeth).Run()
	}()
	runNetworkCommand(t, "ip", "netns", "add", namespace)
	runNetworkCommand(t, "ip", "link", "add", hostVeth, "type", "veth", "peer", "name", namespaceVeth)
	runNetworkCommand(t, "ip", "link", "set", hostVeth, "master", created.Bridge)
	runNetworkCommand(t, "ip", "link", "set", hostVeth, "up")
	runNetworkCommand(t, "ip", "link", "set", namespaceVeth, "netns", namespace)
	runNetworkCommand(t, "ip", "-n", namespace, "link", "set", namespaceVeth, "up")
	runNetworkCommand(t, "ip", "-n", namespace, "link", "set", "lo", "up")
	runNetworkCommand(t, "ip", "netns", "exec", namespace, "dhcpcd", "-6", "-1", "-t", "20", namespaceVeth)
	addresses := runNetworkCommand(t, "ip", "-n", namespace, "-6", "-o", "addr", "show", "dev", namespaceVeth)
	if !strings.Contains(addresses, ipv6+":") || !strings.Contains(addresses, "/64") {
		t.Fatalf("namespace did not obtain a SLAAC address from %s/64: %s", ipv6, addresses)
	}
	runNetworkCommand(t, "ip", "netns", "exec", namespace, "ping", "-6", "-c", "1", "-W", "3", ipv6+"::1")
}

func runNetworkCommand(t *testing.T, name string, args ...string) string {
	t.Helper()
	command := exec.Command(name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}

func hostIPv6InternetAvailable() bool {
	command := exec.Command("ping", "-6", "-c", "1", "-W", "2", "2606:4700:4700::1111")
	return command.Run() == nil
}

func staticHostsOf(network domain.VirtualNetwork) []domain.NetworkDHCPHost {
	hosts := make([]domain.NetworkDHCPHost, 0)
	for _, address := range network.Addresses {
		hosts = append(hosts, address.Hosts...)
	}
	return hosts
}

func containsDHCPHost(hosts []domain.NetworkDHCPHost, want domain.NetworkDHCPHost) bool {
	for _, host := range hosts {
		if strings.EqualFold(host.MAC, want.MAC) && host.IP == want.IP && host.Name == want.Name {
			return true
		}
	}
	return false
}

func containsDNSHost(hosts []domain.NetworkDNSHost, want domain.NetworkDNSHost) bool {
	for _, host := range hosts {
		if host.IP != want.IP || len(host.Hostnames) != len(want.Hostnames) {
			continue
		}
		matched := true
		for _, hostname := range want.Hostnames {
			if !slices.Contains(host.Hostnames, hostname) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func TestRealLibvirtUpdateVMResources(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolName := "virtmgr-resources-it-" + suffix
	poolDir := t.TempDir()
	pool, err := client.CreateStoragePool(ctx, domain.StoragePoolCreateSpec{
		Name: poolName, Type: "dir", TargetPath: poolDir, Start: true,
	})
	if err != nil {
		t.Fatalf("CreateStoragePool() error = %v", err)
	}
	defer func() {
		_ = client.PerformStoragePoolAction(ctx, pool.Name, domain.StoragePoolActionStop)
		_ = client.DeleteStoragePool(ctx, pool.Name)
	}()
	vm, err := client.CreateVM(ctx, domain.VMCreateSpec{
		Name: "virtmgr-resources-it-" + suffix, VCPUs: 1, MemoryMiB: 128,
		DiskPath: filepath.Join(poolDir, "disk.qcow2"), DiskFormat: "qcow2", Network: "default",
		Firmware: domain.VMFirmwareBIOS,
	})
	if err != nil {
		t.Fatalf("CreateVM() error = %v", err)
	}
	defer func() { _ = client.DeleteVM(ctx, vm.UUID) }()
	if vm.State != domain.VMStateShutoff {
		t.Fatalf("created VM state = %s", vm.State)
	}
	updated, err := client.UpdateVMResources(ctx, vm.UUID, domain.VMResourcesSpec{VCPUs: 2, MemoryMiB: 192})
	if err != nil {
		t.Fatalf("UpdateVMResources() error = %v", err)
	}
	if updated.VCPUs != 2 || updated.MaxVCPUs < 2 || updated.MemoryBytes != 192<<20 || updated.MaxMemoryBytes < 192<<20 {
		t.Fatalf("updated VM = %+v", updated)
	}
	xmlResult, err := client.GetVMXML(ctx, vm.UUID, domain.VMXMLFormatConfig)
	if err != nil {
		t.Fatalf("GetVMXML() error = %v", err)
	}
	if !strings.Contains(xmlResult.XML, "<vcpu") || !strings.Contains(xmlResult.XML, ">2</vcpu>") || !strings.Contains(xmlResult.XML, "196608") {
		t.Fatalf("updated config XML = %s", xmlResult.XML)
	}
}

func TestRealLibvirtResizeVMDisk(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolName := "virtmgr-disk-resize-it-" + suffix
	poolDir := t.TempDir()
	pool, err := client.CreateStoragePool(ctx, domain.StoragePoolCreateSpec{
		Name: poolName, Type: "dir", TargetPath: poolDir, Start: true,
	})
	if err != nil {
		t.Fatalf("CreateStoragePool() error = %v", err)
	}
	defer func() {
		_ = client.PerformStoragePoolAction(ctx, pool.Name, domain.StoragePoolActionStop)
		_ = client.DeleteStoragePool(ctx, pool.Name)
	}()
	volume, err := client.CreateStorageVolume(ctx, pool.Name, domain.StorageVolumeCreateSpec{
		Name: "disk.qcow2", Format: "qcow2", CapacityBytes: 64 << 20,
	})
	if err != nil {
		t.Fatalf("CreateStorageVolume() error = %v", err)
	}
	vm, err := client.CreateVM(ctx, domain.VMCreateSpec{
		Name: "virtmgr-disk-resize-it-" + suffix, VCPUs: 1, MemoryMiB: 128,
		DiskPath: volume.Path, DiskFormat: "qcow2", Network: "default", Firmware: domain.VMFirmwareBIOS,
	})
	if err != nil {
		t.Fatalf("CreateVM() error = %v", err)
	}
	defer func() { _ = client.DeleteVM(ctx, vm.UUID) }()
	loaded, err := client.GetVM(ctx, vm.UUID)
	if err != nil {
		t.Fatalf("GetVM() error = %v", err)
	}
	if len(loaded.Disks) == 0 || loaded.Disks[0].CapacityBytes != 64<<20 {
		t.Fatalf("loaded VM disks = %+v", loaded.Disks)
	}

	updated, err := client.ResizeVMDisk(ctx, vm.UUID, "vda", domain.VMDiskResizeSpec{CapacityBytes: 96 << 20})
	if err != nil {
		t.Fatalf("ResizeVMDisk() error = %v", err)
	}
	if len(updated.Disks) == 0 || updated.Disks[0].CapacityBytes != 96<<20 {
		t.Fatalf("updated VM disks = %+v", updated.Disks)
	}
	volumes, err := client.ListStorageVolumes(ctx, pool.Name)
	if err != nil {
		t.Fatalf("ListStorageVolumes() error = %v", err)
	}
	if len(volumes) != 1 || volumes[0].CapacityBytes != 96<<20 {
		t.Fatalf("resized volumes = %+v", volumes)
	}
	if _, err := client.ResizeVMDisk(ctx, vm.UUID, "vda", domain.VMDiskResizeSpec{CapacityBytes: 80 << 20}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("shrink error = %v", err)
	}
}

func TestRealLibvirtResizeUnmanagedFileDisk(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolName := "virtmgr-unmanaged-resize-it-" + suffix
	poolDir := t.TempDir()
	pool, err := client.CreateStoragePool(ctx, domain.StoragePoolCreateSpec{
		Name: poolName, Type: "dir", TargetPath: poolDir, Start: true,
	})
	if err != nil {
		t.Fatalf("CreateStoragePool() error = %v", err)
	}
	defer func() {
		_ = client.PerformStoragePoolAction(ctx, pool.Name, domain.StoragePoolActionStop)
		_ = client.DeleteStoragePool(ctx, pool.Name)
	}()
	nestedDir := filepath.Join(poolDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o750); err != nil {
		t.Fatal(err)
	}
	diskPath := filepath.Join(nestedDir, "disk.qcow2")
	if output, err := exec.Command("qemu-img", "create", "-f", "qcow2", diskPath, "64M").CombinedOutput(); err != nil {
		t.Fatalf("qemu-img create: %v: %s", err, output)
	}
	vm, err := client.CreateVM(ctx, domain.VMCreateSpec{
		Name: "virtmgr-unmanaged-resize-it-" + suffix, VCPUs: 1, MemoryMiB: 128,
		DiskPath: diskPath, DiskFormat: "qcow2", Network: "default", Firmware: domain.VMFirmwareBIOS,
	})
	if err != nil {
		t.Fatalf("CreateVM() error = %v", err)
	}
	defer func() { _ = client.DeleteVM(ctx, vm.UUID) }()
	loaded, err := client.GetVM(ctx, vm.UUID)
	if err != nil {
		t.Fatalf("GetVM() error = %v", err)
	}
	if len(loaded.Disks) == 0 || loaded.Disks[0].CapacityBytes != 64<<20 {
		t.Fatalf("loaded VM disks = %+v", loaded.Disks)
	}

	updated, err := client.ResizeVMDisk(ctx, vm.UUID, "vda", domain.VMDiskResizeSpec{CapacityBytes: 96 << 20})
	if err != nil {
		t.Fatalf("ResizeVMDisk(unmanaged) error = %v", err)
	}
	if len(updated.Disks) == 0 || updated.Disks[0].CapacityBytes != 96<<20 {
		t.Fatalf("updated VM disks = %+v", updated.Disks)
	}
	info, err := exec.Command("qemu-img", "info", "--output=json", diskPath).Output()
	if err != nil {
		t.Fatalf("qemu-img info: %v", err)
	}
	if !strings.Contains(string(info), `"virtual-size": 100663296`) {
		t.Fatalf("qemu-img info = %s", info)
	}
}
