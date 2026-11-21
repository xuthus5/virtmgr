package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"virtmgr/internal/domain"
)

type portForwardTestStore struct {
	items       map[string][]domain.PortForward
	createCalls int
}

func newPortForwardTestStore(items map[string][]domain.PortForward) *portForwardTestStore {
	if items == nil {
		items = map[string][]domain.PortForward{}
	}
	return &portForwardTestStore{items: items}
}

func (s *portForwardTestStore) ListPortForwards(_ context.Context, network string) ([]domain.PortForward, error) {
	return append([]domain.PortForward(nil), s.items[network]...), nil
}

func (s *portForwardTestStore) GetPortForward(_ context.Context, id string) (domain.PortForward, error) {
	for _, items := range s.items {
		for _, item := range items {
			if item.ID == id {
				return item, nil
			}
		}
	}
	return domain.PortForward{}, domain.ErrNotFound
}

func (s *portForwardTestStore) CreatePortForward(_ context.Context, item domain.PortForward) (domain.PortForward, error) {
	s.createCalls++
	for _, existing := range s.items[item.Network] {
		if existing.Name == item.Name || (existing.Protocol == item.Protocol && existing.HostPort == item.HostPort) {
			return domain.PortForward{}, domain.ErrConflict
		}
	}
	s.items[item.Network] = append(s.items[item.Network], item)
	return item, nil
}

func (s *portForwardTestStore) UpdatePortForward(_ context.Context, item domain.PortForward) (domain.PortForward, error) {
	for network, items := range s.items {
		for index, current := range items {
			if current.ID == item.ID {
				s.items[network][index] = item
				return item, nil
			}
		}
	}
	return domain.PortForward{}, domain.ErrNotFound
}

func (s *portForwardTestStore) DeletePortForward(_ context.Context, id string) error {
	for network, items := range s.items {
		for index, current := range items {
			if current.ID == id {
				s.items[network] = append(items[:index], items[index+1:]...)
				return nil
			}
		}
	}
	return domain.ErrNotFound
}

type portForwardTestRepository struct {
	*fakeRepository
	applied  [][]domain.PortForward
	applyErr error
}

func (r *portForwardTestRepository) ApplyPortForwards(_ context.Context, forwards []domain.PortForward) error {
	if r.applyErr != nil {
		return r.applyErr
	}
	r.applied = append(r.applied, append([]domain.PortForward(nil), forwards...))
	return nil
}

func natNetwork(name, subnet, guest string) domain.VirtualNetwork {
	return domain.VirtualNetwork{
		Name: name, ForwardMode: "nat", Active: true,
		Addresses: []domain.NetworkAddress{{Family: "ipv4", Address: subnet, Prefix: 24, Hosts: []domain.NetworkDHCPHost{{IP: guest}}}},
	}
}

func TestPortForwardAppliesRulesAcrossAllNetworks(t *testing.T) {
	repository := &portForwardTestRepository{fakeRepository: &fakeRepository{networks: []domain.VirtualNetwork{
		natNetwork("lab-net", "192.168.122.1", "192.168.122.20"),
		natNetwork("dmz-net", "192.168.123.1", "192.168.123.20"),
	}}}
	store := newPortForwardTestStore(map[string][]domain.PortForward{
		"dmz-net": {{ID: "pf_existing", Network: "dmz-net", Name: "web", Family: "ipv4", Protocol: domain.PortForwardProtocolTCP, HostPort: 8080, GuestAddress: "192.168.123.20", GuestPort: 80, Enabled: true}},
	})
	service := New(repository, &auditRecorder{}, false, WithPortForwardStore(store))
	created, err := service.CreatePortForward(context.Background(), PortForwardCommand{
		Network: "lab-net",
		Spec:    domain.PortForwardSpec{Name: "ssh", Protocol: domain.PortForwardProtocolTCP, HostPort: 2222, GuestAddress: "192.168.122.20", GuestPort: 22, Enabled: true},
		Request: adminRequest(),
	})
	if err != nil {
		t.Fatalf("CreatePortForward() error = %v", err)
	}
	if len(repository.applied) != 1 || len(repository.applied[0]) != 2 {
		t.Fatalf("applied rule sets = %+v", repository.applied)
	}
	networks := []string{repository.applied[0][0].Network, repository.applied[0][1].Network}
	slices.Sort(networks)
	if !slices.Equal(networks, []string{"dmz-net", "lab-net"}) || created.Network != "lab-net" {
		t.Fatalf("applied networks = %v, created = %+v", networks, created)
	}
}

func TestPortForwardRejectsHostPortConflictAcrossNetworks(t *testing.T) {
	repository := &portForwardTestRepository{fakeRepository: &fakeRepository{networks: []domain.VirtualNetwork{
		natNetwork("lab-net", "192.168.122.1", "192.168.122.20"),
		natNetwork("dmz-net", "192.168.123.1", "192.168.123.20"),
	}}}
	store := newPortForwardTestStore(map[string][]domain.PortForward{
		"dmz-net": {{ID: "pf_existing", Network: "dmz-net", Name: "web", Family: "ipv4", Protocol: domain.PortForwardProtocolTCP, HostPort: 8080, GuestAddress: "192.168.123.20", GuestPort: 80, Enabled: true}},
	})
	service := New(repository, &auditRecorder{}, false, WithPortForwardStore(store))
	_, err := service.CreatePortForward(context.Background(), PortForwardCommand{
		Network: "lab-net",
		Spec:    domain.PortForwardSpec{Name: "web", Protocol: domain.PortForwardProtocolTCP, HostPort: 8080, GuestAddress: "192.168.122.20", GuestPort: 80, Enabled: true},
		Request: adminRequest(),
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
	if store.createCalls != 0 || len(repository.applied) != 0 {
		t.Fatalf("store calls = %d, firewall applications = %d", store.createCalls, len(repository.applied))
	}
}

func TestPortForwardDoesNotPersistWhenFirewallFails(t *testing.T) {
	repository := &portForwardTestRepository{
		fakeRepository: &fakeRepository{networks: []domain.VirtualNetwork{natNetwork("lab-net", "192.168.122.1", "192.168.122.20")}},
		applyErr:       errors.New("nft failed"),
	}
	store := newPortForwardTestStore(nil)
	service := New(repository, &auditRecorder{}, false, WithPortForwardStore(store))
	_, err := service.CreatePortForward(context.Background(), PortForwardCommand{
		Network: "lab-net",
		Spec:    domain.PortForwardSpec{Name: "ssh", Protocol: domain.PortForwardProtocolTCP, HostPort: 2222, GuestAddress: "192.168.122.20", GuestPort: 22, Enabled: true},
		Request: adminRequest(),
	})
	if err == nil || store.createCalls != 0 {
		t.Fatalf("error = %v, store calls = %d", err, store.createCalls)
	}
}

func TestPortForwardRejectsAddressFamilyMismatchAndNonNATNetwork(t *testing.T) {
	network := natNetwork("lab-net", "192.168.122.1", "192.168.122.20")
	isolated := network
	isolated.ForwardMode = "isolated"
	repository := &portForwardTestRepository{fakeRepository: &fakeRepository{networks: []domain.VirtualNetwork{network, isolated}}}
	store := newPortForwardTestStore(nil)
	service := New(repository, &auditRecorder{}, false, WithPortForwardStore(store))
	for _, test := range []struct {
		network string
		spec    domain.PortForwardSpec
	}{
		{network: "lab-net", spec: domain.PortForwardSpec{Name: "bad-family", Family: "ipv6", Protocol: domain.PortForwardProtocolTCP, HostPort: 2222, GuestAddress: "192.168.122.20", GuestPort: 22, Enabled: true}},
		{network: "lab-net", spec: domain.PortForwardSpec{Name: "bad-guest", Protocol: domain.PortForwardProtocolTCP, HostPort: 2222, GuestAddress: "192.168.122.1", GuestPort: 22, Enabled: true}},
		{network: "lab-net", spec: domain.PortForwardSpec{Name: "bad-host", Protocol: domain.PortForwardProtocolTCP, HostAddress: "2001:db8::1", HostPort: 2222, GuestAddress: "192.168.122.20", GuestPort: 22, Enabled: true}},
		{network: "lab-net", spec: domain.PortForwardSpec{Name: "bad-protocol", Protocol: "icmp", HostPort: 2222, GuestAddress: "192.168.122.20", GuestPort: 22, Enabled: true}},
		{network: "lab-net", spec: domain.PortForwardSpec{Name: "bad-port", Protocol: domain.PortForwardProtocolTCP, HostPort: 0, GuestAddress: "192.168.122.20", GuestPort: 22, Enabled: true}},
		{network: "isolated", spec: domain.PortForwardSpec{Name: "no-nat", Protocol: domain.PortForwardProtocolTCP, HostPort: 2222, GuestAddress: "192.168.122.20", GuestPort: 22, Enabled: true}},
	} {
		if _, err := service.CreatePortForward(context.Background(), PortForwardCommand{Network: test.network, Spec: test.spec, Request: adminRequest()}); err == nil {
			t.Fatalf("spec %+v was accepted", test.spec)
		}
	}
}
