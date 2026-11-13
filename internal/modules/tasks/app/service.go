package app

import (
	"context"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

type Repository interface {
	ListTasks(ctx context.Context, query domain.TaskQuery) (domain.TaskList, error)
	GetTask(ctx context.Context, id string) (domain.Task, error)
}

type Service struct {
	repository Repository
	retryer    Retryer
}

type Retryer interface {
	RetryTask(ctx context.Context, task domain.Task, session domain.Session, requestID, sourceIP string) (domain.Task, error)
}

type Option func(*Service)

func WithRetryer(retryer Retryer) Option {
	return func(service *Service) { service.retryer = retryer }
}

func New(repository Repository, options ...Option) *Service {
	service := &Service{repository: repository}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Retry(ctx context.Context, id string, session domain.Session, requestID, sourceIP string) (domain.Task, error) {
	if s.retryer == nil {
		return domain.Task{}, domain.ErrUnavailable
	}
	task, err := s.Get(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	if task.Status != domain.TaskStatusFailed && task.Status != domain.TaskStatusInterrupted {
		return domain.Task{}, domain.ErrConflict
	}
	return s.retryer.RetryTask(ctx, task, session, requestID, sourceIP)
}

func (s *Service) StreamSnapshot(ctx context.Context) (domain.TaskList, error) {
	return s.repository.ListTasks(ctx, domain.TaskQuery{PageSize: domain.MaxTaskPageSize}.Normalized())
}

func (s *Service) StreamInterval() time.Duration {
	return 2 * time.Second
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
