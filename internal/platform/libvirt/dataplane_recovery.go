package libvirtbackend

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

type recoveryVolume struct {
	Pool    string
	Name    string
	Path    string
	Backing string
}

type recoveryDomain struct {
	Name string
	UUID string
}

func (c *Client) RecoverDataPlaneOperation(ctx context.Context, operation domain.DataPlaneOperation) (domain.DataPlaneRecovery, error) {
	if err := ctx.Err(); err != nil {
		return domain.DataPlaneRecovery{}, err
	}
	conn, err := libvirt.NewConnect(c.uri)
	if err != nil {
		return domain.DataPlaneRecovery{}, fmt.Errorf("connect libvirt %s for data plane recovery: %w", c.uri, err)
	}
	defer func() { _, _ = conn.Close() }()

	switch operation.Kind {
	case domain.DataPlaneOperationBackup:
		return recoverBackupOperation(ctx, conn, operation)
	case domain.DataPlaneOperationBackupRestore:
		return recoverBackupRestore(ctx, conn, operation)
	case domain.DataPlaneOperationClone:
		return recoverCloneOperation(ctx, conn, operation)
	case domain.DataPlaneOperationMigrate:
		return recoverDiskMigration(ctx, conn, operation)
	case domain.DataPlaneOperationImport:
		return recoverImportOperation(ctx, conn, operation)
	case domain.DataPlaneOperationConvert, domain.DataPlaneOperationFlatten:
		return recoverStorageCopy(ctx, conn, operation)
	default:
		return recoveryNeedsAttention("不支持自动恢复该操作类型"), nil
	}
}

type recoveryTargetVolume struct {
	Pool string
	Name string
}

func recoverCloneOperation(ctx context.Context, conn *libvirt.Connect, operation domain.DataPlaneOperation) (domain.DataPlaneRecovery, error) {
	if strings.TrimSpace(operation.VMName) == "" {
		return recoveryNeedsAttention("克隆恢复日志缺少目标虚拟机名称"), nil
	}
	exists, err := recoveryDomainExists(conn, operation.VMName)
	if err != nil {
		return domain.DataPlaneRecovery{}, err
	}
	if exists {
		return recoveryCompleted("克隆目标虚拟机已定义，克隆结果已生效"), nil
	}
	if strings.TrimSpace(operation.VMUUID) == "" {
		return recoveryNeedsAttention("克隆恢复日志缺少源虚拟机 UUID"), nil
	}
	source, err := conn.LookupDomainByUUIDString(operation.VMUUID)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_DOMAIN) {
			return recoveryNeedsAttention("源虚拟机已不存在，无法推断克隆目标卷"), nil
		}
		return domain.DataPlaneRecovery{}, fmt.Errorf("lookup clone source domain during recovery: %w", err)
	}
	defer func() { _ = source.Free() }()
	sourceXML, err := source.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE)
	if err != nil {
		return domain.DataPlaneRecovery{}, fmt.Errorf("read clone source XML during recovery: %w", err)
	}
	disks, err := cloneDisksFromDomainXML(sourceXML)
	if err != nil {
		return domain.DataPlaneRecovery{}, err
	}
	targets := make([]recoveryTargetVolume, 0, len(disks))
	for index, disk := range disks {
		target, err := recoveryCloneTarget(conn, operation, disk, index)
		if err != nil {
			return recoveryNeedsAttention(err.Error()), nil
		}
		targets = append(targets, target)
	}
	return cleanupRecoveryTargets(ctx, conn, targets, "未完成克隆的孤立磁盘已回收")
}

func recoveryCloneTarget(conn *libvirt.Connect, operation domain.DataPlaneOperation, disk cloneDisk, index int) (recoveryTargetVolume, error) {
	sourceVolume, err := conn.LookupStorageVolByPath(disk.source)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
			return recoveryTargetVolume{}, fmt.Errorf("克隆源磁盘 %s 已不在存储池中", disk.source)
		}
		return recoveryTargetVolume{}, fmt.Errorf("lookup clone source volume %s during recovery: %w", disk.source, err)
	}
	sourceName, nameErr := sourceVolume.GetName()
	sourcePool, poolErr := sourceVolume.LookupPoolByVolume()
	_ = sourceVolume.Free()
	if nameErr != nil || poolErr != nil {
		return recoveryTargetVolume{}, fmt.Errorf("inspect clone source volume during recovery: %w", errors.Join(nameErr, poolErr))
	}
	poolName, poolNameErr := sourcePool.GetName()
	_ = sourcePool.Free()
	if poolNameErr != nil {
		return recoveryTargetVolume{}, fmt.Errorf("read clone source pool name during recovery: %w", poolNameErr)
	}
	format, err := cloneDiskFormat(disk.format, sourceName)
	if err != nil {
		return recoveryTargetVolume{}, err
	}
	targetPool := strings.TrimSpace(operation.TargetPool)
	if targetPool == "" {
		targetPool = poolName
	}
	return recoveryTargetVolume{Pool: targetPool, Name: cloneVolumeName(operation.VMName, index, sourceName, format)}, nil
}

func recoverBackupRestore(ctx context.Context, conn *libvirt.Connect, operation domain.DataPlaneOperation) (domain.DataPlaneRecovery, error) {
	if strings.TrimSpace(operation.VMName) == "" || strings.TrimSpace(operation.TargetPool) == "" {
		return recoveryNeedsAttention("备份恢复日志缺少目标虚拟机名称或目标存储池"), nil
	}
	exists, err := recoveryDomainExists(conn, operation.VMName)
	if err != nil {
		return domain.DataPlaneRecovery{}, err
	}
	if exists {
		return recoveryCompleted("恢复出的虚拟机已定义，备份恢复结果已生效"), nil
	}
	if strings.TrimSpace(operation.SourcePool) == "" || strings.TrimSpace(operation.SourceVolume) == "" {
		return recoveryNeedsAttention("备份恢复日志缺少备份清单位置"), nil
	}
	pool, err := conn.LookupStoragePoolByName(operation.SourcePool)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return recoveryNeedsAttention("备份存储池已不存在，无法读取恢复卷清单"), nil
		}
		return domain.DataPlaneRecovery{}, fmt.Errorf("lookup restore source pool during recovery: %w", err)
	}
	defer func() { _ = pool.Free() }()
	manifestBytes, _, err := readBackupVolume(ctx, conn, pool, operation.SourceVolume, 8<<20)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return recoveryNeedsAttention("备份清单已不存在，无法推断恢复目标卷"), nil
		}
		return domain.DataPlaneRecovery{}, fmt.Errorf("read restore manifest during recovery: %w", err)
	}
	var manifest backupManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return recoveryNeedsAttention("备份清单无法解析，无法推断恢复目标卷"), nil
	}
	targets, err := backupRestoreRecoveryTargets(operation, manifest)
	if err != nil {
		return recoveryNeedsAttention(err.Error()), nil
	}
	return cleanupRecoveryTargets(ctx, conn, targets, "未完成备份恢复的孤立磁盘已回收")
}

func backupRestoreRecoveryTargets(operation domain.DataPlaneOperation, manifest backupManifest) ([]recoveryTargetVolume, error) {
	targets := make([]recoveryTargetVolume, 0, len(manifest.Disks))
	for index, disk := range manifest.Disks {
		format, err := cloneDiskFormat(disk.Format, disk.BackupVolume)
		if err != nil {
			return nil, err
		}
		targets = append(targets, recoveryTargetVolume{
			Pool: operation.TargetPool,
			Name: cloneVolumeName(operation.VMName, index, disk.BackupVolume, format),
		})
	}
	return targets, nil
}

func recoveryDomainExists(conn *libvirt.Connect, name string) (bool, error) {
	dom, err := conn.LookupDomainByName(strings.TrimSpace(name))
	if err == nil {
		_ = dom.Free()
		return true, nil
	}
	if errors.Is(err, libvirt.ERR_NO_DOMAIN) {
		return false, nil
	}
	return false, fmt.Errorf("lookup recovery target domain %s: %w", name, err)
}

func cleanupRecoveryTargets(ctx context.Context, conn *libvirt.Connect, targets []recoveryTargetVolume, message string) (domain.DataPlaneRecovery, error) {
	if len(targets) == 0 {
		return recoveryNoop("没有需要回收的目标磁盘"), nil
	}
	cleaned := 0
	references := make([]string, 0)
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return domain.DataPlaneRecovery{}, err
		}
		pool, err := conn.LookupStoragePoolByName(target.Pool)
		if err != nil {
			if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
				continue
			}
			return domain.DataPlaneRecovery{}, fmt.Errorf("lookup recovery target pool %s: %w", target.Pool, err)
		}
		info, infoErr := pool.GetInfo()
		if infoErr != nil {
			_ = pool.Free()
			return domain.DataPlaneRecovery{}, fmt.Errorf("read recovery target pool %s: %w", target.Pool, infoErr)
		}
		if info.State != libvirt.STORAGE_POOL_RUNNING {
			_ = pool.Free()
			return recoveryNeedsAttention("目标存储池未运行，无法确认恢复磁盘引用状态"), nil
		}
		volume, lookupErr := pool.LookupStorageVolByName(target.Name)
		_ = pool.Free()
		if errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
			continue
		}
		if lookupErr != nil {
			return domain.DataPlaneRecovery{}, fmt.Errorf("lookup recovery target volume %s/%s: %w", target.Pool, target.Name, lookupErr)
		}
		path, pathErr := volume.GetPath()
		_ = volume.Free()
		if pathErr != nil {
			return domain.DataPlaneRecovery{}, fmt.Errorf("read recovery target volume path %s/%s: %w", target.Pool, target.Name, pathErr)
		}
		refs, refErr := recoveryDomainsUsingVolume(conn, path, target.Name)
		if refErr != nil {
			return domain.DataPlaneRecovery{}, refErr
		}
		if len(refs) > 0 {
			for _, ref := range refs {
				references = append(references, target.Pool+"/"+target.Name+" -> "+ref.Name)
			}
			continue
		}
		if err := deleteRecoveryVolumeIfUnreferenced(ctx, conn, target.Pool, target.Name, path); err != nil {
			return recoveryNeedsAttention(err.Error()), nil
		}
		cleaned++
	}
	if len(references) > 0 {
		return recoveryNeedsAttention("恢复目标磁盘仍被虚拟机引用：" + strings.Join(references, ", ")), nil
	}
	if cleaned == 0 {
		return recoveryNoop("未发现需要回收的恢复磁盘"), nil
	}
	return recoveryRolledBack(message), nil
}

func recoverBackupOperation(ctx context.Context, conn *libvirt.Connect, operation domain.DataPlaneOperation) (domain.DataPlaneRecovery, error) {
	if operation.Online {
		if err := reconcileOnlineBackup(ctx, conn, operation); err != nil {
			return recoveryNeedsAttention(err.Error()), nil
		}
	}
	completed, err := reconcileBackupArtifacts(ctx, conn, operation)
	if err != nil {
		return recoveryNeedsAttention(err.Error()), nil
	}
	if completed != nil {
		return domain.DataPlaneRecovery{
			Status:          domain.DataPlaneRecoveryCompleted,
			Message:         "在线备份产物完整，已恢复为成功备份",
			CompletedBackup: completed,
		}, nil
	}
	return recoveryCleaned("未完成的备份产物已回收"), nil
}

func reconcileOnlineBackup(ctx context.Context, conn *libvirt.Connect, operation domain.DataPlaneOperation) error {
	if strings.TrimSpace(operation.BackupID) == "" {
		return errors.New("在线备份操作缺少备份 ID")
	}
	dom, err := conn.LookupDomainByUUIDString(operation.VMUUID)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_DOMAIN) {
			return cleanupOrphanBackupOverlays(ctx, conn, operation.BackupID)
		}
		return fmt.Errorf("lookup source domain for online backup recovery: %w", err)
	}
	defer func() { _ = dom.Free() }()

	stateCode, _, err := dom.GetState()
	if err != nil {
		return fmt.Errorf("read source domain state during online backup recovery: %w", err)
	}
	configXML, err := dom.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE)
	if err != nil {
		return fmt.Errorf("read inactive domain XML during online backup recovery: %w", err)
	}
	activeXML := configXML
	if stateCode != libvirt.DOMAIN_SHUTOFF && stateCode != libvirt.DOMAIN_CRASHED {
		activeXML, err = dom.GetXMLDesc(0)
		if err != nil {
			return fmt.Errorf("read active domain XML during online backup recovery: %w", err)
		}
	}
	configDisks, err := cloneDisksFromDomainXML(configXML)
	if err != nil {
		return err
	}
	activeDisks, err := cloneDisksFromDomainXML(activeXML)
	if err != nil {
		return err
	}
	activeByTarget := make(map[string]cloneDisk, len(activeDisks))
	for _, disk := range activeDisks {
		activeByTarget[disk.target] = disk
	}

	pending := make([]onlineBackupDisk, 0)
	for index, disk := range configDisks {
		if err := ctx.Err(); err != nil {
			return err
		}
		overlayName := onlineBackupOverlayName(operation.BackupID, index)
		overlay, err := findRecoveryVolume(conn, overlayName)
		if err != nil {
			return err
		}
		if overlay == nil {
			baseSource := filepath.Clean(strings.TrimSpace(disk.source))
			activeSource := filepath.Clean(strings.TrimSpace(activeByTarget[disk.target].source))
			if baseSource != "" && activeSource != "" && activeSource != baseSource {
				return fmt.Errorf("磁盘 %s 的备份 overlay 已缺失，但活动配置仍引用 %s，无法安全自动恢复", disk.target, activeSource)
			}
			continue
		}
		baseSource := filepath.Clean(strings.TrimSpace(disk.source))
		if baseSource == "" {
			return fmt.Errorf("磁盘 %s 缺少基础卷路径，无法自动恢复", disk.target)
		}
		if filepath.Clean(overlay.Path) == baseSource {
			if strings.TrimSpace(overlay.Backing) == "" {
				return fmt.Errorf("磁盘 %s 的在线备份 overlay 缺少基础卷信息", disk.target)
			}
			baseSource = filepath.Clean(overlay.Backing)
		}
		active := activeByTarget[disk.target]
		if filepath.Clean(strings.TrimSpace(active.source)) == filepath.Clean(strings.TrimSpace(overlay.Path)) {
			if stateCode != libvirt.DOMAIN_RUNNING {
				return fmt.Errorf("虚拟机已停机且磁盘 %s 仍引用在线备份 overlay，需要人工完成 block commit", disk.target)
			}
			pending = append(pending, onlineBackupDisk{
				target: disk.target, source: baseSource,
				overlay: overlayName, sourcePool: overlay.Pool,
			})
			continue
		}
		if filepath.Clean(strings.TrimSpace(active.source)) != baseSource {
			return fmt.Errorf("磁盘 %s 当前来源既不是基础卷也不是备份 overlay，无法安全自动恢复", disk.target)
		}
	}

	if len(pending) > 0 {
		snapshotState := &onlineBackupSnapshot{
			name: onlineBackupSnapshotName(operation.BackupID), disks: pending,
			joinedTargets: make(map[string]bool, len(pending)),
		}
		if snapshot, lookupErr := dom.SnapshotLookupByName(snapshotState.name, 0); lookupErr == nil {
			snapshotState.snapshot = snapshot
		} else if !errors.Is(lookupErr, libvirt.ERR_NO_DOMAIN_SNAPSHOT) {
			return fmt.Errorf("lookup online backup snapshot metadata: %w", lookupErr)
		}
		if err := snapshotState.joinWithTimeout(ctx, conn, dom, 30*time.Minute); err != nil {
			return fmt.Errorf("resume online backup block commit: %w", err)
		}
	} else if err := deleteOnlineBackupSnapshotMetadata(dom, onlineBackupSnapshotName(operation.BackupID)); err != nil {
		return err
	}
	return cleanupOrphanBackupOverlays(ctx, conn, operation.BackupID)
}

func cleanupOrphanBackupOverlays(ctx context.Context, conn *libvirt.Connect, backupID string) error {
	prefix := "virtmgr-backup-" + strings.TrimSpace(backupID)
	pools, err := conn.ListAllStoragePools(libvirt.CONNECT_LIST_STORAGE_POOLS_ACTIVE)
	if err != nil {
		return fmt.Errorf("list active storage pools for online backup recovery: %w", err)
	}
	defer func() {
		for index := range pools {
			_ = pools[index].Free()
		}
	}()
	for index := range pools {
		pool := &pools[index]
		poolName, nameErr := pool.GetName()
		if nameErr != nil {
			return fmt.Errorf("read storage pool name during online backup recovery: %w", nameErr)
		}
		volumes, listErr := pool.ListAllStorageVolumes(0)
		if listErr != nil {
			return fmt.Errorf("list storage volumes in %s during online backup recovery: %w", poolName, listErr)
		}
		for volumeIndex := range volumes {
			volume := &volumes[volumeIndex]
			name, volumeNameErr := volume.GetName()
			if volumeNameErr != nil {
				_ = volume.Free()
				return fmt.Errorf("read storage volume name during online backup recovery: %w", volumeNameErr)
			}
			if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, "-overlay.qcow2") {
				_ = volume.Free()
				continue
			}
			path, pathErr := volume.GetPath()
			_ = volume.Free()
			if pathErr != nil {
				return fmt.Errorf("read orphan overlay path %s: %w", name, pathErr)
			}
			if err := deleteRecoveryVolumeIfUnreferenced(ctx, conn, poolName, name, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func deleteOnlineBackupSnapshotMetadata(dom *libvirt.Domain, name string) error {
	snapshot, err := dom.SnapshotLookupByName(name, 0)
	if errors.Is(err, libvirt.ERR_NO_DOMAIN_SNAPSHOT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lookup online backup snapshot metadata: %w", err)
	}
	defer func() { _ = snapshot.Free() }()
	if err := snapshot.Delete(libvirt.DOMAIN_SNAPSHOT_DELETE_METADATA_ONLY); err != nil && !errors.Is(err, libvirt.ERR_NO_DOMAIN_SNAPSHOT) {
		return fmt.Errorf("delete online backup snapshot metadata: %w", err)
	}
	return nil
}

func reconcileBackupArtifacts(ctx context.Context, conn *libvirt.Connect, operation domain.DataPlaneOperation) (*domain.VMBackup, error) {
	if strings.TrimSpace(operation.TargetPool) == "" || strings.TrimSpace(operation.BackupID) == "" {
		return nil, errors.New("备份恢复日志缺少目标池或备份 ID")
	}
	pool, err := lookupRunningStoragePool(conn, operation.TargetPool)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup backup target pool during recovery: %w", err)
	}
	defer func() { _ = pool.Free() }()
	manifestName := backupVolumeName(operation.BackupID, "-manifest.json")
	configName := backupVolumeName(operation.BackupID, "-domain.xml")
	backup := domain.Backup{
		ID: operation.BackupID, TargetPool: operation.TargetPool, VMUUID: operation.VMUUID,
		VMName: operation.VMName, ManifestVolume: manifestName, ConfigVolume: configName,
	}
	payload, loadErr := loadBackupPayload(ctx, conn, pool, backup)
	if loadErr == nil && payload.valid() {
		return backupFromRecoveredPayload(operation, payload), nil
	}
	if err := deleteBackupArtifacts(ctx, conn, pool, operation.TargetPool, operation.BackupID); err != nil {
		return nil, err
	}
	return nil, nil
}

func backupFromRecoveredPayload(operation domain.DataPlaneOperation, payload backupPayload) *domain.VMBackup {
	name := operation.VMName
	if payload.manifest.Source != nil && payload.manifest.Source.VMName != "" {
		name = payload.manifest.Source.VMName
	}
	disks := make([]domain.VMBackupDisk, 0, len(payload.manifest.Disks))
	totalBytes := uint64(len(payload.configXML))
	for _, disk := range payload.manifest.Disks {
		disks = append(disks, domain.VMBackupDisk{
			SourceVolume: disk.SourceVolume, BackupVolume: disk.BackupVolume,
			Target: disk.Target, Format: disk.Format, CapacityBytes: disk.CapacityBytes,
		})
		totalBytes += disk.CapacityBytes
	}
	createdAt := payload.manifest.CreatedAt
	if createdAt.IsZero() {
		createdAt = operation.CreatedAt
	}
	return &domain.VMBackup{
		BackupID: operation.BackupID, FormatVersion: payload.manifest.FormatVersion,
		CreatedAt: createdAt, VMUUID: operation.VMUUID, VMName: name,
		TargetPool: operation.TargetPool, Name: operation.BackupID,
		ManifestVolume: backupVolumeName(operation.BackupID, "-manifest.json"),
		ConfigVolume:   payload.manifest.ConfigVolume, ConfigBytes: uint64(len(payload.configXML)),
		Disks: disks, TotalBytes: totalBytes, Online: payload.manifest.Online,
		Quiesced: payload.manifest.Quiesced, Consistent: payload.manifest.Consistent,
		ContainsTPM: payload.manifest.ContainsTPM, TPMStateSaved: payload.manifest.TPMStateSaved,
	}
}

func deleteBackupArtifacts(ctx context.Context, conn *libvirt.Connect, pool *libvirt.StoragePool, poolName, backupID string) error {
	manifestName := backupVolumeName(backupID, "-manifest.json")
	configName := backupVolumeName(backupID, "-domain.xml")
	diskPrefix := backupVolumeName(backupID, "-disk-")
	volumes, err := pool.ListAllStorageVolumes(0)
	if err != nil {
		return fmt.Errorf("list backup pool volumes during recovery: %w", err)
	}
	for index := range volumes {
		volume := &volumes[index]
		name, nameErr := volume.GetName()
		path, pathErr := volume.GetPath()
		_ = volume.Free()
		if nameErr != nil || pathErr != nil {
			return fmt.Errorf("inspect backup artifacts during recovery: %w", errors.Join(nameErr, pathErr))
		}
		if name != manifestName && name != configName && !strings.HasPrefix(name, diskPrefix) {
			continue
		}
		if err := deleteRecoveryVolumeIfUnreferenced(ctx, conn, poolName, name, path); err != nil {
			return err
		}
	}
	return nil
}

func recoverDiskMigration(ctx context.Context, conn *libvirt.Connect, operation domain.DataPlaneOperation) (domain.DataPlaneRecovery, error) {
	return recoverReferencedTarget(ctx, conn, operation, operation.VMUUID, false)
}

func recoverImportOperation(ctx context.Context, conn *libvirt.Connect, operation domain.DataPlaneOperation) (domain.DataPlaneRecovery, error) {
	return recoverReferencedTarget(ctx, conn, operation, "", true)
}

func recoverStorageCopy(ctx context.Context, conn *libvirt.Connect, operation domain.DataPlaneOperation) (domain.DataPlaneRecovery, error) {
	return recoverReferencedTarget(ctx, conn, operation, "", false)
}

func recoverReferencedTarget(ctx context.Context, conn *libvirt.Connect, operation domain.DataPlaneOperation, expectedUUID string, matchName bool) (domain.DataPlaneRecovery, error) {
	if strings.TrimSpace(operation.TargetPool) == "" || strings.TrimSpace(operation.TargetName) == "" {
		return recoveryNeedsAttention("恢复日志缺少目标存储池或卷名"), nil
	}
	pool, err := conn.LookupStoragePoolByName(operation.TargetPool)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return recoveryNoop("目标存储池已不存在，无需回收"), nil
		}
		return domain.DataPlaneRecovery{}, fmt.Errorf("lookup target storage pool during recovery: %w", err)
	}
	defer func() { _ = pool.Free() }()
	info, err := pool.GetInfo()
	if err != nil {
		return domain.DataPlaneRecovery{}, fmt.Errorf("read target storage pool state during recovery: %w", err)
	}
	if info.State != libvirt.STORAGE_POOL_RUNNING {
		volume, lookupErr := pool.LookupStorageVolByName(operation.TargetName)
		if errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
			return recoveryNoop("目标卷不存在，无需回收"), nil
		}
		if volume != nil {
			_ = volume.Free()
		}
		return recoveryNeedsAttention("目标存储池未运行，无法确认目标卷引用状态"), nil
	}
	volume, err := pool.LookupStorageVolByName(operation.TargetName)
	if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
		if matchName && operation.VMName != "" {
			if existing, lookupErr := conn.LookupDomainByName(operation.VMName); lookupErr == nil {
				_ = existing.Free()
				return recoveryNeedsAttention("导入后的虚拟机定义存在，但目标系统盘缺失"), nil
			}
		}
		return recoveryNoop("目标卷不存在，无需回收"), nil
	}
	if err != nil {
		return domain.DataPlaneRecovery{}, fmt.Errorf("lookup target volume during recovery: %w", err)
	}
	targetPath, err := volume.GetPath()
	_ = volume.Free()
	if err != nil {
		return domain.DataPlaneRecovery{}, fmt.Errorf("read target volume path during recovery: %w", err)
	}
	refs, err := recoveryDomainsUsingVolume(conn, targetPath, operation.TargetName)
	if err != nil {
		return domain.DataPlaneRecovery{}, err
	}
	for _, ref := range refs {
		if expectedUUID != "" && strings.EqualFold(ref.UUID, expectedUUID) {
			return recoveryCompleted("目标磁盘已被虚拟机配置引用，迁移结果已生效"), nil
		}
		if matchName && ref.Name == operation.VMName {
			return recoveryCompleted("目标磁盘已被导入虚拟机引用，导入结果已生效"), nil
		}
	}
	if len(refs) > 0 {
		names := make([]string, 0, len(refs))
		for _, ref := range refs {
			names = append(names, ref.Name)
		}
		return recoveryNeedsAttention("目标卷仍被其他虚拟机引用：" + strings.Join(names, ", ")), nil
	}
	if err := deleteRecoveryVolumeIfUnreferenced(ctx, conn, operation.TargetPool, operation.TargetName, targetPath); err != nil {
		return recoveryNeedsAttention(err.Error()), nil
	}
	return recoveryRolledBack("未生效的目标卷已回收，原对象保持不变"), nil
}

func deleteRecoveryVolumeIfUnreferenced(ctx context.Context, conn *libvirt.Connect, poolName, volumeName, volumePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	refs, err := recoveryDomainsUsingVolume(conn, volumePath, volumeName)
	if err != nil {
		return err
	}
	if len(refs) > 0 {
		names := make([]string, 0, len(refs))
		for _, ref := range refs {
			names = append(names, ref.Name)
		}
		return fmt.Errorf("卷 %s/%s 仍被虚拟机引用：%s", poolName, volumeName, strings.Join(names, ", "))
	}
	pool, err := conn.LookupStoragePoolByName(poolName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return nil
		}
		return fmt.Errorf("lookup storage pool %s during recovery: %w", poolName, err)
	}
	defer func() { _ = pool.Free() }()
	volume, err := pool.LookupStorageVolByName(volumeName)
	if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lookup orphan volume %s/%s during recovery: %w", poolName, volumeName, err)
	}
	defer func() { _ = volume.Free() }()
	if err := volume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL); err != nil && !errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
		return fmt.Errorf("delete orphan volume %s/%s during recovery: %w", poolName, volumeName, err)
	}
	return nil
}

func findRecoveryVolume(conn *libvirt.Connect, name string) (*recoveryVolume, error) {
	pools, err := conn.ListAllStoragePools(libvirt.CONNECT_LIST_STORAGE_POOLS_ACTIVE)
	if err != nil {
		return nil, fmt.Errorf("list active storage pools for recovery: %w", err)
	}
	defer func() {
		for index := range pools {
			_ = pools[index].Free()
		}
	}()
	for index := range pools {
		pool := &pools[index]
		poolName, nameErr := pool.GetName()
		if nameErr != nil {
			return nil, fmt.Errorf("read storage pool name for recovery: %w", nameErr)
		}
		volume, lookupErr := pool.LookupStorageVolByName(name)
		if errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
			continue
		}
		if lookupErr != nil {
			return nil, fmt.Errorf("lookup volume %s in %s for recovery: %w", name, poolName, lookupErr)
		}
		path, pathErr := volume.GetPath()
		xmlText, xmlErr := volume.GetXMLDesc(0)
		_ = volume.Free()
		if pathErr != nil || xmlErr != nil {
			return nil, fmt.Errorf("inspect volume %s in %s for recovery: %w", name, poolName, errors.Join(pathErr, xmlErr))
		}
		var parsed storageVolumeXML
		_ = xml.Unmarshal([]byte(xmlText), &parsed)
		backing := ""
		if len(parsed.BackingStore) > 0 {
			backing = parsed.BackingStore[0].Path
		}
		return &recoveryVolume{Pool: poolName, Name: name, Path: path, Backing: backing}, nil
	}
	return nil, nil
}

func recoveryDomainsUsingVolume(conn *libvirt.Connect, volumePath, volumeName string) ([]recoveryDomain, error) {
	domains, err := conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE | libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		return nil, fmt.Errorf("list domains during data plane recovery: %w", err)
	}
	refs := make([]recoveryDomain, 0)
	seen := make(map[string]bool, len(domains))
	for index := range domains {
		dom := &domains[index]
		name, nameErr := dom.GetName()
		uuid, uuidErr := dom.GetUUIDString()
		if nameErr != nil || uuidErr != nil {
			_ = dom.Free()
			return nil, fmt.Errorf("inspect domains during data plane recovery: %w", errors.Join(nameErr, uuidErr))
		}
		xmlTexts := make([]string, 0, 2)
		activeXML, xmlErr := dom.GetXMLDesc(0)
		if xmlErr != nil {
			_ = dom.Free()
			return nil, fmt.Errorf("read domain %s XML during data plane recovery: %w", name, xmlErr)
		}
		xmlTexts = append(xmlTexts, activeXML)
		active, activeErr := dom.IsActive()
		if activeErr != nil {
			_ = dom.Free()
			return nil, fmt.Errorf("read domain %s active state during data plane recovery: %w", name, activeErr)
		}
		if active {
			inactiveXML, inactiveErr := dom.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE)
			if inactiveErr != nil {
				_ = dom.Free()
				return nil, fmt.Errorf("read domain %s persistent XML during data plane recovery: %w", name, inactiveErr)
			}
			xmlTexts = append(xmlTexts, inactiveXML)
		}
		_ = dom.Free()
		if seen[uuid] {
			continue
		}
		var used bool
		for _, xmlText := range xmlTexts {
			details, parseErr := parseDomainXML(xmlText)
			if parseErr != nil {
				return nil, fmt.Errorf("parse domain %s during data plane recovery: %w", name, parseErr)
			}
			for _, disk := range details.Disks {
				if diskUsesVolume(disk.Source, volumePath, volumeName) {
					used = true
					break
				}
			}
			if used {
				break
			}
		}
		if used {
			seen[uuid] = true
			refs = append(refs, recoveryDomain{Name: name, UUID: uuid})
		}
	}
	return refs, nil
}

func recoveryCompleted(message string) domain.DataPlaneRecovery {
	return domain.DataPlaneRecovery{Status: domain.DataPlaneRecoveryCompleted, Message: message}
}

func recoveryRolledBack(message string) domain.DataPlaneRecovery {
	return domain.DataPlaneRecovery{Status: domain.DataPlaneRecoveryRolledBack, Message: message}
}

func recoveryCleaned(message string) domain.DataPlaneRecovery {
	return domain.DataPlaneRecovery{Status: domain.DataPlaneRecoveryCleaned, Message: message}
}

func recoveryNoop(message string) domain.DataPlaneRecovery {
	return domain.DataPlaneRecovery{Status: domain.DataPlaneRecoveryNoop, Message: message}
}

func recoveryNeedsAttention(message string) domain.DataPlaneRecovery {
	return domain.DataPlaneRecovery{Status: domain.DataPlaneRecoveryNeedsAttention, Message: message}
}
