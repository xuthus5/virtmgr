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
		`<ip family="ipv6" address="fd00::1" prefix="64"></ip>`,
	} {
		if !strings.Contains(xmlText, fragment) {
			t.Fatalf("updated XML missing %q: %s", fragment, xmlText)
		}
	}
	if strings.Contains(xmlText, "<forward") || strings.Contains(xmlText, "netmask=") {
		t.Fatalf("isolated update retained forward/netmask: %s", xmlText)
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
