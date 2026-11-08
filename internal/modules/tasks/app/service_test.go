package app

import (
	"context"
	"testing"

	"virtmgr/internal/domain"
)

type fakeRepository struct {
	tasks domain.TaskList
	task  domain.Task
	query domain.TaskQuery
	err   error
}

func (f *fakeRepository) ListTasks(_ context.Context, query domain.TaskQuery) (domain.TaskList, error) {
	f.query = query
	return f.tasks, f.err
}

func (f *fakeRepository) GetTask(context.Context, string) (domain.Task, error) {
	return f.task, f.err
}

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
