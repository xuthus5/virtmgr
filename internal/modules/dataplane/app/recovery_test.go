package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type fakeStore struct {
	pending  []domain.DataPlaneOperation
	finished map[string]string
	messages map[string]string
	tasks    map[string]string
}

func (f *fakeStore) ListPendingDataPlaneOperations(context.Context) ([]domain.DataPlaneOperation, error) {
	return append([]domain.DataPlaneOperation(nil), f.pending...), nil
}

func (f *fakeStore) FinishDataPlaneOperation(_ context.Context, id, status, message string, _ time.Time) error {
	if f.finished == nil {
		f.finished = make(map[string]string)
		f.messages = make(map[string]string)
	}
	f.finished[id] = status
	f.messages[id] = message
	return nil
}

func (f *fakeStore) UpdateInterruptedTaskDetail(_ context.Context, id, step, taskError string, _ time.Time) error {
	if f.tasks == nil {
		f.tasks = make(map[string]string)
	}
	f.tasks[id] = step + "|" + taskError
	return nil
}

type fakeBackupRepository struct {
	items   map[string]domain.Backup
	updated []domain.Backup
}

func (f *fakeBackupRepository) GetBackup(_ context.Context, id string) (domain.Backup, error) {
	item, ok := f.items[id]
	if !ok {
		return domain.Backup{}, domain.ErrNotFound
	}
	return item, nil
}

func (f *fakeBackupRepository) UpdateBackup(_ context.Context, item domain.Backup) (domain.Backup, error) {
	f.updated = append(f.updated, item)
	return item, nil
}

type fakeRecoveryBackend struct {
	results map[string]domain.DataPlaneRecovery
	errs    map[string]error
}

func (f fakeRecoveryBackend) RecoverDataPlaneOperation(_ context.Context, operation domain.DataPlaneOperation) (domain.DataPlaneRecovery, error) {
	return f.results[operation.ID], f.errs[operation.ID]
}

type fakeAudit struct{ events []domain.AuditEvent }

func (f *fakeAudit) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	f.events = append(f.events, event)
	return nil
}

type fakeLocker struct{ blocked map[string]string }

func (f *fakeLocker) Block(key, reason string) {
	if f.blocked == nil {
		f.blocked = make(map[string]string)
	}
	f.blocked[key] = reason
}

func TestRecoverCompletesInterruptedBackup(t *testing.T) {
	now := time.Unix(1_700_300_000, 0).UTC()
	operation := domain.DataPlaneOperation{
		ID: "op-1", Kind: domain.DataPlaneOperationBackup, VMUUID: "vm-1", VMName: "web-01",
		TaskID: "task-1", BackupID: "backup-1", TargetPool: "archive", Online: true,
	}
	store := &fakeStore{pending: []domain.DataPlaneOperation{operation}}
	backups := &fakeBackupRepository{items: map[string]domain.Backup{
		"backup-1": {ID: "backup-1", Name: "nightly", VMUUID: "vm-1", VMName: "web-01", Status: domain.BackupStatusInterrupted, Pinned: true},
	}}
	backend := fakeRecoveryBackend{results: map[string]domain.DataPlaneRecovery{
		"op-1": {
			Status: domain.DataPlaneRecoveryCompleted, Message: "产物完整",
			CompletedBackup: &domain.VMBackup{
				FormatVersion: domain.VMBackupFormatVersionV2, ManifestVolume: "backup-1-manifest.json",
				ConfigVolume: "backup-1-domain.xml", ConfigBytes: 100, TotalBytes: 200,
				Disks:  []domain.VMBackupDisk{{SourceVolume: "a.qcow2", BackupVolume: "backup-1-disk-0.qcow2", Format: "qcow2"}},
				Online: true, Consistent: true,
			},
		},
	}}
	audit := &fakeAudit{}
	service := New(store, backups, backend, audit, &fakeLocker{}, WithClock(func() time.Time { return now }))
	summary, err := service.Recover(context.Background())
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if summary.Operations != 1 || summary.Recovered != 1 || summary.Failed != 0 || summary.NeedsAttention != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	if len(backups.updated) != 1 {
		t.Fatalf("updated backups = %+v", backups.updated)
	}
	updated := backups.updated[0]
	if updated.Status != domain.BackupStatusCompleted || updated.Name != "nightly" || !updated.Pinned || !updated.Online || updated.DiskCount != 1 || updated.CompletedAt == nil {
		t.Fatalf("recovered backup = %+v", updated)
	}
	if store.finished["op-1"] != domain.DataPlaneStatusRecovered || store.tasks["task-1"] != "数据面操作崩溃恢复完成|产物完整" {
		t.Fatalf("finished=%+v tasks=%+v", store.finished, store.tasks)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "dataplane.recovery" || audit.events[0].Result != "success" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestRecoverNeedsAttentionBlocksResource(t *testing.T) {
	store := &fakeStore{pending: []domain.DataPlaneOperation{{
		ID: "op-2", Kind: domain.DataPlaneOperationImport, VMName: "imported", TaskID: "task-2",
		TargetPool: "default", TargetName: "imported.qcow2",
	}}}
	backend := fakeRecoveryBackend{results: map[string]domain.DataPlaneRecovery{
		"op-2": {Status: domain.DataPlaneRecoveryNeedsAttention, Message: "目标卷被其他虚拟机引用"},
	}}
	locker := &fakeLocker{}
	service := New(store, &fakeBackupRepository{items: map[string]domain.Backup{}}, backend, &fakeAudit{}, locker)
	summary, err := service.Recover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.NeedsAttention != 1 || store.finished["op-2"] != domain.DataPlaneStatusNeedsAttention {
		t.Fatalf("summary=%+v finished=%+v", summary, store.finished)
	}
	if locker.blocked[domain.VMNameLockKey("imported")] != "目标卷被其他虚拟机引用" {
		t.Fatalf("blocked keys = %+v", locker.blocked)
	}
	if _, ok := locker.blocked[domain.StorageVolumeLockKey("default", "imported.qcow2")]; !ok {
		t.Fatalf("target volume was not blocked: %+v", locker.blocked)
	}
}

func TestRecoverFailureContinuesAndRecordsAudit(t *testing.T) {
	backendErr := errors.New("libvirt unavailable")
	store := &fakeStore{pending: []domain.DataPlaneOperation{
		{ID: "op-3", Kind: domain.DataPlaneOperationMigrate, VMUUID: "vm-3", VMName: "db-01", TaskID: "task-3"},
		{ID: "op-4", Kind: domain.DataPlaneOperationFlatten, TargetPool: "archive", TargetName: "flat.qcow2", TaskID: "task-4"},
	}}
	backend := fakeRecoveryBackend{
		results: map[string]domain.DataPlaneRecovery{
			"op-4": {Status: domain.DataPlaneRecoveryRolledBack, Message: "未生效目标卷已回收"},
		},
		errs: map[string]error{"op-3": backendErr},
	}
	audit := &fakeAudit{}
	locker := &fakeLocker{}
	service := New(store, &fakeBackupRepository{items: map[string]domain.Backup{}}, backend, audit, locker)
	summary, err := service.Recover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Failed != 1 || summary.Recovered != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if store.finished["op-3"] != domain.DataPlaneStatusFailed || store.finished["op-4"] != domain.DataPlaneStatusRecovered {
		t.Fatalf("finished = %+v", store.finished)
	}
	if _, ok := locker.blocked[domain.VMLockKey("vm-3")]; !ok {
		t.Fatalf("failed VM was not blocked: %+v", locker.blocked)
	}
	if _, ok := locker.blocked[domain.StorageVolumeLockKey("archive", "flat.qcow2")]; ok {
		t.Fatalf("rolled back target should not be blocked: %+v", locker.blocked)
	}
	if len(audit.events) != 2 || audit.events[0].Result != "failure" || audit.events[1].Result != "success" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}
