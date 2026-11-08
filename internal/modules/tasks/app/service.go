package app

import (
	"context"
	"strings"

	"virtmgr/internal/domain"
)

type Repository interface {
	ListTasks(ctx context.Context, query domain.TaskQuery) (domain.TaskList, error)
	GetTask(ctx context.Context, id string) (domain.Task, error)
}

type Service struct {
	repository Repository
}

func New(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, input domain.TaskQuery) (domain.TaskList, error) {
	query := input.Normalized()
	if query.Status != "" && !domain.ValidTaskStatus(query.Status) {
		return domain.TaskList{}, domain.ErrInvalidInput
	}
	return s.repository.ListTasks(ctx, query)
}

func (s *Service) Get(ctx context.Context, id string) (domain.Task, error) {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 {
		return domain.Task{}, domain.ErrInvalidInput
	}
	return s.repository.GetTask(ctx, id)
}
