package networkshttp

import (
	stdhttp "net/http"
	"net/url"

	"virtmgr/internal/domain"
	networksapp "virtmgr/internal/modules/networks/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *networksapp.Service
}

func New(service *networksapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.List(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Leases(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.ListLeases(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Create(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.VirtualNetworkCreateSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	network, err := h.service.Create(r.Context(), networksapp.CreateNetworkCommand{
		Spec:    spec,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, network)
}

func (h *Handler) Update(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.VirtualNetworkUpdateSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	network, err := h.service.Update(r.Context(), networksapp.UpdateNetworkCommand{
		Name:    chi.URLParam(r, "name"),
		Spec:    spec,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, network)
}

func (h *Handler) Action(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	action, ok := domain.ValidVirtualNetworkAction(chi.URLParam(r, "action"))
	if !ok {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.service.PerformAction(r.Context(), networksapp.NetworkActionCommand{
		Name:    chi.URLParam(r, "name"),
		Action:  action,
		Request: mutationContext(r, session),
	}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) AddDHCPHost(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.NetworkDHCPHostCreateSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	network, err := h.service.AddDHCPHost(r.Context(), networksapp.AddNetworkDHCPHostCommand{
		Name:    chi.URLParam(r, "name"),
		Spec:    spec,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, network)
}

func (h *Handler) DeleteDHCPHost(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	mac, err := url.PathUnescape(chi.URLParam(r, "mac"))
	if err != nil {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	network, err := h.service.DeleteDHCPHost(r.Context(), networksapp.DeleteNetworkDHCPHostCommand{
		Name:    chi.URLParam(r, "name"),
		MAC:     mac,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, network)
}

func (h *Handler) Delete(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.service.Delete(r.Context(), networksapp.DeleteNetworkCommand{
		Name:    chi.URLParam(r, "name"),
		Request: mutationContext(r, session),
	}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/networks", h.List)
	r.Get("/networks/{name}/leases", h.Leases)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/networks", h.Create)
		mutating.Patch("/networks/{name}", h.Update)
		mutating.Post("/networks/{name}/hosts", h.AddDHCPHost)
		mutating.Delete("/networks/{name}/hosts/{mac}", h.DeleteDHCPHost)
		mutating.Post("/networks/{name}/actions/{action}", h.Action)
		mutating.Delete("/networks/{name}", h.Delete)
	})
}

func mutationContext(r *stdhttp.Request, session domain.Session) networksapp.MutationContext {
	return networksapp.MutationContext{
		Session:   session,
		RequestID: middleware.GetReqID(r.Context()),
		SourceIP:  httpx.SourceIP(r),
	}
}
