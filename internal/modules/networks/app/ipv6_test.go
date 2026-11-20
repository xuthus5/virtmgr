package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"virtmgr/internal/domain"
)

func TestCreateIPv6OnlyNetwork(t *testing.T) {
	repository := &fakeRepository{}
	_, err := New(repository, nil, false).Create(context.Background(), CreateNetworkCommand{
		Spec: domain.VirtualNetworkCreateSpec{
			Name: "ipv6-only-lab", Mode: "isolated", IPv6Subnet: "fd00:60::/64", IPv6Mode: "dhcpv6",
		},
		Request: adminRequest(),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.created.IPv4Address != "" || repository.created.IPv6Address != "fd00:60::1" || repository.created.IPv6Prefix != 64 {
		t.Fatalf("definition = %+v", repository.created)
	}
	if repository.created.IPv6Mode != "dhcpv6" || repository.created.IPv6DHCPStart != "fd00:60::2" || repository.created.IPv6DHCPEnd == "" {
		t.Fatalf("DHCPv6 definition = %+v", repository.created)
	}
}

func TestCreateIPv6SLAACNetwork(t *testing.T) {
	repository := &fakeRepository{}
	_, err := New(repository, nil, false).Create(context.Background(), CreateNetworkCommand{
		Spec: domain.VirtualNetworkCreateSpec{
			Name: "ipv6-slaac-lab", Mode: "isolated", IPv6Subnet: "fd00:61::/64", IPv6Mode: "slaac",
		},
		Request: adminRequest(),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.created.IPv6Mode != "slaac" || repository.created.IPv6DHCPStart != "" || repository.created.IPv6DHCPEnd != "" {
		t.Fatalf("definition = %+v", repository.created)
	}
}

func TestCreateIPv6NAT66DefaultAndExplicitDisable(t *testing.T) {
	repository := &fakeRepository{}
	service := New(repository, nil, false)
	if _, err := service.Create(context.Background(), CreateNetworkCommand{
		Spec:    domain.VirtualNetworkCreateSpec{Name: "dual-nat66", Mode: "nat", Subnet: "10.81.0.0/24", IPv6Subnet: "fd00:81::/64"},
		Request: adminRequest(),
	}); err != nil {
		t.Fatalf("default NAT66 Create() error = %v", err)
	}
	if !repository.created.NATIPv6 {
		t.Fatalf("NAT66 should default to enabled: %+v", repository.created)
	}

	disabled := false
	if _, err := service.Create(context.Background(), CreateNetworkCommand{
		Spec:    domain.VirtualNetworkCreateSpec{Name: "dual-no-nat66", Mode: "nat", Subnet: "10.82.0.0/24", IPv6Subnet: "fd00:82::/64", NATIPv6: &disabled},
		Request: adminRequest(),
	}); err != nil {
		t.Fatalf("disabled NAT66 Create() error = %v", err)
	}
	if repository.created.NATIPv6 {
		t.Fatalf("NAT66 should be disabled: %+v", repository.created)
	}
}

func TestCreateNetworkIPv6Validation(t *testing.T) {
	enabled := true
	tests := []struct {
		name string
		spec domain.VirtualNetworkCreateSpec
	}{
		{name: "slaac requires 64", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", IPv6Subnet: "fd00:63::/72", IPv6Mode: "slaac"}},
		{name: "slaac rejects dhcp range", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", IPv6Subnet: "fd00:63::/64", IPv6Mode: "slaac", IPv6DHCPStart: "fd00:63::2", IPv6DHCPEnd: "fd00:63::10"}},
		{name: "link local prefix", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", IPv6Subnet: "fe80::/64"}},
		{name: "multicast prefix", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", IPv6Subnet: "ff00::/64"}},
		{name: "loopback prefix", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", IPv6Subnet: "::1/128"}},
		{name: "unspecified prefix", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", IPv6Subnet: "::/64"}},
		{name: "invalid mode", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", IPv6Subnet: "fd00:64::/64", IPv6Mode: "ra"}},
		{name: "range outside prefix", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", IPv6Subnet: "fd00:65::/64", IPv6DHCPStart: "fd00:66::2", IPv6DHCPEnd: "fd00:66::10"}},
		{name: "range from gateway", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", IPv6Subnet: "fd00:65::/64", IPv6DHCPStart: "fd00:65::1", IPv6DHCPEnd: "fd00:65::10"}},
		{name: "partial range", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", IPv6Subnet: "fd00:65::/64", IPv6DHCPStart: "fd00:65::2"}},
		{name: "nat66 without ipv6", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "nat", Subnet: "10.90.0.0/24", NATIPv6: &enabled}},
		{name: "invalid dns domain", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", Subnet: "10.90.0.0/24", DNSDomain: "bad domain"}},
		{name: "dns multicast", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", Subnet: "10.90.0.0/24", DNSServers: []string{"ff02::1"}}},
		{name: "dns link local", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", Subnet: "10.90.0.0/24", DNSServers: []string{"fe80::1"}}},
		{name: "invalid duid shape", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "isolated", IPv6Subnet: "fd00:67::/64", IPv6Mode: "dhcpv6", DNSServers: []string{"not-an-ip"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			_, err := New(repository, nil, false).Create(context.Background(), CreateNetworkCommand{Spec: test.spec, Request: adminRequest()})
			if !errors.Is(err, domain.ErrInvalidInput) || repository.called {
				t.Fatalf("error = %v called=%v", err, repository.called)
			}
		})
	}
}

func TestCreateNetworkNormalizesDNS(t *testing.T) {
	repository := &fakeRepository{}
	_, err := New(repository, nil, false).Create(context.Background(), CreateNetworkCommand{
		Spec: domain.VirtualNetworkCreateSpec{
			Name: "dns-lab", Mode: "isolated", Subnet: "10.91.0.0/24",
			DNSDomain: " Lab.Example.COM. ", DNSServers: []string{" 1.1.1.1 ", "2606:4700:4700::1111", "1.1.1.1"},
		},
		Request: adminRequest(),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.created.DNSDomain != "lab.example.com" || !reflect.DeepEqual(repository.created.DNSServers, []string{"1.1.1.1", "2606:4700:4700::1111"}) {
		t.Fatalf("DNS definition = %+v", repository.created)
	}
}

func TestCreateNetworkRejectsOverlappingIPv6Subnet(t *testing.T) {
	repository := &fakeRepository{networks: []domain.VirtualNetwork{{
		Name: "existing-ipv6", Addresses: []domain.NetworkAddress{{Family: "ipv6", Address: "fd00:70::1", Prefix: 64}},
	}}}
	_, err := New(repository, nil, false).Create(context.Background(), CreateNetworkCommand{
		Spec:    domain.VirtualNetworkCreateSpec{Name: "overlap-ipv6", Mode: "isolated", IPv6Subnet: "fd00:70::/72"},
		Request: adminRequest(),
	})
	if !errors.Is(err, domain.ErrConflict) || repository.called {
		t.Fatalf("error = %v called=%v", err, repository.called)
	}
}

func TestUpdateNetworkProtectsIPv6StaticHosts(t *testing.T) {
	network := domain.VirtualNetwork{
		Name: "dual-net", Active: false, Persistent: true, ForwardMode: "nat",
		Addresses: []domain.NetworkAddress{
			{Family: "ipv4", Address: "10.20.30.1", Prefix: 24, DHCPEnabled: true, DHCPStart: "10.20.30.2", DHCPEnd: "10.20.30.254"},
			{Family: "ipv6", Address: "fd00:81::1", Prefix: 64, IPv6Mode: "dhcpv6", DHCPEnabled: true, DHCPStart: "fd00:81::2", DHCPEnd: "fd00:81::ffff", Hosts: []domain.NetworkDHCPHost{{ID: "00:03:00:01:52:54:00:12:34:56", IP: "fd00:81::50"}}},
		},
	}
	tests := []struct {
		name string
		spec domain.VirtualNetworkUpdateSpec
	}{
		{name: "move prefix", spec: domain.VirtualNetworkUpdateSpec{Mode: "nat", Subnet: "10.20.30.0/24", IPv6Subnet: "fd00:82::/64", IPv6Mode: "dhcpv6"}},
		{name: "remove ipv6", spec: domain.VirtualNetworkUpdateSpec{Mode: "nat", Subnet: "10.20.30.0/24"}},
		{name: "switch to slaac", spec: domain.VirtualNetworkUpdateSpec{Mode: "nat", Subnet: "10.20.30.0/24", IPv6Subnet: "fd00:81::/64", IPv6Mode: "slaac"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{networks: []domain.VirtualNetwork{network}}
			_, err := New(repository, nil, false).Update(context.Background(), UpdateNetworkCommand{Name: network.Name, Spec: test.spec, Request: adminRequest()})
			if !errors.Is(err, domain.ErrConflict) || repository.called {
				t.Fatalf("error = %v called=%v", err, repository.called)
			}
		})
	}
}

func TestUpdateNetworkProtectsIPv4StaticHosts(t *testing.T) {
	network := domain.VirtualNetwork{
		Name: "dual-net", Active: false, Persistent: true, ForwardMode: "nat",
		Addresses: []domain.NetworkAddress{{
			Family: "ipv4", Address: "10.20.30.1", Prefix: 24, DHCPEnabled: true, DHCPStart: "10.20.30.2", DHCPEnd: "10.20.30.254",
			Hosts: []domain.NetworkDHCPHost{{MAC: "52:54:00:12:34:56", IP: "10.20.30.50"}},
		}},
	}
	repository := &fakeRepository{networks: []domain.VirtualNetwork{network}}
	_, err := New(repository, nil, false).Update(context.Background(), UpdateNetworkCommand{
		Name: network.Name, Spec: domain.VirtualNetworkUpdateSpec{Mode: "isolated", IPv6Subnet: "fd00:83::/64"}, Request: adminRequest(),
	})
	if !errors.Is(err, domain.ErrConflict) || repository.called {
		t.Fatalf("error = %v called=%v", err, repository.called)
	}
}

func TestDeleteDHCPHostByIPv6DUIDIPOrName(t *testing.T) {
	network := domain.VirtualNetwork{
		Name: "dual-net", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{
			Family: "ipv6", Address: "fd00:84::1", Prefix: 64, IPv6Mode: "dhcpv6", DHCPEnabled: true,
			DHCPStart: "fd00:84::2", DHCPEnd: "fd00:84::ffff",
			Hosts: []domain.NetworkDHCPHost{{ID: "00:03:00:01:52:54:00:12:34:56", IP: "fd00:84::50", Name: "db-ipv6"}},
		}},
	}
	for _, key := range []string{"00:03:00:01:52:54:00:12:34:56", "FD00:84::50", "DB-IPV6"} {
		t.Run(key, func(t *testing.T) {
			repository := &fakeRepository{networks: []domain.VirtualNetwork{network}}
			if _, err := New(repository, nil, false).DeleteDHCPHost(context.Background(), DeleteNetworkDHCPHostCommand{Name: network.Name, Key: key, Request: adminRequest()}); err != nil {
				t.Fatalf("DeleteDHCPHost(%q) error = %v", key, err)
			}
			if repository.deletedHost.IP != "fd00:84::50" {
				t.Fatalf("deleted host = %+v", repository.deletedHost)
			}
		})
	}
}

func TestAddDHCPHostIPv6RequiresDUIDOrName(t *testing.T) {
	network := domain.VirtualNetwork{
		Name: "dual-net", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{Family: "ipv6", Address: "fd00:85::1", Prefix: 64, IPv6Mode: "dhcpv6", DHCPEnabled: true, DHCPStart: "fd00:85::2", DHCPEnd: "fd00:85::ffff"}},
	}
	tests := []struct {
		name string
		spec domain.NetworkDHCPHostCreateSpec
		want error
	}{
		{name: "mac rejected", spec: domain.NetworkDHCPHostCreateSpec{MAC: "52:54:00:12:34:56", IP: "fd00:85::50"}, want: domain.ErrInvalidInput},
		{name: "identity required", spec: domain.NetworkDHCPHostCreateSpec{IP: "fd00:85::50"}, want: domain.ErrInvalidInput},
		{name: "invalid duid", spec: domain.NetworkDHCPHostCreateSpec{ID: "not-a-duid", IP: "fd00:85::50"}, want: domain.ErrInvalidInput},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{networks: []domain.VirtualNetwork{network}}
			_, err := New(repository, nil, false).AddDHCPHost(context.Background(), AddNetworkDHCPHostCommand{Name: network.Name, Spec: test.spec, Request: adminRequest()})
			if !errors.Is(err, test.want) || repository.called {
				t.Fatalf("error = %v called=%v", err, repository.called)
			}
		})
	}
}

func TestAddDHCPHostIPv4RejectsDHCPv6Identity(t *testing.T) {
	network := domain.VirtualNetwork{
		Name: "dual-net", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{Family: "ipv4", Address: "10.86.0.1", Prefix: 24, DHCPEnabled: true, DHCPStart: "10.86.0.2", DHCPEnd: "10.86.0.254"}},
	}
	repository := &fakeRepository{networks: []domain.VirtualNetwork{network}}
	_, err := New(repository, nil, false).AddDHCPHost(context.Background(), AddNetworkDHCPHostCommand{
		Name:    network.Name,
		Spec:    domain.NetworkDHCPHostCreateSpec{MAC: "52:54:00:12:34:56", ID: "00:03:00:01:52:54:00:12:34:56", IP: "10.86.0.50"},
		Request: adminRequest(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) || repository.called {
		t.Fatalf("error = %v called=%v", err, repository.called)
	}
}

func TestDiagnosticsNormalizesChecks(t *testing.T) {
	repository := &fakeRepository{diagnostics: domain.NetworkDiagnostics{Active: true}}
	result, err := New(repository, nil, false).Diagnose(context.Background(), " dual-net ")
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if result.Network != "dual-net" || result.Checks == nil {
		t.Fatalf("diagnostics = %+v", result)
	}
}

func TestReserveIPv6LeaseCreatesStaticDUIDReservation(t *testing.T) {
	network := domain.VirtualNetwork{
		Name: "dual-net", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{Family: "ipv6", Address: "fd00:87::1", Prefix: 64, IPv6Mode: "dhcpv6", DHCPEnabled: true, DHCPStart: "fd00:87::2", DHCPEnd: "fd00:87::ffff"}},
	}
	repository := &fakeRepository{
		networks: []domain.VirtualNetwork{network},
		leases:   []domain.DHCPLease{{Family: "ipv6", IPAddress: "fd00:87::50", Prefix: 64, ClientID: "00-03-00-01-52-54-00-12-34-56", Hostname: "db-v6"}},
	}
	updated, err := New(repository, nil, false).ReserveLease(context.Background(), ReserveNetworkLeaseCommand{
		Name: network.Name, Spec: domain.NetworkLeaseReservationSpec{IP: "FD00:87::50"}, Request: adminRequest(),
	})
	if err != nil {
		t.Fatalf("ReserveLease() error = %v", err)
	}
	if updated.Name != network.Name || repository.addedHost.ID != "00:03:00:01:52:54:00:12:34:56" || repository.addedHost.IP != "fd00:87::50" || repository.addedHost.Name != "db-v6" {
		t.Fatalf("reserved host = %+v updated=%+v", repository.addedHost, updated)
	}
}

func TestReserveIPv4LeaseUsesMAC(t *testing.T) {
	network := domain.VirtualNetwork{
		Name: "v4-net", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{Family: "ipv4", Address: "10.87.0.1", Prefix: 24, DHCPEnabled: true, DHCPStart: "10.87.0.2", DHCPEnd: "10.87.0.254"}},
	}
	repository := &fakeRepository{
		networks: []domain.VirtualNetwork{network},
		leases:   []domain.DHCPLease{{Family: "ipv4", IPAddress: "10.87.0.50", Prefix: 24, MAC: "52:54:00:12:34:56", Hostname: "web-v4"}},
	}
	if _, err := New(repository, nil, false).ReserveLease(context.Background(), ReserveNetworkLeaseCommand{
		Name: network.Name, Spec: domain.NetworkLeaseReservationSpec{IP: "10.87.0.50"}, Request: adminRequest(),
	}); err != nil {
		t.Fatalf("ReserveLease() error = %v", err)
	}
	if repository.addedHost.MAC != "52:54:00:12:34:56" || repository.addedHost.Name != "web-v4" {
		t.Fatalf("reserved host = %+v", repository.addedHost)
	}
}

func TestReserveDHCPv6LeaseRequiresIdentity(t *testing.T) {
	network := domain.VirtualNetwork{
		Name: "dual-net", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{Family: "ipv6", Address: "fd00:88::1", Prefix: 64, IPv6Mode: "dhcpv6", DHCPEnabled: true, DHCPStart: "fd00:88::2", DHCPEnd: "fd00:88::ffff"}},
	}
	repository := &fakeRepository{
		networks: []domain.VirtualNetwork{network},
		leases:   []domain.DHCPLease{{Family: "ipv6", IPAddress: "fd00:88::50", Prefix: 64}},
	}
	_, err := New(repository, nil, false).ReserveLease(context.Background(), ReserveNetworkLeaseCommand{
		Name: network.Name, Spec: domain.NetworkLeaseReservationSpec{IP: "fd00:88::50"}, Request: adminRequest(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) || repository.called {
		t.Fatalf("error = %v called=%v", err, repository.called)
	}
}
