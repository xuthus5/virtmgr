package libvirtbackend

import (
	"encoding/xml"
	"errors"
	"strings"
	"testing"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func TestParseNetworkXML(t *testing.T) {
	const text = `<network><name>default</name><forward mode='nat'/><bridge name='virbr0' stp='on' delay='0'/><ip address='192.168.122.1' netmask='255.255.255.0'><dhcp><range start='192.168.122.2' end='192.168.122.254'/><host mac='52:54:00:12:34:56' ip='192.168.122.50' name='web-01'/></dhcp></ip><ip family='ipv6' address='fd00::1' prefix='64'/></network>`
	var parsed networkXML
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Forward.Mode != "nat" || parsed.Bridge.Name != "virbr0" {
		t.Fatalf("parsed network = %+v", parsed)
	}
	if len(parsed.IPs) != 2 || parsed.IPs[0].DHCP.Range.Start != "192.168.122.2" || parsed.IPs[1].Prefix != 64 {
		t.Fatalf("parsed IPs = %+v", parsed.IPs)
	}
	if len(parsed.IPs[0].DHCP.Hosts) != 1 || parsed.IPs[0].DHCP.Hosts[0].MAC != "52:54:00:12:34:56" || parsed.IPs[0].DHCP.Hosts[0].IP != "192.168.122.50" || parsed.IPs[0].DHCP.Hosts[0].Name != "web-01" {
		t.Fatalf("parsed DHCP hosts = %+v", parsed.IPs[0].DHCP.Hosts)
	}
}

func TestNetworkDHCPHostXMLAndParentIndex(t *testing.T) {
	hostXML, err := networkDHCPHostXMLText(domain.NetworkDHCPHost{MAC: "52:54:00:12:34:56", IP: "192.168.122.50", Name: "web-01"})
	if err != nil {
		t.Fatalf("networkDHCPHostXMLText() error = %v", err)
	}
	if !strings.Contains(hostXML, `mac="52:54:00:12:34:56"`) || !strings.Contains(hostXML, `ip="192.168.122.50"`) || !strings.Contains(hostXML, `name="web-01"`) {
		t.Fatalf("host XML = %s", hostXML)
	}

	const text = `<network><ip family='ipv6' address='fd00::1' prefix='64'/><ip address='192.168.122.1' prefix='24'><dhcp><range start='192.168.122.2' end='192.168.122.254'/></dhcp></ip></network>`
	index, err := networkDHCPHostParentIndex(text)
	if err != nil || index != 1 {
		t.Fatalf("parent index = %d, %v", index, err)
	}
	if _, err := networkDHCPHostParentIndex(`<network><name>bridge</name></network>`); !errors.Is(err, domain.ErrNetworkUnsupported) {
		t.Fatalf("missing DHCP error = %v", err)
	}
	ipv6HostXML, err := networkDHCPHostXMLText(domain.NetworkDHCPHost{ID: "00:03:00:01:52:54:00:12:34:56", IP: "fd00::50", Name: "db-ipv6"})
	if err != nil {
		t.Fatalf("IPv6 networkDHCPHostXMLText() error = %v", err)
	}
	if !strings.Contains(ipv6HostXML, `id="00:03:00:01:52:54:00:12:34:56"`) || strings.Contains(ipv6HostXML, `mac=`) {
		t.Fatalf("IPv6 host XML = %s", ipv6HostXML)
	}
	ipv6Text := `<network><ip family='ipv6' address='fd00::1' prefix='64'><dhcp><range start='fd00::2' end='fd00::ffff'/></dhcp></ip><ip address='192.168.122.1' prefix='24'><dhcp><range start='192.168.122.2' end='192.168.122.254'/></dhcp></ip></network>`
	ipv6Index, err := networkDHCPHostParentIndexForFamily(ipv6Text, "ipv6")
	if err != nil || ipv6Index != 0 {
		t.Fatalf("IPv6 parent index = %d, %v", ipv6Index, err)
	}
}

func TestNetworkXMLFromDefinitionIPv6OnlySLAAC(t *testing.T) {
	xmlText, err := networkXMLFromDefinition(domain.VirtualNetworkDefinition{
		Name: "ipv6-only", Mode: "isolated", IPv6Address: "fd00:90::1", IPv6Prefix: 64, IPv6Mode: "slaac",
	})
	if err != nil {
		t.Fatalf("networkXMLFromDefinition() error = %v", err)
	}
	for _, fragment := range []string{`<ip family="ipv6" address="fd00:90::1" prefix="64"></ip>`} {
		if !strings.Contains(xmlText, fragment) {
			t.Fatalf("IPv6-only XML missing %q: %s", fragment, xmlText)
		}
	}
	if strings.Contains(xmlText, `<ip address=`) || strings.Contains(xmlText, `<dhcp>`) || strings.Contains(xmlText, `<forward`) {
		t.Fatalf("IPv6-only SLAAC XML contains unexpected IPv4/DHCP/forward: %s", xmlText)
	}
}

func TestNetworkXMLFromDefinitionDNS(t *testing.T) {
	xmlText, err := networkXMLFromDefinition(domain.VirtualNetworkDefinition{
		Name: "dns-net", Mode: "nat", IPv4Address: "10.91.0.1", Prefix: 24, DHCPStart: "10.91.0.2", DHCPEnd: "10.91.0.254",
		DNSDomain: "lab.example.com", DNSServers: []string{"1.1.1.1", "2606:4700:4700::1111"},
	})
	if err != nil {
		t.Fatalf("networkXMLFromDefinition() error = %v", err)
	}
	for _, fragment := range []string{`<domain name="lab.example.com"></domain>`, `<forwarder addr="1.1.1.1"></forwarder>`, `<forwarder addr="2606:4700:4700::1111"></forwarder>`} {
		if !strings.Contains(xmlText, fragment) {
			t.Fatalf("DNS XML missing %q: %s", fragment, xmlText)
		}
	}
}

func TestPrefixFromNetmask(t *testing.T) {
	if got := prefixFromNetmask("255.255.255.0"); got != 24 {
		t.Fatalf("prefix = %d", got)
	}
	if got := prefixFromNetmask("invalid"); got != 0 {
		t.Fatalf("invalid prefix = %d", got)
	}
}

func TestNetworkXMLFromDefinition(t *testing.T) {
	xmlText, err := networkXMLFromDefinition(domain.VirtualNetworkDefinition{
		Name: "prod-net", Mode: "nat", IPv4Address: "10.20.30.1", Prefix: 24,
		DHCPStart: "10.20.30.10", DHCPEnd: "10.20.30.200",
	})
	if err != nil {
		t.Fatalf("networkXMLFromDefinition() error = %v", err)
	}
	for _, fragment := range []string{
		"<name>prod-net</name>",
		"<forward mode=\"nat\"></forward>",
		"<ip address=\"10.20.30.1\" prefix=\"24\">",
		"<range start=\"10.20.30.10\" end=\"10.20.30.200\"></range>",
	} {
		if !strings.Contains(xmlText, fragment) {
			t.Fatalf("xml missing %q: %s", fragment, xmlText)
		}
	}

	isolated, err := networkXMLFromDefinition(domain.VirtualNetworkDefinition{
		Name: "isolated-net", Mode: "isolated", IPv4Address: "10.40.0.1", Prefix: 24,
		DHCPStart: "10.40.0.2", DHCPEnd: "10.40.0.254",
	})
	if err != nil {
		t.Fatalf("isolated networkXMLFromDefinition() error = %v", err)
	}
	if strings.Contains(isolated, "<forward") {
		t.Fatalf("isolated network has forward: %s", isolated)
	}

	bridge, err := networkXMLFromDefinition(domain.VirtualNetworkDefinition{
		Name: "prod-bridge", Mode: "bridge", Bridge: "br0",
	})
	if err != nil {
		t.Fatalf("bridge networkXMLFromDefinition() error = %v", err)
	}
	for _, fragment := range []string{
		"<name>prod-bridge</name>",
		`<forward mode="bridge"></forward>`,
		`<bridge name="br0"></bridge>`,
	} {
		if !strings.Contains(bridge, fragment) {
			t.Fatalf("bridge XML missing %q: %s", fragment, bridge)
		}
	}
	if strings.Contains(bridge, "<ip") || strings.Contains(bridge, "<dhcp") {
		t.Fatalf("bridge XML must not manage IP or DHCP: %s", bridge)
	}
}

func TestUpdateNetworkXMLSwitchesBridgeAndManagedModes(t *testing.T) {
	const managed = `<network><name>lab-net</name><forward mode='nat'></forward><bridge name='virbr9'/><ip address='10.20.30.1' prefix='24'><dhcp><range start='10.20.30.2' end='10.20.30.254'/></dhcp></ip></network>`
	bridgeXML, err := updateNetworkXML(managed, domain.VirtualNetworkDefinition{Name: "lab-net", Mode: "bridge", Bridge: "br1"})
	if err != nil {
		t.Fatalf("update to bridge error = %v", err)
	}
	for _, fragment := range []string{`<forward mode="bridge"></forward>`, `<bridge name="br1"></bridge>`} {
		if !strings.Contains(bridgeXML, fragment) {
			t.Fatalf("bridge update missing %q: %s", fragment, bridgeXML)
		}
	}
	if strings.Contains(bridgeXML, "<ip") || strings.Contains(bridgeXML, "<dhcp") {
		t.Fatalf("bridge update retained IP or DHCP: %s", bridgeXML)
	}

	const external = `<network><name>bridge-net</name><forward mode='bridge'></forward><bridge name='br1'/></network>`
	managedXML, err := updateNetworkXML(external, domain.VirtualNetworkDefinition{
		Name: "bridge-net", Mode: "nat", IPv4Address: "10.60.0.1", Prefix: 24,
	})
	if err != nil {
		t.Fatalf("update to managed error = %v", err)
	}
	if strings.Contains(managedXML, "br1") || !strings.Contains(managedXML, `<forward mode="nat"></forward>`) || !strings.Contains(managedXML, `<ip address="10.60.0.1" prefix="24">`) {
		t.Fatalf("managed update retained bridge: %s", managedXML)
	}
}

func TestUpdateNetworkXMLPreservesDHCPHosts(t *testing.T) {
	const source = `<network><name>lab-net</name><uuid>a5d720f5-2a70-4282-8f2e-f497037cb4e2</uuid><forward mode='nat'><nat><port start='1024' end='65535'/></nat></forward><bridge name='virbr9' stp='on' delay='0'/><mac address='52:54:00:00:00:01'/><ip address='10.20.30.1' netmask='255.255.255.0'><dhcp><range start='10.20.30.2' end='10.20.30.254'/><host mac='52:54:00:12:34:56' ip='10.20.30.50'/></dhcp></ip><ip family='ipv6' address='fd00::1' prefix='64'/></network>`
	xmlText, err := updateNetworkXML(source, domain.VirtualNetworkDefinition{
		Name: "lab-net", Mode: "isolated", IPv4Address: "10.50.0.1", Prefix: 24,
		DHCPStart: "10.50.0.20", DHCPEnd: "10.50.0.120",
	})
	if err != nil {
		t.Fatalf("updateNetworkXML() error = %v", err)
	}
	for _, fragment := range []string{
		`<name>lab-net</name>`,
		`<uuid>a5d720f5-2a70-4282-8f2e-f497037cb4e2</uuid>`,
		`<ip address="10.50.0.1" prefix="24">`,
		`<range start="10.50.0.20" end="10.50.0.120"></range>`,
		`<host mac="52:54:00:12:34:56" ip="10.20.30.50"></host>`,
	} {
		if !strings.Contains(xmlText, fragment) {
			t.Fatalf("updated XML missing %q: %s", fragment, xmlText)
		}
	}
	if strings.Contains(xmlText, "<forward") || strings.Contains(xmlText, "netmask=") || strings.Contains(xmlText, "family=\"ipv6\"") {
		t.Fatalf("isolated update retained forward/netmask/IPv6: %s", xmlText)
	}
}

func TestNetworkXMLFromDefinitionIncludesIPv6(t *testing.T) {
	xmlText, err := networkXMLFromDefinition(domain.VirtualNetworkDefinition{
		Name: "dual-net", Mode: "nat", IPv4Address: "10.20.30.1", Prefix: 24,
		DHCPStart: "10.20.30.2", DHCPEnd: "10.20.30.254",
		IPv6Address: "fd00:20:30::1", IPv6Prefix: 64, IPv6Mode: "dhcpv6", IPv6DHCPStart: "fd00:20:30::2", IPv6DHCPEnd: "fd00:20:30::ffff",
		NATIPv6: true,
	})
	if err != nil {
		t.Fatalf("networkXMLFromDefinition() error = %v", err)
	}
	for _, fragment := range []string{
		`<ip address="10.20.30.1" prefix="24">`,
		`<ip family="ipv6" address="fd00:20:30::1" prefix="64">`,
		`<range start="fd00:20:30::2" end="fd00:20:30::ffff"></range>`,
		`<nat ipv6="yes"></nat>`,
	} {
		if !strings.Contains(xmlText, fragment) {
			t.Fatalf("dual-stack XML missing %q: %s", fragment, xmlText)
		}
	}
}

func TestUpdateNetworkXMLUpdatesAndRemovesIPv6(t *testing.T) {
	const source = `<network><name>dual-net</name><forward mode='nat'/><ip address='10.20.30.1' prefix='24'><dhcp><range start='10.20.30.2' end='10.20.30.254'/></dhcp></ip><ip family='ipv6' address='fd00:20:30::1' prefix='64'><dhcp><range start='fd00:20:30::2' end='fd00:20:30::ffff'/><host mac='52:54:00:12:34:56' ip='fd00:20:30::50'/></dhcp></ip></network>`
	xmlText, err := updateNetworkXML(source, domain.VirtualNetworkDefinition{
		Name: "dual-net", Mode: "nat", IPv4Address: "10.20.30.1", Prefix: 24, DHCPStart: "10.20.30.2", DHCPEnd: "10.20.30.254",
		IPv6Address: "fd00:99::1", IPv6Prefix: 64, IPv6Mode: "dhcpv6", IPv6DHCPStart: "fd00:99::2", IPv6DHCPEnd: "fd00:99::ffff",
		NATIPv6: true,
	})
	if err != nil {
		t.Fatalf("update IPv6 error = %v", err)
	}
	for _, fragment := range []string{`<ip family="ipv6" address="fd00:99::1" prefix="64">`, `<range start="fd00:99::2" end="fd00:99::ffff"></range>`, `<host mac="52:54:00:12:34:56" ip="fd00:20:30::50"></host>`, `<nat ipv6="yes"></nat>`} {
		if !strings.Contains(xmlText, fragment) {
			t.Fatalf("updated IPv6 XML missing %q: %s", fragment, xmlText)
		}
	}
	withoutIPv6, err := updateNetworkXML(xmlText, domain.VirtualNetworkDefinition{
		Name: "dual-net", Mode: "nat", IPv4Address: "10.20.30.1", Prefix: 24, DHCPStart: "10.20.30.2", DHCPEnd: "10.20.30.254",
	})
	if err != nil {
		t.Fatalf("remove IPv6 error = %v", err)
	}
	if strings.Contains(withoutIPv6, "family=\"ipv6\"") {
		t.Fatalf("IPv6 was not removed: %s", withoutIPv6)
	}
}

func TestUpdateNetworkXMLAddsForwardAndDHCP(t *testing.T) {
	const source = `<network><name>lab-net</name><bridge name='virbr9'/><ip address='10.20.30.1' prefix='24'/></network>`
	xmlText, err := updateNetworkXML(source, domain.VirtualNetworkDefinition{
		Name: "lab-net", Mode: "nat", IPv4Address: "10.50.0.1", Prefix: 24,
		DHCPStart: "10.50.0.20", DHCPEnd: "10.50.0.120",
	})
	if err != nil {
		t.Fatalf("updateNetworkXML() error = %v", err)
	}
	for _, fragment := range []string{
		`<forward mode="nat"></forward>`,
		`<dhcp>`,
		`<range start="10.50.0.20" end="10.50.0.120"></range>`,
	} {
		if !strings.Contains(xmlText, fragment) {
			t.Fatalf("updated XML missing %q: %s", fragment, xmlText)
		}
	}
	if strings.Index(xmlText, "<forward") > strings.Index(xmlText, "<bridge") {
		t.Fatalf("forward must precede bridge: %s", xmlText)
	}
}

func TestUpdateNetworkXMLIPv6OnlyAndDNS(t *testing.T) {
	const source = `<network><name>dual-net</name><forward mode='nat'><nat ipv6='yes'/></forward><ip address='10.20.30.1' prefix='24'><dhcp><range start='10.20.30.2' end='10.20.30.254'/></dhcp></ip><ip family='ipv6' address='fd00:91::1' prefix='64'/></network>`
	xmlText, err := updateNetworkXML(source, domain.VirtualNetworkDefinition{
		Name: "dual-net", Mode: "isolated", IPv6Address: "fd00:91::1", IPv6Prefix: 64, IPv6Mode: "slaac",
		DNSDomain: "lab.example.com", DNSServers: []string{"2606:4700:4700::1111"},
	})
	if err != nil {
		t.Fatalf("updateNetworkXML() error = %v", err)
	}
	for _, fragment := range []string{`<ip family="ipv6" address="fd00:91::1" prefix="64"></ip>`, `<domain name="lab.example.com"></domain>`, `<forwarder addr="2606:4700:4700::1111"></forwarder>`} {
		if !strings.Contains(xmlText, fragment) {
			t.Fatalf("IPv6-only update missing %q: %s", fragment, xmlText)
		}
	}
	if strings.Contains(xmlText, `address="10.20.30.1"`) || strings.Contains(xmlText, `<forward `) || strings.Contains(xmlText, `<nat `) {
		t.Fatalf("IPv6-only update retained IPv4/forward/NAT: %s", xmlText)
	}
}

func TestDHCPLeaseFromLibvirt(t *testing.T) {
	expiry := time.Unix(1_700_000_000, 0).UTC()
	lease := dhcpLeaseFromLibvirt(libvirt.NetworkDHCPLease{
		Iface: "vnet0", ExpiryTime: expiry, Type: libvirt.IP_ADDR_TYPE_IPV4,
		Mac: "52:54:00:12:34:56", Iaid: "iaid-1", IPaddr: "192.168.122.101", Prefix: 24,
		Hostname: "demo-web-01", Clientid: "client-1",
	})
	if lease.Interface != "vnet0" || lease.Family != "ipv4" || lease.IPAddress != "192.168.122.101" || lease.Prefix != 24 {
		t.Fatalf("lease = %+v", lease)
	}
	if lease.ExpiresAt == nil || !lease.ExpiresAt.Equal(expiry) || lease.Hostname != "demo-web-01" || lease.ClientID != "client-1" {
		t.Fatalf("lease metadata = %+v", lease)
	}

	ipv6 := dhcpLeaseFromLibvirt(libvirt.NetworkDHCPLease{Type: libvirt.IP_ADDR_TYPE_IPV6, IPaddr: "fd00::10", Prefix: 64})
	if ipv6.Family != "ipv6" || ipv6.ExpiresAt != nil {
		t.Fatalf("ipv6 lease = %+v", ipv6)
	}
}
