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
	CancelTask(ctx context.Context, id string, cancelledAt time.Time, step string) (domain.Task, error)
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type CancellationRegistry interface {
	MarkCancelled(id string) bool
}

type Service struct {
	repository   Repository
	retryer      Retryer
	registry     CancellationRegistry
	audit        AuditRecorder
	readOnly     bool
	readOnlyMode domain.ReadOnlyProvider
}

type Retryer interface {
	RetryTask(ctx context.Context, task domain.Task, session domain.Session, requestID, sourceIP string) (domain.Task, error)
}

type Option func(*Service)

func WithRetryer(retryer Retryer) Option {
	return func(service *Service) { service.retryer = retryer }
}

func WithCancellationRegistry(registry CancellationRegistry) Option {
	return func(service *Service) { service.registry = registry }
}

func WithAuditRecorder(audit AuditRecorder) Option {
	return func(service *Service) { service.audit = audit }
}

func WithReadOnlyMode(mode domain.ReadOnlyProvider) Option {
	return func(service *Service) { service.readOnlyMode = mode }
}

func (s *Service) isReadOnly() bool {
	if s.readOnlyMode != nil {
		return s.readOnlyMode.ReadOnly()
	}
	return s.readOnly
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
	result, err := s.repository.ListTasks(ctx, domain.TaskQuery{PageSize: domain.MaxTaskPageSize}.Normalized())
	if err != nil {
		return domain.TaskList{}, err
	}
	markTaskListCancellable(&result)
	return result, nil
}

func (s *Service) StreamInterval() time.Duration {
	return 2 * time.Second
}

func (s *Service) List(ctx context.Context, input domain.TaskQuery) (domain.TaskList, error) {
	query := input.Normalized()
	if query.Status != "" && !domain.ValidTaskStatus(query.Status) {
		return domain.TaskList{}, domain.ErrInvalidInput
	}
	result, err := s.repository.ListTasks(ctx, query)
	if err != nil {
		return domain.TaskList{}, err
	}
	markTaskListCancellable(&result)
	return result, nil
}

func (s *Service) Get(ctx context.Context, id string) (domain.Task, error) {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 {
		return domain.Task{}, domain.ErrInvalidInput
	}
	task, err := s.repository.GetTask(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	task.Cancellable = domain.TaskCancellable(task)
	return task, nil
}

func (s *Service) Cancel(ctx context.Context, id string, session domain.Session, requestID, sourceIP string) (domain.Task, error) {
	if session.User.Role != domain.RoleOperator && session.User.Role != domain.RoleAdmin {
		return domain.Task{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.Task{}, domain.ErrReadOnly
	}
	task, err := s.Get(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	if !domain.TaskCancellable(task) || s.registry == nil {
		return domain.Task{}, domain.ErrConflict
	}
	if !s.registry.MarkCancelled(task.ID) {
		return domain.Task{}, domain.ErrConflict
	}
	task, err = s.repository.CancelTask(ctx, task.ID, time.Now().UTC(), "任务已取消")
	if err != nil {
		s.recordCancelAudit(ctx, session, requestID, sourceIP, task, "failure", err)
		return domain.Task{}, err
	}
	task.Cancellable = false
	s.recordCancelAudit(ctx, session, requestID, sourceIP, task, "success", nil)
	return task, nil
}

func (s *Service) recordCancelAudit(ctx context.Context, session domain.Session, requestID, sourceIP string, task domain.Task, result string, cause error) {
	if s.audit == nil {
		return
	}
	detail := map[string]any{"kind": task.Kind, "targetKind": task.TargetKind, "targetId": task.TargetID}
	if cause != nil {
		detail["error"] = cause.Error()
	}
	_ = s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: time.Now().UTC(), RequestID: requestID, Actor: session.User.Username, Role: session.User.Role,
		Action: "task.cancel", TargetKind: "task", TargetID: task.ID, SourceIP: sourceIP, Result: result, Detail: detail,
	})
}

func markTaskListCancellable(list *domain.TaskList) {
	for index := range list.Items {
		list.Items[index].Cancellable = domain.TaskCancellable(list.Items[index])
	}
}
