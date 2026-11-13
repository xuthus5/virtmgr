package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

const (
	minScheduleIntervalHours = 1
	maxScheduleIntervalHours = 24 * 7
	minScheduleRetention     = 1
	maxScheduleRetention     = 50
	maxScheduleRetentionDays = 3650
)

func (s *Service) ListSchedules(ctx context.Context, vmUUID string) (domain.BackupScheduleList, error) {
	items, err := s.repository.ListBackupSchedules(ctx, strings.TrimSpace(vmUUID))
	if err != nil {
		return domain.BackupScheduleList{}, err
	}
	for index := range items {
		if vm, vmErr := s.backend.GetVM(ctx, items[index].VMUUID); vmErr == nil {
			items[index].VMName = vm.Name
		}
	}
	return domain.BackupScheduleList{Items: items, Total: len(items)}, nil
}

func (s *Service) CreateSchedule(ctx context.Context, command ScheduleCommand) (domain.BackupSchedule, error) {
	command.VMUUID = strings.TrimSpace(command.VMUUID)
	command.Spec.VMUUID = command.VMUUID
	spec, err := normalizeScheduleSpec(command.Spec)
	if err != nil {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.create", command.VMUUID, scheduleDetail(spec), err)
	}
	if !domain.CanCreateBackups(command.Request.Session.User.Role) {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.create", command.VMUUID, scheduleDetail(spec), domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.create", command.VMUUID, scheduleDetail(spec), domain.ErrReadOnly)
	}
	vm, err := s.backend.GetVM(ctx, command.VMUUID)
	if err != nil {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.create", command.VMUUID, scheduleDetail(spec), err)
	}
	repository, err := s.repository.GetBackupRepository(ctx, spec.RepositoryID)
	if err != nil {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.create", command.VMUUID, scheduleDetail(spec), err)
	}
	pool, err := s.poolByName(ctx, repository.PoolName)
	if err != nil {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.create", command.VMUUID, scheduleDetail(spec), err)
	}
	if pool.State != domain.StoragePoolStateActive {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.create", command.VMUUID, scheduleDetail(spec), domain.ErrUnavailable)
	}
	now := s.now().UTC()
	item := domain.BackupSchedule{
		VMUUID: command.VMUUID, RepositoryID: repository.ID, TargetPool: repository.PoolName,
		Name: spec.Name, IntervalHours: spec.IntervalHours, ScheduleMode: spec.ScheduleMode, DailyTime: spec.DailyTime,
		RetentionCount: spec.RetentionCount, RetentionDays: spec.RetentionDays, Enabled: spec.Enabled,
		NextRunAt: domain.NextScheduleTime(now, spec.ScheduleMode, time.Duration(spec.IntervalHours)*time.Hour, spec.DailyTime),
		CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.repository.CreateBackupSchedule(ctx, item)
	if err != nil {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.create", command.VMUUID, scheduleDetail(spec), err)
	}
	detail := scheduleDetail(spec)
	detail["scheduleId"] = created.ID
	detail["vmName"] = vm.Name
	if err := s.recordAudit(ctx, command.Request, "backup.schedule.create", scheduleIDString(created.ID), "success", detail, nil); err != nil {
		return domain.BackupSchedule{}, err
	}
	created.VMName = vm.Name
	created.RepositoryName = repository.Name
	return created, nil
}

func (s *Service) UpdateSchedule(ctx context.Context, command ScheduleCommand) (domain.BackupSchedule, error) {
	if !domain.CanCreateBackups(command.Request.Session.User.Role) {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.update", fmt.Sprint(command.ID), map[string]any{"scheduleId": command.ID}, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.update", fmt.Sprint(command.ID), map[string]any{"scheduleId": command.ID}, domain.ErrReadOnly)
	}
	item, err := s.repository.GetBackupSchedule(ctx, command.ID)
	if err != nil {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.update", fmt.Sprint(command.ID), map[string]any{"scheduleId": command.ID}, err)
	}
	command.Spec.VMUUID = item.VMUUID
	spec, err := normalizeScheduleSpec(command.Spec)
	if err != nil {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.update", fmt.Sprint(command.ID), scheduleDetail(spec), err)
	}
	repository, err := s.repository.GetBackupRepository(ctx, spec.RepositoryID)
	if err != nil {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.update", fmt.Sprint(command.ID), scheduleDetail(spec), err)
	}
	item.RepositoryID = repository.ID
	item.TargetPool = repository.PoolName
	item.Name = spec.Name
	item.IntervalHours = spec.IntervalHours
	item.ScheduleMode = spec.ScheduleMode
	item.DailyTime = spec.DailyTime
	item.RetentionCount = spec.RetentionCount
	item.RetentionDays = spec.RetentionDays
	item.Enabled = spec.Enabled
	item.NextRunAt = domain.NextScheduleTime(s.now().UTC(), spec.ScheduleMode, time.Duration(spec.IntervalHours)*time.Hour, spec.DailyTime)
	item.UpdatedAt = s.now().UTC()
	updated, err := s.repository.UpdateBackupSchedule(ctx, item)
	if err != nil {
		return domain.BackupSchedule{}, s.mutationFailure(ctx, command.Request, "backup.schedule.update", fmt.Sprint(command.ID), scheduleDetail(spec), err)
	}
	detail := scheduleDetail(spec)
	detail["scheduleId"] = item.ID
	if err := s.recordAudit(ctx, command.Request, "backup.schedule.update", scheduleIDString(item.ID), "success", detail, nil); err != nil {
		return domain.BackupSchedule{}, err
	}
	updated.RepositoryName = repository.Name
	return updated, nil
}

func (s *Service) DeleteSchedule(ctx context.Context, command ScheduleDeleteCommand) error {
	if !domain.CanCreateBackups(command.Request.Session.User.Role) {
		return s.mutationFailure(ctx, command.Request, "backup.schedule.delete", fmt.Sprint(command.ID), map[string]any{"scheduleId": command.ID}, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return s.mutationFailure(ctx, command.Request, "backup.schedule.delete", fmt.Sprint(command.ID), map[string]any{"scheduleId": command.ID}, domain.ErrReadOnly)
	}
	if err := s.repository.DeleteBackupSchedule(ctx, command.ID); err != nil {
		return s.mutationFailure(ctx, command.Request, "backup.schedule.delete", fmt.Sprint(command.ID), map[string]any{"scheduleId": command.ID}, err)
	}
	return s.recordAudit(ctx, command.Request, "backup.schedule.delete", fmt.Sprint(command.ID), "success", map[string]any{"scheduleId": command.ID}, nil)
}

func (s *Service) RunScheduleNow(ctx context.Context, command ScheduleRunCommand) (domain.Task, error) {
	if !domain.CanCreateBackups(command.Request.Session.User.Role) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.schedule.run", fmt.Sprint(command.ID), map[string]any{"scheduleId": command.ID}, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.schedule.run", fmt.Sprint(command.ID), map[string]any{"scheduleId": command.ID}, domain.ErrReadOnly)
	}
	item, err := s.repository.GetBackupSchedule(ctx, command.ID)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.schedule.run", fmt.Sprint(command.ID), map[string]any{"scheduleId": command.ID}, err)
	}
	task, err := s.Create(ctx, CreateCommand{
		VMUUID: item.VMUUID,
		Spec: domain.BackupCreateSpec{
			Name: scheduledBackupName(item.Name, s.now()), TargetPool: item.TargetPool, RepositoryID: item.RepositoryID,
		},
		Request: command.Request,
	})
	if err != nil {
		return domain.Task{}, err
	}
	now := s.now().UTC()
	item.LastRunAt = &now
	item.NextRunAt = domain.NextScheduleTime(now, item.ScheduleMode, time.Duration(item.IntervalHours)*time.Hour, item.DailyTime)
	item.LastError = ""
	if _, err := s.repository.UpdateBackupSchedule(ctx, item); err != nil {
		_ = s.recordAudit(ctx, command.Request, "backup.schedule.run", fmt.Sprint(item.ID), "failure", map[string]any{"scheduleId": item.ID, "backupId": task.TargetID}, err)
		return domain.Task{}, err
	}
	if err := s.recordAudit(ctx, command.Request, "backup.schedule.run", fmt.Sprint(item.ID), "success", map[string]any{"scheduleId": item.ID, "backupId": task.TargetID, "vmUuid": item.VMUUID}, nil); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.runDueSchedules(ctx); err != nil && s.onError != nil {
				s.onError(err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) runDueSchedules(ctx context.Context) error {
	now := s.now().UTC()
	items, err := s.repository.ListDueBackupSchedules(ctx, now, 20)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !item.Enabled {
			continue
		}
		if err := s.executeScheduledBackup(ctx, item); err != nil {
			item.LastRunAt = &now
			item.NextRunAt = domain.NextScheduleTime(now, item.ScheduleMode, time.Duration(item.IntervalHours)*time.Hour, item.DailyTime)
			item.LastError = err.Error()
			item.UpdatedAt = now
			_, _ = s.repository.UpdateBackupSchedule(ctx, item)
			if s.onError != nil {
				s.onError(fmt.Errorf("backup schedule %d: %w", item.ID, err))
			}
		}
	}
	return nil
}

func (s *Service) executeScheduledBackup(ctx context.Context, schedule domain.BackupSchedule) error {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	request := MutationContext{Session: domain.Session{User: domain.User{Username: "system:scheduler", Role: domain.RoleAdmin}}, RequestID: "backup-scheduler"}
	vm, err := s.backend.GetVM(ctx, schedule.VMUUID)
	if err != nil {
		return err
	}
	detail := map[string]any{"scheduleId": schedule.ID, "vmUuid": vm.UUID, "vmName": vm.Name, "targetPool": schedule.TargetPool}
	if (vm.State != domain.VMStateShutoff && vm.State != domain.VMStateCrashed) || !vm.Persistent {
		cause := fmt.Errorf("%w: virtual machine must be shut off", domain.ErrActionNotAllowed)
		_ = s.recordAudit(ctx, request, "backup.schedule", vm.UUID, "failure", detail, cause)
		return cause
	}
	repository, err := s.repository.GetBackupRepository(ctx, schedule.RepositoryID)
	if err != nil {
		return err
	}
	name := scheduledBackupName(schedule.Name, s.now())
	backupID, err := newBackupID()
	if err != nil {
		return err
	}
	now := s.now().UTC()
	taskID, err := s.tasks.Start(ctx, "vm.backup", "vm", vm.UUID, request.Session.User.Username, fmt.Sprintf("定时备份 %s", name))
	if err != nil {
		return err
	}
	item, err := s.repository.CreateBackup(ctx, domain.Backup{
		ID: backupID, VMUUID: vm.UUID, VMName: vm.Name, RepositoryID: repository.ID,
		TargetPool: repository.PoolName, Name: name, Status: domain.BackupStatusRunning,
		FormatVersion: domain.VMBackupFormatVersionV2, ManifestVolume: backupID + "-manifest.json",
		ConfigVolume: backupID + "-domain.xml", VerifyStatus: domain.BackupVerifyUnknown,
		CreatedBy: request.Session.User.Username, CreatedAt: now,
	})
	if err != nil {
		_ = s.tasks.Fail(ctx, taskID, 0, "创建定时备份记录失败", err)
		return err
	}
	progress := 0
	result, err := s.backend.BackupVM(ctx, vm.UUID, domain.VMBackupSpec{BackupID: backupID, Name: name, TargetPool: repository.PoolName}, func(percent int, step string) {
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
		item.Status = domain.BackupStatusFailed
		item.Error = err.Error()
		_, _ = s.repository.UpdateBackup(ctx, item)
		_ = s.tasks.Fail(ctx, taskID, progress, "定时备份失败", err)
		_ = s.recordAudit(ctx, request, "backup.schedule", vm.UUID, "failure", detail, err)
		return err
	}
	completedAt := s.now().UTC()
	item.Status = domain.BackupStatusCompleted
	item.FormatVersion = result.FormatVersion
	item.ManifestVolume = result.ManifestVolume
	item.ConfigVolume = result.ConfigVolume
	item.ConfigBytes = result.ConfigBytes
	item.TotalBytes = result.TotalBytes
	item.DiskCount = len(result.Disks)
	item.CompletedAt = &completedAt
	item.Disks = backupDisks(result.Disks)
	if _, err := s.repository.UpdateBackup(ctx, item); err != nil {
		_ = s.tasks.Fail(ctx, taskID, 100, "定时备份完成但写入目录失败", err)
		return err
	}
	_ = s.tasks.Succeed(ctx, taskID, fmt.Sprintf("虚拟机 %s 定时备份完成", vm.Name))
	detail["backupId"] = item.ID
	if err := s.recordAudit(ctx, request, "backup.schedule", vm.UUID, "success", detail, nil); err != nil {
		return err
	}
	cleaned, cleanupErr := s.cleanupRetention(ctx, schedule, request)
	schedule.LastRunAt = &completedAt
	schedule.NextRunAt = domain.NextScheduleTime(completedAt, schedule.ScheduleMode, time.Duration(schedule.IntervalHours)*time.Hour, schedule.DailyTime)
	schedule.LastBackupID = item.ID
	schedule.LastError = ""
	if cleanupErr != nil {
		schedule.LastError = "保留清理失败: " + cleanupErr.Error()
	}
	schedule.UpdatedAt = completedAt
	_, err = s.repository.UpdateBackupSchedule(ctx, schedule)
	if cleanupErr != nil {
		return errors.Join(cleanupErr, err)
	}
	_ = cleaned
	return err
}

func (s *Service) cleanupRetention(ctx context.Context, schedule domain.BackupSchedule, request MutationContext) ([]string, error) {
	items, err := s.repository.ListCompletedBackups(ctx, schedule.VMUUID, 200)
	if err != nil {
		return nil, err
	}
	managed := make([]domain.Backup, 0, len(items))
	prefix := schedule.Name + "-"
	for _, item := range items {
		if strings.HasPrefix(item.Name, prefix) {
			managed = append(managed, item)
		}
	}
	sort.Slice(managed, func(i, j int) bool {
		left := managed[i].CreatedAt
		if managed[i].CompletedAt != nil {
			left = *managed[i].CompletedAt
		}
		right := managed[j].CreatedAt
		if managed[j].CompletedAt != nil {
			right = *managed[j].CompletedAt
		}
		return left.After(right)
	})
	now := s.now().UTC()
	deleted := make([]string, 0)
	for index, item := range managed {
		if item.Pinned {
			continue
		}
		if index < schedule.RetentionCount {
			continue
		}
		if schedule.RetentionDays > 0 {
			created := item.CreatedAt
			if item.CompletedAt != nil {
				created = *item.CompletedAt
			}
			if now.Sub(created) < time.Duration(schedule.RetentionDays)*24*time.Hour {
				continue
			}
		}
		if err := s.backend.DeleteVMBackup(ctx, item); err != nil {
			return deleted, err
		}
		if err := s.repository.DeleteBackup(ctx, item.ID); err != nil {
			return deleted, err
		}
		deleted = append(deleted, item.ID)
		detail := map[string]any{"backupId": item.ID, "scheduleId": schedule.ID, "reason": "retention"}
		_ = s.recordAudit(ctx, request, "backup.delete", item.ID, "success", detail, nil)
	}
	return deleted, nil
}

func normalizeScheduleSpec(spec domain.BackupScheduleSpec) (domain.BackupScheduleSpec, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.RepositoryID = strings.TrimSpace(spec.RepositoryID)
	if err := domain.ValidateBackupName(spec.Name); err != nil {
		return spec, err
	}
	if spec.RepositoryID == "" {
		return spec, fmt.Errorf("%w: repository is required", domain.ErrInvalidInput)
	}
	mode, err := domain.NormalizeScheduleMode(spec.ScheduleMode)
	if err != nil {
		return spec, err
	}
	dailyTime, err := domain.NormalizeDailyTime(spec.DailyTime)
	if err != nil {
		return spec, err
	}
	spec.ScheduleMode = mode
	spec.DailyTime = dailyTime
	if mode == domain.ScheduleModeDaily && spec.IntervalHours == 0 {
		spec.IntervalHours = 24
	}
	if spec.IntervalHours < minScheduleIntervalHours || spec.IntervalHours > maxScheduleIntervalHours {
		return spec, fmt.Errorf("%w: schedule interval must be between %d and %d hours", domain.ErrInvalidInput, minScheduleIntervalHours, maxScheduleIntervalHours)
	}
	if spec.RetentionCount < minScheduleRetention || spec.RetentionCount > maxScheduleRetention {
		return spec, fmt.Errorf("%w: retention count must be between %d and %d", domain.ErrInvalidInput, minScheduleRetention, maxScheduleRetention)
	}
	if spec.RetentionDays < 0 || spec.RetentionDays > maxScheduleRetentionDays {
		return spec, fmt.Errorf("%w: retention days must be between 0 and %d", domain.ErrInvalidInput, maxScheduleRetentionDays)
	}
	return spec, nil
}

func scheduleDetail(spec domain.BackupScheduleSpec) map[string]any {
	return map[string]any{
		"name": spec.Name, "repositoryId": spec.RepositoryID, "intervalHours": spec.IntervalHours,
		"scheduleMode": spec.ScheduleMode, "dailyTime": spec.DailyTime,
		"retentionCount": spec.RetentionCount, "retentionDays": spec.RetentionDays, "enabled": spec.Enabled,
	}
}

func scheduledBackupName(name string, now time.Time) string {
	suffix := "-" + now.UTC().Format("20060102T150405")
	name = strings.TrimSpace(name)
	if len(name)+len(suffix) > 64 {
		name = name[:64-len(suffix)]
	}
	return name + suffix
}

func scheduleIDString(id int64) string {
	return fmt.Sprintf("schedule-%d", id)
}
