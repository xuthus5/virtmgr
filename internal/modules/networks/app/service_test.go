package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

type fakeRepository struct {
	networks    []domain.VirtualNetwork
	created     domain.VirtualNetworkDefinition
	updated     domain.VirtualNetworkDefinition
	addedHost   domain.NetworkDHCPHost
	addedHosts  []domain.NetworkDHCPHost
	deletedHost domain.NetworkDHCPHost
	action      domain.VirtualNetworkAction
	actionFor   string
	deleted     string
	leaseFor    string
	leases      []domain.DHCPLease
	err         error
	called      bool
}

func (f *fakeRepository) ListNetworks(context.Context) ([]domain.VirtualNetwork, error) {
	return f.networks, f.err
}

func (f *fakeRepository) ListDHCPLeases(_ context.Context, networkName string) ([]domain.DHCPLease, error) {
	f.leaseFor = networkName
	return append([]domain.DHCPLease(nil), f.leases...), f.err
}

func (f *fakeRepository) CreateNetwork(_ context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error) {
	f.called = true
	f.created = definition
	if f.err != nil {
		return domain.VirtualNetwork{}, f.err
	}
	return domain.VirtualNetwork{Name: definition.Name, Active: definition.Start, ForwardMode: definition.Mode}, nil
}

func (f *fakeRepository) UpdateNetwork(_ context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error) {
	f.called = true
	f.updated = definition
	if f.err != nil {
		return domain.VirtualNetwork{}, f.err
	}
	return domain.VirtualNetwork{Name: definition.Name, ForwardMode: definition.Mode, Autostart: definition.Autostart}, nil
}

func (f *fakeRepository) AddNetworkDHCPHost(_ context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error) {
	f.called = true
	f.addedHost = host
	f.addedHosts = append(f.addedHosts, host)
	if f.err != nil {
		return domain.VirtualNetwork{}, f.err
	}
	return domain.VirtualNetwork{Name: name}, nil
}

func (f *fakeRepository) DeleteNetworkDHCPHost(_ context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error) {
	f.called = true
	f.deletedHost = host
	if f.err != nil {
		return domain.VirtualNetwork{}, f.err
	}
	return domain.VirtualNetwork{Name: name}, nil
}

func (f *fakeRepository) PerformNetworkAction(_ context.Context, name string, action domain.VirtualNetworkAction) error {
	f.called = true
	f.actionFor = name
	f.action = action
	return f.err
}

func (f *fakeRepository) DeleteNetwork(_ context.Context, name string) error {
	f.called = true
	f.deleted = name
	return f.err
}

type auditRecorder struct {
	events []domain.AuditEvent
	err    error
}

func (a *auditRecorder) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	a.events = append(a.events, event)
	return a.err
}

func adminRequest() MutationContext {
	return MutationContext{
		Session:   domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}},
		RequestID: "req-1",
		SourceIP:  "127.0.0.1",
	}
}

type dynamicReadOnly struct{ value bool }

func (m *dynamicReadOnly) ReadOnly() bool { return m.value }

func TestServiceUsesDynamicReadOnlyMode(t *testing.T) {
	mode := &dynamicReadOnly{value: true}
	service := New(&fakeRepository{}, nil, false, WithReadOnlyMode(mode))
	command := CreateNetworkCommand{
		Spec:    domain.VirtualNetworkCreateSpec{Name: "runtime-net", Mode: "isolated", Subnet: "10.99.0.0/24"},
		Request: adminRequest(),
	}
	if _, err := service.Create(context.Background(), command); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only error = %v", err)
	}
	mode.value = false
	if _, err := service.Create(context.Background(), command); err != nil {
		t.Fatalf("writable error = %v", err)
	}
}

func TestListSortsNetworks(t *testing.T) {
	service := New(&fakeRepository{networks: []domain.VirtualNetwork{{Name: "prod"}, {Name: "Default"}, {Name: "archive"}}}, nil, false)
	result, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.Total != 3 || result.Items[0].Name != "archive" || result.Items[1].Name != "Default" {
		t.Fatalf("result = %+v", result)
	}
}

func TestListNetworkErrorPropagates(t *testing.T) {
	want := errors.New("libvirt unavailable")
	_, err := New(&fakeRepository{err: want}, nil, false).List(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("List() error = %v", err)
	}
}

func TestListDHCPLeasesSortsAndValidates(t *testing.T) {
	repository := &fakeRepository{leases: []domain.DHCPLease{
		{IPAddress: "192.168.122.20", Hostname: "web-b", Family: "ipv4"},
		{IPAddress: "192.168.122.3", Hostname: "web-a", Family: "ipv4"},
	}}
	service := New(repository, nil, false)
	result, err := service.ListLeases(context.Background(), " default ")
	if err != nil {
		t.Fatalf("ListLeases() error = %v", err)
	}
	if repository.leaseFor != "default" || result.Network != "default" || result.Total != 2 {
		t.Fatalf("repository=%q result=%+v", repository.leaseFor, result)
	}
	if result.Items[0].IPAddress != "192.168.122.3" || result.Items[1].IPAddress != "192.168.122.20" {
		t.Fatalf("leases not sorted: %+v", result.Items)
	}
	if _, err := service.ListLeases(context.Background(), "../bad"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid name error = %v", err)
	}
}

func TestListDHCPLeasesPropagatesRepositoryError(t *testing.T) {
	want := errors.New("libvirt unavailable")
	_, err := New(&fakeRepository{err: want}, nil, false).ListLeases(context.Background(), "default")
	if !errors.Is(err, want) {
		t.Fatalf("ListLeases() error = %v", err)
	}
}

func TestListDHCPLeasesNormalizesEmptyItems(t *testing.T) {
	result, err := New(&fakeRepository{}, nil, false).ListLeases(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListLeases() error = %v", err)
	}
	if result.Items == nil || result.Total != 0 {
		t.Fatalf("empty lease list = %+v", result)
	}
}

func TestCreateNetworkNormalizesAndAudits(t *testing.T) {
	repository := &fakeRepository{}
	audit := &auditRecorder{}
	service := New(repository, audit, false)
	network, err := service.Create(context.Background(), CreateNetworkCommand{
		Spec: domain.VirtualNetworkCreateSpec{
			Name: " prod-net ", Mode: " NAT ", Subnet: "10.20.30.0/24", Autostart: true, Start: true,
		},
		Request: adminRequest(),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if network.Name != "prod-net" || !network.Active || network.ForwardMode != "nat" {
		t.Fatalf("network = %+v", network)
	}
	want := domain.VirtualNetworkDefinition{
		Name: "prod-net", Mode: "nat", IPv4Address: "10.20.30.1", Prefix: 24,
		DHCPStart: "10.20.30.2", DHCPEnd: "10.20.30.254", Autostart: true, Start: true,
	}
	if repository.created != want {
		t.Fatalf("definition = %+v, want %+v", repository.created, want)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "network.create" || audit.events[0].TargetID != "prod-net" {
		t.Fatalf("audit events = %+v", audit.events)
	}
	if audit.events[0].Detail["mode"] != "nat" || audit.events[0].Detail["dhcpStart"] != "10.20.30.2" {
		t.Fatalf("audit detail = %+v", audit.events[0].Detail)
	}
}

func TestUpdateNetworkNormalizesAndAudits(t *testing.T) {
	repository := &fakeRepository{}
	audit := &auditRecorder{}
	service := New(repository, audit, false)
	network, err := service.Update(context.Background(), UpdateNetworkCommand{
		Name:    "prod-net",
		Spec:    domain.VirtualNetworkUpdateSpec{Mode: "ISOLATED", Subnet: "10.50.0.0/24", DHCPStart: "10.50.0.20", DHCPEnd: "10.50.0.120", Autostart: true},
		Request: adminRequest(),
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if network.Name != "prod-net" || !network.Autostart || network.ForwardMode != "isolated" {
		t.Fatalf("network = %+v", network)
	}
	want := domain.VirtualNetworkDefinition{
		Name: "prod-net", Mode: "isolated", IPv4Address: "10.50.0.1", Prefix: 24,
		DHCPStart: "10.50.0.20", DHCPEnd: "10.50.0.120", Autostart: true,
	}
	if repository.updated != want {
		t.Fatalf("updated definition = %+v, want %+v", repository.updated, want)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "network.update" || audit.events[0].TargetID != "prod-net" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestCreateNetworkCustomDHCP(t *testing.T) {
	repository := &fakeRepository{}
	_, err := New(repository, nil, false).Create(context.Background(), CreateNetworkCommand{
		Spec: domain.VirtualNetworkCreateSpec{
			Name: "lab-net", Mode: "isolated", Subnet: "10.77.0.0/24",
			DHCPStart: "10.77.0.10", DHCPEnd: "10.77.0.100",
		},
		Request: adminRequest(),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.created.IPv4Address != "10.77.0.1" || repository.created.DHCPStart != "10.77.0.10" || repository.created.DHCPEnd != "10.77.0.100" {
		t.Fatalf("definition = %+v", repository.created)
	}
}

func TestCreateNetworkValidation(t *testing.T) {
	tests := []struct {
		name string
		spec domain.VirtualNetworkCreateSpec
	}{
		{name: "invalid name", spec: domain.VirtualNetworkCreateSpec{Name: "a", Mode: "nat", Subnet: "10.0.0.0/24"}},
		{name: "invalid mode", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "bridge", Subnet: "10.0.0.0/24"}},
		{name: "invalid subnet", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "nat", Subnet: "not-a-subnet"}},
		{name: "ipv6 subnet", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "nat", Subnet: "fd00::/64"}},
		{name: "subnet too small", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "nat", Subnet: "10.0.0.0/30"}},
		{name: "partial dhcp", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "nat", Subnet: "10.0.0.0/24", DHCPStart: "10.0.0.10"}},
		{name: "dhcp outside", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "nat", Subnet: "10.0.0.0/24", DHCPStart: "10.0.1.10", DHCPEnd: "10.0.1.100"}},
		{name: "dhcp order", spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "nat", Subnet: "10.0.0.0/24", DHCPStart: "10.0.0.100", DHCPEnd: "10.0.0.10"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			_, err := New(repository, nil, false).Create(context.Background(), CreateNetworkCommand{Spec: test.spec, Request: adminRequest()})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("Create() error = %v", err)
			}
			if repository.called {
				t.Fatal("invalid spec reached repository")
			}
		})
	}
}

func TestNetworkMutationsRequireOperatorAndWritableMode(t *testing.T) {
	for _, role := range []domain.Role{domain.RoleViewer} {
		request := adminRequest()
		request.Session.User.Role = role
		repository := &fakeRepository{}
		service := New(repository, nil, false)
		_, err := service.Create(context.Background(), CreateNetworkCommand{
			Spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "nat", Subnet: "10.0.0.0/24"}, Request: request,
		})
		if !errors.Is(err, domain.ErrForbidden) || repository.called {
			t.Fatalf("role %s create error = %v called=%v", role, err, repository.called)
		}
		if err := service.PerformAction(context.Background(), NetworkActionCommand{Name: "valid-net", Action: domain.VirtualNetworkActionStart, Request: request}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("role %s action error = %v", role, err)
		}
		if err := service.Delete(context.Background(), DeleteNetworkCommand{Name: "valid-net", Request: request}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("role %s delete error = %v", role, err)
		}
		if _, err := service.Update(context.Background(), UpdateNetworkCommand{Name: "valid-net", Spec: domain.VirtualNetworkUpdateSpec{Mode: "nat", Subnet: "10.0.0.0/24"}, Request: request}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("role %s update error = %v", role, err)
		}
		if _, err := service.AddDHCPHost(context.Background(), AddNetworkDHCPHostCommand{Name: "valid-net", Spec: domain.NetworkDHCPHostCreateSpec{MAC: "52:54:00:12:34:56", IP: "10.0.0.50"}, Request: request}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("role %s add DHCP host error = %v", role, err)
		}
		if _, err := service.DeleteDHCPHost(context.Background(), DeleteNetworkDHCPHostCommand{Name: "valid-net", MAC: "52:54:00:12:34:56", Request: request}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("role %s delete DHCP host error = %v", role, err)
		}
	}
	repository := &fakeRepository{}
	service := New(repository, nil, true)
	if _, err := service.Create(context.Background(), CreateNetworkCommand{Spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "nat", Subnet: "10.0.0.0/24"}, Request: adminRequest()}); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only create error = %v", err)
	}
	if err := service.PerformAction(context.Background(), NetworkActionCommand{Name: "valid-net", Action: domain.VirtualNetworkActionStart, Request: adminRequest()}); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only action error = %v", err)
	}
	if err := service.Delete(context.Background(), DeleteNetworkCommand{Name: "valid-net", Request: adminRequest()}); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only delete error = %v", err)
	}
	if _, err := service.Update(context.Background(), UpdateNetworkCommand{Name: "valid-net", Spec: domain.VirtualNetworkUpdateSpec{Mode: "nat", Subnet: "10.0.0.0/24"}, Request: adminRequest()}); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only update error = %v", err)
	}
	if _, err := service.AddDHCPHost(context.Background(), AddNetworkDHCPHostCommand{Name: "valid-net", Spec: domain.NetworkDHCPHostCreateSpec{MAC: "52:54:00:12:34:56", IP: "10.0.0.50"}, Request: adminRequest()}); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only add DHCP host error = %v", err)
	}
	if _, err := service.DeleteDHCPHost(context.Background(), DeleteNetworkDHCPHostCommand{Name: "valid-net", MAC: "52:54:00:12:34:56", Request: adminRequest()}); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only delete DHCP host error = %v", err)
	}
}

func TestAddDHCPHostWorksForActiveNetworkAndAudits(t *testing.T) {
	repository := &fakeRepository{networks: []domain.VirtualNetwork{{
		Name: "default", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{
			Family: "ipv4", Address: "192.168.122.1", Prefix: 24, DHCPEnabled: true,
			DHCPStart: "192.168.122.2", DHCPEnd: "192.168.122.254",
		}},
	}}}
	audit := &auditRecorder{}
	service := New(repository, audit, false)
	network, err := service.AddDHCPHost(context.Background(), AddNetworkDHCPHostCommand{
		Name: " default ",
		Spec: domain.NetworkDHCPHostCreateSpec{
			MAC: "52:54:00:AB:CD:EF", IP: "192.168.122.50", Name: "web-01",
		},
		Request: adminRequest(),
	})
	if err != nil {
		t.Fatalf("AddDHCPHost() error = %v", err)
	}
	if network.Name != "default" || repository.addedHost.MAC != "52:54:00:ab:cd:ef" || repository.addedHost.IP != "192.168.122.50" || repository.addedHost.Name != "web-01" {
		t.Fatalf("network=%+v host=%+v", network, repository.addedHost)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "network.dhcp.host.add" || audit.events[0].TargetID != "default" {
		t.Fatalf("audit events = %+v", audit.events)
	}
	if audit.events[0].Detail["mac"] != "52:54:00:ab:cd:ef" || audit.events[0].Detail["ip"] != "192.168.122.50" {
		t.Fatalf("audit detail = %+v", audit.events[0].Detail)
	}
}

func TestAddDHCPHostValidatesAddressAndConflicts(t *testing.T) {
	network := domain.VirtualNetwork{
		Name: "default", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{
			Family: "ipv4", Address: "192.168.122.1", Prefix: 24, DHCPEnabled: true,
			DHCPStart: "192.168.122.2", DHCPEnd: "192.168.122.254",
			Hosts: []domain.NetworkDHCPHost{{MAC: "52:54:00:12:34:56", IP: "192.168.122.50", Name: "existing"}},
		}},
	}
	tests := []struct {
		name string
		spec domain.NetworkDHCPHostCreateSpec
		want error
	}{
		{name: "invalid MAC", spec: domain.NetworkDHCPHostCreateSpec{MAC: "invalid", IP: "192.168.122.60"}, want: domain.ErrInvalidInput},
		{name: "IPv6 address", spec: domain.NetworkDHCPHostCreateSpec{MAC: "52:54:00:12:34:60", IP: "fd00::60"}, want: domain.ErrInvalidInput},
		{name: "outside subnet", spec: domain.NetworkDHCPHostCreateSpec{MAC: "52:54:00:12:34:60", IP: "192.168.123.60"}, want: domain.ErrInvalidInput},
		{name: "gateway", spec: domain.NetworkDHCPHostCreateSpec{MAC: "52:54:00:12:34:60", IP: "192.168.122.1"}, want: domain.ErrInvalidInput},
		{name: "broadcast", spec: domain.NetworkDHCPHostCreateSpec{MAC: "52:54:00:12:34:60", IP: "192.168.122.255"}, want: domain.ErrInvalidInput},
		{name: "duplicate MAC", spec: domain.NetworkDHCPHostCreateSpec{MAC: "52:54:00:12:34:56", IP: "192.168.122.60"}, want: domain.ErrConflict},
		{name: "duplicate IP", spec: domain.NetworkDHCPHostCreateSpec{MAC: "52:54:00:12:34:60", IP: "192.168.122.50"}, want: domain.ErrConflict},
		{name: "invalid hostname", spec: domain.NetworkDHCPHostCreateSpec{MAC: "52:54:00:12:34:60", IP: "192.168.122.60", Name: "bad name"}, want: domain.ErrInvalidInput},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{networks: []domain.VirtualNetwork{network}}
			_, err := New(repository, nil, false).AddDHCPHost(context.Background(), AddNetworkDHCPHostCommand{
				Name: "default", Spec: test.spec, Request: adminRequest(),
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if repository.called {
				t.Fatal("invalid DHCP host reached repository")
			}
		})
	}
}

func TestAddDHCPHostRequiresDHCPSection(t *testing.T) {
	repository := &fakeRepository{networks: []domain.VirtualNetwork{{
		Name: "bridge-net", Active: true, Persistent: true, ForwardMode: "bridge",
	}}}
	_, err := New(repository, nil, false).AddDHCPHost(context.Background(), AddNetworkDHCPHostCommand{
		Name:    "bridge-net",
		Spec:    domain.NetworkDHCPHostCreateSpec{MAC: "52:54:00:12:34:56", IP: "192.168.122.50"},
		Request: adminRequest(),
	})
	if !errors.Is(err, domain.ErrNetworkUnsupported) || repository.called {
		t.Fatalf("error = %v called=%v", err, repository.called)
	}
}

func TestImportDHCPHostsSkipsExistingAndAudits(t *testing.T) {
	repository := &fakeRepository{networks: []domain.VirtualNetwork{{
		Name: "default", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{
			Family: "ipv4", Address: "192.168.122.1", Prefix: 24, DHCPEnabled: true,
			DHCPStart: "192.168.122.2", DHCPEnd: "192.168.122.254",
			Hosts: []domain.NetworkDHCPHost{{MAC: "52:54:00:12:34:56", IP: "192.168.122.50", Name: "existing"}},
		}},
	}}}
	audit := &auditRecorder{}
	result, err := New(repository, audit, false).ImportDHCPHosts(context.Background(), ImportNetworkDHCPHostsCommand{
		Name: " default ",
		Hosts: []domain.NetworkDHCPHostCreateSpec{
			{MAC: "52:54:00:12:34:56", IP: "192.168.122.50", Name: "existing"},
			{MAC: "52:54:00:AB:CD:EF", IP: "192.168.122.60", Name: "web-01"},
			{MAC: "52:54:00:AB:CD:EF", IP: "192.168.122.60", Name: "web-01"},
			{MAC: "52:54:00:AB:CD:70", IP: "192.168.122.70"},
		},
		Request: adminRequest(),
	})
	if err != nil {
		t.Fatalf("ImportDHCPHosts() error = %v", err)
	}
	if result.Imported != 2 || result.Skipped != 2 || len(repository.addedHosts) != 2 {
		t.Fatalf("result=%+v hosts=%+v", result, repository.addedHosts)
	}
	if repository.addedHosts[0].MAC != "52:54:00:ab:cd:ef" || repository.addedHosts[1].IP != "192.168.122.70" {
		t.Fatalf("imported hosts = %+v", repository.addedHosts)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "network.dhcp.host.import" || audit.events[0].Detail["imported"] != 2 || audit.events[0].Detail["skipped"] != 2 {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestImportDHCPHostsRejectsConflictsBeforeWriting(t *testing.T) {
	network := domain.VirtualNetwork{
		Name: "default", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{
			Family: "ipv4", Address: "192.168.122.1", Prefix: 24, DHCPEnabled: true,
			DHCPStart: "192.168.122.2", DHCPEnd: "192.168.122.254",
			Hosts: []domain.NetworkDHCPHost{{MAC: "52:54:00:12:34:56", IP: "192.168.122.50", Name: "existing"}},
		}},
	}
	tests := []struct {
		name  string
		hosts []domain.NetworkDHCPHostCreateSpec
		want  error
	}{
		{name: "existing conflict", hosts: []domain.NetworkDHCPHostCreateSpec{{MAC: "52:54:00:12:34:56", IP: "192.168.122.60"}}, want: domain.ErrConflict},
		{name: "batch MAC conflict", hosts: []domain.NetworkDHCPHostCreateSpec{{MAC: "52:54:00:AB:CD:EF", IP: "192.168.122.60"}, {MAC: "52:54:00:AB:CD:EF", IP: "192.168.122.70"}}, want: domain.ErrConflict},
		{name: "batch IP conflict", hosts: []domain.NetworkDHCPHostCreateSpec{{MAC: "52:54:00:AB:CD:EF", IP: "192.168.122.60"}, {MAC: "52:54:00:AB:CD:70", IP: "192.168.122.60"}}, want: domain.ErrConflict},
		{name: "outside subnet", hosts: []domain.NetworkDHCPHostCreateSpec{{MAC: "52:54:00:AB:CD:EF", IP: "192.168.123.60"}}, want: domain.ErrInvalidInput},
		{name: "empty", hosts: nil, want: domain.ErrInvalidInput},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{networks: []domain.VirtualNetwork{network}}
			_, err := New(repository, nil, false).ImportDHCPHosts(context.Background(), ImportNetworkDHCPHostsCommand{Name: "default", Hosts: test.hosts, Request: adminRequest()})
			if !errors.Is(err, test.want) || repository.called {
				t.Fatalf("error = %v called=%v", err, repository.called)
			}
		})
	}
}

func TestDHCPHostCSVRoundTrip(t *testing.T) {
	network := domain.VirtualNetwork{Name: "default", Addresses: []domain.NetworkAddress{{
		Family: "ipv4", Address: "192.168.122.1", Prefix: 24,
		Hosts: []domain.NetworkDHCPHost{
			{MAC: "52:54:00:12:34:56", IP: "192.168.122.50", Name: "web-01"},
			{MAC: "52:54:00:ab:cd:ef", IP: "192.168.122.9", Name: "db,01"},
		},
	}}}
	data, err := encodeNetworkDHCPHostsCSV(network)
	if err != nil {
		t.Fatalf("encode error = %v", err)
	}
	if !strings.HasPrefix(string(data), "mac,ip,name\n") || !strings.Contains(string(data), `"db,01"`) {
		t.Fatalf("CSV = %s", data)
	}
	hosts, err := ParseNetworkDHCPHostCSV(data)
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if len(hosts) != 2 || hosts[0].IP != "192.168.122.9" || hosts[1].IP != "192.168.122.50" || hosts[0].Name != "db,01" {
		t.Fatalf("hosts = %+v", hosts)
	}
}

func TestParseDHCPHostCSVValidation(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "empty", data: ""},
		{name: "header only", data: "mac,ip,name\n"},
		{name: "missing ip", data: "52:54:00:12:34:56\n"},
		{name: "too many columns", data: "52:54:00:12:34:56,192.168.122.50,web,extra\n"},
		{name: "malformed quotes", data: `"52:54:00:12:34:56,192.168.122.50`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseNetworkDHCPHostCSV([]byte(test.data)); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestDeleteDHCPHostFindsEntryAndAudits(t *testing.T) {
	repository := &fakeRepository{networks: []domain.VirtualNetwork{{
		Name: "default", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{
			Family: "ipv4", Address: "192.168.122.1", Prefix: 24, DHCPEnabled: true,
			Hosts: []domain.NetworkDHCPHost{{MAC: "52:54:00:12:34:56", IP: "192.168.122.50", Name: "web-01"}},
		}},
	}}}
	audit := &auditRecorder{}
	network, err := New(repository, audit, false).DeleteDHCPHost(context.Background(), DeleteNetworkDHCPHostCommand{
		Name: "default", MAC: "52:54:00:12:34:56", Request: adminRequest(),
	})
	if err != nil {
		t.Fatalf("DeleteDHCPHost() error = %v", err)
	}
	if network.Name != "default" || repository.deletedHost.IP != "192.168.122.50" || repository.deletedHost.Name != "web-01" {
		t.Fatalf("network=%+v host=%+v", network, repository.deletedHost)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "network.dhcp.host.delete" {
		t.Fatalf("audit events = %+v", audit.events)
	}

	repository = &fakeRepository{networks: []domain.VirtualNetwork{{Name: "default", Active: true, Persistent: true}}}
	_, err = New(repository, nil, false).DeleteDHCPHost(context.Background(), DeleteNetworkDHCPHostCommand{
		Name: "default", MAC: "52:54:00:12:34:56", Request: adminRequest(),
	})
	if !errors.Is(err, domain.ErrNotFound) || repository.called {
		t.Fatalf("missing host error = %v called=%v", err, repository.called)
	}
}

func TestNetworkActionAndDeleteAudit(t *testing.T) {
	repository := &fakeRepository{}
	audit := &auditRecorder{}
	request := adminRequest()
	request.Session.User.Role = domain.RoleOperator
	service := New(repository, audit, false)
	if err := service.PerformAction(context.Background(), NetworkActionCommand{Name: "lab-net", Action: domain.VirtualNetworkActionStart, Request: request}); err != nil {
		t.Fatalf("PerformAction() error = %v", err)
	}
	if repository.actionFor != "lab-net" || repository.action != domain.VirtualNetworkActionStart {
		t.Fatalf("repository action = %q %q", repository.actionFor, repository.action)
	}
	if err := service.Delete(context.Background(), DeleteNetworkCommand{Name: "lab-net", Request: request}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if repository.deleted != "lab-net" || len(audit.events) != 2 {
		t.Fatalf("deleted=%q audit=%+v", repository.deleted, audit.events)
	}
	if audit.events[0].Action != "network.start" || audit.events[1].Action != "network.delete" || audit.events[0].Actor != "admin" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestNetworkAuditFailurePropagates(t *testing.T) {
	want := errors.New("audit unavailable")
	service := New(&fakeRepository{}, &auditRecorder{err: want}, false)
	_, err := service.Create(context.Background(), CreateNetworkCommand{
		Spec: domain.VirtualNetworkCreateSpec{Name: "valid-net", Mode: "nat", Subnet: "10.0.0.0/24"}, Request: adminRequest(),
	})
	if !errors.Is(err, want) {
		t.Fatalf("Create() error = %v", err)
	}
}
