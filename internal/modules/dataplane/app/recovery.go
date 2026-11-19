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
	ListDataPlaneOperations(ctx context.Context, limit int) ([]domain.DataPlaneOperation, error)
	ListUnresolvedDataPlaneOperations(ctx context.Context) ([]domain.DataPlaneOperation, error)
	GetDataPlaneOperation(ctx context.Context, id string) (domain.DataPlaneOperation, error)
	FinishDataPlaneOperation(ctx context.Context, id, status, message string, at time.Time) error
	ResolveDataPlaneOperation(ctx context.Context, id, status, message string, at time.Time) error
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
	Blocked(key string) bool
	Unblock(key string) bool
}

type OperationCommand struct {
	ID      string
	Request MutationContext
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
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

func (s *Service) ListOperations(ctx context.Context, limit int) (domain.DataPlaneOperationList, error) {
	items, err := s.store.ListDataPlaneOperations(ctx, limit)
	if err != nil {
		return domain.DataPlaneOperationList{}, err
	}
	return domain.DataPlaneOperationList{Items: items, Total: len(items)}, nil
}

func (s *Service) GetOperation(ctx context.Context, id string) (domain.DataPlaneOperation, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.DataPlaneOperation{}, domain.ErrInvalidInput
	}
	return s.store.GetDataPlaneOperation(ctx, id)
}

// Recheck reconciles a failed or needs-attention operation against the current
// libvirt state. It is safe to call repeatedly.
func (s *Service) Recheck(ctx context.Context, command OperationCommand) (domain.DataPlaneOperationResult, error) {
	return s.reconcileOperatorOperation(ctx, command, false)
}

// Unblock rechecks the operation first and only clears recovery guards when
// the backend can prove that the interrupted mutation is safe to close.
func (s *Service) Unblock(ctx context.Context, command OperationCommand) (domain.DataPlaneOperationResult, error) {
	return s.reconcileOperatorOperation(ctx, command, true)
}

func (s *Service) reconcileOperatorOperation(ctx context.Context, command OperationCommand, unblock bool) (domain.DataPlaneOperationResult, error) {
	command.ID = strings.TrimSpace(command.ID)
	if command.ID == "" || command.Request.Session.User.Role != domain.RoleAdmin {
		if command.Request.Session.User.Role != domain.RoleAdmin {
			return domain.DataPlaneOperationResult{}, domain.ErrForbidden
		}
		return domain.DataPlaneOperationResult{}, domain.ErrInvalidInput
	}
	operation, err := s.store.GetDataPlaneOperation(ctx, command.ID)
	if err != nil {
		return domain.DataPlaneOperationResult{}, err
	}
	if operation.Status != domain.DataPlaneStatusNeedsAttention && operation.Status != domain.DataPlaneStatusFailed {
		return domain.DataPlaneOperationResult{}, errors.Join(domain.ErrActionNotAllowed, errors.New("only failed or needs-attention operations can be rechecked"))
	}
	recovery, recoveryErr := s.backend.RecoverDataPlaneOperation(ctx, operation)
	status, message := recoveryOutcome(recovery, recoveryErr)
	if status == domain.DataPlaneStatusRecovered && recovery.CompletedBackup != nil {
		if err := s.completeBackup(ctx, operation, *recovery.CompletedBackup); err != nil {
			status = domain.DataPlaneStatusFailed
			message = err.Error()
		}
	}
	now := s.now().UTC()
	if err := s.store.ResolveDataPlaneOperation(ctx, operation.ID, status, message, now); err != nil {
		return domain.DataPlaneOperationResult{}, err
	}
	operation.Status = status
	operation.Error = message
	operation.UpdatedAt = now
	s.updateInterruptedTask(ctx, operation, status, message, now)
	action := "dataplane.recheck"
	if unblock {
		action = "dataplane.unblock"
	}
	if err := s.recordOperatorAudit(ctx, command.Request, action, operation, status, recovery.Status, message, now); err != nil && s.onError != nil {
		s.onError(err)
	}
	if status == domain.DataPlaneStatusRecovered {
		if unblock {
			s.unblockResolvedOperation(ctx, operation)
		}
	} else {
		s.blockOperation(operation, message)
	}
	result := domain.DataPlaneOperationResult{Operation: operation, Recovery: recovery}
	if unblock && status != domain.DataPlaneStatusRecovered {
		return result, fmt.Errorf("%w: 重新校验仍未通过，资源继续保持阻塞：%s", domain.ErrOperationConflict, message)
	}
	return result, nil
}

func (s *Service) unblockResolvedOperation(ctx context.Context, operation domain.DataPlaneOperation) {
	if s.locker == nil {
		return
	}
	protected := make(map[string]struct{})
	operations, err := s.store.ListUnresolvedDataPlaneOperations(ctx)
	if err != nil {
		if s.onError != nil {
			s.onError(fmt.Errorf("list unresolved operations before unblock: %w", err))
		}
		return
	}
	for _, item := range operations {
		for _, key := range operationLockKeys(item) {
			protected[key] = struct{}{}
		}
	}
	for _, key := range operationLockKeys(operation) {
		if _, exists := protected[key]; exists {
			continue
		}
		s.locker.Unblock(key)
	}
}

func operationLockKeys(operation domain.DataPlaneOperation) []string {
	keys := make([]string, 0, 3)
	if operation.VMUUID != "" {
		keys = append(keys, domain.VMLockKey(operation.VMUUID))
	}
	if operation.VMName != "" {
		keys = append(keys, domain.VMNameLockKey(operation.VMName))
	}
	if operation.TargetPool != "" && operation.TargetName != "" {
		keys = append(keys, domain.StorageVolumeLockKey(operation.TargetPool, operation.TargetName))
	}
	return keys
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
	for _, key := range operationLockKeys(operation) {
		s.locker.Block(key, reason)
	}
}

func (s *Service) recordOperatorAudit(ctx context.Context, request MutationContext, action string, operation domain.DataPlaneOperation, status, recoveryStatus, message string, now time.Time) error {
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
	return errors.Join(s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: now, RequestID: request.RequestID,
		Actor: request.Session.User.Username, Role: request.Session.User.Role,
		Action: action, TargetKind: targetKind, TargetID: targetID,
		SourceIP: request.SourceIP, Result: result,
		Detail: map[string]any{
			"operationId": operation.ID, "kind": operation.Kind, "taskId": operation.TaskID,
			"status": status, "recoveryStatus": recoveryStatus, "message": message,
		},
	}), nil)
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
