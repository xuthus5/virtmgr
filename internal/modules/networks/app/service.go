package app

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"virtmgr/internal/domain"
)

var networkNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{2,63}$`)
var dhcpHostNamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,251}[A-Za-z0-9])?$`)
var hostBridgeNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}$`)
var duidPattern = regexp.MustCompile(`^[0-9A-Fa-f]{1,4}(?:[:-][0-9A-Fa-f]{1,4})+$`)
var dnsDomainPattern = regexp.MustCompile(`^(?i)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

const (
	ipv6ModeSLAAC  = "slaac"
	ipv6ModeDHCPv6 = "dhcpv6"
)

type Repository interface {
	ListNetworks(ctx context.Context) ([]domain.VirtualNetwork, error)
	ListHostBridges(ctx context.Context) ([]domain.HostBridge, error)
	ListDHCPLeases(ctx context.Context, networkName string) ([]domain.DHCPLease, error)
	NetworkDiagnostics(ctx context.Context, name string) (domain.NetworkDiagnostics, error)
	CreateNetwork(ctx context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error)
	UpdateNetwork(ctx context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error)
	AddNetworkDHCPHost(ctx context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error)
	DeleteNetworkDHCPHost(ctx context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error)
	AddNetworkDNSHost(ctx context.Context, name string, host domain.NetworkDNSHost) (domain.VirtualNetwork, error)
	DeleteNetworkDNSHost(ctx context.Context, name string, host domain.NetworkDNSHost) (domain.VirtualNetwork, error)
	PerformNetworkAction(ctx context.Context, name string, action domain.VirtualNetworkAction) error
	DeleteNetwork(ctx context.Context, name string) error
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type Service struct {
	repository    Repository
	audit         AuditRecorder
	portForwards  PortForwardStore
	portForwardMu sync.Mutex
	readOnly      bool
	readOnlyMode  domain.ReadOnlyProvider
}

type Option func(*Service)

func WithReadOnlyMode(mode domain.ReadOnlyProvider) Option {
	return func(service *Service) {
		service.readOnlyMode = mode
	}
}

func WithPortForwardStore(store PortForwardStore) Option {
	return func(service *Service) {
		service.portForwards = store
	}
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type CreateNetworkCommand struct {
	Spec    domain.VirtualNetworkCreateSpec
	Request MutationContext
}

type UpdateNetworkCommand struct {
	Name    string
	Spec    domain.VirtualNetworkUpdateSpec
	Request MutationContext
}

type NetworkActionCommand struct {
	Name    string
	Action  domain.VirtualNetworkAction
	Request MutationContext
}

type AddNetworkDHCPHostCommand struct {
	Name    string
	Spec    domain.NetworkDHCPHostCreateSpec
	Request MutationContext
}

type DeleteNetworkDHCPHostCommand struct {
	Name    string
	Key     string
	Request MutationContext
}

type AddNetworkDNSHostCommand struct {
	Name    string
	Spec    domain.NetworkDNSHostCreateSpec
	Request MutationContext
}

type DeleteNetworkDNSHostCommand struct {
	Name    string
	IP      string
	Request MutationContext
}

type ReserveNetworkLeaseCommand struct {
	Name    string
	Spec    domain.NetworkLeaseReservationSpec
	Request MutationContext
}

type DeleteNetworkCommand struct {
	Name    string
	Request MutationContext
}

func New(repository Repository, audit AuditRecorder, readOnly bool, options ...Option) *Service {
	service := &Service{repository: repository, audit: audit, readOnly: readOnly}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) isReadOnly() bool {
	if s.readOnlyMode != nil {
		return s.readOnlyMode.ReadOnly()
	}
	return s.readOnly
}

func (s *Service) List(ctx context.Context) (domain.VirtualNetworkList, error) {
	networks, err := s.repository.ListNetworks(ctx)
	if err != nil {
		return domain.VirtualNetworkList{}, err
	}
	slices.SortFunc(networks, func(left, right domain.VirtualNetwork) int {
		return cmp.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	})
	return domain.VirtualNetworkList{Items: networks, Total: len(networks)}, nil
}

func (s *Service) ListHostBridges(ctx context.Context) (domain.HostBridgeList, error) {
	bridges, err := s.repository.ListHostBridges(ctx)
	if err != nil {
		return domain.HostBridgeList{}, err
	}
	if bridges == nil {
		bridges = []domain.HostBridge{}
	}
	slices.SortFunc(bridges, func(left, right domain.HostBridge) int {
		return cmp.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	})
	return domain.HostBridgeList{Items: bridges, Total: len(bridges)}, nil
}

func (s *Service) ListLeases(ctx context.Context, networkName string) (domain.DHCPLeaseList, error) {
	networkName = strings.TrimSpace(networkName)
	if !networkNamePattern.MatchString(networkName) {
		return domain.DHCPLeaseList{}, domain.ErrInvalidInput
	}
	leases, err := s.repository.ListDHCPLeases(ctx, networkName)
	if err != nil {
		return domain.DHCPLeaseList{}, err
	}
	if leases == nil {
		leases = []domain.DHCPLease{}
	}
	slices.SortStableFunc(leases, func(left, right domain.DHCPLease) int {
		leftIP := net.ParseIP(left.IPAddress)
		rightIP := net.ParseIP(right.IPAddress)
		if leftIP != nil && rightIP != nil {
			if order := bytes.Compare(leftIP.To16(), rightIP.To16()); order != 0 {
				return order
			}
		}
		return cmp.Or(
			cmp.Compare(left.IPAddress, right.IPAddress),
			cmp.Compare(strings.ToLower(left.Hostname), strings.ToLower(right.Hostname)),
		)
	})
	return domain.DHCPLeaseList{Network: networkName, Items: leases, Total: len(leases)}, nil
}

func (s *Service) Diagnose(ctx context.Context, networkName string) (domain.NetworkDiagnostics, error) {
	networkName = strings.TrimSpace(networkName)
	if !networkNamePattern.MatchString(networkName) {
		return domain.NetworkDiagnostics{}, domain.ErrInvalidInput
	}
	result, err := s.repository.NetworkDiagnostics(ctx, networkName)
	if err != nil {
		return domain.NetworkDiagnostics{}, err
	}
	if result.Checks == nil {
		result.Checks = []domain.NetworkCheck{}
	}
	return result, nil
}

func (s *Service) Create(ctx context.Context, command CreateNetworkCommand) (domain.VirtualNetwork, error) {
	if !domain.CanManageNetworks(command.Request.Session.User.Role) {
		return domain.VirtualNetwork{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VirtualNetwork{}, domain.ErrReadOnly
	}
	definition, err := normalizeCreateSpec(command.Spec)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	if err := s.validateNetworkOverlap(ctx, definition, ""); err != nil {
		return domain.VirtualNetwork{}, err
	}
	network, err := s.repository.CreateNetwork(ctx, definition)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	detail := map[string]any{
		"mode": definition.Mode, "subnet": command.Spec.Subnet,
		"ipv4Address": definition.IPv4Address, "prefix": definition.Prefix,
		"dhcpStart": definition.DHCPStart, "dhcpEnd": definition.DHCPEnd,
		"ipv6Subnet": command.Spec.IPv6Subnet, "ipv6Address": definition.IPv6Address, "ipv6Prefix": definition.IPv6Prefix,
		"ipv6Mode":      definition.IPv6Mode,
		"ipv6DhcpStart": definition.IPv6DHCPStart, "ipv6DhcpEnd": definition.IPv6DHCPEnd,
		"natIpv6": definition.NATIPv6, "mtu": definition.MTU, "dnsDomain": definition.DNSDomain, "dnsServers": definition.DNSServers,
		"bridge":    definition.Bridge,
		"autostart": definition.Autostart, "start": definition.Start,
	}
	if err := s.recordAudit(ctx, command.Request, "network.create", definition.Name, detail); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("record network creation audit: %w", err)
	}
	return network, nil
}

func (s *Service) Update(ctx context.Context, command UpdateNetworkCommand) (domain.VirtualNetwork, error) {
	if !domain.CanManageNetworks(command.Request.Session.User.Role) {
		return domain.VirtualNetwork{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VirtualNetwork{}, domain.ErrReadOnly
	}
	command.Name = strings.TrimSpace(command.Name)
	if !networkNamePattern.MatchString(command.Name) {
		return domain.VirtualNetwork{}, domain.ErrInvalidInput
	}
	definition, err := normalizeNetworkSpec(command.Name, command.Spec.Mode, command.Spec.Subnet, command.Spec.DHCPStart, command.Spec.DHCPEnd,
		command.Spec.IPv6Subnet, command.Spec.IPv6Mode, command.Spec.IPv6DHCPStart, command.Spec.IPv6DHCPEnd,
		command.Spec.NATIPv6, command.Spec.MTU, command.Spec.DNSDomain, command.Spec.DNSServers, command.Spec.Bridge, command.Spec.Autostart)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	if err := s.validateNetworkOverlap(ctx, definition, command.Name); err != nil {
		return domain.VirtualNetwork{}, err
	}
	if err := s.validateNetworkUpdateHosts(ctx, command.Name, definition); err != nil {
		return domain.VirtualNetwork{}, err
	}
	if s.portForwards != nil {
		existing, err := s.repository.ListNetworks(ctx)
		if err != nil {
			return domain.VirtualNetwork{}, err
		}
		for _, current := range existing {
			if current.Name != command.Name {
				continue
			}
			count, err := s.portForwardCount(ctx, command.Name)
			if err != nil {
				return domain.VirtualNetwork{}, err
			}
			if count > 0 && (definition.Mode != "nat" || current.ForwardMode != "nat" || !networkAddressesEqual(current.Addresses, proposedNetworkAddresses(definition))) {
				return domain.VirtualNetwork{}, fmt.Errorf("%w: remove or disable inbound port forwards before changing the network mode or subnets", domain.ErrConflict)
			}
			break
		}
	}
	network, err := s.repository.UpdateNetwork(ctx, definition)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	detail := map[string]any{
		"mode": definition.Mode, "subnet": strings.TrimSpace(command.Spec.Subnet),
		"ipv4Address": definition.IPv4Address, "prefix": definition.Prefix,
		"dhcpStart": definition.DHCPStart, "dhcpEnd": definition.DHCPEnd,
		"ipv6Subnet": strings.TrimSpace(command.Spec.IPv6Subnet), "ipv6Address": definition.IPv6Address, "ipv6Prefix": definition.IPv6Prefix,
		"ipv6Mode":      definition.IPv6Mode,
		"ipv6DhcpStart": definition.IPv6DHCPStart, "ipv6DhcpEnd": definition.IPv6DHCPEnd,
		"natIpv6": definition.NATIPv6, "mtu": definition.MTU, "dnsDomain": definition.DNSDomain, "dnsServers": definition.DNSServers,
		"bridge":    definition.Bridge,
		"autostart": definition.Autostart,
	}
	if err := s.recordAudit(ctx, command.Request, "network.update", definition.Name, detail); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("record network update audit: %w", err)
	}
	return network, nil
}

func (s *Service) PerformAction(ctx context.Context, command NetworkActionCommand) error {
	if !domain.CanManageNetworks(command.Request.Session.User.Role) {
		return domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.ErrReadOnly
	}
	command.Name = strings.TrimSpace(command.Name)
	if !networkNamePattern.MatchString(command.Name) {
		return domain.ErrInvalidInput
	}
	if err := s.repository.PerformNetworkAction(ctx, command.Name, command.Action); err != nil {
		return err
	}
	detail := map[string]any{"action": string(command.Action)}
	if err := s.recordAudit(ctx, command.Request, "network."+string(command.Action), command.Name, detail); err != nil {
		return fmt.Errorf("record network action audit: %w", err)
	}
	return nil
}

func (s *Service) AddDHCPHost(ctx context.Context, command AddNetworkDHCPHostCommand) (domain.VirtualNetwork, error) {
	if !domain.CanManageNetworks(command.Request.Session.User.Role) {
		return domain.VirtualNetwork{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VirtualNetwork{}, domain.ErrReadOnly
	}
	command.Name = strings.TrimSpace(command.Name)
	if !networkNamePattern.MatchString(command.Name) {
		return domain.VirtualNetwork{}, domain.ErrInvalidInput
	}
	host, err := normalizeNetworkDHCPHost(command.Spec)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	network, err := s.findNetwork(ctx, command.Name)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	_, err = validateNetworkDHCPHost(network, host)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	updated, err := s.repository.AddNetworkDHCPHost(ctx, command.Name, host)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	detail := map[string]any{"mac": host.MAC, "id": host.ID, "ip": host.IP, "name": host.Name}
	if err := s.recordAudit(ctx, command.Request, "network.dhcp.host.add", command.Name, detail); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("record network DHCP host creation audit: %w", err)
	}
	return updated, nil
}

func (s *Service) ReserveLease(ctx context.Context, command ReserveNetworkLeaseCommand) (domain.VirtualNetwork, error) {
	if !domain.CanManageNetworks(command.Request.Session.User.Role) {
		return domain.VirtualNetwork{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VirtualNetwork{}, domain.ErrReadOnly
	}
	command.Name = strings.TrimSpace(command.Name)
	if !networkNamePattern.MatchString(command.Name) {
		return domain.VirtualNetwork{}, domain.ErrInvalidInput
	}
	requestedIP, err := netip.ParseAddr(strings.TrimSpace(command.Spec.IP))
	if err != nil || !requestedIP.IsValid() || requestedIP.IsUnspecified() || requestedIP.IsMulticast() || requestedIP.IsLoopback() || requestedIP.IsLinkLocalUnicast() {
		return domain.VirtualNetwork{}, fmt.Errorf("%w: lease IP must be a unicast IPv4 or IPv6 address", domain.ErrInvalidInput)
	}
	leases, err := s.ListLeases(ctx, command.Name)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	var lease *domain.DHCPLease
	for index := range leases.Items {
		leaseIP, parseErr := netip.ParseAddr(leases.Items[index].IPAddress)
		if parseErr == nil && leaseIP.Unmap() == requestedIP.Unmap() {
			lease = &leases.Items[index]
			break
		}
	}
	if lease == nil {
		return domain.VirtualNetwork{}, domain.ErrNotFound
	}
	name := strings.TrimSpace(command.Spec.Name)
	if name == "" {
		name = strings.TrimSpace(lease.Hostname)
	}
	hostSpec := domain.NetworkDHCPHostCreateSpec{IP: lease.IPAddress, Name: name}
	if lease.Family == "ipv6" || strings.Contains(lease.IPAddress, ":") {
		hostSpec.ID = normalizeDHCPClientID(lease.ClientID)
		if hostSpec.ID == "" && hostSpec.Name == "" {
			return domain.VirtualNetwork{}, fmt.Errorf("%w: DHCPv6 lease has no DUID/client ID or hostname; reserve it from the guest after configuring a hostname", domain.ErrInvalidInput)
		}
	} else {
		hostSpec.MAC = lease.MAC
	}
	host, err := normalizeNetworkDHCPHost(hostSpec)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	network, err := s.findNetwork(ctx, command.Name)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	if _, err := validateNetworkDHCPHost(network, host); err != nil {
		return domain.VirtualNetwork{}, err
	}
	updated, err := s.repository.AddNetworkDHCPHost(ctx, command.Name, host)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	detail := map[string]any{"mac": host.MAC, "id": host.ID, "ip": host.IP, "name": host.Name, "lease": true}
	if err := s.recordAudit(ctx, command.Request, "network.dhcp.host.reserve", command.Name, detail); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("record network DHCP host reservation audit: %w", err)
	}
	return updated, nil
}

func (s *Service) DeleteDHCPHost(ctx context.Context, command DeleteNetworkDHCPHostCommand) (domain.VirtualNetwork, error) {
	if !domain.CanManageNetworks(command.Request.Session.User.Role) {
		return domain.VirtualNetwork{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VirtualNetwork{}, domain.ErrReadOnly
	}
	command.Name = strings.TrimSpace(command.Name)
	if !networkNamePattern.MatchString(command.Name) {
		return domain.VirtualNetwork{}, domain.ErrInvalidInput
	}
	command.Key = strings.TrimSpace(command.Key)
	if command.Key == "" {
		return domain.VirtualNetwork{}, domain.ErrInvalidInput
	}
	network, err := s.findNetwork(ctx, command.Name)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	host, err := findNetworkDHCPHost(network, command.Key)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	updated, err := s.repository.DeleteNetworkDHCPHost(ctx, command.Name, host)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	detail := map[string]any{"mac": host.MAC, "id": host.ID, "ip": host.IP, "name": host.Name}
	if err := s.recordAudit(ctx, command.Request, "network.dhcp.host.delete", command.Name, detail); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("record network DHCP host deletion audit: %w", err)
	}
	return updated, nil
}

func (s *Service) AddDNSHost(ctx context.Context, command AddNetworkDNSHostCommand) (domain.VirtualNetwork, error) {
	if !domain.CanManageNetworks(command.Request.Session.User.Role) {
		return domain.VirtualNetwork{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VirtualNetwork{}, domain.ErrReadOnly
	}
	command.Name = strings.TrimSpace(command.Name)
	if !networkNamePattern.MatchString(command.Name) {
		return domain.VirtualNetwork{}, domain.ErrInvalidInput
	}
	host, err := normalizeNetworkDNSHost(command.Spec)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	network, err := s.findNetwork(ctx, command.Name)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	if err := validateNetworkDNSHost(network, host); err != nil {
		return domain.VirtualNetwork{}, err
	}
	updated, err := s.repository.AddNetworkDNSHost(ctx, command.Name, host)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	detail := map[string]any{"ip": host.IP, "hostnames": host.Hostnames}
	if err := s.recordAudit(ctx, command.Request, "network.dns.host.add", command.Name, detail); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("record network DNS host creation audit: %w", err)
	}
	return updated, nil
}

func (s *Service) DeleteDNSHost(ctx context.Context, command DeleteNetworkDNSHostCommand) (domain.VirtualNetwork, error) {
	if !domain.CanManageNetworks(command.Request.Session.User.Role) {
		return domain.VirtualNetwork{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VirtualNetwork{}, domain.ErrReadOnly
	}
	command.Name = strings.TrimSpace(command.Name)
	if !networkNamePattern.MatchString(command.Name) {
		return domain.VirtualNetwork{}, domain.ErrInvalidInput
	}
	address, err := netip.ParseAddr(strings.TrimSpace(command.IP))
	if err != nil || !address.IsValid() {
		return domain.VirtualNetwork{}, domain.ErrInvalidInput
	}
	address = address.Unmap()
	network, err := s.findNetwork(ctx, command.Name)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	host, err := findNetworkDNSHost(network, address)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	updated, err := s.repository.DeleteNetworkDNSHost(ctx, command.Name, host)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	if err := s.recordAudit(ctx, command.Request, "network.dns.host.delete", command.Name, map[string]any{"ip": host.IP, "hostnames": host.Hostnames}); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("record network DNS host deletion audit: %w", err)
	}
	return updated, nil
}

func (s *Service) findNetwork(ctx context.Context, name string) (domain.VirtualNetwork, error) {
	networks, err := s.repository.ListNetworks(ctx)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	for _, network := range networks {
		if network.Name == name {
			return network, nil
		}
	}
	return domain.VirtualNetwork{}, domain.ErrNotFound
}

func normalizeNetworkDHCPHost(spec domain.NetworkDHCPHostCreateSpec) (domain.NetworkDHCPHost, error) {
	ip, err := netip.ParseAddr(strings.TrimSpace(spec.IP))
	if err != nil || !ip.IsValid() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return domain.NetworkDHCPHost{}, fmt.Errorf("%w: DHCP host IP must be a unicast IPv4 or IPv6 address", domain.ErrInvalidInput)
	}
	ip = ip.Unmap()
	normalizedIP := ip.String()
	name := strings.TrimSpace(spec.Name)
	if name != "" && !dhcpHostNamePattern.MatchString(name) {
		return domain.NetworkDHCPHost{}, fmt.Errorf("%w: invalid DHCP host name", domain.ErrInvalidInput)
	}
	hostID := normalizeDHCPClientID(spec.ID)
	if hostID != "" && !duidPattern.MatchString(hostID) {
		return domain.NetworkDHCPHost{}, fmt.Errorf("%w: invalid DHCPv6 client ID or DUID", domain.ErrInvalidInput)
	}
	if ip.Is4() {
		if hostID != "" {
			return domain.NetworkDHCPHost{}, fmt.Errorf("%w: DHCPv6 client ID/DUID is only valid for IPv6 reservations", domain.ErrInvalidInput)
		}
		mac, err := normalizeNetworkMAC(spec.MAC)
		if err != nil {
			return domain.NetworkDHCPHost{}, err
		}
		return domain.NetworkDHCPHost{MAC: mac, ID: hostID, IP: normalizedIP, Name: name}, nil
	}
	if strings.TrimSpace(spec.MAC) != "" {
		return domain.NetworkDHCPHost{}, fmt.Errorf("%w: DHCPv6 reservations must use a client ID/DUID or hostname, not a MAC address", domain.ErrInvalidInput)
	}
	if hostID == "" && name == "" {
		return domain.NetworkDHCPHost{}, fmt.Errorf("%w: DHCPv6 reservations require a client ID/DUID or hostname", domain.ErrInvalidInput)
	}
	return domain.NetworkDHCPHost{ID: hostID, IP: normalizedIP, Name: name}, nil
}

func normalizeDHCPClientID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "-", ":")
	if duidPattern.MatchString(value) {
		return value
	}
	if len(value)%2 == 0 {
		compact := strings.ReplaceAll(value, ":", "")
		if compact != "" && len(strings.ReplaceAll(compact, " ", "")) == len(compact) {
			valid := true
			for _, character := range compact {
				if !strings.ContainsRune("0123456789abcdef", character) {
					valid = false
					break
				}
			}
			if valid {
				groups := make([]string, 0, len(compact)/2)
				for index := 0; index < len(compact); index += 2 {
					groups = append(groups, compact[index:index+2])
				}
				return strings.Join(groups, ":")
			}
		}
	}
	return value
}

func normalizeNetworkMAC(value string) (string, error) {
	hardwareAddress, err := net.ParseMAC(strings.TrimSpace(value))
	if err != nil || len(hardwareAddress) != 6 {
		return "", fmt.Errorf("%w: invalid MAC address", domain.ErrInvalidInput)
	}
	return strings.ToLower(hardwareAddress.String()), nil
}

func validateNetworkDHCPHost(network domain.VirtualNetwork, host domain.NetworkDHCPHost) (domain.NetworkAddress, error) {
	if !network.Persistent && !network.Active {
		return domain.NetworkAddress{}, domain.ErrNetworkUnsupported
	}
	hostIP := net.ParseIP(host.IP)
	if hostIP == nil {
		return domain.NetworkAddress{}, fmt.Errorf("%w: invalid DHCP host IP", domain.ErrInvalidInput)
	}
	hostFamily := "ipv6"
	if hostIP.To4() != nil {
		hostFamily = "ipv4"
	}
	for _, address := range network.Addresses {
		if networkAddressFamily(address) != hostFamily {
			continue
		}
		dhcpAvailable := address.DHCPEnabled || address.DHCPStart != ""
		if hostFamily == "ipv6" && address.IPv6Mode != ipv6ModeDHCPv6 {
			dhcpAvailable = false
		}
		if !dhcpAvailable || !hostBelongsToNetworkAddress(address, hostIP) {
			return domain.NetworkAddress{}, fmt.Errorf("%w: DHCP host IP must be an available address inside the network subnet", domain.ErrInvalidInput)
		}
		for _, existing := range address.Hosts {
			identityConflict := networkDHCPHostIdentityConflict(existing, host)
			ipConflict := existing.IP == host.IP
			if identityConflict || ipConflict {
				return domain.NetworkAddress{}, domain.ErrConflict
			}
		}
		return address, nil
	}
	return domain.NetworkAddress{}, domain.ErrNetworkUnsupported
}

func normalizeNetworkDNSHost(spec domain.NetworkDNSHostCreateSpec) (domain.NetworkDNSHost, error) {
	address, err := netip.ParseAddr(strings.TrimSpace(spec.IP))
	if err != nil || !address.IsValid() || !address.IsGlobalUnicast() || address.IsUnspecified() || address.IsMulticast() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return domain.NetworkDNSHost{}, fmt.Errorf("%w: DNS host IP must be a unicast IPv4 or IPv6 address", domain.ErrInvalidInput)
	}
	hostnames := make([]string, 0, len(spec.Hostnames))
	seen := map[string]bool{}
	for _, value := range spec.Hostnames {
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if name == "" || len(name) > 253 || !dnsDomainPattern.MatchString(name) {
			return domain.NetworkDNSHost{}, fmt.Errorf("%w: invalid DNS hostname", domain.ErrInvalidInput)
		}
		if !seen[name] {
			seen[name] = true
			hostnames = append(hostnames, name)
		}
	}
	if len(hostnames) == 0 || len(hostnames) > 8 {
		return domain.NetworkDNSHost{}, fmt.Errorf("%w: a DNS host entry requires between 1 and 8 hostnames", domain.ErrInvalidInput)
	}
	return domain.NetworkDNSHost{IP: address.Unmap().String(), Hostnames: hostnames}, nil
}

func validateNetworkDNSHost(network domain.VirtualNetwork, host domain.NetworkDNSHost) error {
	if !network.Persistent && !network.Active {
		return domain.ErrNetworkUnsupported
	}
	hostIP := net.ParseIP(host.IP)
	if hostIP == nil {
		return fmt.Errorf("%w: invalid DNS host IP", domain.ErrInvalidInput)
	}
	found := false
	for _, address := range network.Addresses {
		if networkAddressFamily(address) == ipFamilyFromNetIP(hostIP) && addressBelongsToNetworkAddress(address, hostIP) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: DNS host IP must be inside one of the network subnets", domain.ErrInvalidInput)
	}
	for _, existing := range network.DNSHosts {
		if existing.IP == host.IP {
			return domain.ErrConflict
		}
		for _, left := range existing.Hostnames {
			for _, right := range host.Hostnames {
				if strings.EqualFold(left, right) {
					return domain.ErrConflict
				}
			}
		}
	}
	return nil
}

func findNetworkDNSHost(network domain.VirtualNetwork, address netip.Addr) (domain.NetworkDNSHost, error) {
	for _, host := range network.DNSHosts {
		value, err := netip.ParseAddr(host.IP)
		if err == nil && value.Unmap() == address.Unmap() {
			return host, nil
		}
	}
	return domain.NetworkDNSHost{}, domain.ErrNotFound
}

func ipFamilyFromNetIP(address net.IP) string {
	if address.To4() == nil && address.To16() != nil {
		return "ipv6"
	}
	return "ipv4"
}

func addressBelongsToNetworkAddress(address domain.NetworkAddress, host net.IP) bool {
	if host == nil {
		return false
	}
	if networkAddressFamily(address) == "ipv6" {
		gateway, err := netip.ParseAddr(address.Address)
		hostAddress, hostErr := netip.ParseAddr(host.String())
		if err != nil || hostErr != nil || !gateway.Is6() || !hostAddress.Is6() || !hostAddress.IsGlobalUnicast() || hostAddress.IsLoopback() || hostAddress.IsLinkLocalUnicast() || address.Prefix < 1 || address.Prefix > 128 {
			return false
		}
		prefix := netip.PrefixFrom(gateway, address.Prefix).Masked()
		return prefix.Contains(hostAddress) && hostAddress != prefix.Addr() && hostAddress.Compare(lastIPv6Address(prefix)) < 0
	}
	gateway := net.ParseIP(address.Address).To4()
	hostAddress := host.To4()
	if gateway == nil || hostAddress == nil || address.Prefix < 1 || address.Prefix > 32 {
		return false
	}
	mask := net.CIDRMask(address.Prefix, 32)
	subnet := gateway.Mask(mask)
	broadcast := ipv4Broadcast(subnet, address.Prefix)
	return hostAddress.Mask(mask).Equal(subnet) && !hostAddress.Equal(subnet) && !hostAddress.Equal(broadcast)
}

func networkAddressFamily(address domain.NetworkAddress) string {
	if strings.EqualFold(strings.TrimSpace(address.Family), "ipv6") {
		return "ipv6"
	}
	return "ipv4"
}

func hostBelongsToNetworkAddress(address domain.NetworkAddress, hostIP net.IP) bool {
	if networkAddressFamily(address) == "ipv6" {
		gateway, err := netip.ParseAddr(address.Address)
		host, hostErr := netip.ParseAddr(hostIP.String())
		if err != nil || hostErr != nil || !gateway.Is6() || !host.Is6() || !host.IsGlobalUnicast() || host.IsLoopback() || host.IsLinkLocalUnicast() || address.Prefix < 1 || address.Prefix > 128 {
			return false
		}
		prefix := netip.PrefixFrom(gateway, address.Prefix).Masked()
		last := lastIPv6Address(prefix)
		return prefix.Contains(host) && host != gateway && host != prefix.Addr() && host.Compare(last) < 0
	}
	gateway := net.ParseIP(address.Address).To4()
	host := hostIP.To4()
	if gateway == nil || host == nil || address.Prefix < 1 || address.Prefix > 32 {
		return false
	}
	mask := net.CIDRMask(address.Prefix, 32)
	subnet := gateway.Mask(mask)
	broadcast := ipv4Broadcast(subnet, address.Prefix)
	return host.Mask(mask).Equal(subnet) && !host.Equal(gateway) && !host.Equal(subnet) && !host.Equal(broadcast)
}

func networkDHCPHostIdentityConflict(left, right domain.NetworkDHCPHost) bool {
	if left.MAC != "" && right.MAC != "" && strings.EqualFold(left.MAC, right.MAC) {
		return true
	}
	if left.ID != "" && right.ID != "" && strings.EqualFold(left.ID, right.ID) {
		return true
	}
	if left.Name != "" && right.Name != "" && strings.EqualFold(left.Name, right.Name) {
		return true
	}
	return false
}

func findNetworkDHCPHost(network domain.VirtualNetwork, key string) (domain.NetworkDHCPHost, error) {
	key = strings.TrimSpace(key)
	normalizedIP := key
	if ip, err := netip.ParseAddr(key); err == nil {
		normalizedIP = ip.Unmap().String()
	}
	for _, address := range network.Addresses {
		for _, host := range address.Hosts {
			if host.IP == normalizedIP {
				return host, nil
			}
		}
	}
	for _, address := range network.Addresses {
		for _, host := range address.Hosts {
			if strings.EqualFold(host.ID, key) || strings.EqualFold(host.MAC, key) {
				return host, nil
			}
		}
	}
	for _, address := range network.Addresses {
		for _, host := range address.Hosts {
			if strings.EqualFold(host.Name, key) {
				return host, nil
			}
		}
	}
	return domain.NetworkDHCPHost{}, domain.ErrNotFound
}

func ipv4Broadcast(subnet net.IP, prefix int) net.IP {
	result := append(net.IP(nil), subnet.To4()...)
	for bit := prefix; bit < 32; bit++ {
		result[bit/8] |= 1 << (7 - uint(bit%8))
	}
	return result
}

func (s *Service) Delete(ctx context.Context, command DeleteNetworkCommand) error {
	if !domain.CanManageNetworks(command.Request.Session.User.Role) {
		return domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.ErrReadOnly
	}
	command.Name = strings.TrimSpace(command.Name)
	if !networkNamePattern.MatchString(command.Name) {
		return domain.ErrInvalidInput
	}
	if count, err := s.portForwardCount(ctx, command.Name); err != nil {
		return err
	} else if count > 0 {
		return fmt.Errorf("%w: network still has inbound port forwarding rules", domain.ErrConflict)
	}
	if err := s.repository.DeleteNetwork(ctx, command.Name); err != nil {
		return err
	}
	if err := s.recordAudit(ctx, command.Request, "network.delete", command.Name, map[string]any{}); err != nil {
		return fmt.Errorf("record network delete audit: %w", err)
	}
	return nil
}

func normalizeCreateSpec(spec domain.VirtualNetworkCreateSpec) (domain.VirtualNetworkDefinition, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	if !networkNamePattern.MatchString(spec.Name) {
		return domain.VirtualNetworkDefinition{}, fmt.Errorf("%w: invalid network name", domain.ErrInvalidInput)
	}
	definition, err := normalizeNetworkSpec(spec.Name, spec.Mode, spec.Subnet, spec.DHCPStart, spec.DHCPEnd,
		spec.IPv6Subnet, spec.IPv6Mode, spec.IPv6DHCPStart, spec.IPv6DHCPEnd,
		spec.NATIPv6, spec.MTU, spec.DNSDomain, spec.DNSServers, spec.Bridge, spec.Autostart)
	if err != nil {
		return domain.VirtualNetworkDefinition{}, err
	}
	definition.Start = spec.Start
	return definition, nil
}

func normalizeNetworkSpec(name, mode, subnetText, dhcpStartText, dhcpEndText, ipv6SubnetText, ipv6ModeText, ipv6DHCPStartText, ipv6DHCPEndText string, natIPv6 *bool, mtu *int, dnsDomainText string, dnsServerValues []string, bridgeText string, autostart bool) (domain.VirtualNetworkDefinition, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "bridge" {
		bridge := strings.TrimSpace(bridgeText)
		if !hostBridgeNamePattern.MatchString(bridge) {
			return domain.VirtualNetworkDefinition{}, fmt.Errorf("%w: host bridge name is invalid", domain.ErrInvalidInput)
		}
		if mtu != nil || strings.TrimSpace(subnetText) != "" || strings.TrimSpace(dhcpStartText) != "" || strings.TrimSpace(dhcpEndText) != "" ||
			strings.TrimSpace(ipv6SubnetText) != "" || strings.TrimSpace(ipv6ModeText) != "" || strings.TrimSpace(ipv6DHCPStartText) != "" || strings.TrimSpace(ipv6DHCPEndText) != "" ||
			(natIPv6 != nil && *natIPv6) || strings.TrimSpace(dnsDomainText) != "" || len(dnsServerValues) > 0 {
			return domain.VirtualNetworkDefinition{}, fmt.Errorf("%w: bridge networks use addressing and DNS supplied by the upstream network", domain.ErrInvalidInput)
		}
		return domain.VirtualNetworkDefinition{
			Name: name, Mode: mode, Bridge: bridge, Autostart: autostart,
		}, nil
	}
	if mode != "nat" && mode != "isolated" {
		return domain.VirtualNetworkDefinition{}, fmt.Errorf("%w: network mode must be nat, isolated or bridge", domain.ErrInvalidInput)
	}
	ipv4Address, ipv4Prefix, dhcpStart, dhcpEnd, err := normalizeIPv4Subnet(subnetText, dhcpStartText, dhcpEndText)
	if err != nil {
		return domain.VirtualNetworkDefinition{}, err
	}
	ipv6Address, ipv6Prefix, ipv6Mode, ipv6DHCPStart, ipv6DHCPEnd, err := normalizeIPv6Subnet(ipv6SubnetText, ipv6ModeText, ipv6DHCPStartText, ipv6DHCPEndText)
	if err != nil {
		return domain.VirtualNetworkDefinition{}, err
	}
	if ipv4Address == "" && ipv6Address == "" {
		return domain.VirtualNetworkDefinition{}, fmt.Errorf("%w: an IPv4 or IPv6 subnet is required", domain.ErrInvalidInput)
	}
	mtuValue, mtuPresent, err := normalizeNetworkMTU(mtu, ipv6Address != "")
	if err != nil {
		return domain.VirtualNetworkDefinition{}, err
	}
	dnsDomain, dnsServers, err := normalizeNetworkDNS(dnsDomainText, dnsServerValues)
	if err != nil {
		return domain.VirtualNetworkDefinition{}, err
	}
	natIPv6Enabled := mode == "nat" && ipv6Address != "" && (natIPv6 == nil || *natIPv6)
	if natIPv6 != nil && *natIPv6 && (mode != "nat" || ipv6Address == "") {
		return domain.VirtualNetworkDefinition{}, fmt.Errorf("%w: IPv6 NAT requires a NAT network with an IPv6 subnet", domain.ErrInvalidInput)
	}
	return domain.VirtualNetworkDefinition{
		Name: name, Mode: mode, IPv4Address: ipv4Address, Prefix: ipv4Prefix,
		DHCPStart: dhcpStart, DHCPEnd: dhcpEnd,
		IPv6Address: ipv6Address, IPv6Prefix: ipv6Prefix, IPv6Mode: ipv6Mode,
		IPv6DHCPStart: ipv6DHCPStart, IPv6DHCPEnd: ipv6DHCPEnd,
		NATIPv6: natIPv6Enabled, MTU: mtuValue, MTUPresent: mtuPresent,
		DNSDomain: dnsDomain, DNSServers: dnsServers, Autostart: autostart,
	}, nil
}

func normalizeNetworkMTU(value *int, hasIPv6 bool) (int, bool, error) {
	if value == nil {
		return 0, false, nil
	}
	mtu := *value
	if mtu == 0 {
		return 0, true, nil
	}
	if mtu < 576 || mtu > 9000 {
		return 0, false, fmt.Errorf("%w: network MTU must be between 576 and 9000", domain.ErrInvalidInput)
	}
	if hasIPv6 && mtu < 1280 {
		return 0, false, fmt.Errorf("%w: IPv6 networks require an MTU of at least 1280", domain.ErrInvalidInput)
	}
	return mtu, true, nil
}

func normalizeIPv4Subnet(subnetText, dhcpStartText, dhcpEndText string) (string, int, string, string, error) {
	subnetText = strings.TrimSpace(subnetText)
	dhcpStart := strings.TrimSpace(dhcpStartText)
	dhcpEnd := strings.TrimSpace(dhcpEndText)
	if subnetText == "" {
		if dhcpStart != "" || dhcpEnd != "" {
			return "", 0, "", "", fmt.Errorf("%w: DHCP range requires an IPv4 subnet", domain.ErrInvalidInput)
		}
		return "", 0, "", "", nil
	}
	ip, subnet, err := net.ParseCIDR(subnetText)
	if err != nil || ip.To4() == nil {
		return "", 0, "", "", fmt.Errorf("%w: invalid IPv4 subnet", domain.ErrInvalidInput)
	}
	ipv4 := ip.Mask(subnet.Mask).To4()
	ones, bits := subnet.Mask.Size()
	if bits != 32 || ones > 29 {
		return "", 0, "", "", fmt.Errorf("%w: IPv4 subnet must be /29 or larger", domain.ErrInvalidInput)
	}
	base := binary.BigEndian.Uint32(ipv4)
	broadcast := base | ^binary.BigEndian.Uint32(subnet.Mask)
	gateway := base + 1
	if (dhcpStart == "") != (dhcpEnd == "") {
		return "", 0, "", "", fmt.Errorf("%w: DHCP start and end must be provided together", domain.ErrInvalidInput)
	}
	if dhcpStart == "" {
		dhcpStart = ipv4String(base + 2)
		dhcpEnd = ipv4String(broadcast - 1)
	} else {
		startIP := net.ParseIP(dhcpStart).To4()
		endIP := net.ParseIP(dhcpEnd).To4()
		if startIP == nil || endIP == nil {
			return "", 0, "", "", fmt.Errorf("%w: invalid DHCP address", domain.ErrInvalidInput)
		}
		start := binary.BigEndian.Uint32(startIP)
		end := binary.BigEndian.Uint32(endIP)
		if start <= gateway || end >= broadcast || start >= end {
			return "", 0, "", "", fmt.Errorf("%w: DHCP range must be inside the subnet after the gateway", domain.ErrInvalidInput)
		}
	}
	return ipv4String(gateway), ones, dhcpStart, dhcpEnd, nil
}

func normalizeIPv6Subnet(subnetText, modeText, dhcpStartText, dhcpEndText string) (string, int, string, string, string, error) {
	subnetText = strings.TrimSpace(subnetText)
	modeText = strings.ToLower(strings.TrimSpace(modeText))
	dhcpStartText = strings.TrimSpace(dhcpStartText)
	dhcpEndText = strings.TrimSpace(dhcpEndText)
	if modeText == "disabled" || modeText == "none" {
		modeText = ""
	}
	if subnetText == "" {
		if modeText != "" || dhcpStartText != "" || dhcpEndText != "" {
			return "", 0, "", "", "", fmt.Errorf("%w: IPv6 mode and DHCPv6 range require an IPv6 subnet", domain.ErrInvalidInput)
		}
		return "", 0, "", "", "", nil
	}
	prefix, err := netip.ParsePrefix(subnetText)
	if err != nil || !prefix.Addr().Is6() || prefix.Addr().Is4In6() {
		return "", 0, "", "", "", fmt.Errorf("%w: invalid IPv6 subnet", domain.ErrInvalidInput)
	}
	prefix = prefix.Masked()
	if !prefix.Addr().IsGlobalUnicast() || prefix.Addr().Is4In6() {
		return "", 0, "", "", "", fmt.Errorf("%w: IPv6 subnet must use a global unicast or ULA prefix", domain.ErrInvalidInput)
	}
	ones := prefix.Bits()
	if ones < 64 || ones > 124 {
		return "", 0, "", "", "", fmt.Errorf("%w: IPv6 subnet must be between /64 and /124", domain.ErrInvalidInput)
	}
	gateway := prefix.Addr().Next()
	if modeText == "" {
		modeText = ipv6ModeDHCPv6
	}
	if modeText != ipv6ModeSLAAC && modeText != ipv6ModeDHCPv6 {
		return "", 0, "", "", "", fmt.Errorf("%w: IPv6 mode must be slaac or dhcpv6", domain.ErrInvalidInput)
	}
	if modeText == ipv6ModeSLAAC {
		if ones != 64 {
			return "", 0, "", "", "", fmt.Errorf("%w: SLAAC requires an IPv6 /64 prefix", domain.ErrInvalidInput)
		}
		if dhcpStartText != "" || dhcpEndText != "" {
			return "", 0, "", "", "", fmt.Errorf("%w: DHCPv6 range is not valid in SLAAC-only mode", domain.ErrInvalidInput)
		}
		return gateway.String(), ones, modeText, "", "", nil
	}
	if (dhcpStartText == "") != (dhcpEndText == "") {
		return "", 0, "", "", "", fmt.Errorf("%w: DHCPv6 start and end must be provided together", domain.ErrInvalidInput)
	}
	start := gateway.Next()
	last := lastIPv6Address(prefix)
	end := addIPv6(gateway, 0xffff)
	if end.Compare(last) >= 0 {
		end = last.Prev()
	}
	if dhcpStartText != "" {
		start, err = netip.ParseAddr(dhcpStartText)
		if err != nil || !start.Is6() || start.Is4In6() {
			return "", 0, "", "", "", fmt.Errorf("%w: invalid DHCPv6 start address", domain.ErrInvalidInput)
		}
		end, err = netip.ParseAddr(dhcpEndText)
		if err != nil || !end.Is6() || end.Is4In6() {
			return "", 0, "", "", "", fmt.Errorf("%w: invalid DHCPv6 end address", domain.ErrInvalidInput)
		}
		start, end = start.Unmap(), end.Unmap()
	}
	if !start.IsGlobalUnicast() || !end.IsGlobalUnicast() || !prefix.Contains(start) || !prefix.Contains(end) ||
		start.Compare(gateway) <= 0 || end.Compare(start) < 0 || end.Compare(last) >= 0 {
		return "", 0, "", "", "", fmt.Errorf("%w: DHCPv6 range must contain usable unicast addresses inside the subnet after the gateway", domain.ErrInvalidInput)
	}
	return gateway.String(), ones, modeText, start.String(), end.String(), nil
}

func normalizeNetworkDNS(domainText string, serverValues []string) (string, []string, error) {
	dnsDomain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domainText), "."))
	if dnsDomain != "" {
		if len(dnsDomain) > 253 || !dnsDomainPattern.MatchString(dnsDomain) {
			return "", nil, fmt.Errorf("%w: invalid DNS domain", domain.ErrInvalidInput)
		}
	}
	servers := make([]string, 0, len(serverValues))
	seen := map[string]bool{}
	for _, value := range serverValues {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		address, err := netip.ParseAddr(value)
		if err != nil || !address.IsValid() || !address.IsGlobalUnicast() || address.IsLoopback() || address.IsLinkLocalUnicast() {
			return "", nil, fmt.Errorf("%w: DNS servers must be unicast IPv4 or IPv6 addresses", domain.ErrInvalidInput)
		}
		normalized := address.Unmap().String()
		if !seen[normalized] {
			seen[normalized] = true
			servers = append(servers, normalized)
		}
	}
	return dnsDomain, servers, nil
}

func (s *Service) validateNetworkOverlap(ctx context.Context, definition domain.VirtualNetworkDefinition, excludeName string) error {
	networks, err := s.repository.ListNetworks(ctx)
	if err != nil {
		return err
	}
	candidates := make([]netip.Prefix, 0, 2)
	if definition.IPv4Address != "" {
		if prefix, ok := networkPrefix("ipv4", definition.IPv4Address, definition.Prefix); ok {
			candidates = append(candidates, prefix)
		}
	}
	if definition.IPv6Address != "" {
		if prefix, ok := networkPrefix("ipv6", definition.IPv6Address, definition.IPv6Prefix); ok {
			candidates = append(candidates, prefix)
		}
	}
	for _, network := range networks {
		if network.Name == excludeName {
			continue
		}
		for _, address := range network.Addresses {
			existing, ok := networkPrefix(networkAddressFamily(address), address.Address, address.Prefix)
			if !ok {
				continue
			}
			for _, candidate := range candidates {
				if candidate.Addr().BitLen() != existing.Addr().BitLen() {
					continue
				}
				if candidate.Overlaps(existing) {
					return fmt.Errorf("%w: subnet %s overlaps network %s subnet %s", domain.ErrConflict, candidate, network.Name, existing)
				}
			}
		}
	}
	return nil
}

func (s *Service) validateNetworkUpdateHosts(ctx context.Context, name string, definition domain.VirtualNetworkDefinition) error {
	network, err := s.findNetwork(ctx, name)
	if err != nil {
		return err
	}
	for _, host := range network.DNSHosts {
		hostIP := net.ParseIP(host.IP)
		if hostIP == nil {
			return fmt.Errorf("%w: DNS host %s has an invalid address; remove it before changing the network", domain.ErrConflict, host.IP)
		}
		family := ipFamilyFromNetIP(hostIP)
		target := domain.NetworkAddress{}
		if family == "ipv6" {
			target = domain.NetworkAddress{Family: "ipv6", Address: definition.IPv6Address, Prefix: definition.IPv6Prefix}
		} else {
			target = domain.NetworkAddress{Family: "ipv4", Address: definition.IPv4Address, Prefix: definition.Prefix}
		}
		if target.Address == "" || !addressBelongsToNetworkAddress(target, hostIP) {
			return fmt.Errorf("%w: DNS host %s is outside the new %s subnet; remove or migrate the DNS record first", domain.ErrConflict, host.IP, family)
		}
	}
	for _, address := range network.Addresses {
		if len(address.Hosts) == 0 {
			continue
		}
		family := networkAddressFamily(address)
		target := domain.NetworkAddress{}
		switch family {
		case "ipv6":
			if definition.IPv6Address == "" || definition.IPv6Mode != ipv6ModeDHCPv6 {
				return fmt.Errorf("%w: remove or migrate IPv6 static reservations before disabling DHCPv6 or removing the IPv6 subnet", domain.ErrConflict)
			}
			target = domain.NetworkAddress{Family: "ipv6", Address: definition.IPv6Address, Prefix: definition.IPv6Prefix, IPv6Mode: definition.IPv6Mode}
		default:
			if definition.IPv4Address == "" || definition.DHCPStart == "" {
				return fmt.Errorf("%w: remove or migrate IPv4 static reservations before disabling IPv4 DHCP or removing the IPv4 subnet", domain.ErrConflict)
			}
			target = domain.NetworkAddress{Family: "ipv4", Address: definition.IPv4Address, Prefix: definition.Prefix}
		}
		for _, host := range address.Hosts {
			hostIP := net.ParseIP(host.IP)
			if hostIP == nil || !hostBelongsToNetworkAddress(target, hostIP) {
				return fmt.Errorf("%w: static host %s (%s) is outside the new %s subnet; remove or migrate the reservation first", domain.ErrConflict, host.IP, dhcpHostDisplayKey(host), family)
			}
		}
	}
	return nil
}

func dhcpHostDisplayKey(host domain.NetworkDHCPHost) string {
	for _, value := range []string{host.ID, host.MAC, host.Name} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return host.IP
}

func networkPrefix(family, address string, prefixLength int) (netip.Prefix, bool) {
	addressValue, err := netip.ParseAddr(strings.TrimSpace(address))
	if err != nil || !addressValue.IsValid() {
		return netip.Prefix{}, false
	}
	addressValue = addressValue.Unmap()
	if family == "ipv6" {
		if !addressValue.Is6() || prefixLength < 0 || prefixLength > 128 {
			return netip.Prefix{}, false
		}
		return netip.PrefixFrom(addressValue, prefixLength).Masked(), true
	}
	if !addressValue.Is4() || prefixLength < 0 || prefixLength > 32 {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(addressValue, prefixLength).Masked(), true
}

func addIPv6(address netip.Addr, offset uint64) netip.Addr {
	bytes := address.As16()
	carry := offset
	for index := len(bytes) - 1; index >= 0 && carry > 0; index-- {
		sum := uint64(bytes[index]) + carry
		bytes[index] = byte(sum)
		carry = sum >> 8
	}
	return netip.AddrFrom16(bytes)
}

func lastIPv6Address(prefix netip.Prefix) netip.Addr {
	bytes := prefix.Masked().Addr().As16()
	for bit := prefix.Bits(); bit < 128; bit++ {
		bytes[bit/8] |= 1 << (7 - uint(bit%8))
	}
	return netip.AddrFrom16(bytes)
}

func ipv4String(value uint32) string {
	return net.IPv4(byte(value>>24), byte(value>>16), byte(value>>8), byte(value)).String()
}

func (s *Service) recordAudit(ctx context.Context, request MutationContext, action, targetID string, detail map[string]any) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: time.Now().UTC(),
		RequestID:  request.RequestID,
		Actor:      request.Session.User.Username,
		Role:       request.Session.User.Role,
		Action:     action,
		TargetKind: "network",
		TargetID:   targetID,
		SourceIP:   request.SourceIP,
		Result:     "success",
		Detail:     detail,
	})
}
