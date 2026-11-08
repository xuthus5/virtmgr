package sqlite

import (
	"context"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestTaskLifecycleAndFiltering(t *testing.T) {
	store := openTestDB(t)
	base := time.Unix(1_700_000_000, 0)
	tasks := []domain.Task{
		{ID: "task-1", Kind: "storage.volume.upload", TargetKind: "storage_volume", TargetID: "iso-store/debian.iso", Status: domain.TaskStatusSucceeded, Progress: 100, Step: "上传完成", Actor: "admin", CreatedAt: base, UpdatedAt: base.Add(time.Minute)},
		{ID: "task-2", Kind: "vm.clone", TargetKind: "vm", TargetID: "uuid-2", Status: domain.TaskStatusRunning, Progress: 45, Step: "复制磁盘", Actor: "operator", CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(2 * time.Minute)},
		{ID: "task-3", Kind: "vm.snapshot.revert", TargetKind: "vm_snapshot", TargetID: "uuid-2/baseline", Status: domain.TaskStatusFailed, Progress: 100, Step: "回滚快照", Error: "libvirt error", Actor: "admin", CreatedAt: base.Add(2 * time.Minute), UpdatedAt: base.Add(3 * time.Minute)},
	}
	for _, task := range tasks {
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask() error = %v", err)
		}
	}

	list, err := store.ListTasks(context.Background(), domain.TaskQuery{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if list.Total != 3 || len(list.Items) != 2 || list.Items[0].ID != "task-3" || list.Items[1].ID != "task-2" {
		t.Fatalf("first page = %+v", list)
	}
	if !list.Items[0].CreatedAt.Equal(base.Add(2*time.Minute)) || list.Items[0].Error != "libvirt error" {
		t.Fatalf("task timestamps/error = %+v", list.Items[0])
	}

	failed, err := store.ListTasks(context.Background(), domain.TaskQuery{Status: domain.TaskStatusFailed})
	if err != nil {
		t.Fatalf("ListTasks(failed) error = %v", err)
	}
	if failed.Total != 1 || failed.Items[0].ID != "task-3" {
		t.Fatalf("failed tasks = %+v", failed)
	}

	got, err := store.GetTask(context.Background(), "task-2")
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if got.Kind != "vm.clone" || got.Progress != 45 || got.Status != domain.TaskStatusRunning {
		t.Fatalf("task = %+v", got)
	}
	if _, err := store.GetTask(context.Background(), "missing"); err != domain.ErrNotFound {
		t.Fatalf("missing task error = %v", err)
	}
}

func TestMarkRunningTasksInterrupted(t *testing.T) {
	store := openTestDB(t)
	now := time.Unix(1_700_010_000, 0)
	for _, task := range []domain.Task{
		{ID: "queued", Kind: "vm.clone", TargetKind: "vm", TargetID: "uuid-1", Status: domain.TaskStatusQueued, Actor: "admin"},
		{ID: "running", Kind: "storage.volume.upload", TargetKind: "storage_volume", TargetID: "default/a.iso", Status: domain.TaskStatusRunning, Progress: 30, Actor: "admin"},
		{ID: "done", Kind: "vm.create", TargetKind: "vm", TargetID: "uuid-2", Status: domain.TaskStatusSucceeded, Progress: 100, Actor: "admin"},
	} {
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask() error = %v", err)
		}
	}
	affected, err := store.MarkRunningTasksInterrupted(context.Background(), now)
	if err != nil {
		t.Fatalf("MarkRunningTasksInterrupted() error = %v", err)
	}
	if affected != 2 {
		t.Fatalf("affected = %d, want 2", affected)
	}
	interrupted, err := store.GetTask(context.Background(), "running")
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.Status != domain.TaskStatusInterrupted || interrupted.Step != "服务重启，任务已中断" || !interrupted.UpdatedAt.Equal(now) {
		t.Fatalf("interrupted task = %+v", interrupted)
	}
	done, err := store.GetTask(context.Background(), "done")
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != domain.TaskStatusSucceeded {
		t.Fatalf("done task changed = %+v", done)
	}
}

func TestUpdateTask(t *testing.T) {
	store := openTestDB(t)
	base := time.Unix(1_700_020_000, 0)
	if err := store.CreateTask(context.Background(), domain.Task{
		ID: "task-update", Kind: "storage.volume.upload", TargetKind: "storage_volume", TargetID: "iso-store/install.iso",
		Status: domain.TaskStatusRunning, Progress: 20, Step: "正在上传", Actor: "operator", CreatedAt: base, UpdatedAt: base,
	}); err != nil {
		t.Fatal(err)
	}
	updatedAt := base.Add(time.Minute)
	if err := store.UpdateTask(context.Background(), domain.Task{
		ID: "task-update", Status: domain.TaskStatusSucceeded, Progress: 100, Step: "上传完成", UpdatedAt: updatedAt,
	}); err != nil {
		t.Fatalf("UpdateTask() error = %v", err)
	}
	task, err := store.GetTask(context.Background(), "task-update")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != domain.TaskStatusSucceeded || task.Progress != 100 || task.Step != "上传完成" || !task.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updated task = %+v", task)
	}
	if err := store.UpdateTask(context.Background(), domain.Task{ID: "missing", Status: domain.TaskStatusFailed}); err != domain.ErrNotFound {
		t.Fatalf("missing task error = %v", err)
	}
}
