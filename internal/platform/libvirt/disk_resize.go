package libvirtbackend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) ResizeVMDisk(ctx context.Context, uuid, target string, spec domain.VMDiskResizeSpec) (domain.VM, error) {
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

	persistent, err := dom.IsPersistent()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s persistence: %w", uuid, err)
	}
	if !persistent {
		return domain.VM{}, errors.Join(domain.ErrActionNotAllowed, errors.New("domain is not persistent"))
	}
	xmlText, err := dom.GetXMLDesc(0)
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s xml: %w", uuid, err)
	}
	details, err := parseDomainXML(xmlText)
	if err != nil {
		return domain.VM{}, fmt.Errorf("parse domain %s xml: %w", uuid, err)
	}
	disk, found := findWritableDisk(details.Disks, target)
	if !found {
		return domain.VM{}, domain.ErrNotFound
	}
	blockInfo, err := dom.GetBlockInfo(target, 0)
	if err != nil {
		return domain.VM{}, mapVMDiskResizeError(uuid, target, err)
	}
	if spec.CapacityBytes <= blockInfo.Capacity {
		return domain.VM{}, fmt.Errorf("%w: new capacity must exceed current capacity", domain.ErrInvalidInput)
	}

	active, err := dom.IsActive()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s active state: %w", uuid, err)
	}
	if active {
		if err := dom.BlockResize(target, spec.CapacityBytes, libvirt.DOMAIN_BLOCK_RESIZE_BYTES|libvirt.DOMAIN_BLOCK_RESIZE_EXTEND); err != nil {
			return domain.VM{}, mapVMDiskResizeError(uuid, target, err)
		}
	} else {
		volume, lookupErr := c.conn.LookupStorageVolByPath(disk.Source)
		if lookupErr == nil {
			defer func() { _ = volume.Free() }()
			if err := volume.Resize(spec.CapacityBytes, 0); err != nil {
				return domain.VM{}, mapVMDiskResizeError(uuid, target, err)
			}
		} else if errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) && strings.EqualFold(disk.Type, "file") {
			if err := c.resizeFileDiskLocked(disk.Source, spec.CapacityBytes); err != nil {
				return domain.VM{}, mapVMDiskResizeError(uuid, target, err)
			}
		} else if errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
			return domain.VM{}, errors.Join(domain.ErrDiskResize, errors.New("disk is not backed by a resizable file"))
		} else {
			return domain.VM{}, fmt.Errorf("lookup storage volume for disk %s: %w", target, lookupErr)
		}
	}

	vm, err := domainFromLibvirt(dom)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&vm)
	return vm, nil
}

func (c *Client) resizeFileDiskLocked(source string, capacity uint64) error {
	if !filepath.IsAbs(source) || strings.ContainsRune(source, '\x00') {
		return errors.Join(domain.ErrDiskResize, errors.New("disk source is not an absolute file path"))
	}
	name := filepath.Base(source)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return errors.Join(domain.ErrDiskResize, errors.New("disk source has no file name"))
	}
	poolName := fmt.Sprintf("virtmgr-resize-%d", time.Now().UnixNano())
	poolXML := fmt.Sprintf("<pool type='dir'><name>%s</name><target><path>%s</path></target></pool>", escapeXML(poolName), escapeXML(filepath.Dir(source)))
	pool, err := c.conn.StoragePoolDefineXML(poolXML, 0)
	if err != nil {
		return errors.Join(domain.ErrDiskResize, fmt.Errorf("define temporary storage pool: %w", err))
	}
	defer func() { _ = pool.Free() }()
	cleanup := func() {
		_ = pool.Destroy()
		_ = pool.Undefine()
	}
	if err := pool.Create(0); err != nil {
		cleanup()
		return errors.Join(domain.ErrDiskResize, fmt.Errorf("start temporary storage pool: %w", err))
	}
	if err := pool.Refresh(0); err != nil {
		cleanup()
		return errors.Join(domain.ErrDiskResize, fmt.Errorf("refresh temporary storage pool: %w", err))
	}
	volume, err := pool.LookupStorageVolByName(name)
	if err != nil {
		cleanup()
		if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
			return errors.Join(domain.ErrDiskResize, errors.New("disk file was not found by libvirt"))
		}
		return fmt.Errorf("lookup disk file %s: %w", source, err)
	}
	defer func() { _ = volume.Free() }()
	resizeErr := volume.Resize(capacity, 0)
	cleanup()
	return resizeErr
}

func findWritableDisk(disks []domain.Disk, target string) (domain.Disk, bool) {
	target = strings.TrimSpace(target)
	for _, disk := range disks {
		if disk.Target != target {
			continue
		}
		if disk.Device != "disk" || disk.ReadOnly || strings.TrimSpace(disk.Source) == "" {
			return domain.Disk{}, false
		}
		return disk, true
	}
	return domain.Disk{}, false
}

func mapVMDiskResizeError(uuid, target string, err error) error {
	switch {
	case errors.Is(err, libvirt.ERR_NO_DOMAIN), errors.Is(err, libvirt.ERR_NO_STORAGE_VOL):
		return domain.ErrNotFound
	case errors.Is(err, libvirt.ERR_OPERATION_UNSUPPORTED),
		errors.Is(err, libvirt.ERR_OPERATION_INVALID),
		errors.Is(err, libvirt.ERR_INVALID_ARG),
		errors.Is(err, libvirt.ERR_NO_SUPPORT):
		return errors.Join(domain.ErrDiskResize, fmt.Errorf("resize disk %s on domain %s: %w", target, uuid, err))
	default:
		return fmt.Errorf("resize disk %s on domain %s: %w", target, uuid, err)
	}
}

func (c *Client) populateDiskStorageInfoLocked(vm *domain.VM) {
	for index := range vm.Disks {
		disk := &vm.Disks[index]
		if disk.Device != "disk" || strings.TrimSpace(disk.Source) == "" {
			continue
		}
		if disk.CapacityBytes > 0 {
			continue
		}
		volume, err := c.conn.LookupStorageVolByPath(disk.Source)
		if err != nil {
			continue
		}
		info, infoErr := volume.GetInfo()
		_ = volume.Free()
		if infoErr != nil {
			continue
		}
		disk.CapacityBytes = info.Capacity
		disk.AllocationBytes = info.Allocation
	}
}
