package http

import (
	"net"
	stdhttp "net/http"
	"strconv"
	"strings"

	"virtmgr/internal/domain"
	backupsapp "virtmgr/internal/modules/backups/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *backupsapp.Service
}

type createBackupInput struct {
	VMUUID string `json:"vmUuid"`
	domain.BackupCreateSpec
}

type verifyBackupInput struct {
	Deep bool `json:"deep"`
}

type updateScheduleInput struct {
	RepositoryID   string `json:"repositoryId"`
	Name           string `json:"name"`
	IntervalHours  int    `json:"intervalHours"`
	RetentionCount int    `json:"retentionCount"`
	RetentionDays  int    `json:"retentionDays"`
	Enabled        bool   `json:"enabled"`
}

func New(service *backupsapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListBackups(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	query, err := parseBackupQuery(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	result, err := h.service.List(r.Context(), query)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) GetBackup(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Summary(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.Summary(r.Context(), r.URL.Query().Get("vmUuid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) CreateBackup(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var input createBackupInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	task, err := h.service.Create(r.Context(), backupsapp.CreateCommand{
		VMUUID: input.VMUUID, Spec: input.BackupCreateSpec, Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusAccepted, task)
}

func (h *Handler) Verify(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var input verifyBackupInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	task, err := h.service.Verify(r.Context(), backupsapp.VerifyCommand{ID: chi.URLParam(r, "id"), Deep: input.Deep, Request: mutationContext(r, session)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusAccepted, task)
}

func (h *Handler) Restore(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.VMRestoreSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	task, err := h.service.Restore(r.Context(), backupsapp.RestoreCommand{ID: chi.URLParam(r, "id"), Spec: spec, Request: mutationContext(r, session)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusAccepted, task)
}

func (h *Handler) Delete(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	task, err := h.service.Delete(r.Context(), backupsapp.DeleteCommand{ID: chi.URLParam(r, "id"), Request: mutationContext(r, session)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusAccepted, task)
}

func (h *Handler) UpdatePin(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var input domain.BackupPinSpec
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	item, err := h.service.Pin(r.Context(), backupsapp.PinCommand{ID: chi.URLParam(r, "id"), Pinned: input.Pinned, Request: mutationContext(r, session)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, item)
}

func (h *Handler) ListRepositories(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	items, err := h.service.ListRepositories(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, items)
}

func (h *Handler) CreateRepository(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.BackupRepositorySpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	item, err := h.service.CreateRepository(r.Context(), backupsapp.RepositoryCommand{Spec: spec, Request: mutationContext(r, session)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, item)
}

func (h *Handler) UpdateRepository(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.BackupRepositorySpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	item, err := h.service.UpdateRepository(r.Context(), backupsapp.RepositoryCommand{ID: chi.URLParam(r, "id"), Spec: spec, Request: mutationContext(r, session)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, item)
}

func (h *Handler) DeleteRepository(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.service.DeleteRepository(r.Context(), backupsapp.RepositoryCommand{ID: chi.URLParam(r, "id"), Request: mutationContext(r, session)}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) ListSchedules(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	items, err := h.service.ListSchedules(r.Context(), r.URL.Query().Get("vmUuid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, items)
}

func (h *Handler) CreateSchedule(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.BackupScheduleSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	item, err := h.service.CreateSchedule(r.Context(), backupsapp.ScheduleCommand{VMUUID: spec.VMUUID, Spec: spec, Request: mutationContext(r, session)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, item)
}

func (h *Handler) UpdateSchedule(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var input updateScheduleInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	item, err := h.service.UpdateSchedule(r.Context(), backupsapp.ScheduleCommand{ID: id, Spec: domain.BackupScheduleSpec{
		RepositoryID: input.RepositoryID, Name: input.Name, IntervalHours: input.IntervalHours,
		RetentionCount: input.RetentionCount, RetentionDays: input.RetentionDays, Enabled: input.Enabled,
	}, Request: mutationContext(r, session)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, item)
}

func (h *Handler) DeleteSchedule(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.service.DeleteSchedule(r.Context(), backupsapp.ScheduleDeleteCommand{ID: id, Request: mutationContext(r, session)}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) RunSchedule(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	task, err := h.service.RunScheduleNow(r.Context(), backupsapp.ScheduleRunCommand{ID: id, Request: mutationContext(r, session)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusAccepted, task)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/backups", h.ListBackups)
	r.Get("/backups/summary", h.Summary)
	r.Get("/backups/{id}", h.GetBackup)
	r.Get("/backup-repositories", h.ListRepositories)
	r.Get("/backup-schedules", h.ListSchedules)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/backups", h.CreateBackup)
		mutating.Post("/backups/{id}/verify", h.Verify)
		mutating.Post("/backups/{id}/restore", h.Restore)
		mutating.Patch("/backups/{id}", h.UpdatePin)
		mutating.Delete("/backups/{id}", h.Delete)
		mutating.Post("/backup-repositories", h.CreateRepository)
		mutating.Patch("/backup-repositories/{id}", h.UpdateRepository)
		mutating.Delete("/backup-repositories/{id}", h.DeleteRepository)
		mutating.Post("/backup-schedules", h.CreateSchedule)
		mutating.Patch("/backup-schedules/{id}", h.UpdateSchedule)
		mutating.Delete("/backup-schedules/{id}", h.DeleteSchedule)
		mutating.Post("/backup-schedules/{id}/run", h.RunSchedule)
	})
}

func parseBackupQuery(r *stdhttp.Request) (domain.BackupQuery, error) {
	page, err := parseOptionalPositive(r.URL.Query().Get("page"))
	if err != nil {
		return domain.BackupQuery{}, err
	}
	pageSize, err := parseOptionalPositive(r.URL.Query().Get("pageSize"))
	if err != nil {
		return domain.BackupQuery{}, err
	}
	return domain.BackupQuery{
		VMUUID: r.URL.Query().Get("vmUuid"), Status: domain.BackupStatus(r.URL.Query().Get("status")),
		RepositoryID: r.URL.Query().Get("repositoryId"), Page: page, PageSize: pageSize,
	}, nil
}

func parseOptionalPositive(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, domain.ErrInvalidInput
	}
	return parsed, nil
}

func parseID(r *stdhttp.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		return 0, domain.ErrInvalidInput
	}
	return id, nil
}

func mutationContext(r *stdhttp.Request, session domain.Session) backupsapp.MutationContext {
	return backupsapp.MutationContext{Session: session, RequestID: middleware.GetReqID(r.Context()), SourceIP: sourceIP(r)}
}

func sourceIP(r *stdhttp.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
