package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"virtmgr/internal/domain"
)

type Repository interface {
	ListSnapshotPolicies(ctx context.Context, vmUUID string) ([]domain.SnapshotPolicy, error)
	GetSnapshotPolicy(ctx context.Context, id int64) (domain.SnapshotPolicy, error)
	CreateSnapshotPolicy(ctx context.Context, policy domain.SnapshotPolicy) (domain.SnapshotPolicy, error)
	UpdateSnapshotPolicy(ctx context.Context, policy domain.SnapshotPolicy) (domain.SnapshotPolicy, error)
	DeleteSnapshotPolicy(ctx context.Context, id int64) error
	ListDueSnapshotPolicies(ctx context.Context, now time.Time, limit int) ([]domain.SnapshotPolicy, error)
}

type VMRepository interface {
	GetVM(ctx context.Context, uuid string) (domain.VM, error)
	ListSnapshots(ctx context.Context, uuid string) ([]domain.VMSnapshot, error)
	CreateSnapshot(ctx context.Context, uuid string, spec domain.VMSnapshotCreateSpec) (domain.VMSnapshot, error)
	DeleteSnapshot(ctx context.Context, uuid, name string) error
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type ErrorHandler func(error)

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type CreateCommand struct {
	VMUUID  string
	Spec    domain.SnapshotPolicySpec
	Request MutationContext
}

type UpdateCommand struct {
	ID      int64
	Spec    domain.SnapshotPolicySpec
	Request MutationContext
}

type DeleteCommand struct {
	ID      int64
	Request MutationContext
}

type RunCommand struct {
	ID      int64
	Request MutationContext
}

type Service struct {
	repository   Repository
	vms          VMRepository
	audit        AuditRecorder
	readOnly     bool
	readOnlyMode domain.ReadOnlyProvider
	now          func() time.Time
	onError      ErrorHandler
	runMu        sync.Mutex
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

var (
	policyNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$`)
	minInterval       = 5
	maxInterval       = 60 * 24 * 7
	minRetention      = 1
	maxRetention      = 50
	maxPoliciesPerVM  = 10
)

func New(repository Repository, vms VMRepository, audit AuditRecorder, readOnly bool, options ...Option) *Service {
	service := &Service{
		repository: repository,
		vms:        vms,
		audit:      audit,
		readOnly:   readOnly,
		now:        func() time.Time { return time.Now().UTC() },
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

func (s *Service) List(ctx context.Context, vmUUID string) (domain.SnapshotPolicyList, error) {
	vmUUID = strings.TrimSpace(vmUUID)
	if vmUUID == "" {
		return domain.SnapshotPolicyList{}, domain.ErrInvalidInput
	}
	items, err := s.repository.ListSnapshotPolicies(ctx, vmUUID)
	if err != nil {
		return domain.SnapshotPolicyList{}, err
	}
	return domain.SnapshotPolicyList{Items: items, Total: len(items)}, nil
}

func (s *Service) Create(ctx context.Context, command CreateCommand) (domain.SnapshotPolicy, error) {
	command.VMUUID = strings.TrimSpace(command.VMUUID)
	spec, err := normalizeSpec(command.Spec)
	if err != nil {
		return domain.SnapshotPolicy{}, err
	}
	if command.VMUUID == "" {
		return domain.SnapshotPolicy{}, domain.ErrInvalidInput
	}
	detail := map[string]any{"vmUuid": command.VMUUID, "name": spec.Name, "intervalMinutes": spec.IntervalMinutes, "retention": spec.Retention, "enabled": spec.Enabled}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.SnapshotPolicy{}, s.mutationFailure(ctx, command.Request, "vm.snapshot_policy.create", command.VMUUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.SnapshotPolicy{}, s.mutationFailure(ctx, command.Request, "vm.snapshot_policy.create", command.VMUUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.vms.GetVM(ctx, command.VMUUID)
	if err != nil {
		return domain.SnapshotPolicy{}, s.mutationFailure(ctx, command.Request, "vm.snapshot_policy.create", command.VMUUID, detail, err)
	}
	detail["vmName"] = vm.Name
	existing, err := s.repository.ListSnapshotPolicies(ctx, command.VMUUID)
	if err != nil {
		return domain.SnapshotPolicy{}, s.mutationFailure(ctx, command.Request, "vm.snapshot_policy.create", command.VMUUID, detail, err)
	}
	if len(existing) >= maxPoliciesPerVM {
		return domain.SnapshotPolicy{}, s.mutationFailure(ctx, command.Request, "vm.snapshot_policy.create", command.VMUUID, detail, domain.ErrConflict)
	}
	now := s.now().UTC()
	policy, err := s.repository.CreateSnapshotPolicy(ctx, domain.SnapshotPolicy{
		VMUUID: command.VMUUID, Name: spec.Name, IntervalMinutes: spec.IntervalMinutes,
		Retention: spec.Retention, Enabled: spec.Enabled,
		NextRunAt: now.Add(time.Duration(spec.IntervalMinutes) * time.Minute),
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return domain.SnapshotPolicy{}, s.mutationFailure(ctx, command.Request, "vm.snapshot_policy.create", command.VMUUID, detail, err)
	}
	detail["policyId"] = policy.ID
	if err := s.recordAudit(ctx, command.Request, "vm.snapshot_policy.create", command.VMUUID, "success", detail, nil); err != nil {
		return domain.SnapshotPolicy{}, fmt.Errorf("record snapshot policy create audit: %w", err)
	}
	return policy, nil
}

func (s *Service) Update(ctx context.Context, command UpdateCommand) (domain.SnapshotPolicy, error) {
	if command.ID < 1 {
		return domain.SnapshotPolicy{}, domain.ErrInvalidInput
	}
	spec, err := normalizeSpec(command.Spec)
	if err != nil {
		return domain.SnapshotPolicy{}, err
	}
	policy, err := s.repository.GetSnapshotPolicy(ctx, command.ID)
	if err != nil {
		return domain.SnapshotPolicy{}, err
	}
	detail := map[string]any{"policyId": policy.ID, "name": spec.Name, "intervalMinutes": spec.IntervalMinutes, "retention": spec.Retention, "enabled": spec.Enabled}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.SnapshotPolicy{}, s.mutationFailure(ctx, command.Request, "vm.snapshot_policy.update", policy.VMUUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.SnapshotPolicy{}, s.mutationFailure(ctx, command.Request, "vm.snapshot_policy.update", policy.VMUUID, detail, domain.ErrReadOnly)
	}
	now := s.now().UTC()
	policy.Name = spec.Name
	policy.IntervalMinutes = spec.IntervalMinutes
	policy.Retention = spec.Retention
	policy.Enabled = spec.Enabled
	policy.NextRunAt = now.Add(time.Duration(spec.IntervalMinutes) * time.Minute)
	policy.UpdatedAt = now
	updated, err := s.repository.UpdateSnapshotPolicy(ctx, policy)
	if err != nil {
		return domain.SnapshotPolicy{}, s.mutationFailure(ctx, command.Request, "vm.snapshot_policy.update", policy.VMUUID, detail, err)
	}
	if err := s.recordAudit(ctx, command.Request, "vm.snapshot_policy.update", policy.VMUUID, "success", detail, nil); err != nil {
		return domain.SnapshotPolicy{}, fmt.Errorf("record snapshot policy update audit: %w", err)
	}
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, command DeleteCommand) error {
	if command.ID < 1 {
		return domain.ErrInvalidInput
	}
	policy, err := s.repository.GetSnapshotPolicy(ctx, command.ID)
	if err != nil {
		return err
	}
	detail := map[string]any{"policyId": policy.ID, "name": policy.Name}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return s.mutationFailure(ctx, command.Request, "vm.snapshot_policy.delete", policy.VMUUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return s.mutationFailure(ctx, command.Request, "vm.snapshot_policy.delete", policy.VMUUID, detail, domain.ErrReadOnly)
	}
	if err := s.repository.DeleteSnapshotPolicy(ctx, command.ID); err != nil {
		return s.mutationFailure(ctx, command.Request, "vm.snapshot_policy.delete", policy.VMUUID, detail, err)
	}
	if err := s.recordAudit(ctx, command.Request, "vm.snapshot_policy.delete", policy.VMUUID, "success", detail, nil); err != nil {
		return fmt.Errorf("record snapshot policy delete audit: %w", err)
	}
	return nil
}

func (s *Service) RunNow(ctx context.Context, command RunCommand) (domain.SnapshotPolicyRunResult, error) {
	if command.ID < 1 {
		return domain.SnapshotPolicyRunResult{}, domain.ErrInvalidInput
	}
	policy, err := s.repository.GetSnapshotPolicy(ctx, command.ID)
	if err != nil {
		return domain.SnapshotPolicyRunResult{}, err
	}
	detail := map[string]any{"policyId": policy.ID, "name": policy.Name}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.SnapshotPolicyRunResult{}, s.mutationFailure(ctx, command.Request, "vm.snapshot.schedule", policy.VMUUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.SnapshotPolicyRunResult{}, s.mutationFailure(ctx, command.Request, "vm.snapshot.schedule", policy.VMUUID, detail, domain.ErrReadOnly)
	}
	return s.execute(ctx, policy, command.Request.Session.User.Username, command.Request.RequestID, command.Request.SourceIP)
}

func (s *Service) RunDue(ctx context.Context, now time.Time) ([]domain.SnapshotPolicyRunResult, error) {
	if s.isReadOnly() {
		return nil, nil
	}
	policies, err := s.repository.ListDueSnapshotPolicies(ctx, now.UTC(), 100)
	if err != nil {
		return nil, err
	}
	results := make([]domain.SnapshotPolicyRunResult, 0, len(policies))
	for _, policy := range policies {
		result, runErr := s.execute(ctx, policy, "system", "snapshot-scheduler-"+strconv.FormatInt(policy.ID, 10), "scheduler")
		if runErr != nil {
			if s.onError != nil {
				s.onError(runErr)
			}
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	s.runDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDue(ctx)
		}
	}
}

func (s *Service) runDue(ctx context.Context) {
	if _, err := s.RunDue(ctx, s.now()); err != nil && s.onError != nil {
		s.onError(err)
	}
}

func (s *Service) execute(ctx context.Context, policy domain.SnapshotPolicy, actor, requestID, sourceIP string) (domain.SnapshotPolicyRunResult, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	now := s.now().UTC()
	request := MutationContext{Session: domain.Session{User: domain.User{Username: actor, Role: domain.RoleAdmin}}, RequestID: requestID, SourceIP: sourceIP}
	detail := map[string]any{"policyId": policy.ID, "name": policy.Name}
	vm, err := s.vms.GetVM(ctx, policy.VMUUID)
	if err != nil {
		return domain.SnapshotPolicyRunResult{}, s.failedRun(ctx, request, policy, detail, now, err)
	}
	detail["vmName"] = vm.Name
	snapshotName := policy.Name + "-" + now.Format("20060102T150405")
	description := fmt.Sprintf("VirtMgr 定时快照策略 %s [policy:%d]", policy.Name, policy.ID)
	snapshot, err := s.vms.CreateSnapshot(ctx, policy.VMUUID, domain.VMSnapshotCreateSpec{Name: snapshotName, Description: description})
	if err != nil {
		return domain.SnapshotPolicyRunResult{}, s.failedRun(ctx, request, policy, detail, now, err)
	}
	detail["snapshot"] = snapshot.Name
	created := snapshot
	result := domain.SnapshotPolicyRunResult{Policy: policy, Snapshot: &created}
	deleted, cleanupErr := s.cleanupRetention(ctx, policy)
	if len(deleted) > 0 {
		detail["deletedSnapshots"] = deleted
		result.DeletedSnapshots = deleted
	}
	policy.LastRunAt = &now
	policy.NextRunAt = nextRun(policy, now)
	policy.LastSnapshot = snapshot.Name
	policy.LastError = ""
	if cleanupErr != nil {
		policy.LastError = "保留清理失败: " + cleanupErr.Error()
		detail["cleanupError"] = cleanupErr.Error()
	}
	policy.UpdatedAt = now
	updated, err := s.repository.UpdateSnapshotPolicy(ctx, policy)
	if err != nil {
		return domain.SnapshotPolicyRunResult{}, s.failedRun(ctx, request, policy, detail, now, err)
	}
	result.Policy = updated
	if err := s.recordAudit(ctx, request, "vm.snapshot.schedule", policy.VMUUID, "success", detail, nil); err != nil {
		return domain.SnapshotPolicyRunResult{}, fmt.Errorf("record scheduled snapshot audit: %w", err)
	}
	return result, nil
}

func (s *Service) cleanupRetention(ctx context.Context, policy domain.SnapshotPolicy) ([]string, error) {
	snapshots, err := s.vms.ListSnapshots(ctx, policy.VMUUID)
	if err != nil {
		return nil, err
	}
	marker := fmt.Sprintf("[policy:%d]", policy.ID)
	managed := make([]domain.VMSnapshot, 0)
	for _, snapshot := range snapshots {
		if strings.Contains(snapshot.Description, marker) {
			managed = append(managed, snapshot)
		}
	}
	sort.Slice(managed, func(i, j int) bool {
		if managed[i].CreatedAt != managed[j].CreatedAt {
			return managed[i].CreatedAt > managed[j].CreatedAt
		}
		return managed[i].Name > managed[j].Name
	})
	if len(managed) <= policy.Retention {
		return nil, nil
	}
	deleted := make([]string, 0, len(managed)-policy.Retention)
	for _, snapshot := range managed[policy.Retention:] {
		if err := s.vms.DeleteSnapshot(ctx, policy.VMUUID, snapshot.Name); err != nil {
			return deleted, err
		}
		deleted = append(deleted, snapshot.Name)
	}
	return deleted, nil
}

func (s *Service) failedRun(ctx context.Context, request MutationContext, policy domain.SnapshotPolicy, detail map[string]any, now time.Time, cause error) error {
	policy.LastRunAt = &now
	policy.NextRunAt = nextRun(policy, now)
	policy.LastError = cause.Error()
	policy.UpdatedAt = now
	if _, err := s.repository.UpdateSnapshotPolicy(ctx, policy); err != nil {
		cause = errors.Join(cause, fmt.Errorf("record snapshot policy failure state: %w", err))
	}
	return s.mutationFailure(ctx, request, "vm.snapshot.schedule", policy.VMUUID, detail, cause)
}

func nextRun(policy domain.SnapshotPolicy, now time.Time) time.Time {
	return now.Add(time.Duration(policy.IntervalMinutes) * time.Minute)
}

func normalizeSpec(spec domain.SnapshotPolicySpec) (domain.SnapshotPolicySpec, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	if !policyNamePattern.MatchString(spec.Name) {
		return domain.SnapshotPolicySpec{}, fmt.Errorf("%w: invalid snapshot policy name", domain.ErrInvalidInput)
	}
	if spec.IntervalMinutes < minInterval || spec.IntervalMinutes > maxInterval {
		return domain.SnapshotPolicySpec{}, fmt.Errorf("%w: interval must be between %d and %d minutes", domain.ErrInvalidInput, minInterval, maxInterval)
	}
	if spec.Retention < minRetention || spec.Retention > maxRetention {
		return domain.SnapshotPolicySpec{}, fmt.Errorf("%w: retention must be between %d and %d", domain.ErrInvalidInput, minRetention, maxRetention)
	}
	return spec, nil
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
		Role: request.Session.User.Role, Action: action, TargetKind: "vm", TargetID: targetID,
		SourceIP: request.SourceIP, Result: result, Detail: detail,
	})
}
