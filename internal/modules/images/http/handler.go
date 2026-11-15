package http

import (
	stdhttp "net/http"
	"strconv"

	"virtmgr/internal/domain"
	imagesapp "virtmgr/internal/modules/images/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *imagesapp.Service
}

func New(service *imagesapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/images", h.List)
	r.Get("/images/{id}", h.Get)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/images", h.Register)
		mutating.Post("/images/{id}/refresh", h.Refresh)
		mutating.Delete("/images/{id}", h.Delete)
	})
}

func (h *Handler) List(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.List(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	result, err := h.service.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Register(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	var spec domain.VMImageCreateSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	image, err := h.service.Register(r.Context(), spec, mutationContext(r, session))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, image)
}

func (h *Handler) Refresh(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	image, err := h.service.Refresh(r.Context(), id, mutationContext(r, session))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, image)
}

func (h *Handler) Delete(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.service.Delete(r.Context(), id, mutationContext(r, session)); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func parseID(r *stdhttp.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		return 0, domain.ErrInvalidInput
	}
	return id, nil
}

func mutationContext(r *stdhttp.Request, session domain.Session) imagesapp.MutationContext {
	return imagesapp.MutationContext{
		Session: session, RequestID: middleware.GetReqID(r.Context()), SourceIP: httpx.SourceIP(r),
	}
}
