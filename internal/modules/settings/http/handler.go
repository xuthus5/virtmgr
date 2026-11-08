package settingshttp

import (
	"net"
	stdhttp "net/http"
	"strings"

	"virtmgr/internal/domain"
	settingsapp "virtmgr/internal/modules/settings/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *settingsapp.Service
}

func New(service *settingsapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/settings", h.Get)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Put("/settings", h.Update)
	})
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	httpx.JSON(w, stdhttp.StatusOK, h.service.Get())
}

func (h *Handler) Update(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var input struct {
		ReadOnly *bool `json:"readOnly"`
	}
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if input.ReadOnly == nil {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	result, err := h.service.Update(r.Context(), settingsapp.UpdateCommand{
		ReadOnly: *input.ReadOnly,
		Request: settingsapp.MutationContext{
			Session:   session,
			RequestID: middleware.GetReqID(r.Context()),
			SourceIP:  sourceIP(r),
		},
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func sourceIP(r *stdhttp.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}
