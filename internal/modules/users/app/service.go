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
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	ListSessionsByUser(ctx context.Context, userID int64) ([]domain.UserSession, error)
	DeleteSessionForUser(ctx context.Context, userID int64, sessionID string) error
	DeleteOtherSessionsByUser(ctx context.Context, userID int64, keepTokenHash string) error
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

type ChangePasswordCommand struct {
	CurrentPassword string
	NewPassword     string
	Request         MutationContext
}

type ResetPasswordCommand struct {
	ID       int64
	Password string
	Request  MutationContext
}

type RevokeSessionCommand struct {
	ID        int64
	SessionID string
	Request   MutationContext
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
	if !validPassword(command.Password) {
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

func (s *Service) ChangePassword(ctx context.Context, command ChangePasswordCommand) error {
	if command.Request.Session.User.ID < 1 || command.CurrentPassword == "" || !validPassword(command.NewPassword) || command.CurrentPassword == command.NewPassword {
		return domain.ErrInvalidInput
	}
	if s.isReadOnly() {
		return domain.ErrReadOnly
	}
	target, err := s.repository.UserByID(ctx, command.Request.Session.User.ID)
	if err != nil {
		return err
	}
	if !authapp.VerifyPassword(target.PasswordHash, command.CurrentPassword) {
		return domain.ErrCurrentPassword
	}
	passwordHash, err := authapp.HashPassword(command.NewPassword)
	if err != nil {
		return fmt.Errorf("hash user password: %w", err)
	}
	if err := s.repository.UpdatePassword(ctx, target.ID, passwordHash); err != nil {
		return err
	}
	if err := s.repository.DeleteOtherSessionsByUser(ctx, target.ID, command.Request.Session.TokenHash); err != nil {
		return err
	}
	detail := map[string]any{"username": target.Username}
	if err := s.recordAudit(ctx, command.Request, "user.password.change", target.ID, "success", detail, nil); err != nil {
		return fmt.Errorf("record user password change audit: %w", err)
	}
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, command ResetPasswordCommand) error {
	if command.Request.Session.User.Role != domain.RoleAdmin {
		return domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.ErrReadOnly
	}
	if command.ID < 1 || !validPassword(command.Password) {
		return domain.ErrInvalidInput
	}
	target, err := s.repository.UserByID(ctx, command.ID)
	if err != nil {
		return err
	}
	if target.ID == command.Request.Session.User.ID {
		return domain.ErrSelfModification
	}
	passwordHash, err := authapp.HashPassword(command.Password)
	if err != nil {
		return fmt.Errorf("hash user password: %w", err)
	}
	if err := s.repository.UpdatePassword(ctx, target.ID, passwordHash); err != nil {
		return err
	}
	if err := s.repository.DeleteOtherSessionsByUser(ctx, target.ID, ""); err != nil {
		return err
	}
	detail := map[string]any{"username": target.Username, "sessionsRevoked": true}
	if err := s.recordAudit(ctx, command.Request, "user.password.reset", target.ID, "success", detail, nil); err != nil {
		return fmt.Errorf("record user password reset audit: %w", err)
	}
	return nil
}

func (s *Service) ListSessions(ctx context.Context, id int64, session domain.Session) (domain.UserSessionList, error) {
	if session.User.Role != domain.RoleAdmin {
		return domain.UserSessionList{}, domain.ErrForbidden
	}
	if id < 1 {
		return domain.UserSessionList{}, domain.ErrInvalidInput
	}
	if _, err := s.repository.UserByID(ctx, id); err != nil {
		return domain.UserSessionList{}, err
	}
	sessions, err := s.repository.ListSessionsByUser(ctx, id)
	if err != nil {
		return domain.UserSessionList{}, err
	}
	for index := range sessions {
		sessions[index].Current = session.TokenHash != "" && sessions[index].TokenHash == session.TokenHash
		sessions[index].TokenHash = ""
	}
	return domain.UserSessionList{Items: sessions, Total: len(sessions)}, nil
}

func (s *Service) RevokeSession(ctx context.Context, command RevokeSessionCommand) error {
	if command.Request.Session.User.Role != domain.RoleAdmin {
		return domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.ErrReadOnly
	}
	command.SessionID = strings.TrimSpace(command.SessionID)
	if command.ID < 1 || command.SessionID == "" {
		return domain.ErrInvalidInput
	}
	target, err := s.repository.UserByID(ctx, command.ID)
	if err != nil {
		return err
	}
	if err := s.repository.DeleteSessionForUser(ctx, target.ID, command.SessionID); err != nil {
		return err
	}
	detail := map[string]any{"username": target.Username, "sessionId": command.SessionID}
	if err := s.recordAudit(ctx, command.Request, "user.session.revoke", target.ID, "success", detail, nil); err != nil {
		return fmt.Errorf("record user session revoke audit: %w", err)
	}
	return nil
}

func validPassword(password string) bool {
	length := utf8.RuneCountInString(password)
	return length >= minPasswordLength && length <= maxPasswordLength && !strings.ContainsRune(password, '\x00')
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
