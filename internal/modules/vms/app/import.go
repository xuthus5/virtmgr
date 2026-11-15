package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

const (
	importTimeout                = 2 * time.Hour
	importCleanupTimeout         = 30 * time.Second
	defaultImportISODiskCapacity = uint64(20 << 30)
	minImportISODiskCapacity     = uint64(1 << 30)
	maxImportISODiskCapacity     = uint64(16 << 40)
	importSourceKindDisk         = "disk"
	importSourceKindISO          = "iso"
	importISOSystemDiskFormat    = "qcow2"
)

type importPlan struct {
	createSpec   domain.VMCreateSpec
	source       domain.StorageVolume
	sourcePool   string
	sourceKind   string
	targetPool   string
	targetName   string
	targetPath   string
	format       string
	diskCapacity uint64
	capacity     uint64
	allocation   uint64
	targetOwned  bool
}

func (s *Service) Import(ctx context.Context, command ImportCommand) (domain.Task, error) {
	command.Spec.Name = strings.TrimSpace(command.Spec.Name)
	command.Spec.SourcePool = strings.TrimSpace(command.Spec.SourcePool)
	command.Spec.SourceVolume = strings.TrimSpace(command.Spec.SourceVolume)
	command.Spec.TargetPool = strings.TrimSpace(command.Spec.TargetPool)
	normalized, createSpec, err := normalizeImportSpec(command.Spec)
	detail := importSpecDetail(normalized)
	if err != nil {
		return domain.Task{}, s.importFailure(ctx, command.Request, detail, err)
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.Task{}, s.importFailure(ctx, command.Request, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Task{}, s.importFailure(ctx, command.Request, detail, domain.ErrReadOnly)
	}
	if s.tasks == nil || s.importVolumes == nil {
		return domain.Task{}, s.importFailure(ctx, command.Request, detail, domain.ErrUnavailable)
	}

	plan, err := s.prepareImport(ctx, normalized, createSpec)
	if err != nil {
		return domain.Task{}, s.importFailure(ctx, command.Request, detail, err)
	}
	detail = importPlanDetail(normalized, plan)

	startStep := "正在复制磁盘到目标存储池"
	if plan.sourceKind == importSourceKindISO {
		startStep = "正在创建空白系统盘"
	}
	taskID, err := s.tasks.Start(ctx, "vm.import", "vm_import", plan.sourcePool+"/"+plan.source.Name,
		command.Request.Session.User.Username, startStep)
	if err != nil {
		return domain.Task{}, s.importFailure(ctx, command.Request, detail, fmt.Errorf("start import task: %w", err))
	}
	now := time.Now().UTC()
	task := domain.Task{
		ID: taskID, Kind: "vm.import", TargetKind: "vm_import", TargetID: plan.sourcePool + "/" + plan.source.Name,
		Status: domain.TaskStatusRunning, Progress: 0, Step: startStep,
		Actor: command.Request.Session.User.Username, CreatedAt: now, UpdatedAt: now,
	}
	jobContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), importTimeout)
	if registrar, ok := s.tasks.(interface {
		RegisterCancel(string, context.CancelFunc)
	}); ok {
		registrar.RegisterCancel(task.ID, cancel)
	}
	go func() {
		defer cancel()
		s.runImport(jobContext, task.ID, command, plan, detail)
	}()
	return task, nil
}

func (s *Service) prepareImport(ctx context.Context, spec domain.VMImportSpec, createSpec domain.VMCreateSpec) (importPlan, error) {
	targetPool := spec.TargetPool
	targetVolumes, err := s.importVolumes.ListStorageVolumes(ctx, targetPool)
	if err != nil {
		return importPlan{}, err
	}
	sourceVolumes, err := s.importVolumes.ListStorageVolumes(ctx, spec.SourcePool)
	if err != nil {
		return importPlan{}, err
	}
	var source domain.StorageVolume
	for _, volume := range sourceVolumes {
		if volume.Name == spec.SourceVolume {
			source = volume
			break
		}
	}
	if source.Name == "" {
		return importPlan{}, domain.ErrNotFound
	}
	format := strings.ToLower(strings.TrimSpace(source.Format))
	extension := strings.ToLower(filepath.Ext(source.Name))
	sourceKind := importSourceKindDisk
	targetFormat := format
	if extension == ".iso" {
		if format != "raw" && format != "iso" {
			return importPlan{}, fmt.Errorf("%w: ISO source must use raw storage format", domain.ErrInvalidInput)
		}
		if !validAbsolutePath(sourcePath(source)) {
			return importPlan{}, fmt.Errorf("%w: ISO source path must be absolute", domain.ErrInvalidInput)
		}
		sourceKind = importSourceKindISO
		targetFormat = importISOSystemDiskFormat
	} else if (format != "qcow2" && format != "raw") || extension != "."+format {
		return importPlan{}, fmt.Errorf("%w: source volume must be a qcow2, raw or iso image", domain.ErrInvalidInput)
	}
	targetName := imageDiskName(spec.Name, targetFormat)
	if spec.SourcePool == targetPool && targetName == source.Name {
		return importPlan{}, domain.ErrConflict
	}
	for _, volume := range targetVolumes {
		if volume.Name == targetName {
			return importPlan{}, domain.ErrConflict
		}
	}
	vms, err := s.repository.ListVMs(ctx)
	if err != nil {
		return importPlan{}, err
	}
	for _, vm := range vms {
		if vm.Name == spec.Name {
			return importPlan{}, domain.ErrConflict
		}
	}
	createSpec.DiskPath = "/virtmgr-import/" + targetName
	createSpec.DiskFormat = targetFormat
	createSpec.TargetPool = targetPool
	diskCapacity := spec.DiskCapacityBytes
	if sourceKind == importSourceKindISO {
		createSpec.ISOPath = sourcePath(source)
		createSpec.BootOrder = domain.VMBootOrderCDROMFirst
	} else {
		diskCapacity = source.CapacityBytes
	}
	return importPlan{
		createSpec: createSpec, source: source, sourcePool: spec.SourcePool, sourceKind: sourceKind,
		targetPool: targetPool, targetName: targetName, format: format, diskCapacity: diskCapacity,
		capacity: source.CapacityBytes, allocation: source.AllocationBytes,
	}, nil
}

func (s *Service) runImport(ctx context.Context, taskID string, command ImportCommand, plan importPlan, detail map[string]any) {
	progress := 0
	report := func(percent int, step string) {
		if percent < progress {
			percent = progress
		}
		if percent > 100 {
			percent = 100
		}
		progress = percent
		_ = s.tasks.Progress(ctx, taskID, progress, step)
	}
	fail := func(cause error, step string) {
		finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), importCleanupTimeout)
		defer cancel()
		_ = s.recordImportAudit(finalCtx, command.Request, plan, "", "failure", detail, cause)
		_ = s.tasks.Fail(finalCtx, taskID, progress, step, cause)
	}

	startStep := "正在复制磁盘到目标存储池"
	if plan.sourceKind == importSourceKindISO {
		startStep = "正在创建空白系统盘"
	}
	report(10, startStep)
	if err := ctx.Err(); err != nil {
		fail(err, "导入已取消")
		return
	}
	var target domain.StorageVolume
	var err error
	if plan.sourceKind == importSourceKindISO {
		target, err = s.importVolumes.CreateStorageVolume(ctx, plan.targetPool, domain.StorageVolumeCreateSpec{
			Name: plan.targetName, Format: importISOSystemDiskFormat, CapacityBytes: plan.diskCapacity,
		})
		if err != nil {
			fail(err, "创建空白系统盘失败")
			return
		}
	} else {
		target, err = s.importVolumes.CloneStorageVolume(ctx, plan.sourcePool, plan.source.Name, plan.targetPool, plan.targetName)
		if err != nil {
			fail(err, "复制导入磁盘失败")
			return
		}
	}
	plan.targetPath = target.Path
	plan.targetOwned = target.Name == plan.targetName
	if plan.targetPath == "" {
		s.cleanupImportVolume(ctx, plan)
		fail(domain.ErrUnavailable, "复制导入磁盘失败")
		return
	}
	detail["targetPath"] = plan.targetPath
	if strings.TrimSpace(sourcePath(plan.source)) != "" && filepath.Clean(plan.targetPath) == filepath.Clean(plan.source.Path) {
		fail(domain.ErrConflict, "导入目标不能覆盖来源磁盘")
		return
	}
	if err := ctx.Err(); err != nil {
		s.cleanupImportVolume(ctx, plan)
		fail(err, "导入已取消")
		return
	}

	report(70, "正在定义虚拟机")
	plan.createSpec.DiskPath = plan.targetPath
	vm, err := s.repository.CreateVM(ctx, plan.createSpec)
	if err != nil {
		s.cleanupImportVolume(ctx, plan)
		fail(err, "定义虚拟机失败")
		return
	}
	detail["uuid"] = vm.UUID
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	if err := ctx.Err(); err != nil {
		s.cleanupImportedVM(ctx, vm)
		s.cleanupImportVolume(ctx, plan)
		fail(err, "导入已取消")
		return
	}

	report(90, "正在保存导入记录")
	s.captureConfigVersion(ctx, command.Request, vm.UUID, domain.VMConfigSourceImport, 0, "导入虚拟机后保存初始配置")
	finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), importCleanupTimeout)
	defer cancel()
	if err := s.recordImportAudit(finalCtx, command.Request, plan, vm.UUID, "success", detail, nil); err != nil {
		_ = s.tasks.Fail(finalCtx, taskID, 100, "导入完成但审计写入失败", err)
		return
	}
	if recorder, ok := s.tasks.(interface {
		SetMetadata(context.Context, string, map[string]any) error
	}); ok {
		_ = recorder.SetMetadata(finalCtx, taskID, map[string]any{"vmUuid": vm.UUID, "vmName": vm.Name})
	}
	_ = s.tasks.Succeed(finalCtx, taskID, fmt.Sprintf("虚拟机 %s 导入完成", vm.Name))
}

func (s *Service) cleanupImportVolume(ctx context.Context, plan importPlan) {
	if !plan.targetOwned {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), importCleanupTimeout)
	defer cancel()
	_ = s.importVolumes.DeleteStorageVolume(cleanupCtx, plan.targetPool, plan.targetName)
}

func (s *Service) cleanupImportedVM(ctx context.Context, vm domain.VM) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), importCleanupTimeout)
	defer cancel()
	if vm.State != domain.VMStateShutoff && vm.State != domain.VMStateCrashed {
		_ = s.repository.PerformAction(cleanupCtx, vm.UUID, domain.VMActionDestroy)
	}
	_ = s.repository.DeleteVM(cleanupCtx, vm.UUID)
}

func (s *Service) importFailure(ctx context.Context, request MutationContext, detail map[string]any, cause error) error {
	plan := importPlan{sourcePool: stringValue(detail["sourcePool"]), targetPool: stringValue(detail["targetPool"])}
	if source, ok := detail["sourceVolume"].(string); ok {
		plan.source.Name = source
	}
	if err := s.recordImportAudit(ctx, request, plan, "", "failure", detail, cause); err != nil {
		return errors.Join(cause, fmt.Errorf("record vm.import audit: %w", err))
	}
	return cause
}

func (s *Service) recordImportAudit(ctx context.Context, request MutationContext, plan importPlan, vmUUID, result string, detail map[string]any, cause error) error {
	targetKind := "vm_import"
	targetID := plan.sourcePool + "/" + plan.source.Name
	if vmUUID != "" {
		targetKind = "vm"
		targetID = vmUUID
	}
	return s.recordMutationAuditForKind(ctx, request, targetKind, "vm.import", targetID, result, detail, cause)
}

func normalizeImportSpec(spec domain.VMImportSpec) (domain.VMImportSpec, domain.VMCreateSpec, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.SourcePool = strings.TrimSpace(spec.SourcePool)
	spec.SourceVolume = strings.TrimSpace(spec.SourceVolume)
	spec.TargetPool = strings.TrimSpace(spec.TargetPool)
	spec.Network = strings.TrimSpace(spec.Network)
	if spec.TargetPool == "" {
		spec.TargetPool = spec.SourcePool
	}
	if !storagePoolNamePattern.MatchString(spec.SourcePool) || !storagePoolNamePattern.MatchString(spec.TargetPool) ||
		!storageVolumeNamePattern.MatchString(spec.SourceVolume) {
		return domain.VMImportSpec{}, domain.VMCreateSpec{}, domain.ErrInvalidInput
	}
	extension := strings.ToLower(filepath.Ext(spec.SourceVolume))
	if extension != ".qcow2" && extension != ".raw" && extension != ".iso" {
		return domain.VMImportSpec{}, domain.VMCreateSpec{}, fmt.Errorf("%w: only qcow2, raw and iso images can be imported", domain.ErrInvalidInput)
	}
	format := strings.TrimPrefix(extension, ".")
	targetFormat := format
	if extension == ".iso" {
		targetFormat = importISOSystemDiskFormat
		if spec.DiskCapacityBytes == 0 {
			spec.DiskCapacityBytes = defaultImportISODiskCapacity
		}
		if spec.DiskCapacityBytes < minImportISODiskCapacity || spec.DiskCapacityBytes > maxImportISODiskCapacity {
			return domain.VMImportSpec{}, domain.VMCreateSpec{}, fmt.Errorf("%w: ISO system disk capacity must be between %d and %d bytes", domain.ErrInvalidInput, minImportISODiskCapacity, maxImportISODiskCapacity)
		}
	}
	createSpec := domain.VMCreateSpec{
		Name: spec.Name, VCPUs: spec.VCPUs, MemoryMiB: spec.MemoryMiB,
		DiskPath: "/virtmgr-import/" + imageDiskName(spec.Name, targetFormat), DiskFormat: targetFormat,
		TargetPool: spec.TargetPool, DiskBus: spec.DiskBus, Network: spec.Network, NICModel: spec.NICModel,
		Firmware: spec.Firmware, SecureBoot: spec.SecureBoot, GuestAgent: spec.GuestAgent,
		Autostart: spec.Autostart, Start: spec.Start,
	}
	normalized, err := normalizeCreateSpec(createSpec)
	if err != nil {
		return domain.VMImportSpec{}, domain.VMCreateSpec{}, err
	}
	spec.Name = normalized.Name
	spec.Network = normalized.Network
	spec.DiskBus = normalized.DiskBus
	spec.NICModel = normalized.NICModel
	spec.Firmware = normalized.Firmware
	spec.GuestAgent = normalized.GuestAgent
	return spec, normalized, nil
}

func importSpecDetail(spec domain.VMImportSpec) map[string]any {
	return map[string]any{
		"name": spec.Name, "sourcePool": spec.SourcePool, "sourceVolume": spec.SourceVolume,
		"targetPool": spec.TargetPool, "vcpus": spec.VCPUs, "memoryMiB": spec.MemoryMiB,
		"diskCapacityBytes": spec.DiskCapacityBytes,
		"diskBus":           spec.DiskBus, "network": spec.Network, "nicModel": spec.NICModel,
		"firmware": spec.Firmware, "secureBoot": spec.SecureBoot, "guestAgent": spec.GuestAgentEnabled(),
		"autostart": spec.Autostart, "start": spec.Start,
	}
}

func importPlanDetail(spec domain.VMImportSpec, plan importPlan) map[string]any {
	detail := importSpecDetail(spec)
	detail["sourceFormat"] = plan.format
	detail["targetFormat"] = plan.createSpec.DiskFormat
	detail["systemDiskCapacityBytes"] = plan.diskCapacity
	if plan.sourceKind == importSourceKindISO {
		detail["sourceKind"] = importSourceKindISO
		detail["bootOrder"] = string(plan.createSpec.BootOrder)
	}
	detail["sourceCapacityBytes"] = plan.capacity
	detail["sourceAllocationBytes"] = plan.allocation
	detail["targetPool"] = plan.targetPool
	detail["targetVolume"] = plan.targetName
	return detail
}

func sourcePath(volume domain.StorageVolume) string {
	return strings.TrimSpace(volume.Path)
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
