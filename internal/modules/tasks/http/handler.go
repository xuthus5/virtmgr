package taskshttp

import (
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"time"

	"virtmgr/internal/domain"
	tasksapp "virtmgr/internal/modules/tasks/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

func (h *Handler) Retry(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	task, err := h.service.Retry(r.Context(), chi.URLParam(r, "id"), session, middleware.GetReqID(r.Context()), httpx.SourceIP(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusAccepted, task)
}

func (h *Handler) Stream(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	flusher, ok := w.(stdhttp.Flusher)
	if !ok {
		httpx.Error(w, r, domain.ErrUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(stdhttp.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()
	ticker := time.NewTicker(h.service.StreamInterval())
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			snapshot, err := h.service.StreamSnapshot(r.Context())
			if err != nil {
				return
			}
			payload, err := json.Marshal(snapshot)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "event: tasks\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Handler) Mount(r chi.Router, csrfMiddleware ...func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/tasks", h.List)
	r.Get("/tasks/stream", h.Stream)
	r.Get("/tasks/{id}", h.Get)
	r.Group(func(mutating chi.Router) {
		if len(csrfMiddleware) > 0 && csrfMiddleware[0] != nil {
			mutating.Use(csrfMiddleware[0])
		}
		mutating.Post("/tasks/{id}/retry", h.Retry)
	})
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
