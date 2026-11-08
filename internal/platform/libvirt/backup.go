package libvirtbackend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

type backupArtifact struct {
	name          string
	format        string
	capacityBytes uint64
	sourceVolume  string
}

type backupManifest struct {
	FormatVersion string                `json:"formatVersion"`
	CreatedAt     time.Time             `json:"createdAt"`
	VMUUID        string                `json:"vmUuid"`
	VMName        string                `json:"vmName"`
	ConfigVolume  string                `json:"configVolume"`
	ConfigSHA256  string                `json:"configSha256"`
	Disks         []domain.VMBackupDisk `json:"disks"`
}

func (c *Client) BackupVM(ctx context.Context, uuid string, spec domain.VMBackupSpec, progress VMBackupProgress) (domain.VMBackup, error) {
	if err := ctx.Err(); err != nil {
		return domain.VMBackup{}, err
	}
	conn, err := libvirt.NewConnect(c.uri)
	if err != nil {
		return domain.VMBackup{}, fmt.Errorf("connect libvirt %s for backup: %w", c.uri, err)
	}
	defer func() { _, _ = conn.Close() }()

	reportBackupProgress(progress, 3, "正在检查源虚拟机")
	source, err := conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return domain.VMBackup{}, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = source.Free() }()
	state, _, err := source.GetState()
	if err != nil {
		return domain.VMBackup{}, fmt.Errorf("read source domain %s state: %w", uuid, err)
	}
	if state != libvirt.DOMAIN_SHUTOFF && state != libvirt.DOMAIN_CRASHED {
		return domain.VMBackup{}, domain.ErrActionNotAllowed
	}
	persistent, err := source.IsPersistent()
	if err != nil {
		return domain.VMBackup{}, fmt.Errorf("check source domain %s persistence: %w", uuid, err)
	}
	if !persistent {
		return domain.VMBackup{}, domain.ErrActionNotAllowed
	}
	vmName, err := source.GetName()
	if err != nil {
		return domain.VMBackup{}, fmt.Errorf("read source domain %s name: %w", uuid, err)
	}
	configXML, err := source.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE)
	if err != nil {
		return domain.VMBackup{}, fmt.Errorf("read source domain %s config xml: %w", uuid, err)
	}
	disks, err := cloneDisksFromDomainXML(configXML)
	if err != nil {
		return domain.VMBackup{}, err
	}

	reportBackupProgress(progress, 7, "正在检查目标存储池")
	targetPool, err := conn.LookupStoragePoolByName(spec.TargetPool)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return domain.VMBackup{}, domain.ErrNotFound
		}
		return domain.VMBackup{}, fmt.Errorf("lookup backup storage pool %s: %w", spec.TargetPool, err)
	}
	defer func() { _ = targetPool.Free() }()
	poolInfo, err := targetPool.GetInfo()
	if err != nil {
		return domain.VMBackup{}, fmt.Errorf("read backup storage pool %s info: %w", spec.TargetPool, err)
	}
	if poolInfo.State != libvirt.STORAGE_POOL_RUNNING {
		return domain.VMBackup{}, domain.ErrUnavailable
	}

	sourceVolumes := make([]*libvirt.StorageVol, 0, len(disks))
	defer func() {
		for _, volume := range sourceVolumes {
			_ = volume.Free()
		}
	}()
	artifacts := make([]backupArtifact, 0, len(disks))
	for _, disk := range disks {
		volume, err := conn.LookupStorageVolByPath(disk.source)
		if err != nil {
			if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
				return domain.VMBackup{}, fmt.Errorf("%w: disk %s is not managed by a libvirt storage pool", domain.ErrInvalidInput, disk.source)
			}
			return domain.VMBackup{}, fmt.Errorf("lookup source storage volume %s: %w", disk.source, err)
		}
		sourceVolumes = append(sourceVolumes, volume)
		sourceName, err := volume.GetName()
		if err != nil {
			return domain.VMBackup{}, fmt.Errorf("read source storage volume name: %w", err)
		}
		info, err := volume.GetInfo()
		if err != nil {
			return domain.VMBackup{}, fmt.Errorf("read source storage volume %s info: %w", sourceName, err)
		}
		format, err := cloneDiskFormat(disk.format, sourceName)
		if err != nil {
			return domain.VMBackup{}, err
		}
		artifacts = append(artifacts, backupArtifact{
			name:          backupVolumeName(spec.Name, fmt.Sprintf("-disk-%02d%s", len(artifacts)+1, backupExtension(sourceName, format))),
			format:        format,
			capacityBytes: info.Capacity,
			sourceVolume:  sourceName,
		})
	}
	configName := backupVolumeName(spec.Name, "-domain.xml")
	manifestName := backupVolumeName(spec.Name, "-manifest.json")
	for _, name := range append([]string{configName, manifestName}, backupArtifactNames(artifacts)...) {
		exists, err := storageVolumeExists(targetPool, name)
		if err != nil {
			return domain.VMBackup{}, err
		}
		if exists {
			return domain.VMBackup{}, domain.ErrConflict
		}
	}

	created := make([]string, 0, len(artifacts)+2)
	committed := false
	defer func() {
		if committed {
			return
		}
		for _, name := range created {
			if volume, lookupErr := targetPool.LookupStorageVolByName(name); lookupErr == nil {
				_ = volume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
				_ = volume.Free()
			}
		}
	}()
	backupDisks := make([]domain.VMBackupDisk, 0, len(artifacts))
	totalBytes := uint64(0)
	for index, artifact := range artifacts {
		if err := ctx.Err(); err != nil {
			return domain.VMBackup{}, err
		}
		step := fmt.Sprintf("正在复制磁盘 %d/%d", index+1, len(artifacts))
		reportBackupProgress(progress, 10+index*60/maxInt(1, len(artifacts)), step)
		targetVolume, err := targetPool.StorageVolCreateXMLFrom(buildStorageVolumeXML(domain.StorageVolumeCreateSpec{
			Name: artifact.name, Format: artifact.format, CapacityBytes: artifact.capacityBytes,
		}), sourceVolumes[index], 0)
		if err != nil {
			if errors.Is(err, libvirt.ERR_STORAGE_VOL_EXIST) {
				return domain.VMBackup{}, domain.ErrConflict
			}
			return domain.VMBackup{}, fmt.Errorf("copy backup disk %s: %w", artifact.name, err)
		}
		_ = targetVolume.Free()
		created = append(created, artifact.name)
		totalBytes += artifact.capacityBytes
		backupDisks = append(backupDisks, domain.VMBackupDisk{
			SourceVolume: artifact.sourceVolume, BackupVolume: artifact.name,
			Format: artifact.format, CapacityBytes: artifact.capacityBytes,
		})
	}

	reportBackupProgress(progress, 78, "正在写入虚拟机配置")
	configBytes := []byte(configXML)
	if err := createUploadedBackupVolume(conn, targetPool, configName, configBytes, "raw"); err != nil {
		return domain.VMBackup{}, err
	}
	created = append(created, configName)
	totalBytes += uint64(len(configBytes))

	createdAt := time.Now().UTC()
	configSum := sha256.Sum256(configBytes)
	manifest, err := json.MarshalIndent(backupManifest{
		FormatVersion: domain.VMBackupFormatVersion,
		CreatedAt:     createdAt,
		VMUUID:        uuid,
		VMName:        vmName,
		ConfigVolume:  configName,
		ConfigSHA256:  hex.EncodeToString(configSum[:]),
		Disks:         backupDisks,
	}, "", "  ")
	if err != nil {
		return domain.VMBackup{}, fmt.Errorf("encode backup manifest: %w", err)
	}
	manifest = append(manifest, '\n')
	reportBackupProgress(progress, 90, "正在写入备份清单")
	if err := createUploadedBackupVolume(conn, targetPool, manifestName, manifest, "raw"); err != nil {
		return domain.VMBackup{}, err
	}
	created = append(created, manifestName)
	totalBytes += uint64(len(manifest))

	committed = true
	reportBackupProgress(progress, 100, "备份完成")
	return domain.VMBackup{
		FormatVersion:  domain.VMBackupFormatVersion,
		CreatedAt:      createdAt,
		VMUUID:         uuid,
		VMName:         vmName,
		TargetPool:     spec.TargetPool,
		Name:           spec.Name,
		ManifestVolume: manifestName,
		ConfigVolume:   configName,
		ConfigBytes:    uint64(len(configBytes)),
		Disks:          backupDisks,
		TotalBytes:     totalBytes,
	}, nil
}

func createUploadedBackupVolume(conn *libvirt.Connect, pool *libvirt.StoragePool, name string, content []byte, format string) error {
	size := uint64(len(content))
	if size == 0 {
		return domain.ErrInvalidInput
	}
	volume, err := pool.StorageVolCreateXML(buildStorageVolumeXML(domain.StorageVolumeCreateSpec{
		Name: name, Format: format, CapacityBytes: size,
	}), 0)
	if err != nil {
		if errors.Is(err, libvirt.ERR_STORAGE_VOL_EXIST) {
			return domain.ErrConflict
		}
		return fmt.Errorf("create backup volume %s: %w", name, err)
	}
	defer func() { _ = volume.Free() }()
	committed := false
	defer func() {
		if !committed {
			_ = volume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
		}
	}()
	stream, err := conn.NewStream(0)
	if err != nil {
		return fmt.Errorf("open libvirt backup upload stream for %s: %w", name, err)
	}
	defer func() { _ = stream.Free() }()
	if err := volume.Upload(stream, 0, size, 0); err != nil {
		_ = stream.Abort()
		return fmt.Errorf("start upload for backup volume %s: %w", name, err)
	}
	if err := copyToStorageStream(stream, bytes.NewReader(content), size); err != nil {
		_ = stream.Abort()
		return err
	}
	if err := stream.Finish(); err != nil {
		return fmt.Errorf("finish upload for backup volume %s: %w", name, err)
	}
	committed = true
	return nil
}

func storageVolumeExists(pool *libvirt.StoragePool, name string) (bool, error) {
	volume, err := pool.LookupStorageVolByName(name)
	if err == nil {
		_ = volume.Free()
		return true, nil
	}
	if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
		return false, nil
	}
	return false, fmt.Errorf("check backup storage volume %s: %w", name, err)
}

func backupVolumeName(prefix, suffix string) string {
	prefix = strings.TrimSpace(prefix)
	if len(prefix)+len(suffix) <= 127 {
		return prefix + suffix
	}
	keep := 127 - len(suffix)
	if keep < 1 {
		keep = 1
	}
	return prefix[:keep] + suffix
}

func backupExtension(sourceName, format string) string {
	extension := strings.ToLower(filepath.Ext(filepath.Base(sourceName)))
	if extension == "" {
		extension = "." + format
	}
	return extension
}

func backupArtifactNames(artifacts []backupArtifact) []string {
	names := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		names = append(names, artifact.name)
	}
	return names
}

func reportBackupProgress(progress VMBackupProgress, percent int, step string) {
	if progress != nil {
		progress(percent, step)
	}
}
