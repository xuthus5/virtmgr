package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

type networkXML struct {
	Forward struct {
		Mode string `xml:"mode,attr"`
	} `xml:"forward"`
	Bridge struct {
		Name string `xml:"name,attr"`
	} `xml:"bridge"`
	IPs []networkIPXML `xml:"ip"`
}

type networkIPXML struct {
	Family  string          `xml:"family,attr"`
	Address string          `xml:"address,attr"`
	Netmask string          `xml:"netmask,attr"`
	Prefix  int             `xml:"prefix,attr"`
	DHCP    *networkDHCPXML `xml:"dhcp"`
}

type networkDHCPXML struct {
	Range *networkRangeXML     `xml:"range"`
	Hosts []networkDHCPHostXML `xml:"host"`
}

type networkDHCPHostXML struct {
	XMLName xml.Name `xml:"host"`
	MAC     string   `xml:"mac,attr"`
	IP      string   `xml:"ip,attr"`
	Name    string   `xml:"name,attr,omitempty"`
}

type networkDefinitionXML struct {
	XMLName xml.Name               `xml:"network"`
	Name    string                 `xml:"name"`
	Forward *networkForwardXML     `xml:"forward,omitempty"`
	IP      networkIPDefinitionXML `xml:"ip"`
}

type networkForwardXML struct {
	Mode string `xml:"mode,attr"`
}

type networkIPDefinitionXML struct {
	Address string                    `xml:"address,attr"`
	Prefix  int                       `xml:"prefix,attr"`
	DHCP    *networkDHCPDefinitionXML `xml:"dhcp,omitempty"`
}

type networkDHCPDefinitionXML struct {
	Range networkRangeXML `xml:"range"`
}

type networkRangeXML struct {
	Start string `xml:"start,attr"`
	End   string `xml:"end,attr"`
}

func (c *Client) ListNetworks(ctx context.Context) ([]domain.VirtualNetwork, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return nil, err
	}
	networks, err := c.conn.ListAllNetworks(libvirt.CONNECT_LIST_NETWORKS_ACTIVE | libvirt.CONNECT_LIST_NETWORKS_INACTIVE)
	if err != nil {
		return nil, fmt.Errorf("list libvirt networks: %w", err)
	}
	result := make([]domain.VirtualNetwork, 0, len(networks))
	for i := range networks {
		network := &networks[i]
		item, err := networkFromLibvirt(network)
		_ = network.Free()
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (c *Client) ListDHCPLeases(ctx context.Context, networkName string) ([]domain.DHCPLease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return nil, err
	}
	network, err := c.conn.LookupNetworkByName(networkName)
	if errors.Is(err, libvirt.ERR_NO_NETWORK) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lookup libvirt network %s: %w", networkName, err)
	}
	defer func() { _ = network.Free() }()
	active, err := network.IsActive()
	if err != nil {
		return nil, fmt.Errorf("read network %s state: %w", networkName, err)
	}
	if !active {
		return []domain.DHCPLease{}, nil
	}
	leases, err := network.GetDHCPLeases()
	if err != nil {
		return nil, fmt.Errorf("list DHCP leases for network %s: %w", networkName, err)
	}
	result := make([]domain.DHCPLease, 0, len(leases))
	for _, lease := range leases {
		result = append(result, dhcpLeaseFromLibvirt(lease))
	}
	return result, nil
}

func dhcpLeaseFromLibvirt(lease libvirt.NetworkDHCPLease) domain.DHCPLease {
	family := "unknown"
	switch lease.Type {
	case libvirt.IP_ADDR_TYPE_IPV4:
		family = "ipv4"
	case libvirt.IP_ADDR_TYPE_IPV6:
		family = "ipv6"
	}
	var expiresAt *time.Time
	if lease.ExpiryTime.Unix() > 0 {
		value := lease.ExpiryTime.UTC()
		expiresAt = &value
	}
	return domain.DHCPLease{
		Interface: lease.Iface, ExpiresAt: expiresAt, Family: family, MAC: lease.Mac,
		IPAddress: lease.IPaddr, Prefix: lease.Prefix, Hostname: lease.Hostname,
		ClientID: lease.Clientid, IAID: lease.Iaid,
	}
}

func (c *Client) CreateNetwork(ctx context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VirtualNetwork{}, err
	}
	if existing, err := c.conn.LookupNetworkByName(definition.Name); err == nil {
		_ = existing.Free()
		return domain.VirtualNetwork{}, domain.ErrConflict
	} else if !errors.Is(err, libvirt.ERR_NO_NETWORK) {
		return domain.VirtualNetwork{}, fmt.Errorf("lookup network before create: %w", err)
	}
	xmlText, err := networkXMLFromDefinition(definition)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	network, err := c.conn.NetworkDefineXML(xmlText)
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("define libvirt network: %w", err)
	}
	defer network.Free()
	rollback := func() {
		if definition.Start {
			_ = network.Destroy()
		}
		_ = network.Undefine()
	}
	if definition.Autostart {
		if err := network.SetAutostart(true); err != nil {
			rollback()
			return domain.VirtualNetwork{}, fmt.Errorf("set network autostart: %w", err)
		}
	}
	if definition.Start {
		if err := network.Create(); err != nil {
			rollback()
			return domain.VirtualNetwork{}, fmt.Errorf("start new network: %w", err)
		}
	}
	result, err := networkFromLibvirt(network)
	if err != nil {
		rollback()
		return domain.VirtualNetwork{}, err
	}
	return result, nil
}

func (c *Client) PerformNetworkAction(ctx context.Context, name string, action domain.VirtualNetworkAction) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	network, err := c.conn.LookupNetworkByName(name)
	if errors.Is(err, libvirt.ERR_NO_NETWORK) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lookup network: %w", err)
	}
	defer network.Free()
	active, err := network.IsActive()
	if err != nil {
		return fmt.Errorf("read network state: %w", err)
	}
	switch action {
	case domain.VirtualNetworkActionStart:
		if active {
			return domain.ErrConflict
		}
		if err := network.Create(); err != nil {
			return fmt.Errorf("start network: %w", err)
		}
	case domain.VirtualNetworkActionStop:
		if !active {
			return domain.ErrConflict
		}
		if err := network.Destroy(); err != nil {
			return fmt.Errorf("stop network: %w", err)
		}
	default:
		return domain.ErrInvalidInput
	}
	return nil
}

func (c *Client) DeleteNetwork(ctx context.Context, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	network, err := c.conn.LookupNetworkByName(name)
	if errors.Is(err, libvirt.ERR_NO_NETWORK) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lookup network: %w", err)
	}
	defer network.Free()
	active, err := network.IsActive()
	if err != nil {
		return fmt.Errorf("read network state: %w", err)
	}
	if active {
		return domain.ErrConflict
	}
	if err := network.Undefine(); err != nil {
		return fmt.Errorf("delete network: %w", err)
	}
	return nil
}

func networkXMLFromDefinition(definition domain.VirtualNetworkDefinition) (string, error) {
	item := networkDefinitionXML{
		Name: definition.Name,
		IP: networkIPDefinitionXML{
			Address: definition.IPv4Address,
			Prefix:  definition.Prefix,
		},
	}
	if definition.Mode == "nat" {
		item.Forward = &networkForwardXML{Mode: "nat"}
	}
	if definition.DHCPStart != "" && definition.DHCPEnd != "" {
		item.IP.DHCP = &networkDHCPDefinitionXML{Range: networkRangeXML{Start: definition.DHCPStart, End: definition.DHCPEnd}}
	}
	body, err := xml.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("encode network xml: %w", err)
	}
	return xml.Header + string(body), nil
}

func networkFromLibvirt(network *libvirt.Network) (domain.VirtualNetwork, error) {
	name, err := network.GetName()
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("read network name: %w", err)
	}
	uuid, _ := network.GetUUIDString()
	active, _ := network.IsActive()
	persistent, _ := network.IsPersistent()
	autostart, _ := network.GetAutostart()
	xmlText, err := network.GetXMLDesc(0)
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("read network %s xml: %w", name, err)
	}
	var parsed networkXML
	if err := xml.Unmarshal([]byte(xmlText), &parsed); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("decode network %s xml: %w", name, err)
	}
	addresses := make([]domain.NetworkAddress, 0, len(parsed.IPs))
	for _, item := range parsed.IPs {
		family := item.Family
		if family == "" {
			family = "ipv4"
		}
		prefix := item.Prefix
		if prefix == 0 && family == "ipv4" {
			prefix = prefixFromNetmask(item.Netmask)
		}
		address := domain.NetworkAddress{
			Family: family, Address: item.Address, Prefix: prefix,
		}
		if item.DHCP != nil {
			address.DHCPEnabled = true
			if item.DHCP.Range != nil {
				address.DHCPStart = item.DHCP.Range.Start
				address.DHCPEnd = item.DHCP.Range.End
			}
			for _, host := range item.DHCP.Hosts {
				address.Hosts = append(address.Hosts, domain.NetworkDHCPHost{MAC: host.MAC, IP: host.IP, Name: host.Name})
			}
		}
		addresses = append(addresses, address)
	}
	return domain.VirtualNetwork{
		Name: name, UUID: uuid, Bridge: parsed.Bridge.Name, Active: active,
		Persistent: persistent, Autostart: autostart, ForwardMode: parsed.Forward.Mode,
		Addresses: addresses,
	}, nil
}

func prefixFromNetmask(mask string) int {
	ip := net.ParseIP(mask)
	if ip == nil {
		return 0
	}
	ones, bits := net.IPMask(ip.To4()).Size()
	if bits == 0 {
		return 0
	}
	return ones
}
