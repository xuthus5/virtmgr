package libvirtbackend

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) UpdateNetwork(ctx context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VirtualNetwork{}, err
	}
	network, err := c.conn.LookupNetworkByName(definition.Name)
	if errors.Is(err, libvirt.ERR_NO_NETWORK) {
		return domain.VirtualNetwork{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("lookup network before update: %w", err)
	}
	defer network.Free()
	active, err := network.IsActive()
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("read network state before update: %w", err)
	}
	if active {
		return domain.VirtualNetwork{}, domain.ErrNetworkActive
	}
	persistent, err := network.IsPersistent()
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("read network persistence before update: %w", err)
	}
	if !persistent {
		return domain.VirtualNetwork{}, domain.ErrNetworkUnsupported
	}
	originalXML, err := network.GetXMLDesc(libvirt.NETWORK_XML_INACTIVE)
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("read network definition before update: %w", err)
	}
	var current networkXML
	if err := xml.Unmarshal([]byte(originalXML), &current); err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("decode network definition before update: %w", err)
	}
	if current.Forward.Mode != "" && current.Forward.Mode != "nat" && current.Forward.Mode != "isolated" && current.Forward.Mode != "bridge" {
		return domain.VirtualNetwork{}, domain.ErrNetworkUnsupported
	}
	if definition.Mode == "bridge" {
		if err := c.validateHostBridgeLocked(definition.Bridge, definition.Name); err != nil {
			return domain.VirtualNetwork{}, err
		}
	}
	updatedXML, err := updateNetworkXML(originalXML, definition)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	updated, err := c.conn.NetworkDefineXML(updatedXML)
	if err != nil {
		return domain.VirtualNetwork{}, fmt.Errorf("update network definition: %w", err)
	}
	defer updated.Free()
	oldAutostart, _ := network.GetAutostart()
	if err := updated.SetAutostart(definition.Autostart); err != nil {
		if restored, restoreErr := c.conn.NetworkDefineXML(originalXML); restoreErr == nil {
			_ = restored.SetAutostart(oldAutostart)
			_ = restored.Free()
		}
		return domain.VirtualNetwork{}, fmt.Errorf("update network autostart: %w", err)
	}
	result, err := networkFromLibvirt(updated)
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	return result, nil
}

type networkXMLNode struct {
	Token    xml.Token
	Children []*networkXMLNode
}

func updateNetworkXML(xmlText string, definition domain.VirtualNetworkDefinition) (string, error) {
	var current networkXML
	if err := xml.Unmarshal([]byte(xmlText), &current); err != nil {
		return "", fmt.Errorf("decode current network definition: %w", err)
	}
	root, err := parseNetworkXMLTree(xmlText)
	if err != nil {
		return "", err
	}
	network := firstXMLChild(root, "network")
	if network == nil {
		return "", errors.New("network XML has no network element")
	}
	if err := setNetworkForward(network, definition); err != nil {
		return "", err
	}
	if err := setNetworkMTU(network, definition); err != nil {
		return "", err
	}
	if definition.Mode == "bridge" {
		setNetworkBridge(network, definition.Bridge)
		removeXMLChildren(network, "ip")
		removeXMLChild(network, "domain")
		removeXMLChild(network, "dns")
	} else {
		if current.Forward.Mode == "bridge" {
			removeXMLChild(network, "bridge")
		}
		if err := setNetworkIPv4(network, definition); err != nil {
			return "", err
		}
		setNetworkIPv6(network, definition)
		setNetworkDomainAndDNS(network, definition)
	}
	var buffer bytes.Buffer
	encoder := xml.NewEncoder(&buffer)
	encoder.Indent("", "  ")
	for _, child := range root.Children {
		if err := encodeNetworkXMLNode(encoder, child); err != nil {
			return "", err
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", fmt.Errorf("encode updated network XML: %w", err)
	}
	return xml.Header + buffer.String(), nil
}

func parseNetworkXMLTree(xmlText string) (*networkXMLNode, error) {
	decoder := xml.NewDecoder(strings.NewReader(xmlText))
	root := &networkXMLNode{}
	stack := []*networkXMLNode{root}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode network XML: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			child := &networkXMLNode{Token: value}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, child)
			stack = append(stack, child)
		case xml.EndElement:
			if len(stack) == 1 {
				return nil, errors.New("unexpected network XML end element")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if strings.TrimSpace(string(value)) == "" {
				continue
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, &networkXMLNode{Token: append(xml.CharData(nil), value...)})
		case xml.Comment:
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, &networkXMLNode{Token: append(xml.Comment(nil), value...)})
		}
	}
	if len(stack) != 1 {
		return nil, errors.New("unclosed network XML element")
	}
	return root, nil
}

func encodeNetworkXMLNode(encoder *xml.Encoder, node *networkXMLNode) error {
	start, ok := node.Token.(xml.StartElement)
	if !ok {
		if err := encoder.EncodeToken(node.Token); err != nil {
			return fmt.Errorf("encode network XML token: %w", err)
		}
		return nil
	}
	if err := encoder.EncodeToken(start); err != nil {
		return fmt.Errorf("encode network XML element: %w", err)
	}
	for _, child := range node.Children {
		if err := encodeNetworkXMLNode(encoder, child); err != nil {
			return err
		}
	}
	if err := encoder.EncodeToken(start.End()); err != nil {
		return fmt.Errorf("encode network XML end element: %w", err)
	}
	return nil
}

func setNetworkForward(network *networkXMLNode, definition domain.VirtualNetworkDefinition) error {
	mode := definition.Mode
	forward := firstXMLChild(network, "forward")
	if mode == "isolated" {
		removeXMLChild(network, "forward")
		return nil
	}
	if mode != "nat" && mode != "bridge" {
		return domain.ErrInvalidInput
	}
	if forward == nil {
		forward = &networkXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "forward"}}}
		insertXMLChildBefore(network, forward, "bridge", "mac", "domain", "dns", "ip")
	}
	setXMLAttribute(forward, "mode", mode)
	setNetworkNAT(forward, definition)
	return nil
}

func setNetworkNAT(forward *networkXMLNode, definition domain.VirtualNetworkDefinition) {
	nat := firstXMLChild(forward, "nat")
	enableIPv6 := definition.Mode == "nat" && definition.IPv6Address != "" && definition.NATIPv6
	if nat == nil {
		if !enableIPv6 {
			return
		}
		nat = &networkXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "nat"}}}
		forward.Children = append(forward.Children, nat)
	}
	if enableIPv6 {
		setXMLAttribute(nat, "ipv6", "yes")
	} else {
		removeXMLAttribute(nat, "ipv6")
	}
}

func setNetworkBridge(network *networkXMLNode, name string) {
	bridge := firstXMLChild(network, "bridge")
	if bridge == nil {
		bridge = &networkXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "bridge"}}}
		insertXMLChildBefore(network, bridge, "mac", "domain", "dns", "ip")
	}
	setXMLAttribute(bridge, "name", name)
}

func setNetworkMTU(network *networkXMLNode, definition domain.VirtualNetworkDefinition) error {
	if definition.Mode == "bridge" {
		if definition.MTUPresent {
			return fmt.Errorf("%w: bridge networks use the upstream MTU", domain.ErrInvalidInput)
		}
		removeXMLChild(network, "mtu")
		return nil
	}
	if !definition.MTUPresent {
		return nil
	}
	if definition.MTU == 0 {
		removeXMLChild(network, "mtu")
		return nil
	}
	mtu := firstXMLChild(network, "mtu")
	if mtu == nil {
		mtu = &networkXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "mtu"}}}
		insertXMLChildBefore(network, mtu, "mac", "domain", "dns", "ip")
	}
	setXMLAttribute(mtu, "size", strconv.Itoa(definition.MTU))
	return nil
}

func setNetworkIPv4(network *networkXMLNode, definition domain.VirtualNetworkDefinition) error {
	if definition.IPv4Address == "" {
		for _, child := range append([]*networkXMLNode(nil), network.Children...) {
			start, ok := child.Token.(xml.StartElement)
			if ok && start.Name.Local == "ip" {
				family := xmlAttributeValue(child, "family")
				if family == "" || family == "ipv4" {
					removeNetworkXMLChild(network, child)
				}
			}
		}
		return nil
	}
	var target *networkXMLNode
	for _, child := range network.Children {
		start, ok := child.Token.(xml.StartElement)
		if !ok || start.Name.Local != "ip" {
			continue
		}
		family := xmlAttributeValue(child, "family")
		if family == "" || family == "ipv4" {
			target = child
			break
		}
	}
	if target == nil {
		target = &networkXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "ip"}}}
		network.Children = append(network.Children, target)
	}
	setXMLAttribute(target, "address", definition.IPv4Address)
	setXMLAttribute(target, "prefix", strconv.Itoa(definition.Prefix))
	removeXMLAttribute(target, "netmask")
	setNetworkDHCP(target, definition.DHCPStart, definition.DHCPEnd)
	return nil
}

func setNetworkIPv6(network *networkXMLNode, definition domain.VirtualNetworkDefinition) {
	var target *networkXMLNode
	for _, child := range network.Children {
		start, ok := child.Token.(xml.StartElement)
		if !ok || start.Name.Local != "ip" {
			continue
		}
		if xmlAttributeValue(child, "family") == "ipv6" {
			target = child
			break
		}
	}
	if definition.IPv6Address == "" {
		for _, child := range network.Children {
			start, ok := child.Token.(xml.StartElement)
			if ok && start.Name.Local == "ip" && xmlAttributeValue(child, "family") == "ipv6" {
				removeNetworkXMLChild(network, child)
				return
			}
		}
		return
	}
	if target == nil {
		target = &networkXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "ip"}}}
		network.Children = append(network.Children, target)
	}
	setXMLAttribute(target, "family", "ipv6")
	setXMLAttribute(target, "address", definition.IPv6Address)
	setXMLAttribute(target, "prefix", strconv.Itoa(definition.IPv6Prefix))
	removeXMLAttribute(target, "netmask")
	if definition.IPv6Mode == "dhcpv6" && definition.IPv6DHCPStart != "" && definition.IPv6DHCPEnd != "" {
		setNetworkDHCP(target, definition.IPv6DHCPStart, definition.IPv6DHCPEnd)
	} else {
		removeXMLChild(target, "dhcp")
	}
}

func setNetworkDomainAndDNS(network *networkXMLNode, definition domain.VirtualNetworkDefinition) {
	if definition.DNSDomain != "" {
		domainNode := firstXMLChild(network, "domain")
		if domainNode == nil {
			domainNode = &networkXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "domain"}}}
			insertXMLChildBefore(network, domainNode, "forward", "bridge", "mac", "dns", "ip")
		}
		setXMLAttribute(domainNode, "name", definition.DNSDomain)
	} else {
		removeXMLChild(network, "domain")
	}
	dnsNode := firstXMLChild(network, "dns")
	if len(definition.DNSServers) == 0 {
		if dnsNode != nil {
			removeXMLChildren(dnsNode, "forwarder")
			removeNetworkDNSIfEmpty(network, dnsNode)
		}
		return
	}
	if dnsNode == nil {
		dnsNode = &networkXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "dns"}}}
		insertXMLChildBefore(network, dnsNode, "bridge", "mac", "ip")
	}
	removeXMLChildren(dnsNode, "forwarder")
	for _, server := range definition.DNSServers {
		forwarder := &networkXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "forwarder"}}}
		setXMLAttribute(forwarder, "addr", server)
		dnsNode.Children = append(dnsNode.Children, forwarder)
	}
}

func removeNetworkDNSIfEmpty(network, dnsNode *networkXMLNode) {
	for _, child := range dnsNode.Children {
		if _, ok := child.Token.(xml.StartElement); ok {
			return
		}
	}
	removeNetworkXMLChild(network, dnsNode)
}

func removeNetworkXMLChild(parent, target *networkXMLNode) {
	children := parent.Children[:0]
	for _, child := range parent.Children {
		if child != target {
			children = append(children, child)
		}
	}
	parent.Children = children
}

func setNetworkDHCP(ip *networkXMLNode, startAddress, endAddress string) {
	dhcp := firstXMLChild(ip, "dhcp")
	if dhcp == nil {
		dhcp = &networkXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "dhcp"}}}
		ip.Children = append(ip.Children, dhcp)
	}
	ipRange := firstXMLChild(dhcp, "range")
	if ipRange == nil {
		ipRange = &networkXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "range"}}}
		dhcp.Children = append([]*networkXMLNode{ipRange}, dhcp.Children...)
	}
	setXMLAttribute(ipRange, "start", startAddress)
	setXMLAttribute(ipRange, "end", endAddress)
}

func firstXMLChild(parent *networkXMLNode, name string) *networkXMLNode {
	for _, child := range parent.Children {
		if start, ok := child.Token.(xml.StartElement); ok && start.Name.Local == name {
			return child
		}
	}
	return nil
}

func removeXMLChild(parent *networkXMLNode, name string) {
	children := parent.Children[:0]
	for _, child := range parent.Children {
		if start, ok := child.Token.(xml.StartElement); ok && start.Name.Local == name {
			continue
		}
		children = append(children, child)
	}
	parent.Children = children
}

func removeXMLChildren(parent *networkXMLNode, name string) {
	children := parent.Children[:0]
	for _, child := range parent.Children {
		if start, ok := child.Token.(xml.StartElement); ok && start.Name.Local == name {
			continue
		}
		children = append(children, child)
	}
	parent.Children = children
}

func insertXMLChildBefore(parent, child *networkXMLNode, names ...string) {
	for index, current := range parent.Children {
		start, ok := current.Token.(xml.StartElement)
		if !ok {
			continue
		}
		if slices.Contains(names, start.Name.Local) {
			parent.Children = append(parent.Children, nil)
			copy(parent.Children[index+1:], parent.Children[index:])
			parent.Children[index] = child
			return
		}
	}
	parent.Children = append(parent.Children, child)
}

func xmlAttributeValue(node *networkXMLNode, name string) string {
	start, ok := node.Token.(xml.StartElement)
	if !ok {
		return ""
	}
	for _, attribute := range start.Attr {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

func setXMLAttribute(node *networkXMLNode, name, value string) {
	start, ok := node.Token.(xml.StartElement)
	if !ok {
		return
	}
	for index := range start.Attr {
		if start.Attr[index].Name.Local == name {
			start.Attr[index].Value = value
			node.Token = start
			return
		}
	}
	start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: name}, Value: value})
	node.Token = start
}

func removeXMLAttribute(node *networkXMLNode, name string) {
	start, ok := node.Token.(xml.StartElement)
	if !ok {
		return
	}
	attributes := start.Attr[:0]
	for _, attribute := range start.Attr {
		if attribute.Name.Local == name {
			continue
		}
		attributes = append(attributes, attribute)
	}
	start.Attr = attributes
	node.Token = start
}
