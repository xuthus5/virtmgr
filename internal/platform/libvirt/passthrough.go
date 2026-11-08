package libvirtbackend

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

var (
	pciAddressPattern = regexp.MustCompile(`^([0-9a-fA-F]{4}):([0-9a-fA-F]{2}):([0-9a-fA-F]{2})\.([0-7])$`)
	usbAddressPattern = regexp.MustCompile(`bus\s+([0-9]+)\s*[^\d]+\s*device\s+([0-9]+)`)
)

func (c *Client) AttachHostDevice(ctx context.Context, uuid, deviceName string) (domain.VM, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VM{}, err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return domain.VM{}, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()

	hostDevice, err := c.lookupHostDeviceLocked(strings.TrimSpace(deviceName))
	if err != nil {
		return domain.VM{}, err
	}
	assignment, err := passthroughFromHostDevice(hostDevice)
	if err != nil {
		return domain.VM{}, err
	}
	hostdevXML, err := passthroughDeviceXML(assignment)
	if err != nil {
		return domain.VM{}, err
	}
	flags, err := domainDeviceModifyFlags(dom)
	if err != nil {
		return domain.VM{}, err
	}
	if err := dom.AttachDeviceFlags(hostdevXML, flags); err != nil {
		return domain.VM{}, mapHostDeviceOperationError("attach", uuid, assignment.Name, err)
	}
	vm, err := domainFromLibvirt(dom)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&vm)
	return vm, nil
}

func (c *Client) DetachHostDevice(ctx context.Context, uuid, deviceName string) (domain.VM, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VM{}, err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return domain.VM{}, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()

	assignment, err := passthroughDeviceFromName(strings.TrimSpace(deviceName))
	if err != nil {
		return domain.VM{}, err
	}
	hostdevXML, err := passthroughDeviceXML(assignment)
	if err != nil {
		return domain.VM{}, err
	}
	flags, err := domainDeviceModifyFlags(dom)
	if err != nil {
		return domain.VM{}, err
	}
	if err := dom.DetachDeviceFlags(hostdevXML, flags); err != nil {
		return domain.VM{}, mapHostDeviceOperationError("detach", uuid, assignment.Name, err)
	}
	vm, err := domainFromLibvirt(dom)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&vm)
	return vm, nil
}

func (c *Client) lookupHostDeviceLocked(name string) (domain.HostDevice, error) {
	if name == "" {
		return domain.HostDevice{}, domain.ErrInvalidInput
	}
	device, err := c.conn.LookupDeviceByName(name)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_DEVICE) {
			return domain.HostDevice{}, domain.ErrNotFound
		}
		return domain.HostDevice{}, fmt.Errorf("lookup host device %s: %w", name, err)
	}
	defer func() { _ = device.Free() }()
	xmlDescription, err := device.GetXMLDesc(0)
	if err != nil {
		return domain.HostDevice{}, fmt.Errorf("read host device %s xml: %w", name, err)
	}
	parsed, err := parseHostDeviceXML(xmlDescription)
	if err != nil {
		return domain.HostDevice{}, err
	}
	if parsed.Name == "" {
		parsed.Name = name
	}
	return parsed, nil
}

func domainDeviceModifyFlags(dom *libvirt.Domain) (libvirt.DomainDeviceModifyFlags, error) {
	active, err := dom.IsActive()
	if err != nil {
		return 0, fmt.Errorf("read domain active state: %w", err)
	}
	persistent, err := dom.IsPersistent()
	if err != nil {
		return 0, fmt.Errorf("read domain persistent state: %w", err)
	}
	if active {
		flags := libvirt.DOMAIN_DEVICE_MODIFY_LIVE
		if persistent {
			flags |= libvirt.DOMAIN_DEVICE_MODIFY_CONFIG
		}
		return flags, nil
	}
	if !persistent {
		return 0, errors.Join(domain.ErrActionNotAllowed, errors.New("domain is not persistent"))
	}
	return libvirt.DOMAIN_DEVICE_MODIFY_CONFIG, nil
}

func mapHostDeviceOperationError(operation, uuid, deviceName string, err error) error {
	switch {
	case errors.Is(err, libvirt.ERR_NO_DOMAIN):
		return domain.ErrNotFound
	case errors.Is(err, libvirt.ERR_NO_DEVICE):
		return domain.ErrNotFound
	case errors.Is(err, libvirt.ERR_OPERATION_UNSUPPORTED), errors.Is(err, libvirt.ERR_OPERATION_INVALID):
		return errors.Join(domain.ErrActionNotAllowed, err)
	case errors.Is(err, libvirt.ERR_OPERATION_FAILED):
		return errors.Join(domain.ErrConflict, err)
	default:
		return fmt.Errorf("%s host device %s on domain %s: %w", operation, deviceName, uuid, err)
	}
}

func passthroughFromHostDevice(device domain.HostDevice) (domain.PassthroughDevice, error) {
	deviceType := strings.ToLower(strings.TrimSpace(device.Type))
	if deviceType == "usb_device" {
		deviceType = "usb"
	}
	if deviceType != "pci" && deviceType != "usb" {
		return domain.PassthroughDevice{}, fmt.Errorf("%w: host device %s is not a PCI or USB device", domain.ErrInvalidInput, device.Name)
	}
	if strings.TrimSpace(device.Name) == "" {
		return domain.PassthroughDevice{}, errors.Join(domain.ErrInvalidInput, errors.New("host device name is required"))
	}
	return domain.PassthroughDevice{
		Name:      strings.TrimSpace(device.Name),
		Type:      deviceType,
		Vendor:    strings.TrimSpace(device.Vendor),
		VendorID:  strings.TrimSpace(device.VendorID),
		Product:   strings.TrimSpace(device.Product),
		ProductID: strings.TrimSpace(device.ProductID),
		Address:   strings.TrimSpace(device.Address),
		Driver:    strings.TrimSpace(device.Driver),
	}, nil
}

func passthroughFromHostDevXML(mode, deviceType string, source domainHostDevSource) (domain.PassthroughDevice, bool) {
	if mode = strings.ToLower(strings.TrimSpace(mode)); mode != "" && mode != "subsystem" {
		return domain.PassthroughDevice{}, false
	}
	switch strings.ToLower(strings.TrimSpace(deviceType)) {
	case "pci":
		host := normalizePCIHex(source.Address.Domain, 4)
		bus := normalizePCIHex(source.Address.Bus, 2)
		slot := normalizePCIHex(source.Address.Slot, 2)
		function := normalizePCIHex(source.Address.Function, 1)
		if host == "" || bus == "" || slot == "" || function == "" {
			return domain.PassthroughDevice{}, false
		}
		return domain.PassthroughDevice{
			Name:    "pci_" + host + "_" + bus + "_" + slot + "_" + function,
			Type:    "pci",
			Address: host + ":" + bus + ":" + slot + "." + function,
		}, true
	case "usb":
		vendorID := normalizeDisplayHexID(source.Vendor.ID, 4)
		productID := normalizeDisplayHexID(source.Product.ID, 4)
		if vendorID == "" || productID == "" {
			return domain.PassthroughDevice{}, false
		}
		bus := strings.TrimSpace(source.Address.Bus)
		device := strings.TrimSpace(source.Address.Device)
		assignment := domain.PassthroughDevice{
			Type: "usb", VendorID: vendorID, ProductID: productID,
			Name: "usb_device_" + strings.TrimPrefix(vendorID, "0x") + "_" + strings.TrimPrefix(productID, "0x"),
		}
		if bus != "" && device != "" {
			assignment.Address = "bus " + bus + " · device " + device
			assignment.Name += "_" + bus + "_" + device
		}
		return assignment, true
	default:
		return domain.PassthroughDevice{}, false
	}
}

func passthroughDeviceFromName(name string) (domain.PassthroughDevice, error) {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "pci_") {
		parts := strings.Split(strings.TrimPrefix(name, "pci_"), "_")
		if len(parts) != 4 {
			return domain.PassthroughDevice{}, errors.Join(domain.ErrInvalidInput, errors.New("invalid PCI host device name"))
		}
		host := normalizePCIHex(parts[0], 4)
		bus := normalizePCIHex(parts[1], 2)
		slot := normalizePCIHex(parts[2], 2)
		function := normalizePCIHex(parts[3], 1)
		if host == "" || bus == "" || slot == "" || function == "" {
			return domain.PassthroughDevice{}, errors.Join(domain.ErrInvalidInput, errors.New("invalid PCI host device name"))
		}
		return domain.PassthroughDevice{
			Name: name, Type: "pci", Address: host + ":" + bus + ":" + slot + "." + function,
		}, nil
	}
	if strings.HasPrefix(name, "usb_device_") {
		parts := strings.Split(strings.TrimPrefix(name, "usb_device_"), "_")
		if len(parts) != 2 && len(parts) != 4 {
			return domain.PassthroughDevice{}, errors.Join(domain.ErrInvalidInput, errors.New("invalid USB host device name"))
		}
		vendorID := normalizeDisplayHexID(parts[0], 4)
		productID := normalizeDisplayHexID(parts[1], 4)
		if vendorID == "" || productID == "" {
			return domain.PassthroughDevice{}, errors.Join(domain.ErrInvalidInput, errors.New("invalid USB host device name"))
		}
		assignment := domain.PassthroughDevice{
			Name: name, Type: "usb", VendorID: vendorID, ProductID: productID,
		}
		if len(parts) == 4 {
			if parts[2] == "" || parts[3] == "" {
				return domain.PassthroughDevice{}, errors.Join(domain.ErrInvalidInput, errors.New("invalid USB host device name"))
			}
			assignment.Address = "bus " + parts[2] + " · device " + parts[3]
		}
		return assignment, nil
	}
	return domain.PassthroughDevice{}, errors.Join(domain.ErrInvalidInput, errors.New("unsupported host device name"))
}

func passthroughDeviceXML(device domain.PassthroughDevice) (string, error) {
	switch strings.ToLower(strings.TrimSpace(device.Type)) {
	case "pci":
		host, bus, slot, function, ok := pciAddressParts(device)
		if !ok {
			return "", errors.Join(domain.ErrInvalidInput, errors.New("PCI host device address is required"))
		}
		return fmt.Sprintf("<hostdev mode='subsystem' type='pci' managed='yes'><source><address domain='0x%s' bus='0x%s' slot='0x%s' function='0x%s'/></source></hostdev>", host, bus, slot, function), nil
	case "usb":
		vendorID, err := normalizeHexID(device.VendorID)
		if err != nil {
			return "", errors.Join(domain.ErrInvalidInput, err)
		}
		productID, err := normalizeHexID(device.ProductID)
		if err != nil {
			return "", errors.Join(domain.ErrInvalidInput, err)
		}
		address := ""
		if bus, dev, ok := usbAddressParts(device); ok {
			address = fmt.Sprintf("<address bus='%s' device='%s'/>", bus, dev)
		}
		return fmt.Sprintf("<hostdev mode='subsystem' type='usb' managed='yes'><source><vendor id='%s'/><product id='%s'/>%s</source></hostdev>", vendorID, productID, address), nil
	default:
		return "", errors.Join(domain.ErrInvalidInput, errors.New("host device type must be pci or usb"))
	}
}

func pciAddressParts(device domain.PassthroughDevice) (string, string, string, string, bool) {
	if match := pciAddressPattern.FindStringSubmatch(strings.TrimSpace(device.Address)); match != nil {
		return normalizePCIHex(match[1], 4), normalizePCIHex(match[2], 2), normalizePCIHex(match[3], 2), normalizePCIHex(match[4], 1), true
	}
	assignment, err := passthroughDeviceFromName(device.Name)
	if err != nil || assignment.Type != "pci" {
		return "", "", "", "", false
	}
	match := pciAddressPattern.FindStringSubmatch(assignment.Address)
	if match == nil {
		return "", "", "", "", false
	}
	return normalizePCIHex(match[1], 4), normalizePCIHex(match[2], 2), normalizePCIHex(match[3], 2), normalizePCIHex(match[4], 1), true
}

func usbAddressParts(device domain.PassthroughDevice) (string, string, bool) {
	if match := usbAddressPattern.FindStringSubmatch(strings.TrimSpace(device.Address)); match != nil {
		return match[1], match[2], true
	}
	assignment, err := passthroughDeviceFromName(device.Name)
	if err != nil || assignment.Type != "usb" {
		return "", "", false
	}
	match := usbAddressPattern.FindStringSubmatch(assignment.Address)
	if match == nil {
		return "", "", false
	}
	return match[1], match[2], true
}

func normalizePCIHex(value string, width int) string {
	value = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(value), "0x"))
	if value == "" {
		return ""
	}
	parsed, err := strconv.ParseUint(value, 16, 16)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%0*x", width, parsed)
}

func normalizeDisplayHexID(value string, width int) string {
	normalized := normalizePCIHex(value, width)
	if normalized == "" {
		return ""
	}
	return "0x" + normalized
}

func normalizeHexID(value string) (string, error) {
	normalized := normalizePCIHex(value, 4)
	if normalized == "" {
		return "", fmt.Errorf("invalid hexadecimal ID %q", value)
	}
	return "0x" + normalized, nil
}
