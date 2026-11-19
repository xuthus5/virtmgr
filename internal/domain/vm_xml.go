package domain

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"slices"
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

type VMISOAttachResult struct {
	XML      string
	Target   string
	Source   string
	Replaced bool
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

// AttachVMISOXML attaches or replaces the installer ISO in a persistent
// domain definition. The cloud-init CD-ROM is always preserved, and the
// installer disc becomes the first boot device.
func AttachVMISOXML(text, isoPath string) (VMISOAttachResult, error) {
	isoPath = strings.TrimSpace(isoPath)
	if isoPath == "" || !strings.HasPrefix(isoPath, "/") || strings.ContainsAny(isoPath, "'\r\n\x00") ||
		!strings.HasSuffix(strings.ToLower(isoPath), ".iso") {
		return VMISOAttachResult{}, fmt.Errorf("%w: installer ISO path must be an absolute .iso path", ErrInvalidInput)
	}
	root, err := parseVMXMLTree(text)
	if err != nil {
		return VMISOAttachResult{}, err
	}
	domainNode := firstVMXMLChild(root, "domain")
	if domainNode == nil {
		return VMISOAttachResult{}, fmt.Errorf("%w: domain XML has no domain element", ErrInvalidInput)
	}
	devices := firstVMXMLChild(domainNode, "devices")
	if devices == nil {
		return VMISOAttachResult{}, fmt.Errorf("%w: domain XML has no devices element", ErrInvalidInput)
	}

	var installer *vmXMLNode
	var primary *vmXMLNode
	targets := make(map[string]bool)
	for _, child := range devices.Children {
		if vmXMLNodeName(child) != "disk" {
			continue
		}
		target := strings.TrimSpace(vmXMLChildText(child, "target", "dev"))
		if target != "" {
			targets[target] = true
		}
		device := strings.ToLower(strings.TrimSpace(vmXMLAttribute(child, "device")))
		cloudInit := strings.EqualFold(strings.TrimSpace(vmXMLChildText(child, "serial", "")), "cloud-init")
		source := strings.TrimSpace(vmXMLChildText(child, "source", "file"))
		if device == "cdrom" && !cloudInit {
			if installer == nil || strings.HasSuffix(strings.ToLower(source), ".iso") {
				installer = child
			}
		}
		if (device == "" || device == "disk") && vmXMLChild(child, "readonly") == nil &&
			(primary == nil || source != "") {
			primary = child
		}
	}
	if primary == nil {
		return VMISOAttachResult{}, fmt.Errorf("%w: no writable system disk is available for ISO boot", ErrInvalidInput)
	}

	result := VMISOAttachResult{Source: isoPath}
	if installer != nil {
		result.Replaced = strings.TrimSpace(vmXMLChildText(installer, "source", "file")) != ""
		setVMXMLAttribute(installer, "device", "cdrom")
		setOrReplaceVMXMLChildAttribute(installer, "source", "file", isoPath)
		ensureVMXMLReadOnly(installer)
		result.Target = vmXMLChildText(installer, "target", "dev")
		if result.Target == "" {
			result.Target = nextVMXMLCDROMTarget(targets)
			setOrReplaceVMXMLChildAttribute(installer, "target", "dev", result.Target)
			setVMXMLChildAttribute(installer, "target", "bus", "sata")
		}
	} else {
		result.Target = nextVMXMLCDROMTarget(targets)
		installer = &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "disk"}, Attr: []xml.Attr{
			{Name: xml.Name{Local: "type"}, Value: "file"},
			{Name: xml.Name{Local: "device"}, Value: "cdrom"},
		}}}
		installer.Children = []*vmXMLNode{
			{Token: xml.StartElement{Name: xml.Name{Local: "driver"}, Attr: []xml.Attr{
				{Name: xml.Name{Local: "name"}, Value: "qemu"},
				{Name: xml.Name{Local: "type"}, Value: "raw"},
			}}},
			{Token: xml.StartElement{Name: xml.Name{Local: "source"}, Attr: []xml.Attr{{Name: xml.Name{Local: "file"}, Value: isoPath}}}},
			{Token: xml.StartElement{Name: xml.Name{Local: "target"}, Attr: []xml.Attr{
				{Name: xml.Name{Local: "dev"}, Value: result.Target},
				{Name: xml.Name{Local: "bus"}, Value: "sata"},
			}}},
			{Token: xml.StartElement{Name: xml.Name{Local: "readonly"}}},
		}
		devices.Children = append(devices.Children, installer)
	}
	setOrReplaceVMXMLChildAttribute(installer, "boot", "order", "1")
	primaryBoot := vmXMLChild(primary, "boot")
	if primaryBoot == nil || vmXMLAttribute(primaryBoot, "order") == "" || vmXMLAttribute(primaryBoot, "order") == "1" {
		setOrReplaceVMXMLChildAttribute(primary, "boot", "order", "2")
	}
	if osNode := firstVMXMLChild(domainNode, "os"); osNode != nil {
		for _, boot := range childrenByName(osNode.Children, "boot") {
			if strings.EqualFold(strings.TrimSpace(vmXMLAttribute(boot, "dev")), "cdrom") {
				osNode.Children = removeVMXMLNode(osNode.Children, boot)
			}
		}
		osNode.Children = append([]*vmXMLNode{
			{Token: xml.StartElement{Name: xml.Name{Local: "boot"}, Attr: []xml.Attr{{Name: xml.Name{Local: "dev"}, Value: "cdrom"}}}},
			{Token: xml.StartElement{Name: xml.Name{Local: "boot"}, Attr: []xml.Attr{{Name: xml.Name{Local: "dev"}, Value: "hd"}}}},
		}, osNode.Children...)
	}

	var buffer bytes.Buffer
	encoder := xml.NewEncoder(&buffer)
	encoder.Indent("", "  ")
	for _, child := range root.Children {
		if err := encodeVMXMLNode(encoder, child); err != nil {
			return VMISOAttachResult{}, err
		}
	}
	if err := encoder.Flush(); err != nil {
		return VMISOAttachResult{}, fmt.Errorf("encode attached ISO XML: %w", err)
	}
	result.XML = xml.Header + buffer.String()
	return result, nil
}

func nextVMXMLCDROMTarget(used map[string]bool) string {
	for index := range 26 {
		target := fmt.Sprintf("sd%c", 'a'+index)
		if !used[target] {
			return target
		}
	}
	return "sdz"
}

func setOrReplaceVMXMLChildAttribute(parent *vmXMLNode, childName, attribute, value string) {
	child := firstVMXMLChild(parent, childName)
	if child == nil {
		child = &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: childName}}}
		parent.Children = append(parent.Children, child)
	}
	setVMXMLAttribute(child, attribute, value)
}

func setVMXMLChildAttribute(parent *vmXMLNode, childName, attribute, value string) {
	setOrReplaceVMXMLChildAttribute(parent, childName, attribute, value)
}

func ensureVMXMLReadOnly(disk *vmXMLNode) {
	if vmXMLChild(disk, "readonly") == nil {
		disk.Children = append(disk.Children, &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "readonly"}}})
	}
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

func removeVMXMLAttribute(node *vmXMLNode, name string) {
	start, ok := node.Token.(xml.StartElement)
	if !ok {
		return
	}
	attributes := start.Attr[:0]
	for _, attribute := range start.Attr {
		if attribute.Name.Local != name {
			attributes = append(attributes, attribute)
		}
	}
	start.Attr = attributes
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
	return slices.Contains(nodes, target)
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

type VMNetworkQoSUpdate struct {
	XML       string
	DeviceXML string
}

func SetVMXMLBootDevices(text string, devices []VMBootDevice) (string, error) {
	normalized := make([]string, 0, len(devices))
	for _, device := range devices {
		normalized = append(normalized, string(device))
	}
	parsedDevices, ok := NormalizeVMBootDevices(normalized)
	if !ok {
		return "", fmt.Errorf("%w: boot devices must be a unique subset of hd, cdrom and network", ErrInvalidInput)
	}
	root, err := parseVMXMLTree(text)
	if err != nil {
		return "", err
	}
	domainNode := firstVMXMLChild(root, "domain")
	if domainNode == nil {
		return "", fmt.Errorf("%w: domain XML has no domain element", ErrInvalidInput)
	}
	osNode := firstVMXMLChild(domainNode, "os")
	if osNode == nil {
		osNode = &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "os"}}}
		domainNode.Children = append([]*vmXMLNode{osNode}, domainNode.Children...)
	}
	osNode.Children = removeVMXMLChildren(osNode.Children, func(node *vmXMLNode) bool {
		return vmXMLNodeName(node) == "boot"
	})
	bootNodes := make([]*vmXMLNode, 0, len(parsedDevices))
	for _, device := range parsedDevices {
		bootNodes = append(bootNodes, &vmXMLNode{Token: xml.StartElement{
			Name: xml.Name{Local: "boot"}, Attr: []xml.Attr{{Name: xml.Name{Local: "dev"}, Value: string(device)}},
		}})
	}
	osNode.Children = append(bootNodes, osNode.Children...)
	devicesNode := firstVMXMLChild(domainNode, "devices")
	if devicesNode != nil {
		for _, device := range devicesNode.Children {
			switch vmXMLNodeName(device) {
			case "disk", "interface", "hostdev":
				for _, boot := range childrenByName(device.Children, "boot") {
					device.Children = removeVMXMLNode(device.Children, boot)
				}
			}
		}
	}
	return encodeVMXMLDocument(root)
}

func SetVMXMLCPUConfig(text string, spec VMCPUConfigSpec) (string, error) {
	spec.Model = strings.TrimSpace(spec.Model)
	if spec.Model == "" || len(spec.Model) > 64 || spec.Sockets == 0 || spec.Cores == 0 || spec.Threads == 0 {
		return "", fmt.Errorf("%w: invalid CPU model or topology", ErrInvalidInput)
	}
	root, err := parseVMXMLTree(text)
	if err != nil {
		return "", err
	}
	domainNode := firstVMXMLChild(root, "domain")
	if domainNode == nil {
		return "", fmt.Errorf("%w: domain XML has no domain element", ErrInvalidInput)
	}
	cpuNode := firstVMXMLChild(domainNode, "cpu")
	if cpuNode == nil {
		cpuNode = &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "cpu"}}}
		insertAt := len(domainNode.Children)
		for index, child := range domainNode.Children {
			if vmXMLNodeName(child) == "os" {
				insertAt = index + 1
				break
			}
		}
		domainNode.Children = append(domainNode.Children, nil)
		copy(domainNode.Children[insertAt+1:], domainNode.Children[insertAt:])
		domainNode.Children[insertAt] = cpuNode
	}
	switch spec.Model {
	case "host-passthrough":
		setVMXMLAttribute(cpuNode, "mode", "host-passthrough")
		setVMXMLAttribute(cpuNode, "check", "none")
		removeVMXMLAttribute(cpuNode, "match")
		cpuNode.Children = removeVMXMLChildren(cpuNode.Children, func(node *vmXMLNode) bool { return vmXMLNodeName(node) == "model" })
	case "host-model":
		setVMXMLAttribute(cpuNode, "mode", "host-model")
		setVMXMLAttribute(cpuNode, "check", "partial")
		removeVMXMLAttribute(cpuNode, "match")
		cpuNode.Children = removeVMXMLChildren(cpuNode.Children, func(node *vmXMLNode) bool { return vmXMLNodeName(node) == "model" })
	default:
		setVMXMLAttribute(cpuNode, "mode", "custom")
		setVMXMLAttribute(cpuNode, "match", "exact")
		removeVMXMLAttribute(cpuNode, "check")
		modelNode := firstVMXMLChild(cpuNode, "model")
		if modelNode == nil {
			modelNode = &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "model"}}}
			cpuNode.Children = append([]*vmXMLNode{modelNode}, cpuNode.Children...)
		}
		modelNode.Children = []*vmXMLNode{{Token: xml.CharData(spec.Model)}}
		setVMXMLAttribute(modelNode, "fallback", "allow")
	}
	setOrReplaceVMXMLChildAttribute(cpuNode, "topology", "sockets", fmt.Sprintf("%d", spec.Sockets))
	setOrReplaceVMXMLChildAttribute(cpuNode, "topology", "cores", fmt.Sprintf("%d", spec.Cores))
	setOrReplaceVMXMLChildAttribute(cpuNode, "topology", "threads", fmt.Sprintf("%d", spec.Threads))
	return encodeVMXMLDocument(root)
}

func SetVMNetworkQoSXML(text, mac string, spec VMNetworkQoSSpec) (VMNetworkQoSUpdate, error) {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return VMNetworkQoSUpdate{}, fmt.Errorf("%w: network interface MAC is required", ErrInvalidInput)
	}
	root, err := parseVMXMLTree(text)
	if err != nil {
		return VMNetworkQoSUpdate{}, err
	}
	domainNode := firstVMXMLChild(root, "domain")
	devicesNode := firstVMXMLChild(domainNode, "devices")
	if domainNode == nil || devicesNode == nil {
		return VMNetworkQoSUpdate{}, fmt.Errorf("%w: domain XML has no devices element", ErrInvalidInput)
	}
	var target *vmXMLNode
	for _, device := range childrenByName(devicesNode.Children, "interface") {
		if strings.EqualFold(vmXMLChildText(device, "mac", "address"), mac) {
			target = device
			break
		}
	}
	if target == nil {
		return VMNetworkQoSUpdate{}, fmt.Errorf("%w: network interface %s was not found", ErrNotFound, mac)
	}
	bandwidth := firstVMXMLChild(target, "bandwidth")
	if spec == (VMNetworkQoSSpec{}) {
		if bandwidth != nil {
			target.Children = removeVMXMLNode(target.Children, bandwidth)
		}
	} else {
		if bandwidth == nil {
			bandwidth = &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "bandwidth"}}}
			target.Children = append(target.Children, bandwidth)
		}
		bandwidth.Children = removeVMXMLChildren(bandwidth.Children, func(node *vmXMLNode) bool {
			return vmXMLNodeName(node) == "inbound" || vmXMLNodeName(node) == "outbound"
		})
		if spec.InboundAverageBytes > 0 || spec.InboundPeakBytes > 0 || spec.InboundBurstBytes > 0 {
			bandwidth.Children = append(bandwidth.Children, qosBandwidthNode("inbound", spec.InboundAverageBytes, spec.InboundPeakBytes, spec.InboundBurstBytes))
		}
		if spec.OutboundAverageBytes > 0 || spec.OutboundPeakBytes > 0 || spec.OutboundBurstBytes > 0 {
			bandwidth.Children = append(bandwidth.Children, qosBandwidthNode("outbound", spec.OutboundAverageBytes, spec.OutboundPeakBytes, spec.OutboundBurstBytes))
		}
		if len(bandwidth.Children) == 0 {
			target.Children = removeVMXMLNode(target.Children, bandwidth)
		}
	}
	documentXML, err := encodeVMXMLDocument(root)
	if err != nil {
		return VMNetworkQoSUpdate{}, err
	}
	deviceXML, err := encodeVMXMLSubtree(target)
	if err != nil {
		return VMNetworkQoSUpdate{}, err
	}
	return VMNetworkQoSUpdate{XML: documentXML, DeviceXML: deviceXML}, nil
}

func SetVMNetworkModelXML(text, mac string, model VMNICModel) (string, error) {
	mac = strings.TrimSpace(mac)
	normalized, ok := NormalizeVMNICModel(string(model))
	if mac == "" || !ok {
		return "", fmt.Errorf("%w: network interface MAC and model are required", ErrInvalidInput)
	}
	root, err := parseVMXMLTree(text)
	if err != nil {
		return "", err
	}
	domainNode := firstVMXMLChild(root, "domain")
	devicesNode := firstVMXMLChild(domainNode, "devices")
	if domainNode == nil || devicesNode == nil {
		return "", fmt.Errorf("%w: domain XML has no devices element", ErrInvalidInput)
	}
	var target *vmXMLNode
	for _, device := range childrenByName(devicesNode.Children, "interface") {
		if strings.EqualFold(vmXMLChildText(device, "mac", "address"), mac) {
			target = device
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("%w: network interface %s was not found", ErrNotFound, mac)
	}
	setOrReplaceVMXMLChildAttribute(target, "model", "type", string(normalized))
	return encodeVMXMLDocument(root)
}

func qosBandwidthNode(name string, average, peak, burst uint64) *vmXMLNode {
	attributes := make([]xml.Attr, 0, 3)
	if average > 0 {
		attributes = append(attributes, xml.Attr{Name: xml.Name{Local: "average"}, Value: fmt.Sprintf("%d", average)})
	}
	if peak > 0 {
		attributes = append(attributes, xml.Attr{Name: xml.Name{Local: "peak"}, Value: fmt.Sprintf("%d", peak)})
	}
	if burst > 0 {
		attributes = append(attributes, xml.Attr{Name: xml.Name{Local: "burst"}, Value: fmt.Sprintf("%d", burst)})
	}
	return &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: name}, Attr: attributes}}
}

func encodeVMXMLDocument(root *vmXMLNode) (string, error) {
	var buffer bytes.Buffer
	encoder := xml.NewEncoder(&buffer)
	encoder.Indent("", "  ")
	for _, child := range root.Children {
		if err := encodeVMXMLNode(encoder, child); err != nil {
			return "", err
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", fmt.Errorf("encode domain XML: %w", err)
	}
	return xml.Header + buffer.String(), nil
}

func encodeVMXMLSubtree(node *vmXMLNode) (string, error) {
	var buffer bytes.Buffer
	encoder := xml.NewEncoder(&buffer)
	encoder.Indent("", "  ")
	if err := encodeVMXMLNode(encoder, node); err != nil {
		return "", err
	}
	if err := encoder.Flush(); err != nil {
		return "", fmt.Errorf("encode domain device XML: %w", err)
	}
	return buffer.String(), nil
}
