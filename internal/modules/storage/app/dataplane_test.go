package app

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
	"virtmgr/internal/platform/dataplane"
)

func TestFlattenRejectsConflictingSourceOrTargetVolume(t *testing.T) {
	for _, test := range []struct {
		name string
		key  string
	}{
		{name: "source", key: domain.StorageVolumeLockKey("default", "source.qcow2")},
		{name: "target", key: domain.StorageVolumeLockKey("default", "source-flat.qcow2")},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := linkedVolumeRepository()
			locks := dataplane.NewLockManager()
			blocker, err := locks.Acquire(test.key, "转换存储卷")
			if err != nil {
				t.Fatal(err)
			}
			tasks := newConvertTaskRecorder()
			service := New(repository, &fakeAuditRecorder{}, false,
				WithTaskRecorder(tasks), WithDataPlane(locks, nil))
			_, err = service.FlattenVolume(context.Background(), FlattenVolumeCommand{
				Pool: "default", Name: "source.qcow2",
				Spec:    domain.StorageVolumeFlattenSpec{TargetPool: "default", TargetName: "source-flat.qcow2"},
				Request: MutationContext{Session: operatorSession()},
			})
			if !errors.Is(err, domain.ErrOperationConflict) {
				t.Fatalf("FlattenVolume() error = %v, want ErrOperationConflict", err)
			}
			starts, _, _ := tasks.snapshot()
			if len(starts) != 0 || repository.calls.Load() != 0 {
				t.Fatalf("conflict started task=%+v calls=%d", starts, repository.calls.Load())
			}
			blocker.Release()
		})
	}
}
