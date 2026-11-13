package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

var hotplugTargetPattern = regexp.MustCompile(`^[a-z][a-z0-9]{0,15}$`)

type hotplugVolumeXML struct {
	Target struct {
		Path string `xml:"path"`
	} `xml:"target"`
	Format struct {
		Type string `xml:"type,attr"`
	} `xml:"format"`
}

func (c *Client) AttachVMDisk(ctx context.Context, uuid string, spec domain.VMDiskAttachSpec) (domain.VM, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VM{}, err
	}
	dom, err := c.lookupDomainLocked(uuid)
	if err != nil {
		return domain.VM{}, err
	}
	defer func() { _ = dom.Free() }()
	details, err := domainDetailsForLocked(dom)
	if err != nil {
		return domain.VM{}, err
	}
	bus := spec.Bus
	if bus == "" {
		bus = domain.VMDiskBusVirtio
	}
	target := strings.TrimSpace(spec.Target)
	if target == "" {
		target, err = nextHotplugDiskTarget(details.Disks, bus)
		if err != nil {
			return domain.VM{}, err
		}
	}
	if !hotplugTargetPattern.MatchString(target) || diskTargetInUse(details.Disks, target) {
		return domain.VM{}, domain.ErrConflict
	}
	poolName := strings.TrimSpace(spec.Pool)
	volumeName := strings.TrimSpace(spec.Volume)
	if poolName == "" || volumeName == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	pool, err := c.conn.LookupStoragePoolByName(poolName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return domain.VM{}, domain.ErrNotFound
		}
		return domain.VM{}, fmt.Errorf("lookup disk pool %s: %w", poolName, err)
	}
	defer func() { _ = pool.Free() }()
	volume, err := pool.LookupStorageVolByName(volumeName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
			return domain.VM{}, domain.ErrNotFound
		}
		return domain.VM{}, fmt.Errorf("lookup disk volume %s/%s: %w", poolName, volumeName, err)
	}
	defer func() { _ = volume.Free() }()
	volumeXML, err := volume.GetXMLDesc(0)
	if err != nil {
		return domain.VM{}, fmt.Errorf("read disk volume %s/%s xml: %w", poolName, volumeName, err)
	}
	var parsed hotplugVolumeXML
	if err := xml.Unmarshal([]byte(volumeXML), &parsed); err != nil {
		return domain.VM{}, fmt.Errorf("parse disk volume %s/%s xml: %w", poolName, volumeName, err)
	}
	source := strings.TrimSpace(parsed.Target.Path)
	format := strings.ToLower(strings.TrimSpace(parsed.Format.Type))
	if source == "" || !filepath.IsAbs(source) {
		return domain.VM{}, fmt.Errorf("%w: storage volume has no absolute path", domain.ErrInvalidInput)
	}
	if format != "qcow2" && format != "raw" {
		return domain.VM{}, fmt.Errorf("%w: storage volume format must be qcow2 or raw", domain.ErrInvalidInput)
	}
	deviceXML := fmt.Sprintf("<disk type='file' device='disk'><driver name='qemu' type='%s'/><source file='%s'/><target dev='%s' bus='%s'/></disk>", escapeXML(format), escapeXML(source), escapeXML(target), escapeXML(string(bus)))
	if err := c.attachDomainDeviceLocked(dom, deviceXML); err != nil {
		return domain.VM{}, mapDeviceHotplugError("attach disk", uuid, target, err)
	}
	vm, err := domainFromLibvirt(dom)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&vm)
	return vm, nil
}

func (c *Client) DetachVMDisk(ctx context.Context, uuid, target string) (domain.VM, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VM{}, err
	}
	dom, err := c.lookupDomainLocked(uuid)
	if err != nil {
		return domain.VM{}, err
	}
	defer func() { _ = dom.Free() }()
	details, err := domainDetailsForLocked(dom)
	if err != nil {
		return domain.VM{}, err
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	if root, ok := firstWritableDisk(details.Disks); ok && root.Target == target {
		return domain.VM{}, errors.Join(domain.ErrActionNotAllowed, errors.New("cannot hot-unplug the primary system disk"))
	}
	disk, found := findWritableDisk(details.Disks, target)
	if !found {
		return domain.VM{}, domain.ErrNotFound
	}
	deviceXML := fmt.Sprintf("<disk type='file' device='disk'><driver name='qemu' type='%s'/><source file='%s'/><target dev='%s' bus='%s'/></disk>", escapeXML(disk.Format), escapeXML(disk.Source), escapeXML(disk.Target), escapeXML(disk.Bus))
	if err := c.detachDomainDeviceLocked(dom, deviceXML); err != nil {
		return domain.VM{}, mapDeviceHotplugError("detach disk", uuid, target, err)
	}
	vm, err := domainFromLibvirt(dom)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&vm)
	return vm, nil
}

func (c *Client) AttachVMNetwork(ctx context.Context, uuid string, spec domain.VMNetworkAttachSpec) (domain.VM, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VM{}, err
	}
	dom, err := c.lookupDomainLocked(uuid)
	if err != nil {
		return domain.VM{}, err
	}
	defer func() { _ = dom.Free() }()
	network := strings.TrimSpace(spec.Network)
	model := spec.Model
	if model == "" {
		model = domain.VMNICModelVirtio
	}
	if network == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	deviceXML := fmt.Sprintf("<interface type='network'><source network='%s'/><model type='%s'/></interface>", escapeXML(network), escapeXML(string(model)))
	if err := c.attachDomainDeviceLocked(dom, deviceXML); err != nil {
		return domain.VM{}, mapDeviceHotplugError("attach network", uuid, network, err)
	}
	return domainFromLibvirt(dom)
}

func (c *Client) DetachVMNetwork(ctx context.Context, uuid, mac string) (domain.VM, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VM{}, err
	}
	dom, err := c.lookupDomainLocked(uuid)
	if err != nil {
		return domain.VM{}, err
	}
	defer func() { _ = dom.Free() }()
	details, err := domainDetailsForLocked(dom)
	if err != nil {
		return domain.VM{}, err
	}
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	var found *domain.NetworkInterface
	for index := range details.Interfaces {
		if strings.EqualFold(details.Interfaces[index].MAC, mac) {
			found = &details.Interfaces[index]
			break
		}
	}
	if found == nil {
		return domain.VM{}, domain.ErrNotFound
	}
	if len(details.Interfaces) <= 1 {
		return domain.VM{}, errors.Join(domain.ErrActionNotAllowed, errors.New("cannot remove the last network interface"))
	}
	deviceXML := networkInterfaceXML(*found)
	if err := c.detachDomainDeviceLocked(dom, deviceXML); err != nil {
		return domain.VM{}, mapDeviceHotplugError("detach network", uuid, mac, err)
	}
	return domainFromLibvirt(dom)
}

func (c *Client) lookupDomainLocked(uuid string) (*libvirt.Domain, error) {
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return nil, mapDomainLookupError(uuid, err)
	}
	return dom, nil
}

func domainDetailsForLocked(dom *libvirt.Domain) (domainDetails, error) {
	text, err := dom.GetXMLDesc(0)
	if err != nil {
		return domainDetails{}, fmt.Errorf("read domain xml: %w", err)
	}
	return parseDomainXML(text)
}

func (c *Client) attachDomainDeviceLocked(dom *libvirt.Domain, deviceXML string) error {
	flags, err := domainDeviceModifyFlags(dom)
	if err != nil {
		return err
	}
	if err := dom.AttachDeviceFlags(deviceXML, flags); err != nil {
		_ = dom.DetachDeviceFlags(deviceXML, flags)
		return err
	}
	return nil
}

func (c *Client) detachDomainDeviceLocked(dom *libvirt.Domain, deviceXML string) error {
	flags, err := domainDeviceModifyFlags(dom)
	if err != nil {
		return err
	}
	if err := dom.DetachDeviceFlags(deviceXML, flags); err != nil {
		_ = dom.AttachDeviceFlags(deviceXML, flags)
		return err
	}
	return nil
}

func mapDeviceHotplugError(operation, uuid, name string, err error) error {
	switch {
	case errors.Is(err, libvirt.ERR_NO_DOMAIN):
		return domain.ErrNotFound
	case errors.Is(err, libvirt.ERR_OPERATION_UNSUPPORTED), errors.Is(err, libvirt.ERR_OPERATION_INVALID):
		return errors.Join(domain.ErrActionNotAllowed, err)
	case errors.Is(err, libvirt.ERR_OPERATION_FAILED):
		return errors.Join(domain.ErrConflict, err)
	default:
		return fmt.Errorf("%s %s on domain %s: %w", operation, name, uuid, err)
	}
}

func firstWritableDisk(disks []domain.Disk) (domain.Disk, bool) {
	for _, disk := range disks {
		if disk.Device == "disk" && !disk.ReadOnly && strings.TrimSpace(disk.Source) != "" {
			return disk, true
		}
	}
	return domain.Disk{}, false
}

func diskTargetInUse(disks []domain.Disk, target string) bool {
	for _, disk := range disks {
		if disk.Target == target {
			return true
		}
	}
	return false
}

func nextHotplugDiskTarget(disks []domain.Disk, bus domain.VMDiskBus) (string, error) {
	prefix := "v"
	if bus == domain.VMDiskBusSATA || bus == domain.VMDiskBusSCSI {
		prefix = "s"
	}
	for char := 'b'; char <= 'z'; char++ {
		candidate := prefix + string(char)
		if !diskTargetInUse(disks, candidate) {
			return candidate, nil
		}
	}
	return "", errors.Join(domain.ErrConflict, errors.New("no free disk target is available"))
}

func networkInterfaceXML(device domain.NetworkInterface) string {
	ifaceType := firstNonEmpty(device.Type, "network")
	sourceTag := "network"
	if ifaceType == "bridge" {
		sourceTag = "bridge"
	}
	model := firstNonEmpty(device.Model, string(domain.VMNICModelVirtio))
	return fmt.Sprintf("<interface type='%s'><source %s='%s'/><model type='%s'/><mac address='%s'/></interface>",
		escapeXML(ifaceType), sourceTag, escapeXML(device.Source), escapeXML(model), escapeXML(device.MAC))
}
