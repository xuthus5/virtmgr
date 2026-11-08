package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) AddNetworkDHCPHost(ctx context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error) {
	return c.updateNetworkDHCPHost(ctx, name, host, libvirt.NETWORK_UPDATE_COMMAND_ADD_LAST)
}

func (c *Client) DeleteNetworkDHCPHost(ctx context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error) {
	return c.updateNetworkDHCPHost(ctx, name, host, libvirt.NETWORK_UPDATE_COMMAND_DELETE)
}

func (c *Client) updateNetworkDHCPHost(ctx context.Context, name string, host domain.NetworkDHCPHost, command libvirt.NetworkUpdateCommand) (domain.VirtualNetwork, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VirtualNetwork{}, err
	}
	network, err := c.conn.LookupNetworkByName(name)
	if errors.Is(err, libvirt.ERR_NO_NETWORK) {
		return domain.VirtualNetwork{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("lookup network before DHCP host update: %w", err)
	}
	defer network.Free()

	active, err := network.IsActive()
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("read network state before DHCP host update: %w", err)
	}
	persistent, err := network.IsPersistent()
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("read network persistence before DHCP host update: %w", err)
	}
	if !active && !persistent {
		return domain.VirtualNetwork{}, domain.ErrNetworkUnsupported
	}

	xmlFlags := libvirt.NETWORK_XML_INACTIVE
	updateFlags := libvirt.NETWORK_UPDATE_AFFECT_CONFIG
	if active {
		xmlFlags = 0
		updateFlags = libvirt.NETWORK_UPDATE_AFFECT_LIVE
		if persistent {
			updateFlags |= libvirt.NETWORK_UPDATE_AFFECT_CONFIG
		}
	}
	xmlText, err := network.GetXMLDesc(xmlFlags)
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("read network XML before DHCP host update: %w", err)
	}
	parentIndex, err := networkDHCPHostParentIndex(xmlText)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	hostXML, err := networkDHCPHostXMLText(host)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	if err := network.Update(command, libvirt.NETWORK_SECTION_IP_DHCP_HOST, parentIndex, hostXML, updateFlags); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("update network DHCP host: %w", err)
	}
	result, err := networkFromLibvirt(network)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	return result, nil
}

func networkDHCPHostXMLText(host domain.NetworkDHCPHost) (string, error) {
	body, err := xml.Marshal(networkDHCPHostXML{MAC: host.MAC, IP: host.IP, Name: host.Name})
	if err != nil {
		return "", fmt.Errorf("encode network DHCP host XML: %w", err)
	}
	return string(body), nil
}

func networkDHCPHostParentIndex(xmlText string) (int, error) {
	root, err := parseNetworkXMLTree(xmlText)
	if err != nil {
		return 0, err
	}
	network := firstXMLChild(root, "network")
	if network == nil {
		return 0, errors.New("network XML has no network element")
	}
	index := 0
	for _, child := range network.Children {
		start, ok := child.Token.(xml.StartElement)
		if !ok || start.Name.Local != "ip" {
			continue
		}
		family := xmlAttributeValue(child, "family")
		if (family == "" || family == "ipv4") && firstXMLChild(child, "dhcp") != nil {
			return index, nil
		}
		index++
	}
	return 0, domain.ErrNetworkUnsupported
}
