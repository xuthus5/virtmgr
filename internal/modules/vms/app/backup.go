package app

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

const backupTimeout = 6 * time.Hour

var backupPoolPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{1,63}$`)

func (s *Service) Backup(ctx context.Context, command BackupCommand) (domain.Task, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Spec.Name = strings.TrimSpace(command.Spec.Name)
	command.Spec.TargetPool = strings.TrimSpace(command.Spec.TargetPool)
	detail := map[string]any{
		"backupName": command.Spec.Name,
		"targetPool": command.Spec.TargetPool,
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.backup", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.backup", command.UUID, detail, domain.ErrReadOnly)
	}
	if s.tasks == nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.backup", command.UUID, detail, domain.ErrUnavailable)
	}
	if command.UUID == "" || !vmNamePattern.MatchString(command.Spec.Name) || !backupPoolPattern.MatchString(command.Spec.TargetPool) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.backup", command.UUID, detail, domain.ErrInvalidInput)
	}
	source, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.backup", command.UUID, detail, err)
	}
	detail["sourceName"] = source.Name
	detail["sourceState"] = string(source.State)
	detail["diskCount"] = backupableDiskCount(source.Disks)
	if (source.State != domain.VMStateShutoff && source.State != domain.VMStateCrashed) || !source.Persistent {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.backup", command.UUID, detail, domain.ErrActionNotAllowed)
	}

	startStep := fmt.Sprintf("准备备份 %s", command.Spec.Name)
	taskID, err := s.tasks.Start(ctx, "vm.backup", "vm", command.UUID, command.Request.Session.User.Username, startStep)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.backup", command.UUID, detail, err)
	}
	now := time.Now().UTC()
	task := domain.Task{
		ID: taskID, Kind: "vm.backup", TargetKind: "vm", TargetID: command.UUID,
		Status: domain.TaskStatusRunning, Progress: 0, Step: startStep,
		Actor: command.Request.Session.User.Username, CreatedAt: now, UpdatedAt: now,
	}
	jobContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), backupTimeout)
	go func() {
		defer cancel()
		s.runBackup(jobContext, task.ID, command, detail)
	}()
	return task, nil
}

func (s *Service) runBackup(ctx context.Context, taskID string, command BackupCommand, detail map[string]any) {
	progress := 0
	backup, err := s.repository.BackupVM(ctx, command.UUID, command.Spec, func(percent int, step string) {
		if percent < progress {
			percent = progress
		}
		if percent > 100 {
			percent = 100
		}
		progress = percent
		_ = s.tasks.Progress(ctx, taskID, progress, step)
	})
	if err != nil {
		_ = s.recordMutationAudit(ctx, command.Request, "vm.backup", command.UUID, "failure", detail, err)
		_ = s.tasks.Fail(ctx, taskID, progress, "备份虚拟机失败", err)
		return
	}
	detail["diskCount"] = len(backup.Disks)
	detail["totalBytes"] = backup.TotalBytes
	detail["configBytes"] = backup.ConfigBytes
	detail["manifestVolume"] = backup.ManifestVolume
	detail["configVolume"] = backup.ConfigVolume
	if err := s.recordMutationAudit(ctx, command.Request, "vm.backup", command.UUID, "success", detail, nil); err != nil {
		_ = s.tasks.Fail(ctx, taskID, 100, "备份完成但审计写入失败", err)
		return
	}
	_ = s.tasks.Succeed(ctx, taskID, fmt.Sprintf("虚拟机 %s 备份完成", backup.VMName))
}

func backupableDiskCount(disks []domain.Disk) int {
	count := 0
	for _, disk := range disks {
		if disk.Device == "disk" && !disk.ReadOnly && strings.TrimSpace(disk.Source) != "" {
			count++
		}
	}
	return count
}
