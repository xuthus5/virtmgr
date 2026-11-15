package domain

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

const qemuDomainNamespace = "http://libvirt.org/schemas/domain/qemu/1.0"

type VMXMLIdentity struct {
	Name string
	UUID string
}

type VMISOEjectResult struct {
	XML       string
	Ejected   bool
	Target    string
	Source    string
	EmptySlot bool
}

type vmXMLIdentityEnvelope struct {
	XMLName xml.Name
	Name    string `xml:"name"`
	UUID    string `xml:"uuid"`
}

func ParseVMXMLIdentity(text string) (VMXMLIdentity, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return VMXMLIdentity{}, fmt.Errorf("%w: domain XML is empty", ErrInvalidInput)
	}
	decoder := xml.NewDecoder(strings.NewReader(text))
	decoder.Strict = true
	var parsed vmXMLIdentityEnvelope
	if err := decoder.Decode(&parsed); err != nil {
		return VMXMLIdentity{}, fmt.Errorf("%w: invalid domain XML: %v", ErrInvalidInput, err)
	}
	if parsed.XMLName.Local != "domain" {
		return VMXMLIdentity{}, fmt.Errorf("%w: root element must be domain", ErrInvalidInput)
	}
	identity := VMXMLIdentity{Name: strings.TrimSpace(parsed.Name), UUID: strings.TrimSpace(parsed.UUID)}
	if identity.Name == "" || identity.UUID == "" {
		return VMXMLIdentity{}, fmt.Errorf("%w: domain name and uuid are required", ErrInvalidInput)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return VMXMLIdentity{}, fmt.Errorf("%w: multiple root elements are not allowed", ErrInvalidInput)
	}

	hookDecoder := xml.NewDecoder(strings.NewReader(text))
	hookDecoder.Strict = true
	for {
		token, err := hookDecoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return VMXMLIdentity{}, fmt.Errorf("%w: invalid domain XML: %v", ErrInvalidInput, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(start.Name.Space), qemuDomainNamespace) {
			return VMXMLIdentity{}, fmt.Errorf("%w: QEMU namespace overrides are not allowed", ErrInvalidInput)
		}
	}
	return identity, nil
}

// EjectVMISOXML removes an attached installer ISO from a persistent domain
// definition while preserving cloud-init media and making the primary disk
// the first boot device. It intentionally does not touch snapshots or storage.
func EjectVMISOXML(text string) (VMISOEjectResult, error) {
	root, err := parseVMXMLTree(text)
	if err != nil {
		return VMISOEjectResult{}, err
	}
	domainNode := firstVMXMLChild(root, "domain")
	if domainNode == nil {
		return VMISOEjectResult{}, fmt.Errorf("%w: domain XML has no domain element", ErrInvalidInput)
	}
	devices := firstVMXMLChild(domainNode, "devices")
	if devices == nil {
		return VMISOEjectResult{}, fmt.Errorf("%w: domain XML has no devices element", ErrInvalidInput)
	}

	result := VMISOEjectResult{}
	disks := make([]*vmXMLNode, 0)
	for _, child := range devices.Children {
		if vmXMLNodeName(child) == "disk" {
			disks = append(disks, child)
		}
	}
	remaining := make([]*vmXMLNode, 0, len(disks))
	for _, disk := range disks {
		source := vmXMLChildText(disk, "source", "file")
		if strings.EqualFold(vmXMLAttribute(disk, "device"), "cdrom") &&
			strings.HasSuffix(strings.ToLower(strings.TrimSpace(source)), ".iso") &&
			!strings.EqualFold(strings.TrimSpace(vmXMLChildText(disk, "serial", "")), "cloud-init") {
			result.Ejected = true
			result.Target = vmXMLChildText(disk, "target", "dev")
			result.Source = source
			continue
		}
		remaining = append(remaining, disk)
	}
	if !result.Ejected {
		return result, nil
	}
	devices.Children = removeVMXMLChildren(devices.Children, func(node *vmXMLNode) bool {
		for _, disk := range disks {
			if node == disk && !containsVMXMLNode(remaining, disk) {
				return true
			}
		}
		return false
	})

	var primary *vmXMLNode
	for _, disk := range remaining {
		device := strings.ToLower(strings.TrimSpace(vmXMLAttribute(disk, "device")))
		if device == "" || device == "disk" {
			if primary == nil || vmXMLChild(disk, "readonly") == nil {
				primary = disk
			}
			if vmXMLChild(disk, "readonly") == nil {
				break
			}
		}
	}
	if primary == nil {
		return VMISOEjectResult{}, fmt.Errorf("%w: no writable system disk remains after ejecting ISO", ErrInvalidInput)
	}
	for _, disk := range remaining {
		boot := vmXMLChild(disk, "boot")
		if boot == nil {
			continue
		}
		if disk == primary {
			setVMXMLAttribute(boot, "order", "1")
			continue
		}
		if vmXMLAttribute(boot, "order") == "1" {
			setVMXMLAttribute(boot, "order", "2")
		}
	}
	primaryBoot := vmXMLChild(primary, "boot")
	if primaryBoot == nil {
		primaryBoot = &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "boot"}, Attr: []xml.Attr{{Name: xml.Name{Local: "order"}, Value: "1"}}}}
		primary.Children = append(primary.Children, primaryBoot)
	}
	setVMXMLAttribute(primaryBoot, "order", "1")
	if osNode := firstVMXMLChild(domainNode, "os"); osNode != nil {
		boots := childrenByName(osNode.Children, "boot")
		if len(boots) > 0 {
			for _, boot := range boots {
				if strings.EqualFold(strings.TrimSpace(vmXMLAttribute(boot, "dev")), "cdrom") {
					osNode.Children = removeVMXMLNode(osNode.Children, boot)
				}
			}
			if firstVMXMLChild(osNode, "boot") == nil {
				osNode.Children = append([]*vmXMLNode{{Token: xml.StartElement{Name: xml.Name{Local: "boot"}, Attr: []xml.Attr{{Name: xml.Name{Local: "dev"}, Value: "hd"}}}}}, osNode.Children...)
			}
		}
	}

	var buffer bytes.Buffer
	encoder := xml.NewEncoder(&buffer)
	encoder.Indent("", "  ")
	for _, child := range root.Children {
		if err := encodeVMXMLNode(encoder, child); err != nil {
			return VMISOEjectResult{}, err
		}
	}
	if err := encoder.Flush(); err != nil {
		return VMISOEjectResult{}, fmt.Errorf("encode ejected ISO XML: %w", err)
	}
	result.XML = xml.Header + buffer.String()
	return result, nil
}

type vmXMLNode struct {
	Token    xml.Token
	Children []*vmXMLNode
}

func parseVMXMLTree(text string) (*vmXMLNode, error) {
	decoder := xml.NewDecoder(strings.NewReader(strings.TrimSpace(text)))
	root := &vmXMLNode{}
	stack := []*vmXMLNode{root}
	for {
		token, err := decoder.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: invalid domain XML: %v", ErrInvalidInput, err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			child := &vmXMLNode{Token: value}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, child)
			stack = append(stack, child)
		case xml.EndElement:
			if len(stack) == 1 {
				return nil, fmt.Errorf("%w: unexpected domain XML end element", ErrInvalidInput)
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if strings.TrimSpace(string(value)) == "" {
				continue
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, &vmXMLNode{Token: append(xml.CharData(nil), value...)})
		case xml.Comment, xml.Directive:
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, &vmXMLNode{Token: token})
		}
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("%w: unclosed domain XML element", ErrInvalidInput)
	}
	return root, nil
}

func encodeVMXMLNode(encoder *xml.Encoder, node *vmXMLNode) error {
	start, ok := node.Token.(xml.StartElement)
	if !ok {
		if err := encoder.EncodeToken(node.Token); err != nil {
			return fmt.Errorf("encode domain XML token: %w", err)
		}
		return nil
	}
	if err := encoder.EncodeToken(start); err != nil {
		return fmt.Errorf("encode domain XML element: %w", err)
	}
	for _, child := range node.Children {
		if err := encodeVMXMLNode(encoder, child); err != nil {
			return err
		}
	}
	if err := encoder.EncodeToken(start.End()); err != nil {
		return fmt.Errorf("encode domain XML end element: %w", err)
	}
	return nil
}

func vmXMLNodeName(node *vmXMLNode) string {
	start, ok := node.Token.(xml.StartElement)
	if !ok {
		return ""
	}
	return start.Name.Local
}

func firstVMXMLChild(node *vmXMLNode, name string) *vmXMLNode {
	if node == nil {
		return nil
	}
	for _, child := range node.Children {
		if vmXMLNodeName(child) == name {
			return child
		}
	}
	return nil
}

func vmXMLChild(node *vmXMLNode, name string) *vmXMLNode {
	return firstVMXMLChild(node, name)
}

func vmXMLChildText(node *vmXMLNode, childName, attribute string) string {
	child := firstVMXMLChild(node, childName)
	if child == nil {
		return ""
	}
	if attribute != "" {
		return vmXMLAttribute(child, attribute)
	}
	for _, childNode := range child.Children {
		if value, ok := childNode.Token.(xml.CharData); ok {
			return strings.TrimSpace(string(value))
		}
	}
	return ""
}

func vmXMLAttribute(node *vmXMLNode, name string) string {
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

func setVMXMLAttribute(node *vmXMLNode, name, value string) {
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

func childrenByName(children []*vmXMLNode, name string) []*vmXMLNode {
	result := make([]*vmXMLNode, 0)
	for _, child := range children {
		if vmXMLNodeName(child) == name {
			result = append(result, child)
		}
	}
	return result
}

func containsVMXMLNode(nodes []*vmXMLNode, target *vmXMLNode) bool {
	for _, node := range nodes {
		if node == target {
			return true
		}
	}
	return false
}

func removeVMXMLChildren(children []*vmXMLNode, remove func(*vmXMLNode) bool) []*vmXMLNode {
	result := children[:0]
	for _, child := range children {
		if !remove(child) {
			result = append(result, child)
		}
	}
	return result
}

func removeVMXMLNode(children []*vmXMLNode, target *vmXMLNode) []*vmXMLNode {
	return removeVMXMLChildren(children, func(node *vmXMLNode) bool { return node == target })
}
