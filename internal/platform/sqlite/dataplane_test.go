package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestDataPlaneOperationLifecycle(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_100_000, 0).UTC()
	if err := store.CreateDataPlaneOperation(ctx, domain.DataPlaneOperation{
		ID: "op-1", Kind: domain.DataPlaneOperationBackup, VMUUID: "vm-1", VMName: "web-01",
		TaskID: "task-1", BackupID: "backup-1", Phase: "started", Online: true,
		TargetPool: "archive", CreatedAt: base, UpdatedAt: base,
	}); err != nil {
		t.Fatalf("CreateDataPlaneOperation() error = %v", err)
	}
	if err := store.CreateDataPlaneOperation(ctx, domain.DataPlaneOperation{
		ID: "op-2", Kind: domain.DataPlaneOperationConvert, TaskID: "task-2", Phase: "target_creating",
		SourcePool: "default", SourceVolume: "a.qcow2", TargetPool: "archive", TargetName: "b.raw",
		CreatedAt: base.Add(time.Second), UpdatedAt: base.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDataPlaneOperationPhase(ctx, "op-2", "target_created", "/archive/b.raw", base.Add(2*time.Second)); err != nil {
		t.Fatalf("UpdateDataPlaneOperationPhase() error = %v", err)
	}
	pending, err := store.ListPendingDataPlaneOperations(ctx)
	if err != nil {
		t.Fatalf("ListPendingDataPlaneOperations() error = %v", err)
	}
	if len(pending) != 2 || pending[0].ID != "op-1" || pending[1].Phase != "target_created" || pending[1].TargetPath != "/archive/b.raw" {
		t.Fatalf("pending operations = %+v", pending)
	}
	if err := store.FinishDataPlaneOperation(ctx, "op-1", domain.DataPlaneStatusRecovered, "恢复完成", base.Add(3*time.Second)); err != nil {
		t.Fatalf("FinishDataPlaneOperation() error = %v", err)
	}
	if err := store.FinishDataPlaneOperation(ctx, "op-2", domain.DataPlaneStatusNeedsAttention, "需要人工处理", base.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	pending, err = store.ListPendingDataPlaneOperations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending after finish = %+v", pending)
	}
	recovered, err := store.GetDataPlaneOperation(ctx, "op-1")
	if err != nil {
		t.Fatalf("GetDataPlaneOperation() error = %v", err)
	}
	if recovered.Status != domain.DataPlaneStatusRecovered || recovered.Error != "恢复完成" || !recovered.Online {
		t.Fatalf("recovered operation = %+v", recovered)
	}
	if err := store.FinishDataPlaneOperation(ctx, "op-1", domain.DataPlaneStatusRunning, "invalid", base); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid terminal status error = %v", err)
	}
}

func TestUpdateInterruptedTaskDetail(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_200_000, 0).UTC()
	if err := store.CreateTask(ctx, domain.Task{
		ID: "task-1", Kind: "vm.backup", TargetKind: "vm", TargetID: "vm-1",
		Status: domain.TaskStatusInterrupted, Actor: "admin", CreatedAt: base, UpdatedAt: base,
	}); err != nil {
		t.Fatal(err)
	}
	updatedAt := base.Add(time.Minute)
	if err := store.UpdateInterruptedTaskDetail(ctx, "task-1", "恢复完成", "备份已恢复", updatedAt); err != nil {
		t.Fatalf("UpdateInterruptedTaskDetail() error = %v", err)
	}
	task, err := store.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if task.Step != "恢复完成" || task.Error != "备份已恢复" || !task.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("task = %+v", task)
	}
}
