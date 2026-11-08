package taskshttp

import (
	stdhttp "net/http"
	"strconv"

	"virtmgr/internal/domain"
	tasksapp "virtmgr/internal/modules/tasks/app"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *tasksapp.Service
}

func New(service *tasksapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	query, err := parseQuery(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	tasks, err := h.service.List(r.Context(), query)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, tasks)
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	task, err := h.service.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, task)
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/tasks", h.List)
	r.Get("/tasks/{id}", h.Get)
}

func parseQuery(r *stdhttp.Request) (domain.TaskQuery, error) {
	values := r.URL.Query()
	page, err := parsePositiveInt(values.Get("page"))
	if err != nil {
		return domain.TaskQuery{}, domain.ErrInvalidInput
	}
	pageSize, err := parsePositiveInt(values.Get("pageSize"))
	if err != nil {
		return domain.TaskQuery{}, domain.ErrInvalidInput
	}
	return domain.TaskQuery{
		Status:   domain.TaskStatus(values.Get("status")),
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func parsePositiveInt(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, domain.ErrInvalidInput
	}
	return parsed, nil
}
