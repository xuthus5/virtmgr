package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"virtmgr/internal/domain"
	authapp "virtmgr/internal/modules/auth/app"
)

const (
	minPasswordLength = 12
	maxPasswordLength = 256
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{2,63}$`)

type Repository interface {
	ListUsers(ctx context.Context) ([]domain.User, error)
	UserByID(ctx context.Context, id int64) (domain.User, error)
	UserByUsername(ctx context.Context, username string) (domain.User, error)
	CreateUser(ctx context.Context, username, passwordHash string, role domain.Role) (domain.User, error)
	UpdateUser(ctx context.Context, id int64, role domain.Role, disabled bool) (domain.User, error)
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type Service struct {
	repository   Repository
	audit        AuditRecorder
	readOnly     bool
	readOnlyMode domain.ReadOnlyProvider
}

type Option func(*Service)

func WithReadOnlyMode(mode domain.ReadOnlyProvider) Option {
	return func(service *Service) {
		service.readOnlyMode = mode
	}
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type CreateUserCommand struct {
	Username string
	Password string
	Role     domain.Role
	Request  MutationContext
}

type UpdateUserCommand struct {
	ID       int64
	Role     domain.Role
	Disabled bool
	Request  MutationContext
}

func New(repository Repository, audit AuditRecorder, readOnly bool, options ...Option) *Service {
	service := &Service{repository: repository, audit: audit, readOnly: readOnly}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) isReadOnly() bool {
	if s.readOnlyMode != nil {
		return s.readOnlyMode.ReadOnly()
	}
	return s.readOnly
}

func (s *Service) List(ctx context.Context, session domain.Session) (domain.UserList, error) {
	if session.User.Role != domain.RoleAdmin {
		return domain.UserList{}, domain.ErrForbidden
	}
	users, err := s.repository.ListUsers(ctx)
	if err != nil {
		return domain.UserList{}, err
	}
	return domain.UserList{Items: users, Total: len(users)}, nil
}

func (s *Service) Create(ctx context.Context, command CreateUserCommand) (domain.User, error) {
	if command.Request.Session.User.Role != domain.RoleAdmin {
		return domain.User{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.User{}, domain.ErrReadOnly
	}
	command.Username = strings.TrimSpace(command.Username)
	if !usernamePattern.MatchString(command.Username) || !domain.ValidRole(command.Role) {
		return domain.User{}, domain.ErrInvalidInput
	}
	passwordLength := utf8.RuneCountInString(command.Password)
	if passwordLength < minPasswordLength || passwordLength > maxPasswordLength {
		return domain.User{}, domain.ErrInvalidInput
	}
	if _, err := s.repository.UserByUsername(ctx, command.Username); err == nil {
		return domain.User{}, domain.ErrConflict
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, err
	}
	passwordHash, err := authapp.HashPassword(command.Password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash user password: %w", err)
	}
	user, err := s.repository.CreateUser(ctx, command.Username, passwordHash, command.Role)
	if err != nil {
		return domain.User{}, err
	}
	detail := map[string]any{"username": user.Username, "role": user.Role, "disabled": user.Disabled}
	if err := s.recordAudit(ctx, command.Request, "user.create", user.ID, "success", detail, nil); err != nil {
		return domain.User{}, fmt.Errorf("record user creation audit: %w", err)
	}
	user.PasswordHash = ""
	return user, nil
}

func (s *Service) Update(ctx context.Context, command UpdateUserCommand) (domain.User, error) {
	if command.Request.Session.User.Role != domain.RoleAdmin {
		return domain.User{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.User{}, domain.ErrReadOnly
	}
	if command.ID < 1 || !domain.ValidRole(command.Role) {
		return domain.User{}, domain.ErrInvalidInput
	}
	target, err := s.repository.UserByID(ctx, command.ID)
	if err != nil {
		return domain.User{}, err
	}
	if target.ID == command.Request.Session.User.ID {
		return domain.User{}, domain.ErrSelfModification
	}
	user, err := s.repository.UpdateUser(ctx, command.ID, command.Role, command.Disabled)
	if err != nil {
		return domain.User{}, err
	}
	detail := map[string]any{
		"username": user.Username, "role": user.Role, "disabled": user.Disabled,
		"previousRole": target.Role, "previousDisabled": target.Disabled,
	}
	if err := s.recordAudit(ctx, command.Request, "user.update", user.ID, "success", detail, nil); err != nil {
		return domain.User{}, fmt.Errorf("record user update audit: %w", err)
	}
	user.PasswordHash = ""
	return user, nil
}

func (s *Service) recordAudit(ctx context.Context, request MutationContext, action string, targetID int64, result string, detail map[string]any, cause error) error {
	if s.audit == nil {
		return nil
	}
	if cause != nil {
		detail["error"] = cause.Error()
	}
	return s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: time.Now().UTC(),
		RequestID:  request.RequestID,
		Actor:      request.Session.User.Username,
		Role:       request.Session.User.Role,
		Action:     action,
		TargetKind: "user",
		TargetID:   strconv.FormatInt(targetID, 10),
		SourceIP:   request.SourceIP,
		Result:     result,
		Detail:     detail,
	})
}
