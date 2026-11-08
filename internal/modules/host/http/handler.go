package hosthttp

import (
	stdhttp "net/http"

	hostapp "virtmgr/internal/modules/host/app"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *hostapp.Service
}

func New(service *hostapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	details, err := h.service.Get(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, details)
}

func (h *Handler) Devices(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	devices, err := h.service.Devices(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, devices)
}

func (h *Handler) Metrics(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	metrics, err := h.service.Metrics(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, metrics)
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/host", h.Get)
	r.Get("/host/devices", h.Devices)
	r.Get("/host/metrics", h.Metrics)
}
