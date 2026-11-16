package libvirtbackend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"virtmgr/internal/domain"
)

func (d *DemoBackend) CloneStorageVolume(_ context.Context, sourcePoolName, sourceVolumeName, targetPoolName, targetName string) (domain.StorageVolume, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	sourcePool, ok := d.pools[sourcePoolName]
	if !ok {
		return domain.StorageVolume{}, domain.ErrNotFound
	}
	if sourcePool.State != domain.StoragePoolStateActive {
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	source, ok := d.demoVolumeLocked(sourcePoolName, sourceVolumeName)
	if !ok {
		return domain.StorageVolume{}, domain.ErrNotFound
	}
	targetPool, ok := d.pools[targetPoolName]
	if !ok {
		return domain.StorageVolume{}, domain.ErrNotFound
	}
	if targetPool.State != domain.StoragePoolStateActive {
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	if _, exists := d.demoVolumeLocked(targetPoolName, targetName); exists {
		return domain.StorageVolume{}, domain.ErrConflict
	}
	target := domain.StorageVolume{
		Name: targetName, Path: filepath.Join(targetPool.TargetPath, targetName), Type: "file", Format: source.Format,
		CapacityBytes: source.CapacityBytes, AllocationBytes: source.AllocationBytes,
	}
	d.volumes[targetPoolName] = append(d.volumes[targetPoolName], target)
	if body, exists := d.volumeData[sourcePoolName+"/"+sourceVolumeName]; exists {
		d.volumeData[targetPoolName+"/"+targetName] = append([]byte(nil), body...)
	}
	return target, nil
}

func (d *DemoBackend) CloneStorageVolumeLinked(_ context.Context, sourcePoolName, sourceVolumeName, targetPoolName, targetName string) (domain.StorageVolume, error) {
	if sourcePoolName != targetPoolName {
		return domain.StorageVolume{}, fmt.Errorf("%w: linked clone source and target must use the same storage pool", domain.ErrInvalidInput)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	sourcePool, ok := d.pools[sourcePoolName]
	if !ok {
		return domain.StorageVolume{}, domain.ErrNotFound
	}
	if sourcePool.State != domain.StoragePoolStateActive {
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	source, ok := d.demoVolumeLocked(sourcePoolName, sourceVolumeName)
	if !ok {
		return domain.StorageVolume{}, domain.ErrNotFound
	}
	if source.Format != "qcow2" {
		return domain.StorageVolume{}, fmt.Errorf("%w: linked clone requires a qcow2 source volume", domain.ErrInvalidInput)
	}
	if _, exists := d.demoVolumeLocked(sourcePoolName, targetName); exists {
		return domain.StorageVolume{}, domain.ErrConflict
	}
	target := domain.StorageVolume{
		Name: targetName, Path: filepath.Join(sourcePool.TargetPath, targetName), Type: "file", Format: "qcow2",
		CapacityBytes: source.CapacityBytes, AllocationBytes: 0,
	}
	d.volumes[sourcePoolName] = append(d.volumes[sourcePoolName], target)
	d.volumeBacking[sourcePoolName+"/"+targetName] = source.Path
	return target, nil
}

func (d *DemoBackend) StorageVolumeChecksum(_ context.Context, poolName, volumeName string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	pool, ok := d.pools[poolName]
	if !ok {
		return "", domain.ErrNotFound
	}
	if pool.State != domain.StoragePoolStateActive {
		return "", domain.ErrUnavailable
	}
	if _, ok := d.demoVolumeLocked(poolName, volumeName); !ok {
		return "", domain.ErrNotFound
	}
	body := d.volumeData[poolName+"/"+volumeName]
	if len(body) == 0 {
		body = []byte(fmt.Sprintf("virtmgr-demo-image:%s/%s", poolName, volumeName))
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
