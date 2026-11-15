package http

import (
	stdhttp "net/http"

	"virtmgr/internal/domain"
	guestagentapp "virtmgr/internal/modules/guestagent/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *guestagentapp.Service
}

func New(service *guestagentapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	refresh := r.URL.Query().Get("refresh") == "true"
	result, err := h.service.Get(r.Context(), chi.URLParam(r, "uuid"), refresh)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Action(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.GuestAgentActionSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.service.Action(r.Context(), guestagentapp.ActionCommand{
		UUID: chi.URLParam(r, "uuid"), Spec: spec, Request: mutationContext(r, session),
	}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) EnsureChannel(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	vm, err := h.service.EnsureChannel(r.Context(), guestagentapp.ChannelCommand{
		UUID: chi.URLParam(r, "uuid"), Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/vms/{uuid}/guest-agent", h.Get)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/vms/{uuid}/guest-agent/actions", h.Action)
		mutating.Post("/vms/{uuid}/guest-agent/channel", h.EnsureChannel)
	})
}

func mutationContext(r *stdhttp.Request, session domain.Session) guestagentapp.MutationContext {
	return guestagentapp.MutationContext{
		Session: session, RequestID: middleware.GetReqID(r.Context()), SourceIP: httpx.SourceIP(r),
	}
}
