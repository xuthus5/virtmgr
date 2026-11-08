package storagehttp

import (
	"net"
	stdhttp "net/http"
	"strings"

	"virtmgr/internal/domain"
	storageapp "virtmgr/internal/modules/storage/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *storageapp.Service
}

func New(service *storageapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListPools(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.ListPools(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) GetPool(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.GetPool(r.Context(), chi.URLParam(r, "pool"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) CreatePool(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.StoragePoolCreateSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	pool, err := h.service.CreatePool(r.Context(), storageapp.CreatePoolCommand{
		Spec:    spec,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, pool)
}

func (h *Handler) PoolAction(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	action, ok := domain.ValidStoragePoolAction(chi.URLParam(r, "action"))
	if !ok {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.service.PerformPoolAction(r.Context(), storageapp.StoragePoolActionCommand{
		Name:    chi.URLParam(r, "pool"),
		Action:  action,
		Request: mutationContext(r, session),
	}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) DeletePool(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.service.DeletePool(r.Context(), storageapp.DeletePoolCommand{
		Name:    chi.URLParam(r, "pool"),
		Request: mutationContext(r, session),
	}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) ListVolumes(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.ListVolumes(r.Context(), chi.URLParam(r, "pool"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) CreateVolume(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.StorageVolumeCreateSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	volume, err := h.service.CreateVolume(r.Context(), storageapp.CreateVolumeCommand{
		Pool: chi.URLParam(r, "pool"),
		Spec: spec,
		Request: storageapp.MutationContext{
			Session:   session,
			RequestID: middleware.GetReqID(r.Context()),
			SourceIP:  sourceIP(r),
		},
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, volume)
}

func (h *Handler) UploadVolume(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	volume, err := h.service.UploadVolume(r.Context(), storageapp.UploadVolumeCommand{
		Pool: chi.URLParam(r, "pool"),
		Name: r.URL.Query().Get("name"),
		Size: r.ContentLength,
		Body: r.Body,
		Request: storageapp.MutationContext{
			Session:   session,
			RequestID: middleware.GetReqID(r.Context()),
			SourceIP:  sourceIP(r),
		},
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, volume)
}

func (h *Handler) DeleteVolume(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.service.DeleteVolume(r.Context(), storageapp.DeleteVolumeCommand{
		Pool: chi.URLParam(r, "pool"),
		Name: chi.URLParam(r, "volume"),
		Request: storageapp.MutationContext{
			Session:   session,
			RequestID: middleware.GetReqID(r.Context()),
			SourceIP:  sourceIP(r),
		},
	}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/storage/pools", h.ListPools)
	r.Get("/storage/pools/{pool}", h.GetPool)
	r.Get("/storage/pools/{pool}/volumes", h.ListVolumes)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/storage/pools", h.CreatePool)
		mutating.Post("/storage/pools/{pool}/actions/{action}", h.PoolAction)
		mutating.Delete("/storage/pools/{pool}", h.DeletePool)
		mutating.Post("/storage/pools/{pool}/volumes", h.CreateVolume)
		mutating.Post("/storage/pools/{pool}/volumes/upload", h.UploadVolume)
		mutating.Delete("/storage/pools/{pool}/volumes/{volume}", h.DeleteVolume)
	})
}

func mutationContext(r *stdhttp.Request, session domain.Session) storageapp.MutationContext {
	return storageapp.MutationContext{
		Session:   session,
		RequestID: middleware.GetReqID(r.Context()),
		SourceIP:  sourceIP(r),
	}
}

func sourceIP(r *stdhttp.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
