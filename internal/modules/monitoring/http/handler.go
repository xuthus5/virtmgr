package http

import (
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"virtmgr/internal/domain"
	monitoringapp "virtmgr/internal/modules/monitoring/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *monitoringapp.Service
}

func New(service *monitoringapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListRules(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.ListRules(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) CreateRule(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.AlertRuleSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	item, err := h.service.CreateRule(r.Context(), monitoringapp.AlertRuleCommand{Spec: spec, Request: mutationContext(r, session)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, item)
}

func (h *Handler) UpdateRule(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var spec domain.AlertRuleSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	item, err := h.service.UpdateRule(r.Context(), monitoringapp.AlertRuleCommand{ID: id, Spec: spec, Request: mutationContext(r, session)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, item)
}

func (h *Handler) DeleteRule(w stdhttp.ResponseWriter, r *stdhttp.Request) {
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
	if err := h.service.DeleteRule(r.Context(), monitoringapp.AlertRuleCommand{ID: id, Request: mutationContext(r, session)}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) History(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	query, err := parseHistoryQuery(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	result, err := h.service.History(r.Context(), query)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/metrics/history", h.History)
	r.Get("/alert-rules", h.ListRules)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/alert-rules", h.CreateRule)
		mutating.Put("/alert-rules/{id}", h.UpdateRule)
		mutating.Delete("/alert-rules/{id}", h.DeleteRule)
	})
}

func parseHistoryQuery(r *stdhttp.Request) (monitoringapp.HistoryQuery, error) {
	values := r.URL.Query()
	query := monitoringapp.HistoryQuery{
		Scope: values.Get("scope"), TargetID: values.Get("targetId"),
	}
	if raw := strings.TrimSpace(values.Get("from")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return monitoringapp.HistoryQuery{}, domain.ErrInvalidInput
		}
		query.From = value
	}
	if raw := strings.TrimSpace(values.Get("to")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return monitoringapp.HistoryQuery{}, domain.ErrInvalidInput
		}
		query.To = value
	}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return monitoringapp.HistoryQuery{}, domain.ErrInvalidInput
		}
		query.Limit = value
	}
	return query, nil
}

func parseID(r *stdhttp.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		return 0, domain.ErrInvalidInput
	}
	return id, nil
}

func mutationContext(r *stdhttp.Request, session domain.Session) monitoringapp.MutationContext {
	return monitoringapp.MutationContext{Session: session, RequestID: middleware.GetReqID(r.Context()), SourceIP: httpx.SourceIP(r)}
}
