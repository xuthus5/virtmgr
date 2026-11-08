package app

import (
	"context"

	"virtmgr/internal/domain"
)

const maxAuditFilterLength = 128

type Repository interface {
	ListAuditEvents(ctx context.Context, query domain.AuditEventQuery) (domain.AuditEventList, error)
}

type Service struct {
	repository Repository
}

func New(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, session domain.Session, input domain.AuditEventQuery) (domain.AuditEventList, error) {
	if session.User.Role != domain.RoleAdmin {
		return domain.AuditEventList{}, domain.ErrForbidden
	}
	query := input.Normalized()
	if query.Result != "" && query.Result != "success" && query.Result != "failure" {
		return domain.AuditEventList{}, domain.ErrInvalidInput
	}
	if len(query.Actor) > maxAuditFilterLength || len(query.Action) > maxAuditFilterLength ||
		len(query.TargetKind) > maxAuditFilterLength || len(query.Result) > maxAuditFilterLength {
		return domain.AuditEventList{}, domain.ErrInvalidInput
	}
	return s.repository.ListAuditEvents(ctx, query)
}
