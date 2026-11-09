package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"virtmgr/internal/domain"
)

func (s *Service) CreateRepository(ctx context.Context, command RepositoryCommand) (domain.BackupRepository, error) {
	command.Spec.Name = strings.TrimSpace(command.Spec.Name)
	command.Spec.PoolName = strings.TrimSpace(command.Spec.PoolName)
	detail := map[string]any{"name": command.Spec.Name, "poolName": command.Spec.PoolName}
	if command.Request.Session.User.Role != domain.RoleAdmin {
		return domain.BackupRepository{}, s.mutationFailure(ctx, command.Request, "backup.repository.create", command.Spec.PoolName, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.BackupRepository{}, s.mutationFailure(ctx, command.Request, "backup.repository.create", command.Spec.PoolName, detail, domain.ErrReadOnly)
	}
	if err := domain.ValidateBackupName(command.Spec.Name); err != nil {
		return domain.BackupRepository{}, s.mutationFailure(ctx, command.Request, "backup.repository.create", command.Spec.PoolName, detail, err)
	}
	pool, err := s.poolByName(ctx, command.Spec.PoolName)
	if err != nil {
		return domain.BackupRepository{}, s.mutationFailure(ctx, command.Request, "backup.repository.create", command.Spec.PoolName, detail, err)
	}
	if pool.State != domain.StoragePoolStateActive {
		return domain.BackupRepository{}, s.mutationFailure(ctx, command.Request, "backup.repository.create", command.Spec.PoolName, detail, domain.ErrUnavailable)
	}
	item, err := s.createRepositoryForPool(ctx, command.Spec.Name, command.Spec.PoolName, command.Spec.IsDefault)
	if err != nil {
		return domain.BackupRepository{}, s.mutationFailure(ctx, command.Request, "backup.repository.create", command.Spec.PoolName, detail, err)
	}
	detail["repositoryId"] = item.ID
	if err := s.recordAudit(ctx, command.Request, "backup.repository.create", item.ID, "success", detail, nil); err != nil {
		return domain.BackupRepository{}, err
	}
	return item, nil
}

func (s *Service) UpdateRepository(ctx context.Context, command RepositoryCommand) (domain.BackupRepository, error) {
	command.ID = strings.TrimSpace(command.ID)
	command.Spec.Name = strings.TrimSpace(command.Spec.Name)
	detail := map[string]any{"name": command.Spec.Name, "repositoryId": command.ID}
	if command.Request.Session.User.Role != domain.RoleAdmin {
		return domain.BackupRepository{}, s.mutationFailure(ctx, command.Request, "backup.repository.update", command.ID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.BackupRepository{}, s.mutationFailure(ctx, command.Request, "backup.repository.update", command.ID, detail, domain.ErrReadOnly)
	}
	if err := domain.ValidateBackupName(command.Spec.Name); err != nil {
		return domain.BackupRepository{}, s.mutationFailure(ctx, command.Request, "backup.repository.update", command.ID, detail, err)
	}
	item, err := s.repository.GetBackupRepository(ctx, command.ID)
	if err != nil {
		return domain.BackupRepository{}, s.mutationFailure(ctx, command.Request, "backup.repository.update", command.ID, detail, err)
	}
	item.Name = command.Spec.Name
	item.IsDefault = command.Spec.IsDefault
	item.UpdatedAt = s.now().UTC()
	updated, err := s.repository.UpdateBackupRepository(ctx, item)
	if err != nil {
		return domain.BackupRepository{}, s.mutationFailure(ctx, command.Request, "backup.repository.update", command.ID, detail, err)
	}
	if err := s.recordAudit(ctx, command.Request, "backup.repository.update", item.ID, "success", detail, nil); err != nil {
		return domain.BackupRepository{}, err
	}
	return updated, nil
}

func (s *Service) DeleteRepository(ctx context.Context, command RepositoryCommand) error {
	command.ID = strings.TrimSpace(command.ID)
	detail := map[string]any{"repositoryId": command.ID}
	if command.Request.Session.User.Role != domain.RoleAdmin {
		return s.mutationFailure(ctx, command.Request, "backup.repository.delete", command.ID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return s.mutationFailure(ctx, command.Request, "backup.repository.delete", command.ID, detail, domain.ErrReadOnly)
	}
	if err := s.repository.DeleteBackupRepository(ctx, command.ID); err != nil {
		return s.mutationFailure(ctx, command.Request, "backup.repository.delete", command.ID, detail, err)
	}
	return s.recordAudit(ctx, command.Request, "backup.repository.delete", command.ID, "success", detail, nil)
}

func (s *Service) Verify(ctx context.Context, command VerifyCommand) (domain.Task, error) {
	command.ID = strings.TrimSpace(command.ID)
	detail := map[string]any{"backupId": command.ID, "deep": command.Deep}
	if !domain.CanCreateBackups(command.Request.Session.User.Role) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.verify", command.ID, detail, domain.ErrForbidden)
	}
	if s.tasks == nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.verify", command.ID, detail, domain.ErrUnavailable)
	}
	item, err := s.repository.GetBackup(ctx, command.ID)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.verify", command.ID, detail, err)
	}
	if item.Status != domain.BackupStatusCompleted {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.verify", command.ID, detail, domain.ErrActionNotAllowed)
	}
	step := "正在执行快速校验"
	if command.Deep {
		step = "正在执行深度校验"
	}
	taskID, err := s.tasks.Start(ctx, "vm.backup.verify", "backup", command.ID, command.Request.Session.User.Username, step)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.verify", command.ID, detail, err)
	}
	now := s.now().UTC()
	task := domain.Task{
		ID: taskID, Kind: "vm.backup.verify", TargetKind: "backup", TargetID: command.ID,
		Status: domain.TaskStatusRunning, Step: step, Actor: command.Request.Session.User.Username,
		CreatedAt: now, UpdatedAt: now,
	}
	jobContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), verifyTimeout)
	go func() {
		defer cancel()
		s.executeVerify(jobContext, taskID, item, command, detail)
	}()
	return task, nil
}

func (s *Service) executeVerify(ctx context.Context, taskID string, item domain.Backup, command VerifyCommand, detail map[string]any) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	result, err := s.backend.VerifyVMBackup(ctx, item, command.Deep)
	if err != nil {
		_ = s.tasks.Fail(ctx, taskID, 0, "校验备份失败", err)
		_ = s.recordAudit(ctx, command.Request, "backup.verify", item.ID, "failure", detail, err)
		return
	}
	checkedAt := result.CheckedAt
	item.VerifyStatus = result.Status
	item.VerifiedAt = &checkedAt
	item.VerifyMessage = result.Message
	if _, err := s.repository.UpdateBackup(ctx, item); err != nil {
		_ = s.tasks.Fail(ctx, taskID, 100, "校验完成但写入结果失败", err)
		return
	}
	detail["status"] = string(result.Status)
	detail["message"] = result.Message
	if result.Status != domain.BackupVerifyVerified {
		cause := errors.New(result.Message)
		_ = s.recordAudit(ctx, command.Request, "backup.verify", item.ID, "failure", detail, cause)
		_ = s.tasks.Fail(ctx, taskID, 100, "备份校验未通过", cause)
		return
	}
	if err := s.recordAudit(ctx, command.Request, "backup.verify", item.ID, "success", detail, nil); err != nil {
		_ = s.tasks.Fail(ctx, taskID, 100, "校验完成但审计写入失败", err)
		return
	}
	_ = s.tasks.Succeed(ctx, taskID, "备份校验通过")
}

func (s *Service) Restore(ctx context.Context, command RestoreCommand) (domain.Task, error) {
	command.ID = strings.TrimSpace(command.ID)
	command.Spec.Name = strings.TrimSpace(command.Spec.Name)
	command.Spec.TargetPool = strings.TrimSpace(command.Spec.TargetPool)
	detail := map[string]any{"backupId": command.ID, "restoreName": command.Spec.Name, "targetPool": command.Spec.TargetPool}
	if !domain.CanDeleteBackups(command.Request.Session.User.Role) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.restore", command.ID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.restore", command.ID, detail, domain.ErrReadOnly)
	}
	if s.tasks == nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.restore", command.ID, detail, domain.ErrUnavailable)
	}
	if err := domain.ValidateVMRestoreName(command.Spec.Name); err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.restore", command.ID, detail, err)
	}
	item, err := s.repository.GetBackup(ctx, command.ID)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.restore", command.ID, detail, err)
	}
	if item.Status != domain.BackupStatusCompleted {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.restore", command.ID, detail, domain.ErrActionNotAllowed)
	}
	if command.Spec.TargetPool == "" {
		command.Spec.TargetPool = item.TargetPool
	}
	pool, err := s.poolByName(ctx, command.Spec.TargetPool)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.restore", command.ID, detail, err)
	}
	if pool.State != domain.StoragePoolStateActive {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.restore", command.ID, detail, domain.ErrUnavailable)
	}
	detail["sourceName"] = item.VMName
	startStep := fmt.Sprintf("正在恢复为新虚拟机 %s", command.Spec.Name)
	taskID, err := s.tasks.Start(ctx, "vm.backup.restore", "backup", command.ID, command.Request.Session.User.Username, startStep)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.restore", command.ID, detail, err)
	}
	now := s.now().UTC()
	task := domain.Task{
		ID: taskID, Kind: "vm.backup.restore", TargetKind: "backup", TargetID: command.ID,
		Status: domain.TaskStatusRunning, Step: startStep, Actor: command.Request.Session.User.Username,
		CreatedAt: now, UpdatedAt: now,
	}
	jobContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreTimeout)
	go func() {
		defer cancel()
		s.executeRestore(jobContext, taskID, item, command, detail)
	}()
	return task, nil
}

func (s *Service) executeRestore(ctx context.Context, taskID string, item domain.Backup, command RestoreCommand, detail map[string]any) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	progress := 0
	vm, err := s.backend.RestoreVMBackup(ctx, item, command.Spec, func(percent int, step string) {
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
		_ = s.recordAudit(ctx, command.Request, "backup.restore", item.ID, "failure", detail, err)
		_ = s.tasks.Fail(ctx, taskID, progress, "从备份恢复虚拟机失败", err)
		return
	}
	detail["restoredVmUuid"] = vm.UUID
	detail["restoredVmName"] = vm.Name
	if s.configVersions != nil {
		if _, captureErr := s.configVersions.CaptureVM(ctx, vm.UUID, domain.VMConfigCapture{
			Source:    domain.VMConfigSourceBackupRestore,
			Actor:     command.Request.Session.User.Username,
			Role:      command.Request.Session.User.Role,
			RequestID: command.Request.RequestID,
			SourceIP:  command.Request.SourceIP,
			Note:      "从备份恢复为新虚拟机后保存初始配置",
		}); captureErr != nil && s.onError != nil {
			s.onError(fmt.Errorf("capture restored VM config version: %w", captureErr))
		}
	}
	if err := s.recordAudit(ctx, command.Request, "backup.restore", item.ID, "success", detail, nil); err != nil {
		_ = s.tasks.Fail(ctx, taskID, 100, "恢复完成但审计写入失败", err)
		return
	}
	_ = s.tasks.Succeed(ctx, taskID, fmt.Sprintf("已从备份恢复虚拟机 %s", vm.Name))
}

func (s *Service) Delete(ctx context.Context, command DeleteCommand) (domain.Task, error) {
	command.ID = strings.TrimSpace(command.ID)
	detail := map[string]any{"backupId": command.ID}
	if !domain.CanDeleteBackups(command.Request.Session.User.Role) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.delete", command.ID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.delete", command.ID, detail, domain.ErrReadOnly)
	}
	if s.tasks == nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.delete", command.ID, detail, domain.ErrUnavailable)
	}
	item, err := s.repository.GetBackup(ctx, command.ID)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.delete", command.ID, detail, err)
	}
	if item.Status == domain.BackupStatusRunning || item.Status == domain.BackupStatusDeleting {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.delete", command.ID, detail, domain.ErrActionNotAllowed)
	}
	detail["sourceName"] = item.VMName
	startStep := "正在删除备份产物"
	taskID, err := s.tasks.Start(ctx, "vm.backup.delete", "backup", command.ID, command.Request.Session.User.Username, startStep)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.delete", command.ID, detail, err)
	}
	item.Status = domain.BackupStatusDeleting
	item.Error = ""
	if _, err := s.repository.UpdateBackup(ctx, item); err != nil {
		_ = s.tasks.Fail(ctx, taskID, 0, "更新备份删除状态失败", err)
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.delete", command.ID, detail, err)
	}
	now := s.now().UTC()
	task := domain.Task{
		ID: taskID, Kind: "vm.backup.delete", TargetKind: "backup", TargetID: command.ID,
		Status: domain.TaskStatusRunning, Step: startStep, Actor: command.Request.Session.User.Username,
		CreatedAt: now, UpdatedAt: now,
	}
	jobContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), deleteTimeout)
	go func() {
		defer cancel()
		s.executeDelete(jobContext, taskID, item, command, detail)
	}()
	return task, nil
}

func (s *Service) executeDelete(ctx context.Context, taskID string, item domain.Backup, command DeleteCommand, detail map[string]any) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if err := s.backend.DeleteVMBackup(ctx, item); err != nil {
		item.Status = domain.BackupStatusDeleteFailed
		item.Error = err.Error()
		_, _ = s.repository.UpdateBackup(ctx, item)
		_ = s.recordAudit(ctx, command.Request, "backup.delete", item.ID, "failure", detail, err)
		_ = s.tasks.Fail(ctx, taskID, 0, "删除备份产物失败", err)
		return
	}
	if err := s.repository.DeleteBackup(ctx, item.ID); err != nil {
		_ = s.tasks.Fail(ctx, taskID, 100, "备份产物已删除但清理目录失败", err)
		return
	}
	if err := s.recordAudit(ctx, command.Request, "backup.delete", item.ID, "success", detail, nil); err != nil {
		_ = s.tasks.Fail(ctx, taskID, 100, "备份已删除但审计写入失败", err)
		return
	}
	_ = s.tasks.Succeed(ctx, taskID, "备份已删除")
}

func (s *Service) Pin(ctx context.Context, command PinCommand) (domain.Backup, error) {
	command.ID = strings.TrimSpace(command.ID)
	detail := map[string]any{"backupId": command.ID, "pinned": command.Pinned}
	if !domain.CanCreateBackups(command.Request.Session.User.Role) {
		return domain.Backup{}, s.mutationFailure(ctx, command.Request, "backup.pin", command.ID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Backup{}, s.mutationFailure(ctx, command.Request, "backup.pin", command.ID, detail, domain.ErrReadOnly)
	}
	item, err := s.repository.GetBackup(ctx, command.ID)
	if err != nil {
		return domain.Backup{}, s.mutationFailure(ctx, command.Request, "backup.pin", command.ID, detail, err)
	}
	item.Pinned = command.Pinned
	updated, err := s.repository.UpdateBackup(ctx, item)
	if err != nil {
		return domain.Backup{}, s.mutationFailure(ctx, command.Request, "backup.pin", command.ID, detail, err)
	}
	if err := s.recordAudit(ctx, command.Request, "backup.pin", item.ID, "success", detail, nil); err != nil {
		return domain.Backup{}, err
	}
	return updated, nil
}
