package snapshotpolicieshttp

import (
	"net"
	stdhttp "net/http"
	"strconv"
	"strings"

	"virtmgr/internal/domain"
	snapshotpoliciesapp "virtmgr/internal/modules/snapshotpolicies/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *snapshotpoliciesapp.Service
}

func New(service *snapshotpoliciesapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.List(r.Context(), chi.URLParam(r, "uuid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Create(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.SnapshotPolicySpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	policy, err := h.service.Create(r.Context(), snapshotpoliciesapp.CreateCommand{
		VMUUID: chi.URLParam(r, "uuid"), Spec: spec, Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, policy)
}

func (h *Handler) Update(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var spec domain.SnapshotPolicySpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	policy, err := h.service.Update(r.Context(), snapshotpoliciesapp.UpdateCommand{
		ID: id, Spec: spec, Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, policy)
}

func (h *Handler) Delete(w stdhttp.ResponseWriter, r *stdhttp.Request) {
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
	if err := h.service.Delete(r.Context(), snapshotpoliciesapp.DeleteCommand{ID: id, Request: mutationContext(r, session)}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) RunNow(w stdhttp.ResponseWriter, r *stdhttp.Request) {
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
	result, err := h.service.RunNow(r.Context(), snapshotpoliciesapp.RunCommand{ID: id, Request: mutationContext(r, session)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/vms/{uuid}/snapshot-policies", h.List)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/vms/{uuid}/snapshot-policies", h.Create)
		mutating.Patch("/snapshot-policies/{id}", h.Update)
		mutating.Delete("/snapshot-policies/{id}", h.Delete)
		mutating.Post("/snapshot-policies/{id}/run", h.RunNow)
	})
}

func parseID(r *stdhttp.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		return 0, domain.ErrInvalidInput
	}
	return id, nil
}

func mutationContext(r *stdhttp.Request, session domain.Session) snapshotpoliciesapp.MutationContext {
	return snapshotpoliciesapp.MutationContext{
		Session: session, RequestID: middleware.GetReqID(r.Context()), SourceIP: sourceIP(r),
	}
}

func sourceIP(r *stdhttp.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
