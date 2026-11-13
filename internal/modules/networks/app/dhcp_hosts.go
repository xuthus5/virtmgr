package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"net"
	"sort"
	"strings"

	"virtmgr/internal/domain"
)

const maxNetworkDHCPHostImport = 1000

type ImportNetworkDHCPHostsCommand struct {
	Name    string
	Hosts   []domain.NetworkDHCPHostCreateSpec
	Request MutationContext
}

func (s *Service) ExportDHCPHostsCSV(ctx context.Context, networkName string) ([]byte, error) {
	networkName = strings.TrimSpace(networkName)
	if !networkNamePattern.MatchString(networkName) {
		return nil, domain.ErrInvalidInput
	}
	network, err := s.findNetwork(ctx, networkName)
	if err != nil {
		return nil, err
	}
	return encodeNetworkDHCPHostsCSV(network)
}

func (s *Service) ImportDHCPHostsCSV(ctx context.Context, name string, data []byte, request MutationContext) (domain.NetworkDHCPHostImportResult, error) {
	hosts, err := ParseNetworkDHCPHostCSV(data)
	if err != nil {
		return domain.NetworkDHCPHostImportResult{}, err
	}
	return s.ImportDHCPHosts(ctx, ImportNetworkDHCPHostsCommand{Name: name, Hosts: hosts, Request: request})
}

func (s *Service) ImportDHCPHosts(ctx context.Context, command ImportNetworkDHCPHostsCommand) (domain.NetworkDHCPHostImportResult, error) {
	if !domain.CanManageNetworks(command.Request.Session.User.Role) {
		return domain.NetworkDHCPHostImportResult{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.NetworkDHCPHostImportResult{}, domain.ErrReadOnly
	}
	command.Name = strings.TrimSpace(command.Name)
	if !networkNamePattern.MatchString(command.Name) {
		return domain.NetworkDHCPHostImportResult{}, domain.ErrInvalidInput
	}
	if len(command.Hosts) == 0 || len(command.Hosts) > maxNetworkDHCPHostImport {
		return domain.NetworkDHCPHostImportResult{}, fmt.Errorf("%w: import must contain between 1 and %d hosts", domain.ErrInvalidInput, maxNetworkDHCPHostImport)
	}
	network, err := s.findNetwork(ctx, command.Name)
	if err != nil {
		return domain.NetworkDHCPHostImportResult{}, err
	}

	validation := cloneVirtualNetwork(network)
	existingByMAC := map[string]domain.NetworkDHCPHost{}
	existingByIP := map[string]domain.NetworkDHCPHost{}
	for _, address := range network.Addresses {
		for _, host := range address.Hosts {
			existingByMAC[strings.ToLower(host.MAC)] = host
			existingByIP[host.IP] = host
		}
	}
	batchByMAC := map[string]domain.NetworkDHCPHost{}
	batchByIP := map[string]domain.NetworkDHCPHost{}
	accepted := make([]domain.NetworkDHCPHost, 0, len(command.Hosts))
	skipped := 0
	for _, raw := range command.Hosts {
		host, err := normalizeNetworkDHCPHost(raw)
		if err != nil {
			return domain.NetworkDHCPHostImportResult{}, err
		}
		existing, hasMAC := existingByMAC[strings.ToLower(host.MAC)]
		byIP, hasIP := existingByIP[host.IP]
		if hasMAC || hasIP {
			if hasMAC && hasIP && sameNetworkDHCPHost(existing, host) && sameNetworkDHCPHost(byIP, host) {
				skipped++
				continue
			}
			return domain.NetworkDHCPHostImportResult{}, fmt.Errorf("%w: DHCP host %s or %s already exists with different values", domain.ErrConflict, host.MAC, host.IP)
		}
		batchMAC, seenMAC := batchByMAC[strings.ToLower(host.MAC)]
		batchIP, seenIP := batchByIP[host.IP]
		if seenMAC || seenIP {
			if seenMAC && seenIP && sameNetworkDHCPHost(batchMAC, host) && sameNetworkDHCPHost(batchIP, host) {
				skipped++
				continue
			}
			return domain.NetworkDHCPHostImportResult{}, fmt.Errorf("%w: duplicate DHCP host %s or %s in import", domain.ErrConflict, host.MAC, host.IP)
		}
		address, err := validateNetworkDHCPHost(validation, host)
		if err != nil {
			return domain.NetworkDHCPHostImportResult{}, err
		}
		for index := range validation.Addresses {
			if validation.Addresses[index].Family == address.Family && validation.Addresses[index].Address == address.Address && validation.Addresses[index].Prefix == address.Prefix {
				validation.Addresses[index].Hosts = append(validation.Addresses[index].Hosts, host)
				break
			}
		}
		batchByMAC[strings.ToLower(host.MAC)] = host
		batchByIP[host.IP] = host
		accepted = append(accepted, host)
	}

	updated := network
	imported := make([]domain.NetworkDHCPHost, 0, len(accepted))
	for _, host := range accepted {
		next, err := s.repository.AddNetworkDHCPHost(ctx, command.Name, host)
		if err != nil {
			for index := len(imported) - 1; index >= 0; index-- {
				_, _ = s.repository.DeleteNetworkDHCPHost(ctx, command.Name, imported[index])
			}
			return domain.NetworkDHCPHostImportResult{}, fmt.Errorf("import DHCP host %s: %w", host.MAC, err)
		}
		updated = next
		imported = append(imported, host)
	}

	if len(imported) > 0 {
		detail := map[string]any{"imported": len(imported), "skipped": skipped, "total": len(command.Hosts)}
		if err := s.recordAudit(ctx, command.Request, "network.dhcp.host.import", command.Name, detail); err != nil {
			return domain.NetworkDHCPHostImportResult{}, fmt.Errorf("record network DHCP host import audit: %w", err)
		}
	}
	return domain.NetworkDHCPHostImportResult{Network: updated, Imported: len(imported), Skipped: skipped}, nil
}

func ParseNetworkDHCPHostCSV(data []byte) ([]domain.NetworkDHCPHostCreateSpec, error) {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%w: CSV file is empty", domain.ErrInvalidInput)
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: invalid CSV: %v", domain.ErrInvalidInput, err)
	}
	hosts := make([]domain.NetworkDHCPHostCreateSpec, 0, len(records))
	for index, record := range records {
		fields := make([]string, len(record))
		empty := true
		for fieldIndex, field := range record {
			fields[fieldIndex] = strings.TrimSpace(field)
			if fields[fieldIndex] != "" {
				empty = false
			}
		}
		if empty {
			continue
		}
		if index == 0 && len(fields) >= 2 && strings.EqualFold(fields[0], "mac") && strings.EqualFold(fields[1], "ip") {
			continue
		}
		if len(fields) < 2 || len(fields) > 3 {
			return nil, fmt.Errorf("%w: CSV row %d must contain mac, ip and optional name", domain.ErrInvalidInput, index+1)
		}
		hosts = append(hosts, domain.NetworkDHCPHostCreateSpec{MAC: fields[0], IP: fields[1], Name: optionalCSVField(fields, 2)})
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("%w: CSV file contains no DHCP hosts", domain.ErrInvalidInput)
	}
	if len(hosts) > maxNetworkDHCPHostImport {
		return nil, fmt.Errorf("%w: CSV import exceeds %d hosts", domain.ErrInvalidInput, maxNetworkDHCPHostImport)
	}
	return hosts, nil
}

func encodeNetworkDHCPHostsCSV(network domain.VirtualNetwork) ([]byte, error) {
	hosts := make([]domain.NetworkDHCPHost, 0)
	for _, address := range network.Addresses {
		hosts = append(hosts, address.Hosts...)
	}
	sort.SliceStable(hosts, func(left, right int) bool {
		leftIP := net.ParseIP(hosts[left].IP)
		rightIP := net.ParseIP(hosts[right].IP)
		if leftIP != nil && rightIP != nil {
			if order := bytes.Compare(leftIP.To16(), rightIP.To16()); order != 0 {
				return order < 0
			}
		}
		return hosts[left].IP < hosts[right].IP
	})
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	if err := writer.Write([]string{"mac", "ip", "name"}); err != nil {
		return nil, fmt.Errorf("write DHCP host CSV header: %w", err)
	}
	for _, host := range hosts {
		if err := writer.Write([]string{host.MAC, host.IP, host.Name}); err != nil {
			return nil, fmt.Errorf("write DHCP host CSV row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("flush DHCP host CSV: %w", err)
	}
	return output.Bytes(), nil
}

func cloneVirtualNetwork(network domain.VirtualNetwork) domain.VirtualNetwork {
	cloned := network
	cloned.Addresses = make([]domain.NetworkAddress, len(network.Addresses))
	for index, address := range network.Addresses {
		cloned.Addresses[index] = address
		cloned.Addresses[index].Hosts = append([]domain.NetworkDHCPHost(nil), address.Hosts...)
	}
	return cloned
}

func sameNetworkDHCPHost(left, right domain.NetworkDHCPHost) bool {
	return strings.EqualFold(left.MAC, right.MAC) && left.IP == right.IP && left.Name == right.Name
}

func optionalCSVField(fields []string, index int) string {
	if index >= len(fields) {
		return ""
	}
	return fields[index]
}
