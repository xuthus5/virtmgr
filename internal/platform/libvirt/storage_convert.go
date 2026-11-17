package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) ConvertStorageVolume(ctx context.Context, spec domain.StorageVolumeConvertSpec) (domain.StorageVolume, error) {
	spec.SourcePool = strings.TrimSpace(spec.SourcePool)
	spec.SourceVolume = strings.TrimSpace(spec.SourceVolume)
	spec.TargetPool = strings.TrimSpace(spec.TargetPool)
	spec.TargetName = strings.TrimSpace(spec.TargetName)
	spec.TargetFormat = strings.ToLower(strings.TrimSpace(spec.TargetFormat))
	if spec.TargetFormat != "qcow2" && spec.TargetFormat != "raw" {
		return domain.StorageVolume{}, fmt.Errorf("%w: target format must be qcow2 or raw", domain.ErrInvalidInput)
	}
	if spec.SourcePool == "" || spec.SourceVolume == "" || spec.TargetPool == "" || spec.TargetName == "" ||
		(spec.SourcePool == spec.TargetPool && spec.SourceVolume == spec.TargetName) {
		return domain.StorageVolume{}, domain.ErrInvalidInput
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.StorageVolume{}, err
	}
	sourcePool, err := c.conn.LookupStoragePoolByName(spec.SourcePool)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return domain.StorageVolume{}, domain.ErrNotFound
		}
		return domain.StorageVolume{}, fmt.Errorf("lookup source storage pool %s: %w", spec.SourcePool, err)
	}
	defer func() { _ = sourcePool.Free() }()
	sourcePoolInfo, err := sourcePool.GetInfo()
	if err != nil || sourcePoolInfo.State != libvirt.STORAGE_POOL_RUNNING {
		if err != nil {
			return domain.StorageVolume{}, fmt.Errorf("read source storage pool %s: %w", spec.SourcePool, err)
		}
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	sourceVolume, err := sourcePool.LookupStorageVolByName(spec.SourceVolume)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
			return domain.StorageVolume{}, domain.ErrNotFound
		}
		return domain.StorageVolume{}, fmt.Errorf("lookup source storage volume %s: %w", spec.SourceVolume, err)
	}
	defer func() { _ = sourceVolume.Free() }()
	source, err := storageVolumeFromLibvirt(sourceVolume)
	if err != nil {
		return domain.StorageVolume{}, err
	}
	if source.Format != "qcow2" && source.Format != "raw" {
		return domain.StorageVolume{}, fmt.Errorf("%w: only qcow2 and raw source volumes can be converted", domain.ErrInvalidInput)
	}
	sourceXML, err := sourceVolume.GetXMLDesc(0)
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("read source storage volume XML: %w", err)
	}
	var parsed storageVolumeXML
	if err := xml.Unmarshal([]byte(sourceXML), &parsed); err != nil {
		return domain.StorageVolume{}, fmt.Errorf("decode source storage volume XML: %w", err)
	}
	if len(parsed.BackingStore) > 0 {
		return domain.StorageVolume{}, fmt.Errorf("%w: linked-clone source volumes must be flattened before conversion", domain.ErrActionNotAllowed)
	}
	children, err := c.storageVolumesUsingBackingLocked(source.Path)
	if err != nil {
		return domain.StorageVolume{}, err
	}
	if len(children) > 0 {
		return domain.StorageVolume{}, fmt.Errorf("%w: source volume is a backing store for %s", domain.ErrVolumeInUse, strings.Join(children, ", "))
	}
	attached, err := c.domainsActivelyUsingVolumeLocked(source.Path, spec.SourceVolume)
	if err != nil {
		return domain.StorageVolume{}, err
	}
	if len(attached) > 0 {
		return domain.StorageVolume{}, fmt.Errorf("%w: source volume is attached to running VM %s", domain.ErrVolumeInUse, strings.Join(attached, ", "))
	}

	targetPool, err := c.conn.LookupStoragePoolByName(spec.TargetPool)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return domain.StorageVolume{}, domain.ErrNotFound
		}
		return domain.StorageVolume{}, fmt.Errorf("lookup target storage pool %s: %w", spec.TargetPool, err)
	}
	defer func() { _ = targetPool.Free() }()
	targetPoolInfo, err := targetPool.GetInfo()
	if err != nil || targetPoolInfo.State != libvirt.STORAGE_POOL_RUNNING {
		if err != nil {
			return domain.StorageVolume{}, fmt.Errorf("read target storage pool %s: %w", spec.TargetPool, err)
		}
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	if existing, lookupErr := targetPool.LookupStorageVolByName(spec.TargetName); lookupErr == nil {
		_ = existing.Free()
		return domain.StorageVolume{}, domain.ErrConflict
	} else if !errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
		return domain.StorageVolume{}, fmt.Errorf("check target storage volume %s: %w", spec.TargetName, lookupErr)
	}

	targetVolume, err := targetPool.StorageVolCreateXMLFrom(buildCloneVolumeXML(spec.TargetName, source.CapacityBytes, spec.TargetFormat), sourceVolume, 0)
	if err != nil {
		if errors.Is(err, libvirt.ERR_STORAGE_VOL_EXIST) {
			return domain.StorageVolume{}, domain.ErrConflict
		}
		return domain.StorageVolume{}, fmt.Errorf("convert storage volume %s to %s: %w", spec.SourceVolume, spec.TargetName, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = targetVolume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
		}
		_ = targetVolume.Free()
	}()
	result, err := storageVolumeFromLibvirt(targetVolume)
	if err != nil {
		return domain.StorageVolume{}, err
	}
	if result.Format != spec.TargetFormat {
		return domain.StorageVolume{}, fmt.Errorf("libvirt created target volume %s as %s instead of requested %s", spec.TargetName, result.Format, spec.TargetFormat)
	}
	committed = true
	if err := targetPool.Refresh(0); err == nil {
		if refreshed, lookupErr := targetPool.LookupStorageVolByName(spec.TargetName); lookupErr == nil {
			defer func() { _ = refreshed.Free() }()
			if refreshedVolume, infoErr := storageVolumeFromLibvirt(refreshed); infoErr == nil {
				return refreshedVolume, nil
			}
		}
	}
	return result, nil
}

func (c *Client) domainsActivelyUsingVolumeLocked(volumePath, volumeName string) ([]string, error) {
	domains, err := c.conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE | libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		return nil, fmt.Errorf("list libvirt domains before storage conversion: %w", err)
	}
	attached := make([]string, 0)
	for index := range domains {
		dom := &domains[index]
		name, nameErr := dom.GetName()
		state, _, stateErr := dom.GetState()
		xmlText, xmlErr := dom.GetXMLDesc(0)
		_ = dom.Free()
		if nameErr != nil || stateErr != nil || xmlErr != nil {
			return nil, fmt.Errorf("inspect domains before storage conversion: %w", errors.Join(nameErr, stateErr, xmlErr))
		}
		switch state {
		case libvirt.DOMAIN_SHUTOFF, libvirt.DOMAIN_CRASHED:
			continue
		}
		details, err := parseDomainXML(xmlText)
		if err != nil {
			return nil, fmt.Errorf("inspect domain %s before storage conversion: %w", name, err)
		}
		for _, disk := range details.Disks {
			if diskUsesVolume(disk.Source, filepath.Clean(volumePath), volumeName) {
				attached = append(attached, name)
				break
			}
		}
	}
	return attached, nil
}
