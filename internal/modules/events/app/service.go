package app

import "virtmgr/internal/domain"

const (
	DefaultLimit = 50
	MaxLimit     = 200
)

type Repository interface {
	Recent(limit int) []domain.SystemEvent
	Subscribe(limit int) ([]domain.SystemEvent, <-chan domain.SystemEvent, func())
}

type Service struct {
	repository Repository
}

func New(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(limit int) domain.SystemEventList {
	limit = normalizeLimit(limit)
	events := s.repository.Recent(limit)
	return domain.SystemEventList{Items: events, Total: len(events)}
}

func (s *Service) Stream(limit int) ([]domain.SystemEvent, <-chan domain.SystemEvent, func()) {
	return s.repository.Subscribe(normalizeLimit(limit))
}

func normalizeLimit(limit int) int {
	if limit < 1 {
		return DefaultLimit
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}
