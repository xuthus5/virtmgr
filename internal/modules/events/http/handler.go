package eventshttp

import (
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"time"

	"virtmgr/internal/domain"
	eventsapp "virtmgr/internal/modules/events/app"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *eventsapp.Service
}

func New(service *eventsapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	limit, err := parseLimit(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, h.service.List(limit))
}

func (h *Handler) Stream(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	flusher, ok := w.(stdhttp.Flusher)
	if !ok {
		httpx.Error(w, r, fmt.Errorf("streaming unsupported"))
		return
	}
	limit, err := parseLimit(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	controller := stdhttp.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(stdhttp.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 3000\n\n: connected\n\n")
	flusher.Flush()

	initial, channel, cancel := h.service.Stream(limit)
	defer cancel()
	for _, event := range initial {
		if writeEvent(w, event) != nil {
			return
		}
	}
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case event, open := <-channel:
			if !open {
				return
			}
			if writeEvent(w, event) != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeEvent(w stdhttp.ResponseWriter, event domain.SystemEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.ID, payload)
	return err
}

func parseLimit(r *stdhttp.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return eventsapp.DefaultLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > eventsapp.MaxLimit {
		return 0, domain.ErrInvalidInput
	}
	return limit, nil
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/events", h.List)
	r.Get("/events/stream", h.Stream)
}
