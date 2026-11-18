package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) FlattenStorageVolume(ctx context.Context, spec domain.StorageVolumeFlattenSpec) (domain.StorageVolume, error) {
	spec.SourcePool = strings.TrimSpace(spec.SourcePool)
	spec.SourceVolume = strings.TrimSpace(spec.SourceVolume)
	spec.TargetPool = strings.TrimSpace(spec.TargetPool)
	spec.TargetName = strings.TrimSpace(spec.TargetName)
	if spec.SourcePool == "" || spec.SourceVolume == "" || spec.TargetPool == "" || spec.TargetName == "" ||
		(spec.SourcePool == spec.TargetPool && spec.SourceVolume == spec.TargetName) ||
		!strings.HasSuffix(strings.ToLower(spec.TargetName), ".qcow2") {
		return domain.StorageVolume{}, domain.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return domain.StorageVolume{}, err
	}
	conn, err := libvirt.NewConnect(c.uri)
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("connect libvirt %s for storage flatten: %w", c.uri, err)
	}
	defer func() { _, _ = conn.Close() }()

	sourcePool, err := conn.LookupStoragePoolByName(spec.SourcePool)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return domain.StorageVolume{}, domain.ErrNotFound
		}
		return domain.StorageVolume{}, fmt.Errorf("lookup source storage pool %s: %w", spec.SourcePool, err)
	}
	defer func() { _ = sourcePool.Free() }()
	sourcePoolInfo, err := sourcePool.GetInfo()
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("read source storage pool %s: %w", spec.SourcePool, err)
	}
	if sourcePoolInfo.State != libvirt.STORAGE_POOL_RUNNING {
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
	if source.Format != "qcow2" || !source.Linked {
		return domain.StorageVolume{}, fmt.Errorf("%w: only linked qcow2 volumes can be flattened", domain.ErrInvalidInput)
	}
	sourceXML, err := sourceVolume.GetXMLDesc(0)
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("read source storage volume XML: %w", err)
	}
	var parsed storageVolumeXML
	if err := xml.Unmarshal([]byte(sourceXML), &parsed); err != nil {
		return domain.StorageVolume{}, fmt.Errorf("decode source storage volume XML: %w", err)
	}
	if len(parsed.BackingStore) == 0 {
		return domain.StorageVolume{}, fmt.Errorf("%w: source volume is not a linked clone", domain.ErrInvalidInput)
	}
	children, err := storageVolumesUsingBacking(conn, source.Path)
	if err != nil {
		return domain.StorageVolume{}, err
	}
	if len(children) > 0 {
		return domain.StorageVolume{}, fmt.Errorf("%w: source volume is a backing store for %s", domain.ErrVolumeInUse, strings.Join(children, ", "))
	}
	attached, err := domainsActivelyUsingVolume(conn, source.Path, spec.SourceVolume)
	if err != nil {
		return domain.StorageVolume{}, err
	}
	if len(attached) > 0 {
		return domain.StorageVolume{}, fmt.Errorf("%w: source volume is attached to running VM %s", domain.ErrVolumeInUse, strings.Join(attached, ", "))
	}

	targetPool, err := conn.LookupStoragePoolByName(spec.TargetPool)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return domain.StorageVolume{}, domain.ErrNotFound
		}
		return domain.StorageVolume{}, fmt.Errorf("lookup target storage pool %s: %w", spec.TargetPool, err)
	}
	defer func() { _ = targetPool.Free() }()
	targetPoolInfo, err := targetPool.GetInfo()
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("read target storage pool %s: %w", spec.TargetPool, err)
	}
	if targetPoolInfo.State != libvirt.STORAGE_POOL_RUNNING {
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	targetDefinition, err := storagePoolDefinition(targetPool)
	if err != nil {
		return domain.StorageVolume{}, err
	}
	if targetDefinition.Type == "logical" {
		return domain.StorageVolume{}, fmt.Errorf("%w: logical LVM pools cannot store flattened qcow2 volumes", domain.ErrInvalidInput)
	}
	if existing, lookupErr := targetPool.LookupStorageVolByName(spec.TargetName); lookupErr == nil {
		_ = existing.Free()
		return domain.StorageVolume{}, domain.ErrConflict
	} else if !errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
		return domain.StorageVolume{}, fmt.Errorf("check target storage volume %s: %w", spec.TargetName, lookupErr)
	}
	if err := ctx.Err(); err != nil {
		return domain.StorageVolume{}, err
	}
	targetVolume, err := targetPool.StorageVolCreateXMLFrom(buildCloneVolumeXML(spec.TargetName, source.CapacityBytes, "qcow2"), sourceVolume, 0)
	if err != nil {
		if errors.Is(err, libvirt.ERR_STORAGE_VOL_EXIST) {
			return domain.StorageVolume{}, domain.ErrConflict
		}
		return domain.StorageVolume{}, fmt.Errorf("flatten storage volume %s to %s: %w", spec.SourceVolume, spec.TargetName, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = targetVolume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
		}
		_ = targetVolume.Free()
	}()
	if err := ctx.Err(); err != nil {
		return domain.StorageVolume{}, err
	}
	result, err := storageVolumeFromLibvirt(targetVolume)
	if err != nil {
		return domain.StorageVolume{}, err
	}
	if result.Format != "qcow2" || result.Linked {
		return domain.StorageVolume{}, fmt.Errorf("libvirt created target volume %s with an unexpected backing store or format", spec.TargetName)
	}
	if err := ctx.Err(); err != nil {
		return domain.StorageVolume{}, err
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
