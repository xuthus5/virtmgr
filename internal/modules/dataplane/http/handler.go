package http

import (
	stdhttp "net/http"
	"strconv"

	"virtmgr/internal/domain"
	dataplaneapp "virtmgr/internal/modules/dataplane/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *dataplaneapp.Service
}

func New(service *dataplaneapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	limit := 200
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 500 {
			httpx.Error(w, r, domain.ErrInvalidInput)
			return
		}
		limit = value
	}
	result, err := h.service.ListOperations(r.Context(), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	item, err := h.service.GetOperation(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, item)
}

func (h *Handler) requireAdmin(w stdhttp.ResponseWriter, r *stdhttp.Request) bool {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return false
	}
	if session.User.Role != domain.RoleAdmin {
		httpx.Error(w, r, domain.ErrForbidden)
		return false
	}
	return true
}

func (h *Handler) Recheck(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.runAction(w, r, false)
}

func (h *Handler) Unblock(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.runAction(w, r, true)
}

func (h *Handler) runAction(w stdhttp.ResponseWriter, r *stdhttp.Request, unblock bool) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	command := dataplaneapp.OperationCommand{
		ID: chi.URLParam(r, "id"),
		Request: dataplaneapp.MutationContext{
			Session: session, RequestID: middleware.GetReqID(r.Context()), SourceIP: httpx.SourceIP(r),
		},
	}
	var (
		result domain.DataPlaneOperationResult
		err    error
	)
	if unblock {
		result, err = h.service.Unblock(r.Context(), command)
	} else {
		result, err = h.service.Recheck(r.Context(), command)
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/dataplane/operations", h.List)
	r.Get("/dataplane/operations/{id}", h.Get)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/dataplane/operations/{id}/recheck", h.Recheck)
		mutating.Post("/dataplane/operations/{id}/unblock", h.Unblock)
	})
}
