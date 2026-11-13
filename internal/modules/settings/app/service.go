package app

import (
	"context"
	"fmt"
	"time"

	"virtmgr/internal/domain"
)

type Mode interface {
	ReadOnly() bool
	SetReadOnly(ctx context.Context, readOnly bool) error
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type AuditRetention interface {
	Days() int
	SetDays(ctx context.Context, days int) error
}

type Service struct {
	mode      Mode
	audit     AuditRecorder
	retention AuditRetention
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type UpdateCommand struct {
	ReadOnly bool
	Request  MutationContext
}

type AuditRetentionCommand struct {
	Days    int
	Request MutationContext
}

type Option func(*Service)

func WithAuditRetention(retention AuditRetention) Option {
	return func(service *Service) {
		service.retention = retention
	}
}

func New(mode Mode, audit AuditRecorder, options ...Option) *Service {
	service := &Service{mode: mode, audit: audit}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Get() domain.Settings {
	days := 30
	if s.retention != nil {
		days = s.retention.Days()
	}
	return domain.Settings{ReadOnly: s.mode.ReadOnly(), AuditRetentionDays: days}
}

func (s *Service) Update(ctx context.Context, command UpdateCommand) (domain.Settings, error) {
	previous := s.mode.ReadOnly()
	detail := map[string]any{"previousReadOnly": previous, "newReadOnly": command.ReadOnly}
	if command.Request.Session.User.Role != domain.RoleAdmin {
		return domain.Settings{}, s.failure(ctx, command.Request, detail, domain.ErrForbidden)
	}
	if err := s.mode.SetReadOnly(ctx, command.ReadOnly); err != nil {
		return domain.Settings{}, s.failure(ctx, command.Request, detail, err)
	}
	if s.audit != nil {
		if err := s.audit.RecordAudit(ctx, domain.AuditEvent{
			OccurredAt: time.Now().UTC(),
			RequestID:  command.Request.RequestID,
			Actor:      command.Request.Session.User.Username,
			Role:       command.Request.Session.User.Role,
			Action:     "settings.readonly.update",
			TargetKind: "settings",
			TargetID:   "read_only",
			SourceIP:   command.Request.SourceIP,
			Result:     "success",
			Detail:     detail,
		}); err != nil {
			return domain.Settings{}, fmt.Errorf("record settings audit: %w", err)
		}
	}
	return s.Get(), nil
}

func (s *Service) failure(ctx context.Context, request MutationContext, detail map[string]any, cause error) error {
	if s.audit == nil {
		return cause
	}
	detail["error"] = cause.Error()
	if err := s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: time.Now().UTC(),
		RequestID:  request.RequestID,
		Actor:      request.Session.User.Username,
		Role:       request.Session.User.Role,
		Action:     "settings.readonly.update",
		TargetKind: "settings",
		TargetID:   "read_only",
		SourceIP:   request.SourceIP,
		Result:     "failure",
		Detail:     detail,
	}); err != nil {
		return fmt.Errorf("%w: record settings audit: %v", cause, err)
	}
	return cause
}

func (s *Service) UpdateAuditRetention(ctx context.Context, command AuditRetentionCommand) (domain.Settings, error) {
	previous := s.Get().AuditRetentionDays
	detail := map[string]any{"previousDays": previous, "newDays": command.Days}
	if command.Request.Session.User.Role != domain.RoleAdmin {
		return domain.Settings{}, s.recordFailure(ctx, command.Request, "settings.audit_retention.update", "audit_retention", detail, domain.ErrForbidden)
	}
	if command.Days < 1 || command.Days > 3650 || s.retention == nil {
		cause := domain.ErrInvalidInput
		if s.retention == nil {
			cause = domain.ErrUnavailable
		}
		return domain.Settings{}, s.recordFailure(ctx, command.Request, "settings.audit_retention.update", "audit_retention", detail, cause)
	}
	if err := s.retention.SetDays(ctx, command.Days); err != nil {
		return domain.Settings{}, s.recordFailure(ctx, command.Request, "settings.audit_retention.update", "audit_retention", detail, err)
	}
	if s.audit != nil {
		if err := s.audit.RecordAudit(ctx, domain.AuditEvent{
			OccurredAt: time.Now().UTC(), RequestID: command.Request.RequestID,
			Actor: command.Request.Session.User.Username, Role: command.Request.Session.User.Role,
			Action: "settings.audit_retention.update", TargetKind: "settings", TargetID: "audit_retention",
			SourceIP: command.Request.SourceIP, Result: "success", Detail: detail,
		}); err != nil {
			return domain.Settings{}, fmt.Errorf("record audit retention audit: %w", err)
		}
	}
	return s.Get(), nil
}

func (s *Service) recordFailure(ctx context.Context, request MutationContext, action, targetID string, detail map[string]any, cause error) error {
	if s.audit == nil {
		return cause
	}
	detail["error"] = cause.Error()
	if err := s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: time.Now().UTC(), RequestID: request.RequestID,
		Actor: request.Session.User.Username, Role: request.Session.User.Role,
		Action: action, TargetKind: "settings", TargetID: targetID,
		SourceIP: request.SourceIP, Result: "failure", Detail: detail,
	}); err != nil {
		return fmt.Errorf("%w: record settings audit: %v", cause, err)
	}
	return cause
}
