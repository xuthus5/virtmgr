package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

const diskMigrateTimeout = 6 * time.Hour

func (s *Service) MigrateDisk(ctx context.Context, command MigrateDiskCommand) (domain.Task, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Target = strings.TrimSpace(command.Target)
	command.Spec.TargetPool = strings.TrimSpace(command.Spec.TargetPool)
	command.Spec.TargetName = strings.TrimSpace(command.Spec.TargetName)
	command.Spec.TargetFormat = strings.ToLower(strings.TrimSpace(command.Spec.TargetFormat))
	detail := map[string]any{
		"diskTarget": command.Target, "targetPool": command.Spec.TargetPool,
		"targetName": command.Spec.TargetName, "targetFormat": command.Spec.TargetFormat,
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.disk.migrate", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.disk.migrate", command.UUID, detail, domain.ErrReadOnly)
	}
	if s.tasks == nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.disk.migrate", command.UUID, detail, domain.ErrUnavailable)
	}
	if command.UUID == "" || !diskTargetPattern.MatchString(command.Target) || !storagePoolNamePattern.MatchString(command.Spec.TargetPool) ||
		!storageVolumeNamePattern.MatchString(command.Spec.TargetName) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.disk.migrate", command.UUID, detail, domain.ErrInvalidInput)
	}
	if command.Spec.TargetFormat != "" && command.Spec.TargetFormat != "qcow2" && command.Spec.TargetFormat != "raw" {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.disk.migrate", command.UUID, detail, domain.ErrInvalidInput)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.disk.migrate", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	if !vm.Persistent || (vm.State != domain.VMStateShutoff && vm.State != domain.VMStateCrashed) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.disk.migrate", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	var disk *domain.Disk
	for index := range vm.Disks {
		if vm.Disks[index].Target == command.Target {
			disk = &vm.Disks[index]
			break
		}
	}
	if disk == nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.disk.migrate", command.UUID, detail, domain.ErrNotFound)
	}
	detail["source"] = disk.Source
	detail["sourceFormat"] = disk.Format
	detail["capacityBytes"] = disk.CapacityBytes
	if disk.Device != "disk" || disk.ReadOnly || strings.TrimSpace(disk.Source) == "" {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.disk.migrate", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	if command.Spec.TargetFormat == "" {
		command.Spec.TargetFormat = strings.ToLower(strings.TrimSpace(disk.Format))
	}
	if command.Spec.TargetFormat != "qcow2" && command.Spec.TargetFormat != "raw" {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.disk.migrate", command.UUID, detail, domain.ErrInvalidInput)
	}
	detail["targetFormat"] = command.Spec.TargetFormat
	migrator, ok := s.repository.(VMDiskMigrationRepository)
	if !ok {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.disk.migrate", command.UUID, detail, domain.ErrUnavailable)
	}
	startStep := fmt.Sprintf("正在复制磁盘 %s 到 %s/%s", command.Target, command.Spec.TargetPool, command.Spec.TargetName)
	taskID, err := s.tasks.Start(ctx, "vm.disk.migrate", "vm", command.UUID, command.Request.Session.User.Username, startStep)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.disk.migrate", command.UUID, detail, err)
	}
	now := time.Now().UTC()
	task := domain.Task{
		ID: taskID, Kind: "vm.disk.migrate", TargetKind: "vm", TargetID: command.UUID,
		Status: domain.TaskStatusRunning, Progress: 0, Step: startStep,
		Actor: command.Request.Session.User.Username, CreatedAt: now, UpdatedAt: now,
	}
	jobContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), diskMigrateTimeout)
	if registrar, ok := s.tasks.(interface {
		RegisterCancel(string, context.CancelFunc)
	}); ok {
		registrar.RegisterCancel(task.ID, cancel)
	}
	go func() {
		defer cancel()
		_ = s.tasks.Progress(jobContext, task.ID, 5, startStep)
		updated, migrateErr := migrator.MigrateVMDisk(jobContext, command.UUID, command.Target, command.Spec)
		finalizeContext, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer finalizeCancel()
		if migrateErr != nil {
			_ = s.recordMutationAudit(finalizeContext, command.Request, "vm.disk.migrate", command.UUID, "failure", detail, migrateErr)
			_ = s.tasks.Fail(finalizeContext, task.ID, 100, "磁盘跨池迁移失败", migrateErr)
			return
		}
		for _, candidate := range updated.Disks {
			if candidate.Target == command.Target {
				detail["targetPath"] = candidate.Source
				detail["targetCapacityBytes"] = candidate.CapacityBytes
				break
			}
		}
		s.captureConfigVersion(finalizeContext, command.Request, command.UUID, domain.VMConfigSourceDiskMigrate, 0, "跨池迁移磁盘并切换持久配置引用")
		if auditErr := s.recordMutationAudit(finalizeContext, command.Request, "vm.disk.migrate", command.UUID, "success", detail, nil); auditErr != nil {
			_ = s.tasks.Fail(finalizeContext, task.ID, 100, "磁盘迁移完成但审计写入失败", auditErr)
			return
		}
		_ = s.tasks.Succeed(finalizeContext, task.ID, fmt.Sprintf("磁盘 %s 已迁移到 %s", command.Target, command.Spec.TargetPool))
	}()
	return task, nil
}
