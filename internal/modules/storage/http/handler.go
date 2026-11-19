package storagehttp

import (
	"mime"
	stdhttp "net/http"
	"path/filepath"
	"strings"
	"time"

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
			SourceIP:  httpx.SourceIP(r),
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
	_ = stdhttp.NewResponseController(w).SetReadDeadline(time.Time{})
	volume, err := h.service.UploadVolume(r.Context(), storageapp.UploadVolumeCommand{
		Pool: chi.URLParam(r, "pool"),
		Name: r.URL.Query().Get("name"),
		Size: r.ContentLength,
		Body: r.Body,
		Request: storageapp.MutationContext{
			Session:   session,
			RequestID: middleware.GetReqID(r.Context()),
			SourceIP:  httpx.SourceIP(r),
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
			SourceIP:  httpx.SourceIP(r),
		},
	}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) DownloadVolume(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	name := filepath.Base(strings.TrimSpace(chi.URLParam(r, "volume")))
	if name == "." || name == string(filepath.Separator) || name == "" {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	stream := &downloadWriter{
		ResponseWriter: w,
		contentType:    "application/octet-stream",
		disposition:    mime.FormatMediaType("attachment", map[string]string{"filename": name}),
	}
	_, err := h.service.DownloadVolume(r.Context(), storageapp.DownloadVolumeCommand{
		Pool: chi.URLParam(r, "pool"), Name: name, Writer: stream, Request: mutationContext(r, session),
	})
	if err != nil {
		if !stream.started {
			httpx.Error(w, r, err)
		}
		return
	}
	stream.start()
}

type downloadWriter struct {
	stdhttp.ResponseWriter
	started     bool
	contentType string
	disposition string
}

func (w *downloadWriter) start() {
	if w.started {
		return
	}
	w.started = true
	w.Header().Set("Content-Type", w.contentType)
	w.Header().Set("Content-Disposition", w.disposition)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(stdhttp.StatusOK)
}

func (w *downloadWriter) Write(p []byte) (int, error) {
	w.start()
	return w.ResponseWriter.Write(p)
}

func (h *Handler) ConvertVolume(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.StorageVolumeConvertSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	task, err := h.service.ConvertVolume(r.Context(), storageapp.ConvertVolumeCommand{
		Pool: chi.URLParam(r, "pool"), Name: chi.URLParam(r, "volume"), Spec: spec, Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusAccepted, task)
}

func (h *Handler) FlattenVolume(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.StorageVolumeFlattenSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	task, err := h.service.FlattenVolume(r.Context(), storageapp.FlattenVolumeCommand{
		Pool: chi.URLParam(r, "pool"), Name: chi.URLParam(r, "volume"), Spec: spec, Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusAccepted, task)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/storage/pools", h.ListPools)
	r.Get("/storage/pools/{pool}", h.GetPool)
	r.Get("/storage/pools/{pool}/volumes", h.ListVolumes)
	r.Get("/storage/pools/{pool}/volumes/{volume}/download", h.DownloadVolume)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/storage/pools", h.CreatePool)
		mutating.Post("/storage/pools/{pool}/actions/{action}", h.PoolAction)
		mutating.Delete("/storage/pools/{pool}", h.DeletePool)
		mutating.Post("/storage/pools/{pool}/volumes", h.CreateVolume)
		mutating.Post("/storage/pools/{pool}/volumes/upload", h.UploadVolume)
		mutating.Post("/storage/pools/{pool}/volumes/{volume}/convert", h.ConvertVolume)
		mutating.Post("/storage/pools/{pool}/volumes/{volume}/flatten", h.FlattenVolume)
		mutating.Delete("/storage/pools/{pool}/volumes/{volume}", h.DeleteVolume)
	})
}

func mutationContext(r *stdhttp.Request, session domain.Session) storageapp.MutationContext {
	return storageapp.MutationContext{
		Session:   session,
		RequestID: middleware.GetReqID(r.Context()),
		SourceIP:  httpx.SourceIP(r),
	}
}
