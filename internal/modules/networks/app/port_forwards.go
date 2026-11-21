package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

type PortForwardStore interface {
	ListPortForwards(ctx context.Context, network string) ([]domain.PortForward, error)
	GetPortForward(ctx context.Context, id string) (domain.PortForward, error)
	CreatePortForward(ctx context.Context, item domain.PortForward) (domain.PortForward, error)
	UpdatePortForward(ctx context.Context, item domain.PortForward) (domain.PortForward, error)
	DeletePortForward(ctx context.Context, id string) error
}

type PortForwardFirewall interface {
	ApplyPortForwards(ctx context.Context, forwards []domain.PortForward) error
}

type PortForwardCommand struct {
	Network string
	Spec    domain.PortForwardSpec
	Request MutationContext
}

type PortForwardUpdateCommand struct {
	Network string
	ID      string
	Spec    domain.PortForwardSpec
	Request MutationContext
}

type PortForwardDeleteCommand struct {
	Network string
	ID      string
	Request MutationContext
}

func (s *Service) ListPortForwards(ctx context.Context, network string) (domain.PortForwardList, error) {
	network = strings.TrimSpace(network)
	if !networkNamePattern.MatchString(network) {
		return domain.PortForwardList{}, domain.ErrInvalidInput
	}
	if _, err := s.findNetwork(ctx, network); err != nil {
		return domain.PortForwardList{}, err
	}
	store, err := s.portForwardStore()
	if err != nil {
		return domain.PortForwardList{}, err
	}
	items, err := store.ListPortForwards(ctx, network)
	if err != nil {
		return domain.PortForwardList{}, err
	}
	if items == nil {
		items = []domain.PortForward{}
	}
	slices.SortFunc(items, func(left, right domain.PortForward) int {
		return strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	})
	return domain.PortForwardList{Network: network, Items: items, Total: len(items)}, nil
}

func (s *Service) CreatePortForward(ctx context.Context, command PortForwardCommand) (domain.PortForward, error) {
	s.portForwardMu.Lock()
	defer s.portForwardMu.Unlock()
	network, items, store, firewall, err := s.preparePortForwardMutation(ctx, command.Network, command.Request)
	if err != nil {
		return domain.PortForward{}, err
	}
	id, err := newPortForwardID()
	if err != nil {
		return domain.PortForward{}, err
	}
	now := time.Now().UTC()
	item := domain.PortForward{
		ID: id, Network: network.Name, CreatedAt: now, UpdatedAt: now,
	}
	item, err = normalizePortForward(network, item, command.Spec, items)
	if err != nil {
		return domain.PortForward{}, err
	}
	candidate := append(append([]domain.PortForward(nil), items...), item)
	if err := firewall.ApplyPortForwards(ctx, candidate); err != nil {
		return domain.PortForward{}, err
	}
	created, err := store.CreatePortForward(ctx, item)
	if err != nil {
		_ = firewall.ApplyPortForwards(context.WithoutCancel(ctx), items)
		return domain.PortForward{}, err
	}
	if err := s.recordAudit(ctx, command.Request, "network.port_forward.create", network.Name, portForwardDetail(created)); err != nil {
		return domain.PortForward{}, fmt.Errorf("record port forwarding creation audit: %w", err)
	}
	return created, nil
}

func (s *Service) UpdatePortForward(ctx context.Context, command PortForwardUpdateCommand) (domain.PortForward, error) {
	s.portForwardMu.Lock()
	defer s.portForwardMu.Unlock()
	network, items, store, firewall, err := s.preparePortForwardMutation(ctx, command.Network, command.Request)
	if err != nil {
		return domain.PortForward{}, err
	}
	command.ID = strings.TrimSpace(command.ID)
	index := slices.IndexFunc(items, func(item domain.PortForward) bool { return item.ID == command.ID })
	if index < 0 {
		return domain.PortForward{}, domain.ErrNotFound
	}
	item := items[index]
	updated, err := normalizePortForward(network, item, command.Spec, items)
	if err != nil {
		return domain.PortForward{}, err
	}
	updated.UpdatedAt = time.Now().UTC()
	candidate := append([]domain.PortForward(nil), items...)
	candidate[index] = updated
	if err := firewall.ApplyPortForwards(ctx, candidate); err != nil {
		return domain.PortForward{}, err
	}
	saved, err := store.UpdatePortForward(ctx, updated)
	if err != nil {
		_ = firewall.ApplyPortForwards(context.WithoutCancel(ctx), items)
		return domain.PortForward{}, err
	}
	if err := s.recordAudit(ctx, command.Request, "network.port_forward.update", network.Name, portForwardDetail(saved)); err != nil {
		return domain.PortForward{}, fmt.Errorf("record port forwarding update audit: %w", err)
	}
	return saved, nil
}

func (s *Service) DeletePortForward(ctx context.Context, command PortForwardDeleteCommand) error {
	s.portForwardMu.Lock()
	defer s.portForwardMu.Unlock()
	network, items, store, firewall, err := s.preparePortForwardMutation(ctx, command.Network, command.Request)
	if err != nil {
		return err
	}
	command.ID = strings.TrimSpace(command.ID)
	index := slices.IndexFunc(items, func(item domain.PortForward) bool { return item.ID == command.ID })
	if index < 0 {
		return domain.ErrNotFound
	}
	removed := items[index]
	candidate := append([]domain.PortForward(nil), items[:index]...)
	candidate = append(candidate, items[index+1:]...)
	if err := firewall.ApplyPortForwards(ctx, candidate); err != nil {
		return err
	}
	if err := store.DeletePortForward(ctx, command.ID); err != nil {
		_ = firewall.ApplyPortForwards(context.WithoutCancel(ctx), items)
		return err
	}
	if err := s.recordAudit(ctx, command.Request, "network.port_forward.delete", network.Name, portForwardDetail(removed)); err != nil {
		return fmt.Errorf("record port forwarding deletion audit: %w", err)
	}
	return nil
}

func (s *Service) preparePortForwardMutation(ctx context.Context, name string, request MutationContext) (domain.VirtualNetwork, []domain.PortForward, PortForwardStore, PortForwardFirewall, error) {
	if !domain.CanManageNetworks(request.Session.User.Role) {
		return domain.VirtualNetwork{}, nil, nil, nil, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VirtualNetwork{}, nil, nil, nil, domain.ErrReadOnly
	}
	name = strings.TrimSpace(name)
	if !networkNamePattern.MatchString(name) {
		return domain.VirtualNetwork{}, nil, nil, nil, domain.ErrInvalidInput
	}
	store, err := s.portForwardStore()
	if err != nil {
		return domain.VirtualNetwork{}, nil, nil, nil, err
	}
	firewall, ok := s.repository.(PortForwardFirewall)
	if !ok {
		return domain.VirtualNetwork{}, nil, nil, nil, domain.ErrUnavailable
	}
	network, err := s.findNetwork(ctx, name)
	if err != nil {
		return domain.VirtualNetwork{}, nil, nil, nil, err
	}
	items, err := s.allPortForwards(ctx, store)
	if err != nil {
		return domain.VirtualNetwork{}, nil, nil, nil, err
	}
	return network, items, store, firewall, nil
}

func (s *Service) allPortForwards(ctx context.Context, store PortForwardStore) ([]domain.PortForward, error) {
	networks, err := s.repository.ListNetworks(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]domain.PortForward, 0)
	for _, network := range networks {
		current, err := store.ListPortForwards(ctx, network.Name)
		if err != nil {
			return nil, err
		}
		items = append(items, current...)
	}
	return items, nil
}

func (s *Service) portForwardStore() (PortForwardStore, error) {
	if s.portForwards == nil {
		return nil, domain.ErrUnavailable
	}
	return s.portForwards, nil
}

func (s *Service) portForwardCount(ctx context.Context, network string) (int, error) {
	if s.portForwards == nil {
		return 0, nil
	}
	items, err := s.portForwards.ListPortForwards(ctx, network)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

func normalizePortForward(network domain.VirtualNetwork, item domain.PortForward, spec domain.PortForwardSpec, existing []domain.PortForward) (domain.PortForward, error) {
	if network.ForwardMode != "nat" {
		return domain.PortForward{}, fmt.Errorf("%w: inbound port forwarding requires a NAT network", domain.ErrActionNotAllowed)
	}
	name := strings.TrimSpace(spec.Name)
	if len(name) < 2 || len(name) > 64 || strings.IndexFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return domain.PortForward{}, domain.ErrInvalidInput
	}
	protocol, ok := domain.NormalizePortForwardProtocol(strings.ToLower(strings.TrimSpace(string(spec.Protocol))))
	if !ok || spec.HostPort == 0 || spec.GuestPort == 0 {
		return domain.PortForward{}, fmt.Errorf("%w: protocol and both ports are required", domain.ErrInvalidInput)
	}
	guest, err := netip.ParseAddr(strings.TrimSpace(spec.GuestAddress))
	if err != nil || !guest.IsValid() || guest.IsUnspecified() || guest.IsMulticast() || guest.IsLoopback() || guest.IsLinkLocalUnicast() {
		return domain.PortForward{}, fmt.Errorf("%w: guest address must be a unicast IP address", domain.ErrInvalidInput)
	}
	guest = guest.Unmap()
	family := "ipv6"
	if guest.Is4() {
		family = "ipv4"
	}
	if requested := strings.ToLower(strings.TrimSpace(spec.Family)); requested != "" && requested != family {
		return domain.PortForward{}, fmt.Errorf("%w: address family does not match guest address", domain.ErrInvalidInput)
	}
	matched := false
	for _, address := range network.Addresses {
		if networkAddressFamily(address) == family && hostBelongsToNetworkAddress(address, net.IP(guest.AsSlice())) {
			matched = true
			break
		}
	}
	if !matched {
		return domain.PortForward{}, fmt.Errorf("%w: guest address must belong to the network subnet and differ from its gateway", domain.ErrInvalidInput)
	}
	hostAddress := strings.TrimSpace(spec.HostAddress)
	if hostAddress != "" {
		address, err := netip.ParseAddr(hostAddress)
		if err != nil || !address.IsValid() || address.IsUnspecified() || address.IsMulticast() || address.IsLoopback() || address.IsLinkLocalUnicast() {
			return domain.PortForward{}, fmt.Errorf("%w: host address must be a unicast IP address", domain.ErrInvalidInput)
		}
		if (address.Unmap().Is4() && family != "ipv4") || (address.Is6() && !address.Is4In6() && family != "ipv6") {
			return domain.PortForward{}, fmt.Errorf("%w: host address family does not match guest address", domain.ErrInvalidInput)
		}
		hostAddress = address.Unmap().String()
	}
	if spec.Name != "" && name == "" {
		return domain.PortForward{}, domain.ErrInvalidInput
	}
	for _, other := range existing {
		if other.ID == item.ID {
			continue
		}
		if other.Network == network.Name && strings.EqualFold(other.Name, name) {
			return domain.PortForward{}, fmt.Errorf("%w: port forwarding name already exists", domain.ErrConflict)
		}
		if other.Family == family && other.Protocol == protocol && other.HostPort == spec.HostPort && (other.HostAddress == "" || hostAddress == "" || other.HostAddress == hostAddress) {
			return domain.PortForward{}, fmt.Errorf("%w: host port is already forwarded for %s/%s", domain.ErrConflict, family, protocol)
		}
	}
	item.Network = network.Name
	item.Name = name
	item.Family = family
	item.Protocol = protocol
	item.HostAddress = hostAddress
	item.HostPort = spec.HostPort
	item.GuestAddress = guest.String()
	item.GuestPort = spec.GuestPort
	item.Enabled = spec.Enabled
	return item, nil
}

func proposedNetworkAddresses(definition domain.VirtualNetworkDefinition) []domain.NetworkAddress {
	addresses := make([]domain.NetworkAddress, 0, 2)
	if definition.IPv4Address != "" {
		addresses = append(addresses, domain.NetworkAddress{Family: "ipv4", Address: definition.IPv4Address, Prefix: definition.Prefix})
	}
	if definition.IPv6Address != "" {
		addresses = append(addresses, domain.NetworkAddress{Family: "ipv6", Address: definition.IPv6Address, Prefix: definition.IPv6Prefix})
	}
	return addresses
}

func portForwardDetail(item domain.PortForward) map[string]any {
	return map[string]any{
		"id": item.ID, "name": item.Name, "family": item.Family, "protocol": item.Protocol,
		"hostAddress": item.HostAddress, "hostPort": item.HostPort, "guestAddress": item.GuestAddress,
		"guestPort": item.GuestPort, "enabled": item.Enabled,
	}
}

func newPortForwardID() (string, error) {
	var value [10]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate port forwarding id: %w", err)
	}
	return "pf_" + hex.EncodeToString(value[:]), nil
}

func networkAddressesEqual(left, right []domain.NetworkAddress) bool {
	normalized := func(addresses []domain.NetworkAddress) []string {
		values := make([]string, 0, len(addresses))
		for _, address := range addresses {
			values = append(values, strings.ToLower(address.Family)+"|"+address.Address+"/"+strconv.Itoa(address.Prefix))
		}
		slices.Sort(values)
		return values
	}
	return slices.Equal(normalized(left), normalized(right))
}
