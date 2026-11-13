package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"virtmgr/internal/domain"
)

const (
	backupTimeout  = 6 * time.Hour
	restoreTimeout = 8 * time.Hour
	deleteTimeout  = 2 * time.Hour
	verifyTimeout  = 30 * time.Minute
)

type Repository interface {
	ListBackupRepositories(ctx context.Context) ([]domain.BackupRepository, error)
	GetBackupRepository(ctx context.Context, id string) (domain.BackupRepository, error)
	BackupRepositoryByPool(ctx context.Context, poolName string) (domain.BackupRepository, error)
	DefaultBackupRepository(ctx context.Context) (domain.BackupRepository, error)
	CreateBackupRepository(ctx context.Context, item domain.BackupRepository) (domain.BackupRepository, error)
	UpdateBackupRepository(ctx context.Context, item domain.BackupRepository) (domain.BackupRepository, error)
	DeleteBackupRepository(ctx context.Context, id string) error
	ListBackups(ctx context.Context, query domain.BackupQuery) (domain.BackupList, error)
	GetBackup(ctx context.Context, id string) (domain.Backup, error)
	CreateBackup(ctx context.Context, item domain.Backup) (domain.Backup, error)
	UpdateBackup(ctx context.Context, item domain.Backup) (domain.Backup, error)
	DeleteBackup(ctx context.Context, id string) error
	BackupSummary(ctx context.Context, vmUUID string) (domain.BackupSummary, error)
	ListCompletedBackups(ctx context.Context, vmUUID string, limit int) ([]domain.Backup, error)
	ListBackupSchedules(ctx context.Context, vmUUID string) ([]domain.BackupSchedule, error)
	GetBackupSchedule(ctx context.Context, id int64) (domain.BackupSchedule, error)
	CreateBackupSchedule(ctx context.Context, item domain.BackupSchedule) (domain.BackupSchedule, error)
	UpdateBackupSchedule(ctx context.Context, item domain.BackupSchedule) (domain.BackupSchedule, error)
	DeleteBackupSchedule(ctx context.Context, id int64) error
	ListDueBackupSchedules(ctx context.Context, now time.Time, limit int) ([]domain.BackupSchedule, error)
}

type VMBackend interface {
	GetVM(ctx context.Context, uuid string) (domain.VM, error)
	ListStoragePools(ctx context.Context) ([]domain.StoragePool, error)
	ListStorageVolumes(ctx context.Context, poolName string) ([]domain.StorageVolume, error)
	BackupVM(ctx context.Context, uuid string, spec domain.VMBackupSpec, progress func(percent int, step string)) (domain.VMBackup, error)
	VerifyVMBackup(ctx context.Context, backup domain.Backup, deep bool) (domain.BackupVerification, error)
	RestoreVMBackup(ctx context.Context, backup domain.Backup, spec domain.VMRestoreSpec, progress func(percent int, step string)) (domain.VM, error)
	DeleteVMBackup(ctx context.Context, backup domain.Backup) error
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type TaskRecorder interface {
	Start(ctx context.Context, kind, targetKind, targetID, actor, step string) (string, error)
	Progress(ctx context.Context, id string, progress int, step string) error
	Succeed(ctx context.Context, id, step string) error
	Fail(ctx context.Context, id string, progress int, step string, cause error) error
}

type TaskMetadataRecorder interface {
	SetMetadata(ctx context.Context, id string, metadata map[string]any) error
}

type ConfigVersionRecorder interface {
	CaptureVM(ctx context.Context, vmUUID string, capture domain.VMConfigCapture) (domain.VMConfigVersion, error)
}

type ErrorHandler func(error)

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type CreateCommand struct {
	VMUUID  string
	Spec    domain.BackupCreateSpec
	Request MutationContext
}

type VerifyCommand struct {
	ID      string
	Deep    bool
	Request MutationContext
}

type RestoreCommand struct {
	ID      string
	Spec    domain.VMRestoreSpec
	Request MutationContext
}

type DeleteCommand struct {
	ID      string
	Request MutationContext
}

type PinCommand struct {
	ID      string
	Pinned  bool
	Request MutationContext
}

type RepositoryCommand struct {
	ID      string
	Spec    domain.BackupRepositorySpec
	Request MutationContext
}

type ScheduleCommand struct {
	ID      int64
	VMUUID  string
	Spec    domain.BackupScheduleSpec
	Request MutationContext
}

type ScheduleDeleteCommand struct {
	ID      int64
	Request MutationContext
}

type ScheduleRunCommand struct {
	ID      int64
	Request MutationContext
}

type Service struct {
	repository     Repository
	backend        VMBackend
	audit          AuditRecorder
	tasks          TaskRecorder
	readOnly       bool
	readOnlyMode   domain.ReadOnlyProvider
	now            func() time.Time
	onError        ErrorHandler
	configVersions ConfigVersionRecorder
	runMu          sync.Mutex
}

type Option func(*Service)

func WithReadOnlyMode(mode domain.ReadOnlyProvider) Option {
	return func(service *Service) { service.readOnlyMode = mode }
}

func WithClock(now func() time.Time) Option {
	return func(service *Service) { service.now = now }
}

func WithErrorHandler(handler ErrorHandler) Option {
	return func(service *Service) { service.onError = handler }
}

func WithConfigVersionRecorder(recorder ConfigVersionRecorder) Option {
	return func(service *Service) { service.configVersions = recorder }
}

func New(repository Repository, backend VMBackend, audit AuditRecorder, tasks TaskRecorder, readOnly bool, options ...Option) *Service {
	service := &Service{
		repository: repository, backend: backend, audit: audit, tasks: tasks, readOnly: readOnly,
		now: func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) isReadOnly() bool {
	if s.readOnlyMode != nil {
		return s.readOnlyMode.ReadOnly()
	}
	return s.readOnly
}

func (s *Service) List(ctx context.Context, query domain.BackupQuery) (domain.BackupList, error) {
	query = query.Normalized()
	if query.Status != "" && !domain.ValidBackupStatus(query.Status) {
		return domain.BackupList{}, domain.ErrInvalidInput
	}
	return s.repository.ListBackups(ctx, query)
}

func (s *Service) Get(ctx context.Context, id string) (domain.Backup, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Backup{}, domain.ErrInvalidInput
	}
	return s.repository.GetBackup(ctx, id)
}

func (s *Service) Summary(ctx context.Context, vmUUID string) (domain.BackupSummary, error) {
	return s.repository.BackupSummary(ctx, vmUUID)
}

func (s *Service) ListRepositories(ctx context.Context) (domain.BackupRepositoryList, error) {
	if err := s.syncRepositories(ctx); err != nil {
		return domain.BackupRepositoryList{}, err
	}
	items, err := s.repository.ListBackupRepositories(ctx)
	if err != nil {
		return domain.BackupRepositoryList{}, err
	}
	return domain.BackupRepositoryList{Items: items, Total: len(items)}, nil
}

func (s *Service) Create(ctx context.Context, command CreateCommand) (domain.Task, error) {
	command.VMUUID = strings.TrimSpace(command.VMUUID)
	command.Spec.Name = strings.TrimSpace(command.Spec.Name)
	command.Spec.TargetPool = strings.TrimSpace(command.Spec.TargetPool)
	command.Spec.RepositoryID = strings.TrimSpace(command.Spec.RepositoryID)
	detail := map[string]any{"backupName": command.Spec.Name, "targetPool": command.Spec.TargetPool, "repositoryId": command.Spec.RepositoryID}
	if !domain.CanCreateBackups(command.Request.Session.User.Role) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.create", command.VMUUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.create", command.VMUUID, detail, domain.ErrReadOnly)
	}
	if s.tasks == nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.create", command.VMUUID, detail, domain.ErrUnavailable)
	}
	if command.VMUUID == "" {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.create", command.VMUUID, detail, domain.ErrInvalidInput)
	}
	if err := domain.ValidateBackupName(command.Spec.Name); err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.create", command.VMUUID, detail, err)
	}
	source, err := s.backend.GetVM(ctx, command.VMUUID)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.create", command.VMUUID, detail, err)
	}
	detail["sourceName"] = source.Name
	detail["sourceState"] = string(source.State)
	if (source.State != domain.VMStateShutoff && source.State != domain.VMStateCrashed) || !source.Persistent {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.create", command.VMUUID, detail, domain.ErrActionNotAllowed)
	}
	repository, err := s.resolveRepository(ctx, command.Spec)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.create", command.VMUUID, detail, err)
	}
	command.Spec.TargetPool = repository.PoolName
	detail["targetPool"] = repository.PoolName
	detail["repositoryId"] = repository.ID
	backupID, err := newBackupID()
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.create", command.VMUUID, detail, err)
	}
	detail["backupId"] = backupID
	startStep := fmt.Sprintf("准备备份 %s", command.Spec.Name)
	taskID, err := s.tasks.Start(ctx, "vm.backup", "vm", command.VMUUID, command.Request.Session.User.Username, startStep)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.create", command.VMUUID, detail, err)
	}
	if recorder, ok := s.tasks.(TaskMetadataRecorder); ok {
		_ = recorder.SetMetadata(ctx, taskID, map[string]any{
			"vmUuid": command.VMUUID, "name": command.Spec.Name,
			"targetPool": command.Spec.TargetPool, "repositoryId": command.Spec.RepositoryID,
		})
	}
	now := s.now().UTC()
	item, err := s.repository.CreateBackup(ctx, domain.Backup{
		ID: backupID, VMUUID: command.VMUUID, VMName: source.Name, RepositoryID: repository.ID,
		TargetPool: repository.PoolName, Name: command.Spec.Name, Status: domain.BackupStatusRunning,
		FormatVersion: domain.VMBackupFormatVersionV2, ManifestVolume: backupID + "-manifest.json",
		ConfigVolume: backupID + "-domain.xml", VerifyStatus: domain.BackupVerifyUnknown,
		CreatedBy: command.Request.Session.User.Username, CreatedAt: now,
	})
	if err != nil {
		_ = s.tasks.Fail(ctx, taskID, 0, "创建备份记录失败", err)
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "backup.create", command.VMUUID, detail, err)
	}
	task := domain.Task{
		ID: taskID, Kind: "vm.backup", TargetKind: "vm", TargetID: command.VMUUID,
		Status: domain.TaskStatusRunning, Step: startStep, Actor: command.Request.Session.User.Username,
		CreatedAt: now, UpdatedAt: now,
	}
	jobContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), backupTimeout)
	go func() {
		defer cancel()
		s.executeBackup(jobContext, taskID, item, command, detail)
	}()
	return task, nil
}

func (s *Service) RetryTask(ctx context.Context, task domain.Task, session domain.Session, requestID, sourceIP string) (domain.Task, error) {
	if task.Kind != "vm.backup" || (task.Status != domain.TaskStatusFailed && task.Status != domain.TaskStatusInterrupted) {
		return domain.Task{}, domain.ErrInvalidInput
	}
	vmUUID, _ := task.Metadata["vmUuid"].(string)
	spec := domain.BackupCreateSpec{
		Name:         stringMetadata(task.Metadata, "name"),
		TargetPool:   stringMetadata(task.Metadata, "targetPool"),
		RepositoryID: stringMetadata(task.Metadata, "repositoryId"),
	}
	if vmUUID == "" || spec.Name == "" || (spec.TargetPool == "" && spec.RepositoryID == "") {
		return domain.Task{}, domain.ErrInvalidInput
	}
	return s.Create(ctx, CreateCommand{
		VMUUID: vmUUID, Spec: spec,
		Request: MutationContext{Session: session, RequestID: requestID, SourceIP: sourceIP},
	})
}

func stringMetadata(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func (s *Service) executeBackup(ctx context.Context, taskID string, item domain.Backup, command CreateCommand, detail map[string]any) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	progress := 0
	result, err := s.backend.BackupVM(ctx, item.VMUUID, domain.VMBackupSpec{
		BackupID: item.ID, Name: item.Name, TargetPool: item.TargetPool,
	}, func(percent int, step string) {
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
		_ = s.recordAudit(ctx, command.Request, "backup.create", item.ID, "failure", detail, err)
		_ = s.tasks.Fail(ctx, taskID, progress, "备份虚拟机失败", err)
		return
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
	updated, saveErr := s.repository.UpdateBackup(ctx, item)
	if saveErr != nil {
		_ = s.recordAudit(ctx, command.Request, "backup.create", item.ID, "failure", detail, saveErr)
		_ = s.tasks.Fail(ctx, taskID, 100, "备份完成但写入目录失败", saveErr)
		return
	}
	detail["diskCount"] = updated.DiskCount
	detail["totalBytes"] = updated.TotalBytes
	if err := s.recordAudit(ctx, command.Request, "backup.create", item.ID, "success", detail, nil); err != nil {
		_ = s.tasks.Fail(ctx, taskID, 100, "备份完成但审计写入失败", err)
		return
	}
	_ = s.tasks.Succeed(ctx, taskID, fmt.Sprintf("虚拟机 %s 备份完成", result.VMName))
}

func (s *Service) resolveRepository(ctx context.Context, spec domain.BackupCreateSpec) (domain.BackupRepository, error) {
	if spec.RepositoryID != "" {
		item, err := s.repository.GetBackupRepository(ctx, spec.RepositoryID)
		if err != nil {
			return domain.BackupRepository{}, err
		}
		if spec.TargetPool != "" && spec.TargetPool != item.PoolName {
			return domain.BackupRepository{}, domain.ErrInvalidInput
		}
		return item, nil
	}
	if spec.TargetPool != "" {
		item, err := s.repository.BackupRepositoryByPool(ctx, spec.TargetPool)
		if err == nil {
			return item, nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return domain.BackupRepository{}, err
		}
		pool, err := s.poolByName(ctx, spec.TargetPool)
		if err != nil {
			return domain.BackupRepository{}, err
		}
		if pool.State != domain.StoragePoolStateActive {
			return domain.BackupRepository{}, domain.ErrUnavailable
		}
		existing, listErr := s.repository.ListBackupRepositories(ctx)
		if listErr != nil {
			return domain.BackupRepository{}, listErr
		}
		pools, listErr := s.backend.ListStoragePools(ctx)
		if listErr != nil {
			return domain.BackupRepository{}, listErr
		}
		isDefault := len(existing) == 0 && preferredDefaultPoolName(pools) == pool.Name
		return s.createRepositoryForPool(ctx, pool.Name, pool.Name, isDefault)
	}
	item, err := s.repository.DefaultBackupRepository(ctx)
	if err != nil {
		return domain.BackupRepository{}, err
	}
	return item, nil
}

func (s *Service) syncRepositories(ctx context.Context) error {
	pools, err := s.backend.ListStoragePools(ctx)
	if err != nil {
		return err
	}
	existing, err := s.repository.ListBackupRepositories(ctx)
	if err != nil {
		return err
	}
	known := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		known[item.PoolName] = struct{}{}
	}
	defaultPoolName := preferredDefaultPoolName(pools)
	for _, pool := range pools {
		if pool.State != domain.StoragePoolStateActive {
			continue
		}
		if _, ok := known[pool.Name]; ok {
			continue
		}
		if _, err := s.createRepositoryForPool(ctx, pool.Name, pool.Name, len(existing) == 0 && pool.Name == defaultPoolName); err != nil && !errors.Is(err, domain.ErrConflict) {
			return err
		}
	}
	return nil
}

func preferredDefaultPoolName(pools []domain.StoragePool) string {
	fallback := ""
	for _, pool := range pools {
		if pool.State != domain.StoragePoolStateActive {
			continue
		}
		if pool.Name == "default" {
			return pool.Name
		}
		if fallback == "" {
			fallback = pool.Name
		}
	}
	return fallback
}

func (s *Service) poolByName(ctx context.Context, name string) (domain.StoragePool, error) {
	pools, err := s.backend.ListStoragePools(ctx)
	if err != nil {
		return domain.StoragePool{}, err
	}
	for _, pool := range pools {
		if pool.Name == name {
			return pool, nil
		}
	}
	return domain.StoragePool{}, domain.ErrNotFound
}

func (s *Service) createRepositoryForPool(ctx context.Context, name, poolName string, isDefault bool) (domain.BackupRepository, error) {
	id, err := newRepositoryID()
	if err != nil {
		return domain.BackupRepository{}, err
	}
	now := s.now().UTC()
	return s.repository.CreateBackupRepository(ctx, domain.BackupRepository{
		ID: id, Name: name, Type: "libvirt-pool", PoolName: poolName, IsDefault: isDefault, CreatedAt: now, UpdatedAt: now,
	})
}

func backupDisks(disks []domain.VMBackupDisk) []domain.BackupDisk {
	result := make([]domain.BackupDisk, 0, len(disks))
	for index, disk := range disks {
		result = append(result, domain.BackupDisk{
			Sequence: index, Target: disk.Target, SourceVolume: disk.SourceVolume, Artifact: disk.BackupVolume,
			Format: disk.Format, CapacityBytes: disk.CapacityBytes, SHA256: disk.SHA256,
		})
	}
	return result
}

func backupRecordInput(item domain.Backup) domain.VMBackup {
	disks := make([]domain.VMBackupDisk, 0, len(item.Disks))
	for _, disk := range item.Disks {
		disks = append(disks, domain.VMBackupDisk{
			SourceVolume: disk.SourceVolume, BackupVolume: disk.Artifact, Target: disk.Target,
			Format: disk.Format, CapacityBytes: disk.CapacityBytes, SHA256: disk.SHA256,
		})
	}
	return domain.VMBackup{
		BackupID: item.ID, FormatVersion: item.FormatVersion, CreatedAt: item.CreatedAt,
		VMUUID: item.VMUUID, VMName: item.VMName, TargetPool: item.TargetPool, Name: item.Name,
		ManifestVolume: item.ManifestVolume, ConfigVolume: item.ConfigVolume, ConfigBytes: item.ConfigBytes,
		Disks: disks, TotalBytes: item.TotalBytes,
	}
}

func newBackupID() (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate backup id: %w", err)
	}
	return "vmb_" + hex.EncodeToString(value[:]), nil
}

func newRepositoryID() (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate backup repository id: %w", err)
	}
	return "brepo_" + hex.EncodeToString(value[:]), nil
}

func (s *Service) mutationFailure(ctx context.Context, request MutationContext, action, targetID string, detail map[string]any, cause error) error {
	if err := s.recordAudit(ctx, request, action, targetID, "failure", detail, cause); err != nil {
		return errors.Join(cause, fmt.Errorf("record %s audit: %w", action, err))
	}
	return cause
}

func (s *Service) recordAudit(ctx context.Context, request MutationContext, action, targetID, result string, detail map[string]any, cause error) error {
	if s.audit == nil {
		return nil
	}
	if detail == nil {
		detail = map[string]any{}
	}
	if cause != nil {
		detail["error"] = cause.Error()
	}
	return s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: s.now().UTC(), RequestID: request.RequestID, Actor: request.Session.User.Username,
		Role: request.Session.User.Role, Action: action, TargetKind: "backup", TargetID: targetID,
		SourceIP: request.SourceIP, Result: result, Detail: detail,
	})
}
