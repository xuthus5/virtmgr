package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

type Store interface {
	ListPendingDataPlaneOperations(ctx context.Context) ([]domain.DataPlaneOperation, error)
	FinishDataPlaneOperation(ctx context.Context, id, status, message string, at time.Time) error
	UpdateInterruptedTaskDetail(ctx context.Context, id, step, taskError string, at time.Time) error
}

type BackupRepository interface {
	GetBackup(ctx context.Context, id string) (domain.Backup, error)
	UpdateBackup(ctx context.Context, item domain.Backup) (domain.Backup, error)
}

type Backend interface {
	RecoverDataPlaneOperation(ctx context.Context, operation domain.DataPlaneOperation) (domain.DataPlaneRecovery, error)
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type Locker interface {
	Block(key, reason string)
}

type ErrorHandler func(error)

type Option func(*Service)

func WithClock(now func() time.Time) Option {
	return func(service *Service) { service.now = now }
}

func WithErrorHandler(handler ErrorHandler) Option {
	return func(service *Service) { service.onError = handler }
}

type Summary struct {
	Operations     int `json:"operations"`
	Recovered      int `json:"recovered"`
	NeedsAttention int `json:"needsAttention"`
	Failed         int `json:"failed"`
}

type Service struct {
	store   Store
	backups BackupRepository
	backend Backend
	audit   AuditRecorder
	locker  Locker
	now     func() time.Time
	onError ErrorHandler
}

func New(store Store, backups BackupRepository, backend Backend, audit AuditRecorder, locker Locker, options ...Option) *Service {
	service := &Service{
		store: store, backups: backups, backend: backend, audit: audit, locker: locker,
		now: func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// Recover reconciles every operation that was still running when the process
// exited. A failure for one operation blocks that resource and continues with
// the remaining entries so one damaged VM cannot prevent host startup.
func (s *Service) Recover(ctx context.Context) (Summary, error) {
	operations, err := s.store.ListPendingDataPlaneOperations(ctx)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Operations: len(operations)}
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		recovery, recoveryErr := s.backend.RecoverDataPlaneOperation(ctx, operation)
		status, message := recoveryOutcome(recovery, recoveryErr)
		switch status {
		case domain.DataPlaneStatusNeedsAttention:
			summary.NeedsAttention++
		case domain.DataPlaneStatusFailed:
			summary.Failed++
		default:
			summary.Recovered++
		}

		if status == domain.DataPlaneStatusRecovered && recovery.CompletedBackup != nil {
			if err := s.completeBackup(ctx, operation, *recovery.CompletedBackup); err != nil {
				status = domain.DataPlaneStatusFailed
				message = err.Error()
				summary.Recovered--
				summary.Failed++
			}
		}
		now := s.now().UTC()
		if err := s.store.FinishDataPlaneOperation(ctx, operation.ID, status, message, now); err != nil && !errors.Is(err, domain.ErrNotFound) {
			return summary, fmt.Errorf("finish recovered data plane operation %s: %w", operation.ID, err)
		}
		s.updateInterruptedTask(ctx, operation, status, message, now)
		if status == domain.DataPlaneStatusFailed || status == domain.DataPlaneStatusNeedsAttention {
			s.blockOperation(operation, message)
		}
		if err := s.recordAudit(ctx, operation, status, recovery.Status, message, now); err != nil && s.onError != nil {
			s.onError(fmt.Errorf("record data plane recovery audit %s: %w", operation.ID, err))
		}
	}
	return summary, nil
}

func recoveryOutcome(recovery domain.DataPlaneRecovery, err error) (string, string) {
	if err != nil {
		return domain.DataPlaneStatusFailed, err.Error()
	}
	message := strings.TrimSpace(recovery.Message)
	switch recovery.Status {
	case domain.DataPlaneRecoveryNeedsAttention:
		if message == "" {
			message = "崩溃恢复需要人工处理"
		}
		return domain.DataPlaneStatusNeedsAttention, message
	case domain.DataPlaneRecoveryCompleted, domain.DataPlaneRecoveryRolledBack,
		domain.DataPlaneRecoveryCleaned, domain.DataPlaneRecoveryNoop:
		if message == "" {
			message = "崩溃恢复已完成"
		}
		return domain.DataPlaneStatusRecovered, message
	default:
		return domain.DataPlaneStatusFailed, "崩溃恢复返回了未知结果"
	}
}

func (s *Service) completeBackup(ctx context.Context, operation domain.DataPlaneOperation, recovered domain.VMBackup) error {
	if s.backups == nil || strings.TrimSpace(operation.BackupID) == "" {
		return nil
	}
	item, err := s.backups.GetBackup(ctx, operation.BackupID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load interrupted backup %s: %w", operation.BackupID, err)
	}
	now := s.now().UTC()
	item.Status = domain.BackupStatusCompleted
	item.Error = ""
	item.CompletedAt = &now
	item.FormatVersion = firstNonEmpty(recovered.FormatVersion, item.FormatVersion)
	item.ManifestVolume = firstNonEmpty(recovered.ManifestVolume, item.ManifestVolume)
	item.ConfigVolume = firstNonEmpty(recovered.ConfigVolume, item.ConfigVolume)
	item.ConfigBytes = recovered.ConfigBytes
	item.TotalBytes = recovered.TotalBytes
	item.DiskCount = len(recovered.Disks)
	item.Disks = recoveredBackupDisks(recovered.Disks)
	item.Online = recovered.Online
	item.Quiesced = recovered.Quiesced
	item.Consistent = recovered.Consistent
	item.ContainsTPM = recovered.ContainsTPM
	item.TPMStateSaved = recovered.TPMStateSaved
	if _, err := s.backups.UpdateBackup(ctx, item); err != nil {
		return fmt.Errorf("complete interrupted backup %s: %w", operation.BackupID, err)
	}
	return nil
}

func recoveredBackupDisks(items []domain.VMBackupDisk) []domain.BackupDisk {
	disks := make([]domain.BackupDisk, 0, len(items))
	for index, disk := range items {
		disks = append(disks, domain.BackupDisk{
			Sequence: index, Target: disk.Target, SourceVolume: disk.SourceVolume,
			Artifact: disk.BackupVolume, Format: disk.Format, CapacityBytes: disk.CapacityBytes, SHA256: disk.SHA256,
		})
	}
	return disks
}

func (s *Service) updateInterruptedTask(ctx context.Context, operation domain.DataPlaneOperation, status, message string, now time.Time) {
	if strings.TrimSpace(operation.TaskID) == "" {
		return
	}
	step := "数据面操作崩溃恢复完成"
	switch status {
	case domain.DataPlaneStatusNeedsAttention:
		step = "数据面操作崩溃恢复需人工处理"
	case domain.DataPlaneStatusFailed:
		step = "数据面操作崩溃恢复失败"
	}
	err := s.store.UpdateInterruptedTaskDetail(ctx, operation.TaskID, step, message, now)
	if err != nil && !errors.Is(err, domain.ErrNotFound) && s.onError != nil {
		s.onError(fmt.Errorf("update interrupted task %s: %w", operation.TaskID, err))
	}
}

func (s *Service) blockOperation(operation domain.DataPlaneOperation, reason string) {
	if s.locker == nil {
		return
	}
	if operation.VMUUID != "" {
		s.locker.Block(domain.VMLockKey(operation.VMUUID), reason)
	}
	if operation.VMName != "" {
		s.locker.Block(domain.VMNameLockKey(operation.VMName), reason)
	}
	if operation.TargetPool != "" && operation.TargetName != "" {
		s.locker.Block(domain.StorageVolumeLockKey(operation.TargetPool, operation.TargetName), reason)
	}
}

func (s *Service) recordAudit(ctx context.Context, operation domain.DataPlaneOperation, status, recoveryStatus, message string, now time.Time) error {
	if s.audit == nil {
		return nil
	}
	result := "success"
	if status == domain.DataPlaneStatusFailed || status == domain.DataPlaneStatusNeedsAttention {
		result = "failure"
	}
	targetKind, targetID := "vm", operation.VMUUID
	if targetID == "" {
		targetID = operation.VMName
	}
	if operation.Kind == domain.DataPlaneOperationConvert || operation.Kind == domain.DataPlaneOperationFlatten {
		targetKind = "storage_volume"
		targetID = operation.SourcePool + "/" + operation.SourceVolume
	}
	return s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: now, Actor: "system", Role: domain.RoleAdmin,
		Action: "dataplane.recovery", TargetKind: targetKind, TargetID: targetID,
		Result: result, Detail: map[string]any{
			"operationId": operation.ID, "kind": operation.Kind, "taskId": operation.TaskID,
			"backupId": operation.BackupID, "phase": operation.Phase, "status": status,
			"recoveryStatus": recoveryStatus, "message": message,
		},
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
