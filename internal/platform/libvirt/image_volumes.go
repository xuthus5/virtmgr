package libvirtbackend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) CloneStorageVolume(ctx context.Context, sourcePoolName, sourceVolumeName, targetPoolName, targetName string) (domain.StorageVolume, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.StorageVolume{}, err
	}
	sourcePool, err := c.conn.LookupStoragePoolByName(sourcePoolName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return domain.StorageVolume{}, domain.ErrNotFound
		}
		return domain.StorageVolume{}, fmt.Errorf("lookup source storage pool %s: %w", sourcePoolName, err)
	}
	defer func() { _ = sourcePool.Free() }()
	sourceInfo, err := sourcePool.GetInfo()
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("read source storage pool %s info: %w", sourcePoolName, err)
	}
	if sourceInfo.State != libvirt.STORAGE_POOL_RUNNING {
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	sourceVolume, err := sourcePool.LookupStorageVolByName(sourceVolumeName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
			return domain.StorageVolume{}, domain.ErrNotFound
		}
		return domain.StorageVolume{}, fmt.Errorf("lookup source storage volume %s: %w", sourceVolumeName, err)
	}
	defer func() { _ = sourceVolume.Free() }()
	source, err := storageVolumeFromLibvirt(sourceVolume)
	if err != nil {
		return domain.StorageVolume{}, err
	}
	targetPool, err := c.conn.LookupStoragePoolByName(targetPoolName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return domain.StorageVolume{}, domain.ErrNotFound
		}
		return domain.StorageVolume{}, fmt.Errorf("lookup target storage pool %s: %w", targetPoolName, err)
	}
	defer func() { _ = targetPool.Free() }()
	targetInfo, err := targetPool.GetInfo()
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("read target storage pool %s info: %w", targetPoolName, err)
	}
	if targetInfo.State != libvirt.STORAGE_POOL_RUNNING {
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	if existing, lookupErr := targetPool.LookupStorageVolByName(targetName); lookupErr == nil {
		_ = existing.Free()
		return domain.StorageVolume{}, domain.ErrConflict
	} else if !errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
		return domain.StorageVolume{}, fmt.Errorf("check target storage volume %s: %w", targetName, lookupErr)
	}
	volumeXML := buildCloneVolumeXML(targetName, source.CapacityBytes, source.Format)
	targetVolume, err := targetPool.StorageVolCreateXMLFrom(volumeXML, sourceVolume, 0)
	if err != nil {
		if errors.Is(err, libvirt.ERR_STORAGE_VOL_EXIST) {
			return domain.StorageVolume{}, domain.ErrConflict
		}
		return domain.StorageVolume{}, fmt.Errorf("clone storage volume %s to %s: %w", sourceVolumeName, targetName, err)
	}
	defer func() { _ = targetVolume.Free() }()
	if err := targetPool.Refresh(0); err == nil {
		if refreshed, lookupErr := targetPool.LookupStorageVolByName(targetName); lookupErr == nil {
			defer func() { _ = refreshed.Free() }()
			if result, infoErr := storageVolumeFromLibvirt(refreshed); infoErr == nil {
				return result, nil
			}
		}
	}
	return storageVolumeFromLibvirt(targetVolume)
}

func (c *Client) StorageVolumeChecksum(ctx context.Context, poolName, volumeName string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return "", err
	}
	pool, err := c.conn.LookupStoragePoolByName(poolName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return "", domain.ErrNotFound
		}
		return "", fmt.Errorf("lookup storage pool %s: %w", poolName, err)
	}
	defer func() { _ = pool.Free() }()
	info, err := pool.GetInfo()
	if err != nil {
		return "", fmt.Errorf("read storage pool %s info: %w", poolName, err)
	}
	if info.State != libvirt.STORAGE_POOL_RUNNING {
		return "", domain.ErrUnavailable
	}
	volume, err := pool.LookupStorageVolByName(volumeName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
			return "", domain.ErrNotFound
		}
		return "", fmt.Errorf("lookup storage volume %s: %w", volumeName, err)
	}
	defer func() { _ = volume.Free() }()
	stream, err := c.conn.NewStream(0)
	if err != nil {
		return "", fmt.Errorf("open storage volume checksum stream: %w", err)
	}
	defer func() { _ = stream.Free() }()
	if err := volume.Download(stream, 0, 0, 0); err != nil {
		return "", fmt.Errorf("download storage volume %s for checksum: %w", volumeName, err)
	}
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			_ = stream.Abort()
			return "", err
		}
		count, recvErr := stream.Recv(buffer)
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			_ = stream.Abort()
			return "", fmt.Errorf("read storage volume %s for checksum: %w", volumeName, recvErr)
		}
		if count == -2 || count == -3 {
			continue
		}
		if _, err := hash.Write(buffer[:count]); err != nil {
			_ = stream.Abort()
			return "", fmt.Errorf("hash storage volume %s: %w", volumeName, err)
		}
	}
	if err := stream.Finish(); err != nil {
		return "", fmt.Errorf("finish checksum stream for storage volume %s: %w", volumeName, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
