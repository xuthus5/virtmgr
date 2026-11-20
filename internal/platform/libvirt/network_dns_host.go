package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) AddNetworkDNSHost(ctx context.Context, name string, host domain.NetworkDNSHost) (domain.VirtualNetwork, error) {
	return c.updateNetworkDNSHost(ctx, name, host, libvirt.NETWORK_UPDATE_COMMAND_ADD_LAST)
}

func (c *Client) DeleteNetworkDNSHost(ctx context.Context, name string, host domain.NetworkDNSHost) (domain.VirtualNetwork, error) {
	return c.updateNetworkDNSHost(ctx, name, host, libvirt.NETWORK_UPDATE_COMMAND_DELETE)
}

func (c *Client) updateNetworkDNSHost(ctx context.Context, name string, host domain.NetworkDNSHost, command libvirt.NetworkUpdateCommand) (domain.VirtualNetwork, error) {
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
		return domain.VirtualNetwork{}, fmt.Errorf("lookup network before DNS host update: %w", err)
	}
	defer network.Free()

	active, err := network.IsActive()
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("read network state before DNS host update: %w", err)
	}
	persistent, err := network.IsPersistent()
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("read network persistence before DNS host update: %w", err)
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
		return domain.VirtualNetwork{}, fmt.Errorf("read network XML before DNS host update: %w", err)
	}
	parentIndex := 0
	if command == libvirt.NETWORK_UPDATE_COMMAND_DELETE {
		parentIndex, err = networkDNSHostIndex(xmlText, host.IP)
		if err != nil {
			return domain.VirtualNetwork{}, err
		}
	}
	hostXML, err := networkDNSHostXMLText(host)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	if err := network.Update(command, libvirt.NETWORK_SECTION_DNS_HOST, parentIndex, hostXML, updateFlags); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("update network DNS host: %w", err)
	}
	result, err := networkFromLibvirt(network)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	return result, nil
}

func networkDNSHostXMLText(host domain.NetworkDNSHost) (string, error) {
	item := networkDNSHostXML{IP: host.IP, Hostnames: append([]string(nil), host.Hostnames...)}
	body, err := xml.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("encode network DNS host XML: %w", err)
	}
	return string(body), nil
}

func networkDNSHostIndex(xmlText, address string) (int, error) {
	root, err := parseNetworkXMLTree(xmlText)
	if err != nil {
		return 0, err
	}
	network := firstXMLChild(root, "network")
	if network == nil {
		return 0, errors.New("network XML has no network element")
	}
	dns := firstXMLChild(network, "dns")
	if dns == nil {
		return 0, domain.ErrNotFound
	}
	index := 0
	for _, child := range dns.Children {
		start, ok := child.Token.(xml.StartElement)
		if !ok || start.Name.Local != "host" {
			continue
		}
		if strings.TrimSpace(xmlAttributeValue(child, "ip")) == strings.TrimSpace(address) {
			return index, nil
		}
		index++
	}
	return 0, domain.ErrNotFound
}
