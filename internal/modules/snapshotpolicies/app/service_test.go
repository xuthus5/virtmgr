package app

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type memoryPolicyRepository struct {
	policies  map[int64]domain.SnapshotPolicy
	next      int64
	dueCalls  int
	listErr   error
	createErr error
	updateErr error
	deleteErr error
	dueErr    error
}

func newMemoryPolicyRepository() *memoryPolicyRepository {
	return &memoryPolicyRepository{policies: map[int64]domain.SnapshotPolicy{}}
}

func (m *memoryPolicyRepository) ListSnapshotPolicies(_ context.Context, vmUUID string) ([]domain.SnapshotPolicy, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	items := make([]domain.SnapshotPolicy, 0)
	for _, policy := range m.policies {
		if policy.VMUUID == vmUUID {
			items = append(items, policy)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if !strings.EqualFold(items[i].Name, items[j].Name) {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func (m *memoryPolicyRepository) GetSnapshotPolicy(_ context.Context, id int64) (domain.SnapshotPolicy, error) {
	policy, ok := m.policies[id]
	if !ok {
		return domain.SnapshotPolicy{}, domain.ErrNotFound
	}
	return policy, nil
}

func (m *memoryPolicyRepository) CreateSnapshotPolicy(_ context.Context, policy domain.SnapshotPolicy) (domain.SnapshotPolicy, error) {
	if m.createErr != nil {
		return domain.SnapshotPolicy{}, m.createErr
	}
	for _, existing := range m.policies {
		if existing.VMUUID == policy.VMUUID && existing.Name == policy.Name {
			return domain.SnapshotPolicy{}, domain.ErrConflict
		}
	}
	m.next++
	policy.ID = m.next
	m.policies[policy.ID] = policy
	return policy, nil
}

func (m *memoryPolicyRepository) UpdateSnapshotPolicy(_ context.Context, policy domain.SnapshotPolicy) (domain.SnapshotPolicy, error) {
	if m.updateErr != nil {
		return domain.SnapshotPolicy{}, m.updateErr
	}
	if _, ok := m.policies[policy.ID]; !ok {
		return domain.SnapshotPolicy{}, domain.ErrNotFound
	}
	for id, existing := range m.policies {
		if id != policy.ID && existing.VMUUID == policy.VMUUID && existing.Name == policy.Name {
			return domain.SnapshotPolicy{}, domain.ErrConflict
		}
	}
	m.policies[policy.ID] = policy
	return policy, nil
}

func (m *memoryPolicyRepository) DeleteSnapshotPolicy(_ context.Context, id int64) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.policies[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.policies, id)
	return nil
}

func (m *memoryPolicyRepository) ListDueSnapshotPolicies(_ context.Context, now time.Time, limit int) ([]domain.SnapshotPolicy, error) {
	m.dueCalls++
	if m.dueErr != nil {
		return nil, m.dueErr
	}
	items := make([]domain.SnapshotPolicy, 0)
	for _, policy := range m.policies {
		if policy.Enabled && !policy.NextRunAt.After(now) {
			items = append(items, policy)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].NextRunAt.Equal(items[j].NextRunAt) {
			return items[i].NextRunAt.Before(items[j].NextRunAt)
		}
		return items[i].ID < items[j].ID
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

type memoryVMRepository struct {
	vms          map[string]domain.VM
	snapshots    map[string][]domain.VMSnapshot
	nextCreated  int64
	createErr    error
	listErr      error
	deleteErr    error
	createCalls  []domain.VMSnapshotCreateSpec
	deletedNames []string
}

func newMemoryVMRepository() *memoryVMRepository {
	return &memoryVMRepository{
		vms:         map[string]domain.VM{"vm-a": {UUID: "vm-a", Name: "api-01"}},
		snapshots:   map[string][]domain.VMSnapshot{},
		nextCreated: 1_700_000_000,
	}
}

func (m *memoryVMRepository) GetVM(_ context.Context, uuid string) (domain.VM, error) {
	vm, ok := m.vms[uuid]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	return vm, nil
}

func (m *memoryVMRepository) ListSnapshots(_ context.Context, uuid string) ([]domain.VMSnapshot, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return append([]domain.VMSnapshot(nil), m.snapshots[uuid]...), nil
}

func (m *memoryVMRepository) CreateSnapshot(_ context.Context, uuid string, spec domain.VMSnapshotCreateSpec) (domain.VMSnapshot, error) {
	m.createCalls = append(m.createCalls, spec)
	if m.createErr != nil {
		return domain.VMSnapshot{}, m.createErr
	}
	for _, snapshot := range m.snapshots[uuid] {
		if snapshot.Name == spec.Name {
			return domain.VMSnapshot{}, domain.ErrConflict
		}
	}
	m.nextCreated++
	snapshot := domain.VMSnapshot{
		Name: spec.Name, Description: spec.Description, CreatedAt: m.nextCreated,
		State: domain.VMStateShutoff, Current: true,
	}
	m.snapshots[uuid] = append(m.snapshots[uuid], snapshot)
	return snapshot, nil
}

func (m *memoryVMRepository) DeleteSnapshot(_ context.Context, uuid, name string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	snapshots := m.snapshots[uuid]
	for index, snapshot := range snapshots {
		if snapshot.Name == name {
			m.snapshots[uuid] = append(snapshots[:index], snapshots[index+1:]...)
			m.deletedNames = append(m.deletedNames, name)
			return nil
		}
	}
	return domain.ErrNotFound
}

type memoryAuditRecorder struct{ events []domain.AuditEvent }

func (m *memoryAuditRecorder) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

type policyReadOnlyMode struct{ value bool }

func (m *policyReadOnlyMode) ReadOnly() bool { return m.value }

func policyRequest(role domain.Role) MutationContext {
	return MutationContext{
		Session:   domain.Session{User: domain.User{ID: 1, Username: "operator", Role: role}},
		RequestID: "req-policy",
		SourceIP:  "127.0.0.1",
	}
}

func validPolicySpec() domain.SnapshotPolicySpec {
	return domain.SnapshotPolicySpec{Name: "nightly", IntervalMinutes: 60, Retention: 3, Enabled: true}
}

func fixedPolicyClock() time.Time {
	return time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
}

func TestSnapshotPolicyServiceCRUDAudit(t *testing.T) {
	repository := newMemoryPolicyRepository()
	audit := &memoryAuditRecorder{}
	now := fixedPolicyClock()
	service := New(repository, newMemoryVMRepository(), audit, false, WithClock(func() time.Time { return now }))

	created, err := service.Create(context.Background(), CreateCommand{VMUUID: " vm-a ", Spec: validPolicySpec(), Request: policyRequest(domain.RoleOperator)})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != 1 || created.VMUUID != "vm-a" || !created.NextRunAt.Equal(now.Add(time.Hour)) || created.LastRunAt != nil {
		t.Fatalf("created policy = %+v", created)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.snapshot_policy.create" || audit.events[0].TargetID != "vm-a" || audit.events[0].Detail["vmName"] != "api-01" {
		t.Fatalf("create audit = %+v", audit.events)
	}

	updated, err := service.Update(context.Background(), UpdateCommand{
		ID:      created.ID,
		Spec:    domain.SnapshotPolicySpec{Name: "weekly", IntervalMinutes: 120, Retention: 5, Enabled: false},
		Request: policyRequest(domain.RoleAdmin),
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "weekly" || updated.IntervalMinutes != 120 || updated.Retention != 5 || updated.Enabled || !updated.NextRunAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("updated policy = %+v", updated)
	}
	if len(audit.events) != 2 || audit.events[1].Action != "vm.snapshot_policy.update" || audit.events[1].Detail["name"] != "weekly" {
		t.Fatalf("update audit = %+v", audit.events)
	}

	if err := service.Delete(context.Background(), DeleteCommand{ID: created.ID, Request: policyRequest(domain.RoleOperator)}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(audit.events) != 3 || audit.events[2].Action != "vm.snapshot_policy.delete" {
		t.Fatalf("delete audit = %+v", audit.events)
	}
	if _, err := repository.GetSnapshotPolicy(context.Background(), created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted policy error = %v", err)
	}
}

func TestSnapshotPolicyServiceValidationAndPermissions(t *testing.T) {
	repository := newMemoryPolicyRepository()
	service := New(repository, newMemoryVMRepository(), nil, false)
	for name, mutate := range map[string]func(*domain.SnapshotPolicySpec){
		"empty name":         func(spec *domain.SnapshotPolicySpec) { spec.Name = "  " },
		"unsafe name":        func(spec *domain.SnapshotPolicySpec) { spec.Name = "../nightly" },
		"name too long":      func(spec *domain.SnapshotPolicySpec) { spec.Name = strings.Repeat("a", 33) },
		"interval too small": func(spec *domain.SnapshotPolicySpec) { spec.IntervalMinutes = 4 },
		"interval too large": func(spec *domain.SnapshotPolicySpec) { spec.IntervalMinutes = 10081 },
		"retention zero":     func(spec *domain.SnapshotPolicySpec) { spec.Retention = 0 },
		"retention too high": func(spec *domain.SnapshotPolicySpec) { spec.Retention = 51 },
	} {
		t.Run(name, func(t *testing.T) {
			spec := validPolicySpec()
			mutate(&spec)
			if _, err := service.Create(context.Background(), CreateCommand{VMUUID: "vm-a", Spec: spec, Request: policyRequest(domain.RoleOperator)}); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
	if _, err := service.Create(context.Background(), CreateCommand{Spec: validPolicySpec(), Request: policyRequest(domain.RoleOperator)}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("empty VM error = %v", err)
	}
	if _, err := service.Create(context.Background(), CreateCommand{VMUUID: "vm-a", Spec: validPolicySpec(), Request: policyRequest(domain.RoleViewer)}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("viewer create error = %v", err)
	}
	mode := &policyReadOnlyMode{value: true}
	readOnlyService := New(repository, newMemoryVMRepository(), nil, false, WithReadOnlyMode(mode))
	if _, err := readOnlyService.Create(context.Background(), CreateCommand{VMUUID: "vm-a", Spec: validPolicySpec(), Request: policyRequest(domain.RoleAdmin)}); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only create error = %v", err)
	}
}

func TestSnapshotPolicyServiceLimitPerVM(t *testing.T) {
	repository := newMemoryPolicyRepository()
	service := New(repository, newMemoryVMRepository(), nil, false)
	for index := 0; index < maxPoliciesPerVM; index++ {
		spec := validPolicySpec()
		spec.Name = "policy-" + string(rune('a'+index))
		if _, err := service.Create(context.Background(), CreateCommand{VMUUID: "vm-a", Spec: spec, Request: policyRequest(domain.RoleOperator)}); err != nil {
			t.Fatalf("create policy %d: %v", index, err)
		}
	}
	extra := validPolicySpec()
	extra.Name = "policy-extra"
	if _, err := service.Create(context.Background(), CreateCommand{VMUUID: "vm-a", Spec: extra, Request: policyRequest(domain.RoleOperator)}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("policy limit error = %v", err)
	}
}

func TestSnapshotPolicyServiceRunNowRetentionAndAudit(t *testing.T) {
	repository := newMemoryPolicyRepository()
	vms := newMemoryVMRepository()
	audit := &memoryAuditRecorder{}
	now := fixedPolicyClock()
	policy, err := repository.CreateSnapshotPolicy(context.Background(), domain.SnapshotPolicy{
		VMUUID: "vm-a", Name: "nightly", IntervalMinutes: 60, Retention: 2, Enabled: true,
		NextRunAt: now, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	vms.snapshots["vm-a"] = []domain.VMSnapshot{
		{Name: "manual-baseline", Description: "manual", CreatedAt: 10},
		{Name: "nightly-oldest", Description: "[policy:1] oldest", CreatedAt: 100},
		{Name: "nightly-older", Description: "[policy:1] older", CreatedAt: 200},
		{Name: "nightly-newer", Description: "[policy:1] newer", CreatedAt: 300},
	}
	service := New(repository, vms, audit, false, WithClock(func() time.Time { return now }))

	result, err := service.RunNow(context.Background(), RunCommand{ID: policy.ID, Request: policyRequest(domain.RoleOperator)})
	if err != nil {
		t.Fatalf("RunNow() error = %v", err)
	}
	if result.Snapshot == nil || result.Snapshot.Name != "nightly-20260501T100000" {
		t.Fatalf("run snapshot = %+v", result.Snapshot)
	}
	if len(vms.createCalls) != 1 || !strings.Contains(vms.createCalls[0].Description, "[policy:1]") {
		t.Fatalf("create calls = %+v", vms.createCalls)
	}
	if len(result.DeletedSnapshots) != 2 || result.DeletedSnapshots[0] != "nightly-older" || result.DeletedSnapshots[1] != "nightly-oldest" {
		t.Fatalf("deleted snapshots = %+v", result.DeletedSnapshots)
	}
	if len(vms.deletedNames) != 2 {
		t.Fatalf("VM delete calls = %+v", vms.deletedNames)
	}
	manualFound := false
	for _, snapshot := range vms.snapshots["vm-a"] {
		if snapshot.Name == "manual-baseline" {
			manualFound = true
			break
		}
	}
	if !manualFound {
		t.Fatal("manual snapshot was removed by retention cleanup")
	}
	if result.Policy.LastSnapshot != result.Snapshot.Name || result.Policy.LastRunAt == nil || !result.Policy.LastRunAt.Equal(now) || !result.Policy.NextRunAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("updated policy = %+v", result.Policy)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.snapshot.schedule" || audit.events[0].Result != "success" || audit.events[0].Detail["snapshot"] != result.Snapshot.Name {
		t.Fatalf("run audit = %+v", audit.events)
	}
}

func TestSnapshotPolicyServiceRunFailureRecordsStateAndAudit(t *testing.T) {
	repository := newMemoryPolicyRepository()
	vms := newMemoryVMRepository()
	audit := &memoryAuditRecorder{}
	now := fixedPolicyClock()
	policy, err := repository.CreateSnapshotPolicy(context.Background(), domain.SnapshotPolicy{
		VMUUID: "vm-a", Name: "nightly", IntervalMinutes: 60, Retention: 2, Enabled: true,
		NextRunAt: now, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	vms.createErr = errors.New("snapshot backend down")
	service := New(repository, vms, audit, false, WithClock(func() time.Time { return now }))

	if _, err := service.RunNow(context.Background(), RunCommand{ID: policy.ID, Request: policyRequest(domain.RoleOperator)}); err == nil {
		t.Fatal("RunNow() expected failure")
	}
	stored, err := repository.GetSnapshotPolicy(context.Background(), policy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastError != "snapshot backend down" || stored.LastRunAt == nil || !stored.LastRunAt.Equal(now) || !stored.NextRunAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("failed policy state = %+v", stored)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.snapshot.schedule" || audit.events[0].Result != "failure" || audit.events[0].Detail["error"] != "snapshot backend down" {
		t.Fatalf("failure audit = %+v", audit.events)
	}
}

func TestSnapshotPolicyServiceRunDueRespectsReadOnlyAndDueTime(t *testing.T) {
	repository := newMemoryPolicyRepository()
	vms := newMemoryVMRepository()
	now := fixedPolicyClock()
	due, err := repository.CreateSnapshotPolicy(context.Background(), domain.SnapshotPolicy{
		VMUUID: "vm-a", Name: "due", IntervalMinutes: 60, Retention: 2, Enabled: true,
		NextRunAt: now.Add(-time.Minute), CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateSnapshotPolicy(context.Background(), domain.SnapshotPolicy{
		VMUUID: "vm-a", Name: "future", IntervalMinutes: 60, Retention: 2, Enabled: true,
		NextRunAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateSnapshotPolicy(context.Background(), domain.SnapshotPolicy{
		VMUUID: "vm-a", Name: "disabled", IntervalMinutes: 60, Retention: 2, Enabled: false,
		NextRunAt: now.Add(-time.Hour), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	readOnly := New(repository, vms, nil, true, WithClock(func() time.Time { return now }))
	results, err := readOnly.RunDue(context.Background(), now)
	if err != nil || len(results) != 0 || repository.dueCalls != 0 {
		t.Fatalf("read-only RunDue() = %+v, %v dueCalls=%d", results, err, repository.dueCalls)
	}

	service := New(repository, vms, &memoryAuditRecorder{}, false, WithClock(func() time.Time { return now }))
	results, err = service.RunDue(context.Background(), now)
	if err != nil {
		t.Fatalf("RunDue() error = %v", err)
	}
	if len(results) != 1 || results[0].Policy.ID != due.ID || len(vms.createCalls) != 1 {
		t.Fatalf("due results = %+v calls=%+v", results, vms.createCalls)
	}
}

func TestSnapshotPolicyServiceMutationFailuresAreAudited(t *testing.T) {
	repository := newMemoryPolicyRepository()
	audit := &memoryAuditRecorder{}
	now := fixedPolicyClock()
	policy, err := repository.CreateSnapshotPolicy(context.Background(), domain.SnapshotPolicy{
		VMUUID: "vm-a", Name: "nightly", IntervalMinutes: 60, Retention: 2, Enabled: true,
		NextRunAt: now, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := New(repository, newMemoryVMRepository(), audit, false)
	if _, err := service.Update(context.Background(), UpdateCommand{ID: policy.ID, Spec: validPolicySpec(), Request: policyRequest(domain.RoleViewer)}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("viewer update error = %v", err)
	}
	if _, err := service.RunNow(context.Background(), RunCommand{ID: policy.ID, Request: policyRequest(domain.RoleViewer)}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("viewer run error = %v", err)
	}
	if err := service.Delete(context.Background(), DeleteCommand{ID: policy.ID, Request: policyRequest(domain.RoleViewer)}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("viewer delete error = %v", err)
	}
	if len(audit.events) != 3 {
		t.Fatalf("failure audits = %+v", audit.events)
	}
	for index, action := range []string{"vm.snapshot_policy.update", "vm.snapshot.schedule", "vm.snapshot_policy.delete"} {
		if audit.events[index].Action != action || audit.events[index].Result != "failure" {
			t.Fatalf("audit %d = %+v", index, audit.events[index])
		}
	}
}
