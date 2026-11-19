package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"virtmgr/internal/domain"
	"virtmgr/internal/platform/dataplane"
	libvirtbackend "virtmgr/internal/platform/libvirt"
	"virtmgr/internal/platform/sqlite"
)

type conflictTaskRecorder struct{ starts int }

func (r *conflictTaskRecorder) Start(context.Context, string, string, string, string, string) (string, error) {
	r.starts++
	return "task-1", nil
}

func (*conflictTaskRecorder) Progress(context.Context, string, int, string) error { return nil }
func (*conflictTaskRecorder) Succeed(context.Context, string, string) error       { return nil }
func (*conflictTaskRecorder) Fail(context.Context, string, int, string, error) error {
	return nil
}

func TestBackupRejectsConflictingVMOperation(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "virtmgr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewStore(db)
	backend := libvirtbackend.NewDemo()
	locks := dataplane.NewLockManager()
	blocker, err := locks.Acquire(domain.VMLockKey("44444444-4444-4444-8444-444444444444"), "快照")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	tasks := &conflictTaskRecorder{}
	service := New(store, backend, store, tasks, false, WithDataPlane(locks, nil))
	_, err = service.Create(context.Background(), CreateCommand{
		VMUUID: "44444444-4444-4444-8444-444444444444",
		Spec:   domain.BackupCreateSpec{Name: "nightly", TargetPool: "default"},
		Request: MutationContext{Session: domain.Session{User: domain.User{
			Username: "admin", Role: domain.RoleAdmin,
		}}},
	})
	if !errors.Is(err, domain.ErrOperationConflict) {
		t.Fatalf("Create() error = %v, want ErrOperationConflict", err)
	}
	if tasks.starts != 0 {
		t.Fatalf("conflicting backup started %d tasks", tasks.starts)
	}
}
