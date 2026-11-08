package libvirtbackend

import (
	"context"
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"virtmgr/internal/domain"
)

var hostDeviceCapabilities = []string{"pci", "usb", "usb_device", "net", "storage", "drm"}

type nodeDeviceXML struct {
	Name         string                 `xml:"name"`
	Parent       string                 `xml:"parent"`
	Capabilities []nodeDeviceCapability `xml:"capability"`
}

type nodeDeviceCapability struct {
	Type      string         `xml:"type,attr"`
	Domain    string         `xml:"domain"`
	Bus       string         `xml:"bus"`
	Slot      string         `xml:"slot"`
	Function  string         `xml:"function"`
	Device    string         `xml:"device"`
	Vendor    nodeDeviceID   `xml:"vendor"`
	Product   nodeDeviceID   `xml:"product"`
	Driver    nodeDeviceName `xml:"driver"`
	Interface string         `xml:"interface"`
	Block     string         `xml:"block"`
}

type nodeDeviceID struct {
	ID   string `xml:"id,attr"`
	Name string `xml:",chardata"`
}

type nodeDeviceName struct {
	Name string `xml:"name"`
}

func (c *Client) ListHostDevices(ctx context.Context) ([]domain.HostDevice, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return nil, err
	}

	names := make(map[string]struct{})
	var lastErr error
	for _, capability := range hostDeviceCapabilities {
		items, err := c.conn.ListDevices(capability, 0)
		if err != nil {
			lastErr = err
			continue
		}
		for _, name := range items {
			names[name] = struct{}{}
		}
	}

	result := make([]domain.HostDevice, 0, len(names))
	for name := range names {
		device, err := c.conn.LookupDeviceByName(name)
		if err != nil {
			lastErr = err
			continue
		}
		xmlDescription, xmlErr := device.GetXMLDesc(0)
		capabilities, _ := device.ListCaps()
		active, _ := device.IsActive()
		persistent, _ := device.IsPersistent()
		_ = device.Free()
		if xmlErr != nil {
			lastErr = xmlErr
			continue
		}
		parsed, parseErr := parseHostDeviceXML(xmlDescription)
		if parseErr != nil {
			lastErr = parseErr
			continue
		}
		if parsed.Name == "" {
			parsed.Name = name
		}
		parsed.Active = active
		parsed.Persistent = persistent
		if len(capabilities) > 0 {
			parsed.Capabilities = capabilities
		} else if parsed.Type != "" {
			parsed.Capabilities = []string{parsed.Type}
		}
		result = append(result, parsed)
	}
	if len(result) == 0 && lastErr != nil {
		return nil, fmt.Errorf("list host devices: %w", lastErr)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			return result[i].Type < result[j].Type
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func parseHostDeviceXML(raw string) (domain.HostDevice, error) {
	var parsed nodeDeviceXML
	if err := xml.Unmarshal([]byte(raw), &parsed); err != nil {
		return domain.HostDevice{}, fmt.Errorf("parse node device xml: %w", err)
	}
	device := domain.HostDevice{Name: strings.TrimSpace(parsed.Name), Parent: strings.TrimSpace(parsed.Parent)}
	if len(parsed.Capabilities) == 0 {
		return device, nil
	}
	capability := parsed.Capabilities[0]
	device.Type = strings.TrimSpace(capability.Type)
	device.Vendor = strings.TrimSpace(capability.Vendor.Name)
	device.VendorID = strings.TrimSpace(capability.Vendor.ID)
	device.Product = strings.TrimSpace(capability.Product.Name)
	device.ProductID = strings.TrimSpace(capability.Product.ID)
	device.Driver = strings.TrimSpace(capability.Driver.Name)
	device.Address = nodeDeviceAddress(capability)
	return device, nil
}

func nodeDeviceAddress(capability nodeDeviceCapability) string {
	switch capability.Type {
	case "pci":
		domain := nodeHex(capability.Domain, 4)
		bus := nodeHex(capability.Bus, 2)
		slot := nodeHex(capability.Slot, 2)
		function := nodeHex(capability.Function, 1)
		if domain == "" || bus == "" || slot == "" || function == "" {
			return ""
		}
		return domain + ":" + bus + ":" + slot + "." + function
	case "usb_device":
		bus := strings.TrimSpace(capability.Bus)
		device := strings.TrimSpace(capability.Device)
		if bus == "" && device == "" {
			return ""
		}
		return "bus " + bus + " · device " + device
	case "net":
		return strings.TrimSpace(capability.Interface)
	case "storage":
		return strings.TrimSpace(capability.Block)
	default:
		return ""
	}
}

func nodeHex(value string, width int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	normalized := strings.TrimPrefix(strings.ToLower(value), "0x")
	parsed, err := strconv.ParseUint(normalized, 16, 32)
	if err != nil {
		return value
	}
	return fmt.Sprintf("%0*x", width, parsed)
}
