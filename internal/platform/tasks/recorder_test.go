package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type fakeRepository struct {
	created   domain.Task
	updated   domain.Task
	updates   []domain.Task
	createErr error
	updateErr error
	err       error
}

func (f *fakeRepository) CreateTask(_ context.Context, task domain.Task) error {
	f.created = task
	return f.createErr
}

func (f *fakeRepository) UpdateTask(_ context.Context, task domain.Task) error {
	f.updated = task
	f.updates = append(f.updates, task)
	return f.updateErr
}

func TestRecorderLifecycle(t *testing.T) {
	repository := &fakeRepository{}
	now := time.Unix(1_700_000_000, 0)
	recorder := New(repository)
	recorder.now = func() time.Time { return now }
	recorder.newID = func() (string, error) { return "task_test", nil }

	id, err := recorder.Start(context.Background(), "storage.volume.upload", "storage_volume", "iso-store/install.iso", "operator", "正在上传")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if id != "task_test" || repository.created.Status != domain.TaskStatusRunning || repository.created.Progress != 0 || repository.created.Actor != "operator" || !repository.created.CreatedAt.Equal(now) {
		t.Fatalf("created task = %+v", repository.created)
	}

	if err := recorder.Progress(context.Background(), id, 42, "正在写入"); err != nil {
		t.Fatalf("Progress() error = %v", err)
	}
	if repository.updated.Status != domain.TaskStatusRunning || repository.updated.Progress != 42 || repository.updated.Step != "正在写入" {
		t.Fatalf("progress task = %+v", repository.updated)
	}

	if err := recorder.Fail(context.Background(), id, 42, "写入失败", errors.New("disk full")); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	if repository.updated.Status != domain.TaskStatusFailed || repository.updated.Error != "disk full" || repository.updated.Progress != 42 {
		t.Fatalf("failed task = %+v", repository.updated)
	}

	if err := recorder.Succeed(context.Background(), id, "上传完成"); err != nil {
		t.Fatalf("Succeed() error = %v", err)
	}
	if repository.updated.Status != domain.TaskStatusSucceeded || repository.updated.Progress != 100 || repository.updated.Error != "" {
		t.Fatalf("succeeded task = %+v", repository.updated)
	}
}

func TestRecorderClampsProgressAndPropagatesErrors(t *testing.T) {
	repository := &fakeRepository{createErr: domain.ErrUnavailable}
	recorder := New(repository)
	if _, err := recorder.Start(context.Background(), "vm.clone", "vm", "uuid-1", "admin", ""); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("Start() error = %v", err)
	}
	repository.createErr = nil
	recorder.newID = func() (string, error) { return "task_test", nil }
	if _, err := recorder.Start(context.Background(), "", "vm", "uuid-1", "admin", ""); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Start() invalid error = %v", err)
	}
	if err := recorder.Progress(context.Background(), "task_test", 150, "clamp"); err != nil {
		t.Fatalf("Progress() error = %v", err)
	}
	if repository.updated.Progress != 100 {
		t.Fatalf("progress = %d, want 100", repository.updated.Progress)
	}
	if err := recorder.Progress(context.Background(), "task_test", -10, "clamp"); err != nil {
		t.Fatalf("Progress() error = %v", err)
	}
	if repository.updated.Progress != 0 {
		t.Fatalf("progress = %d, want 0", repository.updated.Progress)
	}
}

func TestRecorderCancellationWinsOverLateSuccess(t *testing.T) {
	repository := &fakeRepository{}
	recorder := New(repository)
	recorder.newID = func() (string, error) { return "task-cancel", nil }
	id, err := recorder.Start(context.Background(), "vm.clone", "vm", "uuid-1", "admin", "正在克隆")
	if err != nil {
		t.Fatal(err)
	}
	cancelled := false
	recorder.RegisterCancel(id, func() { cancelled = true })
	if !recorder.MarkCancelled(id) {
		t.Fatal("MarkCancelled() = false")
	}
	if !cancelled {
		t.Fatal("cancel function was not called")
	}
	if err := recorder.Succeed(context.Background(), id, "克隆完成"); err != nil {
		t.Fatal(err)
	}
	if repository.updated.Status != domain.TaskStatusCancelled || repository.updated.Error != "" {
		t.Fatalf("late success update = %+v", repository.updated)
	}
}

func TestRecorderApplyCancelBeforeRegistrationAndReleaseResourceLock(t *testing.T) {
	repository := &fakeRepository{}
	recorder := New(repository)
	ids := []string{"task-first", "task-second", "task-third"}
	index := 0
	recorder.newID = func() (string, error) {
		id := ids[index]
		index++
		return id, nil
	}
	first, err := recorder.Start(context.Background(), "vm.clone", "vm", "uuid-1", "admin", "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.Start(context.Background(), "vm.clone", "vm", "uuid-1", "admin", "second"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second start error = %v", err)
	}
	if repository.updated.Status != domain.TaskStatusFailed || repository.updated.Step != "任务被拒绝" {
		t.Fatalf("conflicting task = %+v", repository.updated)
	}
	recorder.RegisterCancel(first, func() {})
	if !recorder.MarkCancelled(first) {
		t.Fatal("MarkCancelled() = false")
	}
	cancelled := false
	recorder.RegisterCancel(first, func() { cancelled = true })
	if !cancelled {
		t.Fatal("cancel registered after cancellation was not invoked")
	}
	if err := recorder.Fail(context.Background(), first, 10, "停止", errors.New("cancelled")); err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.Start(context.Background(), "vm.clone", "vm", "uuid-1", "admin", "third"); err != nil {
		t.Fatalf("resource lock was not released: %v", err)
	}
}

func TestGenerateTaskID(t *testing.T) {
	first, err := generateTaskID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := generateTaskID()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len("task_")+24 || first == second {
		t.Fatalf("ids = %q %q", first, second)
	}
}
