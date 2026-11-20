package libvirtbackend

import (
	"strings"
	"testing"
)

func TestContainsNAT66RuleSupportsNFTablesAndIP6Tables(t *testing.T) {
	const prefix = "fd00:122::/64"
	nftRules := `table ip6 libvirt_network {
  chain guest_nat {
    ip6 saddr fd00:122::/64 ip6 daddr != fd00:122::/64 masquerade
  }
}`
	if !containsNAT66Rule(nftRules, prefix, false) {
		t.Fatal("nftables NAT66 rule was not detected")
	}
	if containsNAT66Rule(nftRules, "fd00:123::/64", false) {
		t.Fatal("unrelated prefix matched NAT66 rule")
	}
	iptablesNFTCompatRules := `table ip6 nat {
  chain LIBVIRT_PRT {
    ip6 saddr fd00:122::/64 ip6 daddr != fd00:122::/64 counter packets 0 bytes 0 xt target "MASQUERADE"
  }
}`
	if !containsNAT66Rule(iptablesNFTCompatRules, prefix, false) {
		t.Fatal("iptables-nft compatibility MASQUERADE rule was not detected")
	}
	legacyRules := "-A POSTROUTING -s fd00:122::/64 ! -d fd00:122::/64 -j MASQUERADE"
	if !containsNAT66Rule(legacyRules, prefix, true) {
		t.Fatal("ip6tables NAT66 rule was not detected")
	}
}

func TestManagedNetworkConfigDetectsRuntimeDrift(t *testing.T) {
	persistent := `<network><name>dual</name><forward mode='nat'><nat ipv6='yes'/></forward><bridge name='virbr9'/><domain name='lab.example'/><dns><forwarder addr='2001:db8::53'/></dns><ip address='10.0.0.1' prefix='24'><dhcp><range start='10.0.0.2' end='10.0.0.254'/></dhcp></ip><ip family='ipv6' address='fd00:90::1' prefix='64'><dhcp><range start='fd00:90::2' end='fd00:90::ff'/></dhcp></ip></network>`
	active := strings.Replace(persistent, "<range start='fd00:90::2' end='fd00:90::ff'/>", "<range start='fd00:90::10' end='fd00:90::ff'/>", 1)
	persistentConfig, err := parseManagedNetworkConfig(persistent)
	if err != nil {
		t.Fatal(err)
	}
	activeConfig, err := parseManagedNetworkConfig(active)
	if err != nil {
		t.Fatal(err)
	}
	details := compareManagedNetworkConfig(activeConfig, persistentConfig)
	if len(details) != 1 || !strings.Contains(details[0], "地址、DHCP") {
		t.Fatalf("drift details = %v", details)
	}
	if !persistentConfig.NATIPv6 || persistentConfig.DNSDomain != "lab.example" || persistentConfig.Bridge != "virbr9" {
		t.Fatalf("persistent config = %+v", persistentConfig)
	}
}

func TestManagedNetworkConfigIgnoresHostOrder(t *testing.T) {
	first := `<network><name>dual</name><ip family='ipv6' address='fd00:91::1' prefix='64'><dhcp><host id='00:03:00:01:aa' ip='fd00:91::50'/><host id='00:03:00:01:bb' ip='fd00:91::51'/></dhcp></ip></network>`
	second := `<network><name>dual</name><ip family='ipv6' address='fd00:91::1' prefix='64'><dhcp><host id='00:03:00:01:bb' ip='fd00:91::51'/><host id='00:03:00:01:aa' ip='fd00:91::50'/></dhcp></ip></network>`
	left, err := parseManagedNetworkConfig(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := parseManagedNetworkConfig(second)
	if err != nil {
		t.Fatal(err)
	}
	if details := compareManagedNetworkConfig(left, right); len(details) != 0 {
		t.Fatalf("unexpected drift = %v", details)
	}
}

func TestManagedNetworkConfigDetectsDNSHostAndMTUDrift(t *testing.T) {
	persistent := `<network><name>dns</name><forward mode='isolated'/><mtu size='9000'/><dns><host ip='fd00:91::50'><hostname>db.lab.example</hostname><hostname>db-v6.lab.example</hostname></host></dns><ip family='ipv6' address='fd00:91::1' prefix='64'/></network>`
	active := strings.Replace(strings.Replace(persistent, `<mtu size='9000'/>`, `<mtu size='1500'/>`, 1), `db-v6.lab.example`, `web.lab.example`, 1)
	persistentConfig, err := parseManagedNetworkConfig(persistent)
	if err != nil {
		t.Fatal(err)
	}
	activeConfig, err := parseManagedNetworkConfig(active)
	if err != nil {
		t.Fatal(err)
	}
	if persistentConfig.MTU != 9000 || activeConfig.MTU != 1500 {
		t.Fatalf("parsed MTU persistent=%d active=%d", persistentConfig.MTU, activeConfig.MTU)
	}
	details := compareManagedNetworkConfig(activeConfig, persistentConfig)
	if len(details) != 2 || !strings.Contains(strings.Join(details, ","), "MTU") || !strings.Contains(strings.Join(details, ","), "DNS") {
		t.Fatalf("drift details = %v", details)
	}
}

func TestParseVersionSupportsLibvirtAndDnsmasqFormats(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  [3]int
	}{
		{name: "libvirt", value: "12.0.0", want: [3]int{12, 0, 0}},
		{name: "dnsmasq multiline", value: "Dnsmasq version 2.90  Copyright (c) 2000-2024 Simon Kelley\nCompile time options: IPv6 GNU-getopt", want: [3]int{2, 90, 0}},
		{name: "dnsmasq release suffix", value: "Dnsmasq version 2.92rel2  Copyright (c) 2000-2025 Simon Kelley", want: [3]int{2, 92, 0}},
		{name: "version prefix", value: "v8.1.2", want: [3]int{8, 1, 2}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseVersion(test.value)
			if !ok {
				t.Fatalf("parseVersion(%q) did not find a version", test.value)
			}
			if got != test.want {
				t.Fatalf("parseVersion(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestCheckNetworkCompatibilityReportsBaseline(t *testing.T) {
	check := checkNetworkCompatibility("7.10.0", "2.90")
	if check.Status != "error" {
		t.Fatalf("status = %q, want error: %+v", check.Status, check)
	}
	check = checkNetworkCompatibility("12.0.0", "2.84")
	if check.Status != "error" {
		t.Fatalf("status = %q, want error: %+v", check.Status, check)
	}
	check = checkNetworkCompatibility("unknown build", "2.90")
	if check.Status != "warning" {
		t.Fatalf("status = %q, want warning: %+v", check.Status, check)
	}
	check = checkNetworkCompatibility("8.0.0", "2.85")
	if check.Status != "ok" {
		t.Fatalf("status = %q, want ok: %+v", check.Status, check)
	}
}
