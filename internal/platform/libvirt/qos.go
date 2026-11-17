package libvirtbackend

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) UpdateVMDiskQoS(ctx context.Context, uuid, target string, spec domain.VMDiskQoSSpec) (domain.VM, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
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
	active, err := dom.IsActive()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s active state: %w", uuid, err)
	}
	flags := libvirt.DomainModificationImpact(libvirt.DOMAIN_AFFECT_CONFIG)
	xmlFlags := libvirt.DomainXMLFlags(libvirt.DOMAIN_XML_INACTIVE)
	if active {
		flags = libvirt.DOMAIN_AFFECT_LIVE | libvirt.DOMAIN_AFFECT_CONFIG
		xmlFlags = 0
	}
	xmlText, err := dom.GetXMLDesc(xmlFlags)
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s XML for disk QoS: %w", uuid, err)
	}
	details, err := parseDomainXML(xmlText)
	if err != nil {
		return domain.VM{}, err
	}
	found := false
	for _, disk := range details.Disks {
		if disk.Target == target && disk.Device == "disk" && !disk.ReadOnly {
			found = true
			break
		}
	}
	if !found {
		return domain.VM{}, domain.ErrNotFound
	}
	params := &libvirt.DomainBlockIoTuneParameters{
		TotalBytesSecSet: true, TotalBytesSec: spec.TotalBytesSec,
		ReadBytesSecSet: true, ReadBytesSec: spec.ReadBytesSec,
		WriteBytesSecSet: true, WriteBytesSec: spec.WriteBytesSec,
		TotalIopsSecSet: true, TotalIopsSec: spec.TotalIOPS,
		ReadIopsSecSet: true, ReadIopsSec: spec.ReadIOPS,
		WriteIopsSecSet: true, WriteIopsSec: spec.WriteIOPS,
	}
	if err := dom.SetBlockIoTune(target, params, flags); err != nil {
		switch {
		case errors.Is(err, libvirt.ERR_NO_DOMAIN):
			return domain.VM{}, domain.ErrNotFound
		case errors.Is(err, libvirt.ERR_OPERATION_UNSUPPORTED), errors.Is(err, libvirt.ERR_OPERATION_INVALID), errors.Is(err, libvirt.ERR_INVALID_ARG):
			return domain.VM{}, errors.Join(domain.ErrActionNotAllowed, fmt.Errorf("update disk QoS: %w", err))
		default:
			return domain.VM{}, fmt.Errorf("update disk %s QoS for domain %s: %w", target, uuid, err)
		}
	}
	updated, err := domainFromLibvirt(dom)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&updated)
	return updated, nil
}

func (c *Client) UpdateVMNetworkQoS(ctx context.Context, uuid, mac string, spec domain.VMNetworkQoSSpec) (domain.VM, error) {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
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
	active, err := dom.IsActive()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s active state: %w", uuid, err)
	}
	xmlFlags := libvirt.DomainXMLFlags(libvirt.DOMAIN_XML_INACTIVE)
	flags := libvirt.DomainDeviceModifyFlags(libvirt.DOMAIN_DEVICE_MODIFY_CONFIG)
	if active {
		xmlFlags = 0
		flags = libvirt.DOMAIN_DEVICE_MODIFY_LIVE | libvirt.DOMAIN_DEVICE_MODIFY_CONFIG
	}
	xmlText, err := dom.GetXMLDesc(xmlFlags)
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s XML for network QoS: %w", uuid, err)
	}
	update, err := domain.SetVMNetworkQoSXML(xmlText, mac, spec)
	if err != nil {
		return domain.VM{}, err
	}
	if err := dom.UpdateDeviceFlags(update.DeviceXML, flags); err != nil {
		switch {
		case errors.Is(err, libvirt.ERR_NO_DOMAIN):
			return domain.VM{}, domain.ErrNotFound
		case errors.Is(err, libvirt.ERR_OPERATION_UNSUPPORTED), errors.Is(err, libvirt.ERR_OPERATION_INVALID), errors.Is(err, libvirt.ERR_INVALID_ARG):
			return domain.VM{}, errors.Join(domain.ErrActionNotAllowed, fmt.Errorf("update network QoS: %w", err))
		default:
			return domain.VM{}, fmt.Errorf("update network interface %s QoS for domain %s: %w", mac, uuid, err)
		}
	}
	updated, err := domainFromLibvirt(dom)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&updated)
	return updated, nil
}
