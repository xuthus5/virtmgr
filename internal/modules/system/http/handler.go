package systemhttp

import (
	stdhttp "net/http"
	"os"
	"strconv"
	"time"

	"virtmgr/internal/domain"
	systemapp "virtmgr/internal/modules/system/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *systemapp.Service
}

func New(service *systemapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Info(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	httpx.JSON(w, stdhttp.StatusOK, h.service.Info(r.Host))
}

func (h *Handler) ExportDatabase(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	result, err := h.service.ExportDatabase(r.Context(), systemapp.MutationContext{
		Session: session, RequestID: middleware.GetReqID(r.Context()), SourceIP: httpx.SourceIP(r),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	defer func() { _ = os.Remove(result.Path) }()
	file, err := os.Open(result.Path)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Length", strconv.FormatInt(result.Size, 10))
	w.Header().Set("Content-Disposition", `attachment; filename="`+result.Filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(stdhttp.StatusOK)
	_, _ = file.WriteTo(w)
}

func (h *Handler) GetDatabaseRestore(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if session.User.Role != domain.RoleAdmin {
		httpx.Error(w, r, domain.ErrForbidden)
		return
	}
	result, err := h.service.DatabaseRestoreStatus()
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) StageDatabaseRestore(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	_ = stdhttp.NewResponseController(w).SetReadDeadline(time.Time{})
	status, err := h.service.StageDatabaseRestore(r.Context(), r.Body, r.ContentLength, r.URL.Query().Get("filename"), systemapp.MutationContext{
		Session: session, RequestID: middleware.GetReqID(r.Context()), SourceIP: httpx.SourceIP(r),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusAccepted, status)
}

func (h *Handler) CancelDatabaseRestore(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	status, err := h.service.CancelDatabaseRestore(r.Context(), systemapp.MutationContext{
		Session: session, RequestID: middleware.GetReqID(r.Context()), SourceIP: httpx.SourceIP(r),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, status)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/system/info", h.Info)
	r.Get("/system/database-restore", h.GetDatabaseRestore)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/system/database-export", h.ExportDatabase)
		mutating.Post("/system/database-restore", h.StageDatabaseRestore)
		mutating.Delete("/system/database-restore", h.CancelDatabaseRestore)
	})
}
