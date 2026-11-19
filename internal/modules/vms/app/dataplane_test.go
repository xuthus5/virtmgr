package app

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
	"virtmgr/internal/platform/dataplane"
)

func TestSnapshotRejectsConflictingDataPlaneOperationAndReleasesLease(t *testing.T) {
	locks := dataplane.NewLockManager()
	blocker, err := locks.Acquire(domain.VMLockKey("uuid-a"), "备份")
	if err != nil {
		t.Fatal(err)
	}
	service := New(fakeVMRepository{vms: []domain.VM{persistentVM()}}, &fakeAuditRecorder{}, false,
		WithDataPlane(locks, nil))
	command := SnapshotCreateCommand{
		UUID: "uuid-a", Spec: domain.VMSnapshotCreateSpec{Name: "before-upgrade"},
		Request: MutationContext{Session: operatorSession()},
	}
	if _, err := service.CreateSnapshot(context.Background(), command); !errors.Is(err, domain.ErrOperationConflict) {
		t.Fatalf("CreateSnapshot() error = %v, want ErrOperationConflict", err)
	}
	blocker.Release()
	if _, err := service.CreateSnapshot(context.Background(), command); err != nil {
		t.Fatalf("CreateSnapshot() after release error = %v", err)
	}
	lease, err := locks.Acquire(domain.VMLockKey("uuid-a"), "扩容磁盘")
	if err != nil {
		t.Fatalf("snapshot did not release its lease: %v", err)
	}
	lease.Release()
}

func TestCreateRejectsConflictingTargetName(t *testing.T) {
	locks := dataplane.NewLockManager()
	blocker, err := locks.Acquire(domain.VMNameLockKey("api-02"), "导入")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	service := New(fakeVMRepository{vms: []domain.VM{persistentVM()}}, &fakeAuditRecorder{}, false,
		WithDataPlane(locks, nil))
	if _, err := service.Create(context.Background(), CreateCommand{
		Spec: validCreateSpec(), Request: MutationContext{Session: operatorSession()},
	}); !errors.Is(err, domain.ErrOperationConflict) {
		t.Fatalf("Create() error = %v, want ErrOperationConflict", err)
	}
}

func TestCloneRejectsConflictingSourceVM(t *testing.T) {
	locks := dataplane.NewLockManager()
	blocker, err := locks.Acquire(domain.VMLockKey("uuid-a"), "备份")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	tasks := newFakeTaskRecorder()
	service := New(fakeVMRepository{vms: []domain.VM{persistentVM()}}, &fakeAuditRecorder{}, false,
		WithTaskRecorder(tasks), WithDataPlane(locks, nil))
	if _, err := service.Clone(context.Background(), CloneCommand{
		UUID: "uuid-a", Spec: domain.VMCloneSpec{Name: "api-clone"},
		Request: MutationContext{Session: operatorSession()},
	}); !errors.Is(err, domain.ErrOperationConflict) {
		t.Fatalf("Clone() error = %v, want ErrOperationConflict", err)
	}
	if tasks.startedKind != "" {
		t.Fatalf("conflicting clone started task %q", tasks.startedKind)
	}
}

func TestImportRejectsConflictingTargetName(t *testing.T) {
	locks := dataplane.NewLockManager()
	blocker, err := locks.Acquire(domain.VMNameLockKey("web-import"), "恢复虚拟机")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	tasks := newFakeTaskRecorder()
	service := New(&recordingRepository{}, &fakeAuditRecorder{}, false,
		WithTaskRecorder(tasks), WithImportVolumes(newImportVolumes("qcow2")), WithDataPlane(locks, nil))
	if _, err := service.Import(context.Background(), ImportCommand{
		Spec: validImportSpec(), Request: MutationContext{Session: operatorSession()},
	}); !errors.Is(err, domain.ErrOperationConflict) {
		t.Fatalf("Import() error = %v, want ErrOperationConflict", err)
	}
	if tasks.startedKind != "" {
		t.Fatalf("conflicting import started task %q", tasks.startedKind)
	}
}

func TestResizeDiskRejectsConflictingVMOperation(t *testing.T) {
	locks := dataplane.NewLockManager()
	blocker, err := locks.Acquire(domain.VMLockKey("uuid-a"), "快照")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	repository := &diskResizeRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{diskResizeVM()}}}
	service := New(repository, &fakeAuditRecorder{}, false, WithDataPlane(locks, nil))
	if _, err := service.ResizeDisk(context.Background(), ResizeDiskCommand{
		UUID: "uuid-a", Target: "vda", Spec: domain.VMDiskResizeSpec{CapacityBytes: 40 << 30},
		Request: MutationContext{Session: operatorSession()},
	}); !errors.Is(err, domain.ErrOperationConflict) {
		t.Fatalf("ResizeDisk() error = %v, want ErrOperationConflict", err)
	}
	if repository.calls != 0 {
		t.Fatalf("conflicting resize called backend %d times", repository.calls)
	}
}
