package libvirtbackend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"virtmgr/internal/domain"
	"virtmgr/internal/version"

	libvirt "libvirt.org/go/libvirt"
)

type backupArtifact struct {
	name          string
	format        string
	capacityBytes uint64
	sourceVolume  string
	target        string
}

type backupManifest struct {
	FormatVersion string                 `json:"formatVersion"`
	BackupID      string                 `json:"backupId,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
	ToolVersion   string                 `json:"toolVersion,omitempty"`
	Source        *backupManifestSource  `json:"source,omitempty"`
	Config        *backupManifestConfig  `json:"config,omitempty"`
	Restore       *backupManifestRestore `json:"restore,omitempty"`
	VMUUID        string                 `json:"vmUuid,omitempty"`
	VMName        string                 `json:"vmName,omitempty"`
	ConfigVolume  string                 `json:"configVolume,omitempty"`
	ConfigSHA256  string                 `json:"configSha256,omitempty"`
	Disks         []domain.VMBackupDisk  `json:"disks"`
}

type backupManifestSource struct {
	VMUUID     string         `json:"vmUuid"`
	VMName     string         `json:"vmName"`
	State      domain.VMState `json:"state"`
	Persistent bool           `json:"persistent"`
}

type backupManifestConfig struct {
	Artifact  string `json:"artifact"`
	SizeBytes uint64 `json:"sizeBytes"`
	SHA256    string `json:"sha256"`
}

type backupManifestRestore struct {
	DefaultTargetPool string `json:"defaultTargetPool"`
	RequiresNewMAC    bool   `json:"requiresNewMac"`
	ContainsMemory    bool   `json:"containsMemory"`
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
			name:          backupVolumeName(backupPrefix(spec), fmt.Sprintf("-disk-%02d%s", len(artifacts), backupExtension(sourceName, format))),
			format:        format,
			capacityBytes: info.Capacity,
			sourceVolume:  sourceName,
			target:        disk.target,
		})
	}
	configName := backupVolumeName(backupPrefix(spec), "-domain.xml")
	manifestName := backupVolumeName(backupPrefix(spec), "-manifest.json")
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
			SourceVolume: artifact.sourceVolume, BackupVolume: artifact.name, Target: artifact.target,
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
	backupID := spec.BackupID
	if backupID == "" {
		backupID = spec.Name
	}
	manifest, err := json.MarshalIndent(backupManifest{
		FormatVersion: domain.VMBackupFormatVersionV2,
		BackupID:      backupID,
		CreatedAt:     createdAt,
		ToolVersion:   version.Version,
		Source:        &backupManifestSource{VMUUID: uuid, VMName: vmName, State: domain.VMStateShutoff, Persistent: true},
		Config:        &backupManifestConfig{Artifact: configName, SizeBytes: uint64(len(configBytes)), SHA256: hex.EncodeToString(configSum[:])},
		Restore:       &backupManifestRestore{DefaultTargetPool: spec.TargetPool, RequiresNewMAC: true, ContainsMemory: false},
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
		BackupID:       backupID,
		FormatVersion:  domain.VMBackupFormatVersionV2,
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

func (c *Client) VerifyVMBackup(ctx context.Context, backup domain.Backup, deep bool) (domain.BackupVerification, error) {
	if err := ctx.Err(); err != nil {
		return domain.BackupVerification{}, err
	}
	conn, err := libvirt.NewConnect(c.uri)
	if err != nil {
		return domain.BackupVerification{}, fmt.Errorf("connect libvirt %s for backup verification: %w", c.uri, err)
	}
	defer func() { _, _ = conn.Close() }()
	pool, err := lookupRunningStoragePool(conn, backup.TargetPool)
	if err != nil {
		return domain.BackupVerification{}, err
	}
	defer func() { _ = pool.Free() }()
	payload, err := loadBackupPayload(ctx, conn, pool, backup)
	if err != nil {
		return domain.BackupVerification{
			BackupID: backup.ID, Status: domain.BackupVerifyFailed, Deep: deep,
			CheckedAt: time.Now().UTC(), Message: err.Error(),
		}, nil
	}
	checks := []domain.BackupArtifactCheck{payload.manifestCheck, payload.configCheck}
	checks = append(checks, payload.diskChecks...)
	status := domain.BackupVerifyVerified
	message := "备份清单、配置和磁盘产物完整"
	for _, check := range checks {
		if !check.Exists || check.Message != "" {
			status = domain.BackupVerifyFailed
			message = "备份校验失败，请查看产物检查结果"
			break
		}
	}
	if deep {
		message = "深度校验通过：产物完整且磁盘容量与格式匹配"
	}
	return domain.BackupVerification{
		BackupID: backup.ID, Status: status, Deep: deep, CheckedAt: time.Now().UTC(),
		Message: message, Artifacts: checks,
	}, nil
}

func (c *Client) DeleteVMBackup(ctx context.Context, backup domain.Backup) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	conn, err := libvirt.NewConnect(c.uri)
	if err != nil {
		return fmt.Errorf("connect libvirt %s for backup deletion: %w", c.uri, err)
	}
	defer func() { _, _ = conn.Close() }()
	pool, err := lookupRunningStoragePool(conn, backup.TargetPool)
	if err != nil {
		return err
	}
	defer func() { _ = pool.Free() }()
	names := make([]string, 0, len(backup.Disks)+2)
	for _, disk := range backup.Disks {
		names = append(names, disk.Artifact)
	}
	if backup.ConfigVolume != "" {
		names = append(names, backup.ConfigVolume)
	}
	if backup.ManifestVolume != "" {
		names = append(names, backup.ManifestVolume)
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if err := ctx.Err(); err != nil {
			return err
		}
		volume, lookupErr := pool.LookupStorageVolByName(name)
		if errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
			continue
		}
		if lookupErr != nil {
			return fmt.Errorf("lookup backup artifact %s: %w", name, lookupErr)
		}
		deleteErr := volume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
		_ = volume.Free()
		if deleteErr != nil && !errors.Is(deleteErr, libvirt.ERR_NO_STORAGE_VOL) {
			return fmt.Errorf("delete backup artifact %s: %w", name, deleteErr)
		}
	}
	return nil
}

func (c *Client) RestoreVMBackup(ctx context.Context, backup domain.Backup, spec domain.VMRestoreSpec, progress VMBackupProgress) (domain.VM, error) {
	if err := ctx.Err(); err != nil {
		return domain.VM{}, err
	}
	conn, err := libvirt.NewConnect(c.uri)
	if err != nil {
		return domain.VM{}, fmt.Errorf("connect libvirt %s for backup restore: %w", c.uri, err)
	}
	defer func() { _, _ = conn.Close() }()
	reportBackupProgress(progress, 3, "正在校验备份集")
	sourcePool, err := lookupRunningStoragePool(conn, backup.TargetPool)
	if err != nil {
		return domain.VM{}, err
	}
	defer func() { _ = sourcePool.Free() }()
	payload, err := loadBackupPayload(ctx, conn, sourcePool, backup)
	if err != nil {
		return domain.VM{}, err
	}
	if !payload.valid() {
		return domain.VM{}, fmt.Errorf("%w: backup artifacts are incomplete", domain.ErrInvalidInput)
	}
	if existing, lookupErr := conn.LookupDomainByName(spec.Name); lookupErr == nil {
		_ = existing.Free()
		return domain.VM{}, domain.ErrConflict
	} else if !errors.Is(lookupErr, libvirt.ERR_NO_DOMAIN) {
		return domain.VM{}, fmt.Errorf("check restored domain name %s: %w", spec.Name, lookupErr)
	}
	targetPool, err := lookupRunningStoragePool(conn, spec.TargetPool)
	if err != nil {
		return domain.VM{}, err
	}
	defer func() { _ = targetPool.Free() }()
	targetInfo, err := targetPool.GetInfo()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read restore target pool %s info: %w", spec.TargetPool, err)
	}
	requiredBytes := uint64(0)
	for _, disk := range payload.manifest.Disks {
		requiredBytes += disk.CapacityBytes
	}
	if requiredBytes > 0 && targetInfo.Available > 0 && requiredBytes > targetInfo.Available {
		return domain.VM{}, errors.Join(domain.ErrInvalidInput, errors.New("restore target pool does not have enough available capacity"))
	}

	sourceDisks, err := cloneDisksFromDomainXML(string(payload.configXML))
	if err != nil {
		return domain.VM{}, err
	}
	if len(sourceDisks) != len(payload.manifest.Disks) {
		return domain.VM{}, fmt.Errorf("%w: backup manifest disk count does not match domain XML", domain.ErrInvalidInput)
	}
	createdVolumes := make([]*libvirt.StorageVol, 0, len(sourceDisks))
	replacedSources := make(map[string]string, len(sourceDisks))
	var targetDomain *libvirt.Domain
	committed := false
	defer func() {
		if targetDomain != nil {
			if !committed {
				_ = targetDomain.Undefine()
			}
			_ = targetDomain.Free()
		}
		for _, volume := range createdVolumes {
			if !committed {
				_ = volume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			}
			_ = volume.Free()
		}
	}()

	for index, manifestDisk := range payload.manifest.Disks {
		if err := ctx.Err(); err != nil {
			return domain.VM{}, err
		}
		reportBackupProgress(progress, 10+index*65/maxInt(1, len(payload.manifest.Disks)), fmt.Sprintf("正在恢复磁盘 %d/%d", index+1, len(payload.manifest.Disks)))
		sourceVolume, err := sourcePool.LookupStorageVolByName(manifestDisk.BackupVolume)
		if err != nil {
			if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
				return domain.VM{}, domain.ErrNotFound
			}
			return domain.VM{}, fmt.Errorf("lookup backup disk %s: %w", manifestDisk.BackupVolume, err)
		}
		format, err := cloneDiskFormat(manifestDisk.Format, manifestDisk.BackupVolume)
		if err != nil {
			_ = sourceVolume.Free()
			return domain.VM{}, err
		}
		targetName := cloneVolumeName(spec.Name, index, manifestDisk.BackupVolume, format)
		if existing, lookupErr := targetPool.LookupStorageVolByName(targetName); lookupErr == nil {
			_ = existing.Free()
			_ = sourceVolume.Free()
			return domain.VM{}, domain.ErrConflict
		} else if !errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
			_ = sourceVolume.Free()
			return domain.VM{}, fmt.Errorf("check restore target volume %s: %w", targetName, lookupErr)
		}
		targetVolume, err := targetPool.StorageVolCreateXMLFrom(buildCloneVolumeXML(targetName, manifestDisk.CapacityBytes, format), sourceVolume, 0)
		_ = sourceVolume.Free()
		if err != nil {
			if errors.Is(err, libvirt.ERR_STORAGE_VOL_EXIST) {
				return domain.VM{}, domain.ErrConflict
			}
			return domain.VM{}, fmt.Errorf("restore disk volume %s: %w", targetName, err)
		}
		targetPath, err := targetVolume.GetPath()
		if err != nil {
			_ = targetVolume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			_ = targetVolume.Free()
			return domain.VM{}, fmt.Errorf("read restored volume path %s: %w", targetName, err)
		}
		createdVolumes = append(createdVolumes, targetVolume)
		replacedSources[sourceDisks[index].source] = targetPath
	}

	reportBackupProgress(progress, 82, "正在生成恢复后的虚拟机定义")
	targetXML, err := rewriteRestoreDomainXML(string(payload.configXML), spec.Name, replacedSources, spec.PreservePassthrough)
	if err != nil {
		return domain.VM{}, err
	}
	targetDomain, err = conn.DomainDefineXML(targetXML)
	if err != nil {
		return domain.VM{}, fmt.Errorf("define restored domain %s: %w", spec.Name, err)
	}
	if spec.Autostart {
		if err := targetDomain.SetAutostart(true); err != nil {
			return domain.VM{}, fmt.Errorf("set restored domain autostart: %w", err)
		}
	}
	if spec.Start {
		reportBackupProgress(progress, 95, "正在启动恢复的虚拟机")
		if err := targetDomain.Create(); err != nil {
			return domain.VM{}, fmt.Errorf("start restored domain %s: %w", spec.Name, err)
		}
	}
	vm, err := domainFromLibvirt(targetDomain)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&vm)
	committed = true
	reportBackupProgress(progress, 100, "恢复完成")
	return vm, nil
}

type backupPayload struct {
	manifest      backupManifest
	configXML     []byte
	manifestCheck domain.BackupArtifactCheck
	configCheck   domain.BackupArtifactCheck
	diskChecks    []domain.BackupArtifactCheck
}

func (p backupPayload) valid() bool {
	if !p.manifestCheck.Exists || !p.configCheck.Exists || p.configCheck.Message != "" {
		return false
	}
	for _, check := range p.diskChecks {
		if !check.Exists {
			return false
		}
	}
	return true
}

func loadBackupPayload(ctx context.Context, conn *libvirt.Connect, pool *libvirt.StoragePool, backup domain.Backup) (backupPayload, error) {
	payload := backupPayload{}
	manifestBytes, manifestInfo, err := readBackupVolume(ctx, conn, pool, backup.ManifestVolume, 8<<20)
	if err != nil {
		return payload, err
	}
	payload.manifestCheck = domain.BackupArtifactCheck{Name: backup.ManifestVolume, Kind: "manifest", Exists: true, ExpectedBytes: manifestInfo.Capacity}
	if err := json.Unmarshal(manifestBytes, &payload.manifest); err != nil {
		return payload, fmt.Errorf("decode backup manifest: %w", err)
	}
	normalizeBackupManifest(&payload.manifest, backup)
	configBytes, configInfo, err := readBackupVolume(ctx, conn, pool, payload.manifest.ConfigVolume, 8<<20)
	if err != nil {
		return payload, err
	}
	payload.configXML = configBytes
	configSum := sha256.Sum256(configBytes)
	payload.configCheck = domain.BackupArtifactCheck{Name: payload.manifest.ConfigVolume, Kind: "config", Exists: true, ExpectedBytes: configInfo.Capacity}
	if payload.manifest.ConfigSHA256 != "" && !strings.EqualFold(hex.EncodeToString(configSum[:]), payload.manifest.ConfigSHA256) {
		payload.configCheck.Message = "配置 SHA-256 与备份清单不一致"
	}
	payload.diskChecks = make([]domain.BackupArtifactCheck, 0, len(payload.manifest.Disks))
	for _, disk := range payload.manifest.Disks {
		if err := ctx.Err(); err != nil {
			return payload, err
		}
		volume, lookupErr := pool.LookupStorageVolByName(disk.BackupVolume)
		if errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
			payload.diskChecks = append(payload.diskChecks, domain.BackupArtifactCheck{Name: disk.BackupVolume, Kind: "disk", Exists: false, Message: "备份磁盘产物不存在"})
			continue
		}
		if lookupErr != nil {
			return payload, fmt.Errorf("lookup backup disk %s: %w", disk.BackupVolume, lookupErr)
		}
		info, infoErr := volume.GetInfo()
		xmlText, xmlErr := volume.GetXMLDesc(0)
		_ = volume.Free()
		check := domain.BackupArtifactCheck{Name: disk.BackupVolume, Kind: "disk", Exists: true, Format: disk.Format, CapacityBytes: disk.CapacityBytes}
		if infoErr != nil {
			check.Message = infoErr.Error()
		} else if disk.CapacityBytes > info.Capacity {
			check.Message = "备份磁盘容量小于清单记录"
		} else if xmlErr == nil {
			var parsed storageVolumeXML
			if err := xml.Unmarshal([]byte(xmlText), &parsed); err == nil && parsed.Target.Format.Type != "" && !strings.EqualFold(parsed.Target.Format.Type, disk.Format) {
				check.Message = "备份磁盘格式与清单记录不一致"
			}
		}
		payload.diskChecks = append(payload.diskChecks, check)
	}
	return payload, nil
}

func normalizeBackupManifest(manifest *backupManifest, backup domain.Backup) {
	if manifest.Config != nil && manifest.Config.Artifact != "" {
		manifest.ConfigVolume = manifest.Config.Artifact
		manifest.ConfigSHA256 = manifest.Config.SHA256
	}
	if manifest.Source != nil {
		if manifest.VMUUID == "" {
			manifest.VMUUID = manifest.Source.VMUUID
		}
		if manifest.VMName == "" {
			manifest.VMName = manifest.Source.VMName
		}
	}
	if manifest.ConfigVolume == "" {
		manifest.ConfigVolume = backup.ConfigVolume
	}
	if manifest.BackupID == "" {
		manifest.BackupID = backup.ID
	}
}

func lookupRunningStoragePool(conn *libvirt.Connect, name string) (*libvirt.StoragePool, error) {
	pool, err := conn.LookupStoragePoolByName(name)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("lookup storage pool %s: %w", name, err)
	}
	info, err := pool.GetInfo()
	if err != nil {
		_ = pool.Free()
		return nil, fmt.Errorf("read storage pool %s info: %w", name, err)
	}
	if info.State != libvirt.STORAGE_POOL_RUNNING {
		_ = pool.Free()
		return nil, domain.ErrUnavailable
	}
	return pool, nil
}

func readBackupVolume(ctx context.Context, conn *libvirt.Connect, pool *libvirt.StoragePool, name string, limit uint64) ([]byte, *libvirt.StorageVolInfo, error) {
	volume, err := pool.LookupStorageVolByName(name)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
			return nil, nil, domain.ErrNotFound
		}
		return nil, nil, fmt.Errorf("lookup backup artifact %s: %w", name, err)
	}
	defer func() { _ = volume.Free() }()
	info, err := volume.GetInfo()
	if err != nil {
		return nil, nil, fmt.Errorf("read backup artifact %s info: %w", name, err)
	}
	if info.Capacity > limit {
		return nil, nil, fmt.Errorf("%w: backup artifact %s is too large", domain.ErrTooLarge, name)
	}
	stream, err := conn.NewStream(0)
	if err != nil {
		return nil, nil, fmt.Errorf("open backup artifact stream %s: %w", name, err)
	}
	defer func() { _ = stream.Free() }()
	if err := volume.Download(stream, 0, 0, 0); err != nil {
		return nil, nil, fmt.Errorf("download backup artifact %s: %w", name, err)
	}
	data := make([]byte, 0, info.Capacity)
	buffer := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			_ = stream.Abort()
			return nil, nil, err
		}
		count, recvErr := stream.Recv(buffer)
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			_ = stream.Abort()
			return nil, nil, fmt.Errorf("read backup artifact %s: %w", name, recvErr)
		}
		if count == -2 || count == -3 {
			continue
		}
		data = append(data, buffer[:count]...)
		if uint64(len(data)) > limit {
			_ = stream.Abort()
			return nil, nil, domain.ErrTooLarge
		}
	}
	if err := stream.Finish(); err != nil {
		return nil, nil, fmt.Errorf("finish backup artifact stream %s: %w", name, err)
	}
	return data, info, nil
}

func rewriteRestoreDomainXML(sourceXML, name string, replacedSources map[string]string, preservePassthrough bool) (string, error) {
	rewritten, err := rewriteCloneDomainXML(sourceXML, name, replacedSources)
	if err != nil || preservePassthrough {
		return rewritten, err
	}
	decoder := xml.NewDecoder(strings.NewReader(rewritten))
	var output strings.Builder
	encoder := xml.NewEncoder(&output)
	stack := make([]string, 0, 16)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read restored domain xml: %w", err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			if parent == "devices" && element.Name.Local == "hostdev" {
				if err := skipXMLElement(decoder, "hostdev"); err != nil {
					return "", err
				}
				continue
			}
			if err := encoder.EncodeToken(element); err != nil {
				return "", err
			}
			stack = append(stack, element.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if err := encoder.EncodeToken(element); err != nil {
				return "", err
			}
		default:
			if err := encoder.EncodeToken(token); err != nil {
				return "", err
			}
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", fmt.Errorf("encode restored domain xml: %w", err)
	}
	return output.String(), nil
}

func backupPrefix(spec domain.VMBackupSpec) string {
	if value := strings.TrimSpace(spec.BackupID); value != "" {
		return value
	}
	return spec.Name
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
