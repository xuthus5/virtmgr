package libvirtbackend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"virtmgr/internal/domain"
)

func (d *DemoBackend) ConvertStorageVolume(_ context.Context, spec domain.StorageVolumeConvertSpec) (domain.StorageVolume, error) {
	spec.SourcePool = strings.TrimSpace(spec.SourcePool)
	spec.SourceVolume = strings.TrimSpace(spec.SourceVolume)
	spec.TargetPool = strings.TrimSpace(spec.TargetPool)
	spec.TargetName = strings.TrimSpace(spec.TargetName)
	spec.TargetFormat = strings.ToLower(strings.TrimSpace(spec.TargetFormat))
	if (spec.TargetFormat != "qcow2" && spec.TargetFormat != "raw") || spec.SourcePool == "" || spec.SourceVolume == "" || spec.TargetPool == "" || spec.TargetName == "" || (spec.SourcePool == spec.TargetPool && spec.SourceVolume == spec.TargetName) {
		return domain.StorageVolume{}, domain.ErrInvalidInput
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	sourcePool, ok := d.pools[spec.SourcePool]
	if !ok {
		return domain.StorageVolume{}, domain.ErrNotFound
	}
	if sourcePool.State != domain.StoragePoolStateActive {
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	source, ok := d.demoVolumeLocked(spec.SourcePool, spec.SourceVolume)
	if !ok {
		return domain.StorageVolume{}, domain.ErrNotFound
	}
	if source.Format != "qcow2" && source.Format != "raw" {
		return domain.StorageVolume{}, fmt.Errorf("%w: only qcow2 and raw source volumes can be converted", domain.ErrInvalidInput)
	}
	if backing, exists := d.volumeBacking[spec.SourcePool+"/"+source.Name]; exists && strings.TrimSpace(backing) != "" {
		return domain.StorageVolume{}, fmt.Errorf("%w: linked-clone source volumes must be flattened before conversion", domain.ErrActionNotAllowed)
	}
	for _, volume := range d.volumes[spec.SourcePool] {
		if source.Name != volume.Name && filepath.Clean(d.volumeBacking[spec.SourcePool+"/"+volume.Name]) == filepath.Clean(source.Path) {
			return domain.StorageVolume{}, fmt.Errorf("%w: source volume is a backing store for %s", domain.ErrVolumeInUse, volume.Name)
		}
	}
	for _, vm := range d.vms {
		if vm.State == domain.VMStateShutoff || vm.State == domain.VMStateCrashed {
			continue
		}
		for _, disk := range vm.Disks {
			if diskUsesVolume(disk.Source, source.Path, source.Name) {
				return domain.StorageVolume{}, fmt.Errorf("%w: source volume is attached to running VM %s", domain.ErrVolumeInUse, vm.Name)
			}
		}
	}
	targetPool, ok := d.pools[spec.TargetPool]
	if !ok {
		return domain.StorageVolume{}, domain.ErrNotFound
	}
	if targetPool.State != domain.StoragePoolStateActive {
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	if _, exists := d.demoVolumeLocked(spec.TargetPool, spec.TargetName); exists {
		return domain.StorageVolume{}, domain.ErrConflict
	}
	if targetPool.Type == "logical" && spec.TargetFormat != "raw" {
		return domain.StorageVolume{}, fmt.Errorf("%w: logical LVM pools only support raw volumes", domain.ErrInvalidInput)
	}
	required := domain.StorageProvisionRequiredBytes(spec.TargetFormat, source.CapacityBytes, source.AllocationBytes)
	if !domain.StoragePoolHasCapacity(targetPool, required) {
		return domain.StorageVolume{}, fmt.Errorf("%w: target pool requires %d bytes", domain.ErrStorageInsufficient, required)
	}
	target := domain.StorageVolume{
		Name: spec.TargetName, Path: filepath.Join(targetPool.TargetPath, spec.TargetName), Type: "file", Format: spec.TargetFormat,
		CapacityBytes: source.CapacityBytes, AllocationBytes: source.AllocationBytes,
	}
	if spec.TargetFormat == "raw" {
		target.AllocationBytes = source.CapacityBytes
	}
	d.volumes[spec.TargetPool] = append(d.volumes[spec.TargetPool], target)
	if body, exists := d.volumeData[spec.SourcePool+"/"+source.Name]; exists {
		d.volumeData[spec.TargetPool+"/"+target.Name] = append([]byte(nil), body...)
	}
	d.publish("storage.volume.converted", "info", "storage_volume", spec.TargetPool+"/"+target.Name, target.Name, "存储卷 "+source.Name+" 已复制并转换为 "+spec.TargetFormat)
	return target, nil
}

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
