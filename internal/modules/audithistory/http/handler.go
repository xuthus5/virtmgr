package audithistoryhttp

import (
	"fmt"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"virtmgr/internal/domain"
	audithistoryapp "virtmgr/internal/modules/audithistory/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

func (h *Handler) Export(w stdhttp.ResponseWriter, r *stdhttp.Request) {
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
	format, ok := domain.ValidAuditExportFormat(r.URL.Query().Get("format"))
	if !ok {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	result, err := h.service.Export(r.Context(), audithistoryapp.ExportCommand{
		Session: session, Query: query, Format: format,
		RequestID: middleware.GetReqID(r.Context()), SourceIP: httpx.SourceIP(r),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	filename := fmt.Sprintf("virtmgr-audit-%s.%s", time.Now().UTC().Format("20060102-150405"), format)
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(stdhttp.StatusOK)
	_, _ = w.Write(result.Data)
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/audit-events", h.List)
	r.Get("/audit-events/export", h.Export)
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
	from, err := parseOptionalTime(values.Get("from"))
	if err != nil {
		return domain.AuditEventQuery{}, invalidQuery("from")
	}
	to, err := parseOptionalTime(values.Get("to"))
	if err != nil {
		return domain.AuditEventQuery{}, invalidQuery("to")
	}
	return domain.AuditEventQuery{
		Actor:      values.Get("actor"),
		Action:     values.Get("action"),
		TargetKind: values.Get("targetKind"),
		TargetID:   values.Get("targetId"),
		Result:     values.Get("result"),
		From:       from,
		To:         to,
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

func parseOptionalTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
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
