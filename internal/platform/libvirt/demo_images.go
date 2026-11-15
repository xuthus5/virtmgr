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
