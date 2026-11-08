package taskshttp

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"virtmgr/internal/domain"
	tasksapp "virtmgr/internal/modules/tasks/app"

	"github.com/go-chi/chi/v5"
)

type handlerRepository struct {
	query domain.TaskQuery
}

func (h *handlerRepository) ListTasks(_ context.Context, query domain.TaskQuery) (domain.TaskList, error) {
	h.query = query
	return domain.TaskList{Items: []domain.Task{{ID: "task-1", Kind: "vm.clone", Status: domain.TaskStatusRunning, Progress: 50}}, Total: 1, Page: query.Page, PageSize: query.PageSize}, nil
}

func (h *handlerRepository) GetTask(_ context.Context, id string) (domain.Task, error) {
	if id != "task-1" {
		return domain.Task{}, domain.ErrNotFound
	}
	return domain.Task{ID: id, Kind: "vm.clone", Status: domain.TaskStatusSucceeded, Progress: 100}, nil
}

func TestListTasks(t *testing.T) {
	repository := &handlerRepository{}
	router := chi.NewRouter()
	New(tasksapp.New(repository)).Mount(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/tasks?status=running&page=2&pageSize=1", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result domain.TaskList
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Items[0].ID != "task-1" || repository.query.Status != domain.TaskStatusRunning || repository.query.Page != 2 {
		t.Fatalf("result = %+v query = %+v", result, repository.query)
	}
}

func TestListTasksRejectsInvalidQuery(t *testing.T) {
	router := chi.NewRouter()
	New(tasksapp.New(&handlerRepository{})).Mount(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/tasks?page=zero", nil))
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGetTask(t *testing.T) {
	router := chi.NewRouter()
	New(tasksapp.New(&handlerRepository{})).Mount(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/tasks/task-1", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var task domain.Task
	if err := json.NewDecoder(recorder.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.ID != "task-1" || task.Status != domain.TaskStatusSucceeded {
		t.Fatalf("task = %+v", task)
	}
}
