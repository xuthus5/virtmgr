package audithistoryhttp

import (
	"fmt"
	stdhttp "net/http"
	"strconv"

	"virtmgr/internal/domain"
	audithistoryapp "virtmgr/internal/modules/audithistory/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *audithistoryapp.Service
}

func New(service *audithistoryapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	query, err := parseQuery(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	result, err := h.service.List(r.Context(), session, query)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/audit-events", h.List)
}

func parseQuery(r *stdhttp.Request) (domain.AuditEventQuery, error) {
	values := r.URL.Query()
	page, err := parsePositiveInt(values.Get("page"))
	if err != nil {
		return domain.AuditEventQuery{}, invalidQuery("page")
	}
	pageSize, err := parsePositiveInt(values.Get("pageSize"))
	if err != nil {
		return domain.AuditEventQuery{}, invalidQuery("pageSize")
	}
	return domain.AuditEventQuery{
		Actor:      values.Get("actor"),
		Action:     values.Get("action"),
		TargetKind: values.Get("targetKind"),
		Result:     values.Get("result"),
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

func parsePositiveInt(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, domain.ErrInvalidInput
	}
	return parsed, nil
}

func invalidQuery(field string) error {
	return fmt.Errorf("%w: invalid %s", domain.ErrInvalidInput, field)
}
