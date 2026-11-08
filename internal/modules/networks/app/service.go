package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

var networkNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{2,63}$`)
var dhcpHostNamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,251}[A-Za-z0-9])?$`)

type Repository interface {
	ListNetworks(ctx context.Context) ([]domain.VirtualNetwork, error)
	ListDHCPLeases(ctx context.Context, networkName string) ([]domain.DHCPLease, error)
	CreateNetwork(ctx context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error)
	UpdateNetwork(ctx context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error)
	AddNetworkDHCPHost(ctx context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error)
	DeleteNetworkDHCPHost(ctx context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error)
	PerformNetworkAction(ctx context.Context, name string, action domain.VirtualNetworkAction) error
	DeleteNetwork(ctx context.Context, name string) error
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type Service struct {
	repository   Repository
	audit        AuditRecorder
	readOnly     bool
	readOnlyMode domain.ReadOnlyProvider
}

type Option func(*Service)

func WithReadOnlyMode(mode domain.ReadOnlyProvider) Option {
	return func(service *Service) {
		service.readOnlyMode = mode
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
	MAC     string
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
	sort.Slice(networks, func(i, j int) bool {
		return strings.ToLower(networks[i].Name) < strings.ToLower(networks[j].Name)
	})
	return domain.VirtualNetworkList{Items: networks, Total: len(networks)}, nil
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
	sort.SliceStable(leases, func(i, j int) bool {
		left := net.ParseIP(leases[i].IPAddress)
		right := net.ParseIP(leases[j].IPAddress)
		if left != nil && right != nil {
			if order := bytes.Compare(left.To16(), right.To16()); order != 0 {
				return order < 0
			}
		}
		if leases[i].IPAddress != leases[j].IPAddress {
			return leases[i].IPAddress < leases[j].IPAddress
		}
		return strings.ToLower(leases[i].Hostname) < strings.ToLower(leases[j].Hostname)
	})
	return domain.DHCPLeaseList{Network: networkName, Items: leases, Total: len(leases)}, nil
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
	network, err := s.repository.CreateNetwork(ctx, definition)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	detail := map[string]any{
		"mode": definition.Mode, "subnet": command.Spec.Subnet,
		"ipv4Address": definition.IPv4Address, "prefix": definition.Prefix,
		"dhcpStart": definition.DHCPStart, "dhcpEnd": definition.DHCPEnd,
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
	definition, err := normalizeNetworkSpec(command.Name, command.Spec.Mode, command.Spec.Subnet, command.Spec.DHCPStart, command.Spec.DHCPEnd, command.Spec.Autostart)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	network, err := s.repository.UpdateNetwork(ctx, definition)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	detail := map[string]any{
		"mode": definition.Mode, "subnet": strings.TrimSpace(command.Spec.Subnet),
		"ipv4Address": definition.IPv4Address, "prefix": definition.Prefix,
		"dhcpStart": definition.DHCPStart, "dhcpEnd": definition.DHCPEnd,
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
	detail := map[string]any{"mac": host.MAC, "ip": host.IP, "name": host.Name}
	if err := s.recordAudit(ctx, command.Request, "network.dhcp.host.add", command.Name, detail); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("record network DHCP host creation audit: %w", err)
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
	mac, err := normalizeNetworkMAC(command.MAC)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	network, err := s.findNetwork(ctx, command.Name)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	host, err := findNetworkDHCPHost(network, mac)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	updated, err := s.repository.DeleteNetworkDHCPHost(ctx, command.Name, host)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	detail := map[string]any{"mac": host.MAC, "ip": host.IP, "name": host.Name}
	if err := s.recordAudit(ctx, command.Request, "network.dhcp.host.delete", command.Name, detail); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("record network DHCP host deletion audit: %w", err)
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
	mac, err := normalizeNetworkMAC(spec.MAC)
	if err != nil {
		return domain.NetworkDHCPHost{}, err
	}
	ip := net.ParseIP(strings.TrimSpace(spec.IP)).To4()
	if ip == nil {
		return domain.NetworkDHCPHost{}, fmt.Errorf("%w: DHCP host IP must be IPv4", domain.ErrInvalidInput)
	}
	name := strings.TrimSpace(spec.Name)
	if name != "" && !dhcpHostNamePattern.MatchString(name) {
		return domain.NetworkDHCPHost{}, fmt.Errorf("%w: invalid DHCP host name", domain.ErrInvalidInput)
	}
	return domain.NetworkDHCPHost{MAC: mac, IP: ip.String(), Name: name}, nil
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
	for _, address := range network.Addresses {
		if address.Family != "" && address.Family != "ipv4" {
			continue
		}
		gateway := net.ParseIP(address.Address).To4()
		if gateway == nil || address.Prefix < 1 || address.Prefix > 32 || (!address.DHCPEnabled && address.DHCPStart == "") {
			return domain.NetworkAddress{}, domain.ErrNetworkUnsupported
		}
		hostIP := net.ParseIP(host.IP).To4()
		mask := net.CIDRMask(address.Prefix, 32)
		subnet := gateway.Mask(mask)
		broadcast := ipv4Broadcast(subnet, address.Prefix)
		if hostIP == nil || !hostIP.Mask(mask).Equal(subnet) || hostIP.Equal(gateway) || hostIP.Equal(subnet) || hostIP.Equal(broadcast) {
			return domain.NetworkAddress{}, fmt.Errorf("%w: DHCP host IP must be an available address inside the network subnet", domain.ErrInvalidInput)
		}
		for _, existing := range address.Hosts {
			macConflict := strings.EqualFold(existing.MAC, host.MAC)
			ipConflict := existing.IP == host.IP
			if macConflict || ipConflict {
				return domain.NetworkAddress{}, domain.ErrConflict
			}
		}
		return address, nil
	}
	return domain.NetworkAddress{}, domain.ErrNetworkUnsupported
}

func findNetworkDHCPHost(network domain.VirtualNetwork, mac string) (domain.NetworkDHCPHost, error) {
	for _, address := range network.Addresses {
		for _, host := range address.Hosts {
			if strings.EqualFold(host.MAC, mac) {
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
	definition, err := normalizeNetworkSpec(spec.Name, spec.Mode, spec.Subnet, spec.DHCPStart, spec.DHCPEnd, spec.Autostart)
	if err != nil {
		return domain.VirtualNetworkDefinition{}, err
	}
	definition.Start = spec.Start
	return definition, nil
}

func normalizeNetworkSpec(name, mode, subnetText, dhcpStartText, dhcpEndText string, autostart bool) (domain.VirtualNetworkDefinition, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "nat" && mode != "isolated" {
		return domain.VirtualNetworkDefinition{}, fmt.Errorf("%w: network mode must be nat or isolated", domain.ErrInvalidInput)
	}
	ip, subnet, err := net.ParseCIDR(strings.TrimSpace(subnetText))
	if err != nil || ip.To4() == nil {
		return domain.VirtualNetworkDefinition{}, fmt.Errorf("%w: invalid IPv4 subnet", domain.ErrInvalidInput)
	}
	ipv4 := ip.Mask(subnet.Mask).To4()
	ones, bits := subnet.Mask.Size()
	if bits != 32 || ones > 29 {
		return domain.VirtualNetworkDefinition{}, fmt.Errorf("%w: IPv4 subnet must be /29 or larger", domain.ErrInvalidInput)
	}
	base := binary.BigEndian.Uint32(ipv4)
	broadcast := base | ^binary.BigEndian.Uint32(subnet.Mask)
	gateway := base + 1
	dhcpStart := strings.TrimSpace(dhcpStartText)
	dhcpEnd := strings.TrimSpace(dhcpEndText)
	if (dhcpStart == "") != (dhcpEnd == "") {
		return domain.VirtualNetworkDefinition{}, fmt.Errorf("%w: DHCP start and end must be provided together", domain.ErrInvalidInput)
	}
	if dhcpStart == "" {
		dhcpStart = ipv4String(base + 2)
		dhcpEnd = ipv4String(broadcast - 1)
	} else {
		startIP := net.ParseIP(dhcpStart).To4()
		endIP := net.ParseIP(dhcpEnd).To4()
		if startIP == nil || endIP == nil {
			return domain.VirtualNetworkDefinition{}, fmt.Errorf("%w: invalid DHCP address", domain.ErrInvalidInput)
		}
		start := binary.BigEndian.Uint32(startIP)
		end := binary.BigEndian.Uint32(endIP)
		if start <= gateway || end >= broadcast || start >= end {
			return domain.VirtualNetworkDefinition{}, fmt.Errorf("%w: DHCP range must be inside the subnet after the gateway", domain.ErrInvalidInput)
		}
	}
	return domain.VirtualNetworkDefinition{
		Name: name, Mode: mode, IPv4Address: ipv4String(gateway), Prefix: ones,
		DHCPStart: dhcpStart, DHCPEnd: dhcpEnd, Autostart: autostart,
	}, nil
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
