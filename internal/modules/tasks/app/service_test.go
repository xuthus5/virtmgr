package app

import (
	"context"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type fakeRepository struct {
	tasks     domain.TaskList
	task      domain.Task
	query     domain.TaskQuery
	cancelled domain.Task
	err       error
}

func (f *fakeRepository) ListTasks(_ context.Context, query domain.TaskQuery) (domain.TaskList, error) {
	f.query = query
	return f.tasks, f.err
}

func (f *fakeRepository) GetTask(context.Context, string) (domain.Task, error) {
	return f.task, f.err
}

func (f *fakeRepository) CancelTask(_ context.Context, id string, cancelledAt time.Time, step string) (domain.Task, error) {
	if f.err != nil {
		return domain.Task{}, f.err
	}
	task := f.task
	task.ID = id
	task.Status = domain.TaskStatusCancelled
	task.Step = step
	task.UpdatedAt = cancelledAt
	f.cancelled = task
	return task, nil
}

type fakeRegistry struct{ marked bool }

func (f *fakeRegistry) MarkCancelled(id string) bool {
	if !f.marked || id == "" {
		return false
	}
	return true
}

type readOnlyMode bool

func (m readOnlyMode) ReadOnly() bool { return bool(m) }

func TestListTasksNormalizesQueryAndValidatesStatus(t *testing.T) {
	repository := &fakeRepository{tasks: domain.TaskList{Total: 1, Items: []domain.Task{{ID: "task-1"}}}}
	service := New(repository)
	result, err := service.List(context.Background(), domain.TaskQuery{Status: "running", Page: 0, PageSize: 999})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.Total != 1 || repository.query.Page != 1 || repository.query.PageSize != domain.MaxTaskPageSize {
		t.Fatalf("result = %+v query = %+v", result, repository.query)
	}
	if _, err := service.List(context.Background(), domain.TaskQuery{Status: "unknown"}); err != domain.ErrInvalidInput {
		t.Fatalf("invalid status error = %v", err)
	}
}

func TestGetTaskValidatesIDAndPropagatesNotFound(t *testing.T) {
	service := New(&fakeRepository{err: domain.ErrNotFound})
	if _, err := service.Get(context.Background(), " "); err != domain.ErrInvalidInput {
		t.Fatalf("blank id error = %v", err)
	}
	if _, err := service.Get(context.Background(), "task-1"); err != domain.ErrNotFound {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestCancelTaskRequiresActiveRegistrationAndWritesAudit(t *testing.T) {
	repository := &fakeRepository{task: domain.Task{ID: "task-1", Kind: "vm.clone", TargetKind: "vm", TargetID: "uuid-1", Status: domain.TaskStatusRunning}}
	registry := &fakeRegistry{}
	service := New(repository, WithCancellationRegistry(registry))
	session := domain.Session{User: domain.User{Username: "operator", Role: domain.RoleOperator}}
	if _, err := service.Cancel(context.Background(), "task-1", session, "req-1", "127.0.0.1"); err != domain.ErrConflict {
		t.Fatalf("Cancel() inactive error = %v", err)
	}
	registry.marked = true
	task, err := service.Cancel(context.Background(), "task-1", session, "req-1", "127.0.0.1")
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if task.Status != domain.TaskStatusCancelled || repository.cancelled.ID != "task-1" || task.Cancellable {
		t.Fatalf("cancelled task = %+v repository = %+v", task, repository.cancelled)
	}
	if _, err := service.Cancel(context.Background(), "task-1", domain.Session{User: domain.User{Username: "viewer", Role: domain.RoleViewer}}, "", ""); err != domain.ErrForbidden {
		t.Fatalf("viewer cancel error = %v", err)
	}
	readOnlyService := New(repository, WithCancellationRegistry(registry), WithReadOnlyMode(readOnlyMode(true)))
	if _, err := readOnlyService.Cancel(context.Background(), "task-1", session, "req-1", "127.0.0.1"); err != domain.ErrReadOnly {
		t.Fatalf("read-only cancel error = %v", err)
	}
}

func TestListMarksOnlyRunningKnownTaskKindsCancellable(t *testing.T) {
	repository := &fakeRepository{tasks: domain.TaskList{Items: []domain.Task{
		{ID: "task-1", Kind: "vm.clone", Status: domain.TaskStatusRunning},
		{ID: "task-2", Kind: "vm.delete", Status: domain.TaskStatusRunning},
		{ID: "task-3", Kind: "vm.backup", Status: domain.TaskStatusSucceeded},
	}}}
	result, err := New(repository).List(context.Background(), domain.TaskQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Items[0].Cancellable || result.Items[1].Cancellable || result.Items[2].Cancellable {
		t.Fatalf("cancellable flags = %+v", result.Items)
	}
}
