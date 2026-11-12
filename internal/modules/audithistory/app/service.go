package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"virtmgr/internal/domain"
)

const maxAuditFilterLength = 128

type Repository interface {
	ListAuditEvents(ctx context.Context, query domain.AuditEventQuery) (domain.AuditEventList, error)
	ExportAuditEvents(ctx context.Context, query domain.AuditEventQuery) ([]domain.AuditEvent, error)
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type Service struct {
	repository Repository
	audit      AuditRecorder
}

type Option func(*Service)

func WithAuditRecorder(recorder AuditRecorder) Option {
	return func(service *Service) {
		service.audit = recorder
	}
}

func New(repository Repository, options ...Option) *Service {
	service := &Service{repository: repository}
	for _, option := range options {
		option(service)
	}
	return service
}

type ExportCommand struct {
	Session   domain.Session
	Query     domain.AuditEventQuery
	Format    domain.AuditExportFormat
	RequestID string
	SourceIP  string
}

func (s *Service) List(ctx context.Context, session domain.Session, input domain.AuditEventQuery) (domain.AuditEventList, error) {
	if session.User.Role != domain.RoleAdmin {
		return domain.AuditEventList{}, domain.ErrForbidden
	}
	query, err := validatedAuditQuery(input)
	if err != nil {
		return domain.AuditEventList{}, err
	}
	return s.repository.ListAuditEvents(ctx, query)
}

func (s *Service) Export(ctx context.Context, command ExportCommand) (domain.AuditEventExport, error) {
	if command.Session.User.Role != domain.RoleAdmin {
		return domain.AuditEventExport{}, domain.ErrForbidden
	}
	format, ok := domain.ValidAuditExportFormat(string(command.Format))
	if !ok {
		return domain.AuditEventExport{}, domain.ErrInvalidInput
	}
	query, err := validatedAuditQuery(command.Query)
	if err != nil {
		return domain.AuditEventExport{}, err
	}
	events, err := s.repository.ExportAuditEvents(ctx, query)
	if err != nil {
		return domain.AuditEventExport{}, err
	}
	if len(events) > domain.MaxAuditExportEvents {
		return domain.AuditEventExport{}, domain.ErrTooLarge
	}
	result := domain.AuditEventExport{Format: format, EventCount: len(events)}
	if format == domain.AuditExportFormatCSV {
		result.ContentType = "text/csv; charset=utf-8"
		result.Data, err = encodeAuditCSV(events)
	} else {
		result.ContentType = "application/json; charset=utf-8"
		result.Data, err = json.MarshalIndent(events, "", "  ")
	}
	if err != nil {
		return domain.AuditEventExport{}, fmt.Errorf("encode audit export: %w", err)
	}
	if s.audit != nil {
		detail := map[string]any{
			"format": string(format), "eventCount": len(events),
			"actor": query.Actor, "action": query.Action, "targetKind": query.TargetKind,
			"targetId": query.TargetID, "result": query.Result,
		}
		if !query.From.IsZero() {
			detail["from"] = query.From.UTC().Format(time.RFC3339)
		}
		if !query.To.IsZero() {
			detail["to"] = query.To.UTC().Format(time.RFC3339)
		}
		if err := s.audit.RecordAudit(ctx, domain.AuditEvent{
			OccurredAt: time.Now().UTC(), RequestID: command.RequestID,
			Actor: command.Session.User.Username, Role: command.Session.User.Role,
			Action: "audit.export", TargetKind: "audit", TargetID: "audit-events",
			SourceIP: command.SourceIP, Result: "success", Detail: detail,
		}); err != nil {
			return domain.AuditEventExport{}, fmt.Errorf("record audit export audit: %w", err)
		}
	}
	return result, nil
}

func validatedAuditQuery(input domain.AuditEventQuery) (domain.AuditEventQuery, error) {
	query := input.Normalized()
	if query.Result != "" && query.Result != "success" && query.Result != "failure" {
		return domain.AuditEventQuery{}, domain.ErrInvalidInput
	}
	if len(query.Actor) > maxAuditFilterLength || len(query.Action) > maxAuditFilterLength ||
		len(query.TargetKind) > maxAuditFilterLength || len(query.TargetID) > maxAuditFilterLength ||
		len(query.Result) > maxAuditFilterLength {
		return domain.AuditEventQuery{}, domain.ErrInvalidInput
	}
	if !query.From.IsZero() && !query.To.IsZero() && query.From.After(query.To) {
		return domain.AuditEventQuery{}, domain.ErrInvalidInput
	}
	return query, nil
}

func encodeAuditCSV(events []domain.AuditEvent) ([]byte, error) {
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	if err := writer.Write([]string{"id", "occurredAt", "requestId", "actor", "role", "action", "targetKind", "targetId", "sourceIp", "result", "detail"}); err != nil {
		return nil, err
	}
	for _, event := range events {
		detail, err := json.Marshal(event.Detail)
		if err != nil {
			return nil, err
		}
		record := []string{
			strconv.FormatInt(event.ID, 10), event.OccurredAt.UTC().Format(time.RFC3339Nano),
			event.RequestID, event.Actor, string(event.Role), event.Action,
			event.TargetKind, event.TargetID, event.SourceIP, event.Result, string(detail),
		}
		for index := range record {
			record[index] = csvSafeCell(record[index])
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return []byte(builder.String()), nil
}

func csvSafeCell(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}
