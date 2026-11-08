package overviewhttp

import (
	stdhttp "net/http"

	overviewapp "virtmgr/internal/modules/overview/app"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *overviewapp.Service
}

func New(service *overviewapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	overview, err := h.service.Get(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, overview)
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/overview", h.Get)
}
