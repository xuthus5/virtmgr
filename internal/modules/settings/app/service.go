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

type Service struct {
	mode  Mode
	audit AuditRecorder
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

func New(mode Mode, audit AuditRecorder) *Service {
	return &Service{mode: mode, audit: audit}
}

func (s *Service) Get() domain.Settings {
	return domain.Settings{ReadOnly: s.mode.ReadOnly()}
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
