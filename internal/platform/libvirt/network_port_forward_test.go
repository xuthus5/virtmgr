package libvirtbackend

import (
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

func TestBuildPortForwardRulesetUsesAddressFamilyGuards(t *testing.T) {
	ruleset, err := buildPortForwardRuleset([]domain.PortForward{
		{Network: "default", Protocol: domain.PortForwardProtocolTCP, GuestAddress: "192.168.122.50", HostPort: 8443, GuestPort: 443, Enabled: true},
		{Network: "default", Protocol: domain.PortForwardProtocolUDP, GuestAddress: "fd00:122::50", HostPort: 5353, GuestPort: 53, Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"meta nfproto ipv4 tcp dport 8443 dnat to 192.168.122.50:443",
		"meta nfproto ipv6 udp dport 5353 dnat to [fd00:122::50]:53",
		"ip daddr 192.168.122.50 tcp dport 443 accept",
		"ip6 daddr fd00:122::50 udp dport 53 accept",
	} {
		if !strings.Contains(ruleset, expected) {
			t.Fatalf("ruleset missing %q:\n%s", expected, ruleset)
		}
	}
}

func TestBuildPortForwardRulesetUsesExplicitHostAddress(t *testing.T) {
	ruleset, err := buildPortForwardRuleset([]domain.PortForward{{
		Network: "default", Protocol: domain.PortForwardProtocolTCP, HostAddress: "2001:db8::10",
		GuestAddress: "fd00:122::50", HostPort: 443, GuestPort: 443, Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ruleset, "ip6 daddr 2001:db8::10 tcp dport 443 dnat to [fd00:122::50]:443") {
		t.Fatalf("explicit host address rule missing:\n%s", ruleset)
	}
}
