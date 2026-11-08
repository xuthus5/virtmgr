package libvirtbackend

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
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
	if current.Forward.Mode != "" && current.Forward.Mode != "nat" && current.Forward.Mode != "isolated" {
		return domain.VirtualNetwork{}, domain.ErrNetworkUnsupported
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
	root, err := parseNetworkXMLTree(xmlText)
	if err != nil {
		return "", err
	}
	network := firstXMLChild(root, "network")
	if network == nil {
		return "", errors.New("network XML has no network element")
	}
	if err := setNetworkForward(network, definition.Mode); err != nil {
		return "", err
	}
	if err := setNetworkIPv4(network, definition); err != nil {
		return "", err
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

func setNetworkForward(network *networkXMLNode, mode string) error {
	forward := firstXMLChild(network, "forward")
	if mode == "isolated" {
		removeXMLChild(network, "forward")
		return nil
	}
	if mode != "nat" {
		return domain.ErrInvalidInput
	}
	if forward == nil {
		forward = &networkXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "forward"}}}
		insertXMLChildBefore(network, forward, "bridge", "mac", "domain", "dns", "ip")
	}
	setXMLAttribute(forward, "mode", "nat")
	return nil
}

func setNetworkIPv4(network *networkXMLNode, definition domain.VirtualNetworkDefinition) error {
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

func insertXMLChildBefore(parent, child *networkXMLNode, names ...string) {
	for index, current := range parent.Children {
		start, ok := current.Token.(xml.StartElement)
		if !ok {
			continue
		}
		for _, name := range names {
			if start.Name.Local == name {
				parent.Children = append(parent.Children, nil)
				copy(parent.Children[index+1:], parent.Children[index:])
				parent.Children[index] = child
				return
			}
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
