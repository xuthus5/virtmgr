package app

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
	authapp "virtmgr/internal/modules/auth/app"
)

type memoryRepository struct {
	users     map[int64]domain.User
	sessions  map[int64][]domain.UserSession
	next      int64
	createErr error
	updateErr error
}

func newMemoryRepository(users ...domain.User) *memoryRepository {
	repository := &memoryRepository{users: map[int64]domain.User{}, sessions: map[int64][]domain.UserSession{}}
	for _, user := range users {
		repository.users[user.ID] = user
		if user.ID > repository.next {
			repository.next = user.ID
		}
	}
	return repository
}

func (m *memoryRepository) ListUsers(context.Context) ([]domain.User, error) {
	users := make([]domain.User, 0, len(m.users))
	for _, user := range m.users {
		users = append(users, user)
	}
	return users, nil
}

func (m *memoryRepository) UserByID(_ context.Context, id int64) (domain.User, error) {
	user, ok := m.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}

func (m *memoryRepository) UserByUsername(_ context.Context, username string) (domain.User, error) {
	for _, user := range m.users {
		if user.Username == username {
			return user, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

func (m *memoryRepository) CreateUser(_ context.Context, username, passwordHash string, role domain.Role) (domain.User, error) {
	if m.createErr != nil {
		return domain.User{}, m.createErr
	}
	m.next++
	user := domain.User{ID: m.next, Username: username, PasswordHash: passwordHash, Role: role}
	m.users[user.ID] = user
	return user, nil
}

func (m *memoryRepository) UpdateUser(_ context.Context, id int64, role domain.Role, disabled bool) (domain.User, error) {
	if m.updateErr != nil {
		return domain.User{}, m.updateErr
	}
	user, ok := m.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	user.Role = role
	user.Disabled = disabled
	m.users[id] = user
	return user, nil
}

func (m *memoryRepository) UpdatePassword(_ context.Context, id int64, passwordHash string) error {
	user, ok := m.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	user.PasswordHash = passwordHash
	m.users[id] = user
	return nil
}

func (m *memoryRepository) ListSessionsByUser(_ context.Context, userID int64) ([]domain.UserSession, error) {
	return append([]domain.UserSession(nil), m.sessions[userID]...), nil
}

func (m *memoryRepository) DeleteSessionForUser(_ context.Context, userID int64, sessionID string) error {
	sessions := m.sessions[userID]
	for index, session := range sessions {
		if session.ID == sessionID {
			m.sessions[userID] = append(sessions[:index], sessions[index+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *memoryRepository) DeleteOtherSessionsByUser(_ context.Context, userID int64, keepTokenHash string) error {
	sessions := m.sessions[userID]
	kept := make([]domain.UserSession, 0, len(sessions))
	for _, session := range sessions {
		if keepTokenHash != "" && session.TokenHash == keepTokenHash {
			kept = append(kept, session)
		}
	}
	m.sessions[userID] = kept
	return nil
}

type auditRecorder struct {
	events []domain.AuditEvent
	err    error
}

func (a *auditRecorder) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	a.events = append(a.events, event)
	return a.err
}

func adminRequest() MutationContext {
	return MutationContext{
		Session:   domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}},
		RequestID: "req-1",
		SourceIP:  "127.0.0.1",
	}
}

type dynamicReadOnly struct{ value bool }

func (m *dynamicReadOnly) ReadOnly() bool { return m.value }

func TestServiceUsesDynamicReadOnlyMode(t *testing.T) {
	mode := &dynamicReadOnly{value: true}
	service := New(newMemoryRepository(), nil, false, WithReadOnlyMode(mode))
	command := CreateUserCommand{Username: "runtime-user", Password: "runtime-secret-123", Role: domain.RoleViewer, Request: adminRequest()}
	if _, err := service.Create(context.Background(), command); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only error = %v", err)
	}
	mode.value = false
	if _, err := service.Create(context.Background(), command); err != nil {
		t.Fatalf("writable error = %v", err)
	}
}

func TestListUsersRequiresAdmin(t *testing.T) {
	repository := newMemoryRepository(domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin})
	service := New(repository, &auditRecorder{}, false)
	result, err := service.List(context.Background(), domain.Session{User: domain.User{ID: 1, Role: domain.RoleAdmin}})
	if err != nil || result.Total != 1 {
		t.Fatalf("List() = %+v, %v", result, err)
	}
	if _, err := service.List(context.Background(), domain.Session{User: domain.User{ID: 2, Role: domain.RoleOperator}}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("operator List() error = %v", err)
	}
}

func TestCreateUserHashesPasswordAndAudits(t *testing.T) {
	repository := newMemoryRepository(domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin})
	audit := &auditRecorder{}
	service := New(repository, audit, false)
	user, err := service.Create(context.Background(), CreateUserCommand{
		Username: " operator.one ",
		Password: "operator-secret-123",
		Role:     domain.RoleOperator,
		Request:  adminRequest(),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if user.ID != 2 || user.Username != "operator.one" || user.Role != domain.RoleOperator || user.PasswordHash != "" {
		t.Fatalf("created user = %+v", user)
	}
	stored := repository.users[user.ID]
	if !authapp.VerifyPassword(stored.PasswordHash, "operator-secret-123") {
		t.Fatal("stored password hash does not verify")
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	event := audit.events[0]
	if event.Action != "user.create" || event.TargetKind != "user" || event.TargetID != "2" || event.Actor != "admin" {
		t.Fatalf("audit event = %+v", event)
	}
	if event.Detail["username"] != "operator.one" || event.Detail["role"] != domain.RoleOperator {
		t.Fatalf("audit detail = %+v", event.Detail)
	}
}

func TestCreateUserValidation(t *testing.T) {
	tests := []struct {
		name    string
		command CreateUserCommand
		want    error
	}{
		{name: "short password", command: CreateUserCommand{Username: "viewer", Password: "short", Role: domain.RoleViewer}, want: domain.ErrInvalidInput},
		{name: "invalid role", command: CreateUserCommand{Username: "viewer", Password: "viewer-secret-123", Role: domain.Role("root")}, want: domain.ErrInvalidInput},
		{name: "invalid username", command: CreateUserCommand{Username: "a", Password: "viewer-secret-123", Role: domain.RoleViewer}, want: domain.ErrInvalidInput},
		{name: "duplicate username", command: CreateUserCommand{Username: "admin", Password: "viewer-secret-123", Role: domain.RoleViewer}, want: domain.ErrConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newMemoryRepository(domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin})
			audit := &auditRecorder{}
			test.command.Request = adminRequest()
			_, err := New(repository, audit, false).Create(context.Background(), test.command)
			if !errors.Is(err, test.want) {
				t.Fatalf("Create() error = %v, want %v", err, test.want)
			}
			if len(audit.events) != 0 {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestCreateUserRequiresAdmin(t *testing.T) {
	request := adminRequest()
	request.Session.User.Role = domain.RoleOperator
	_, err := New(newMemoryRepository(), &auditRecorder{}, false).Create(context.Background(), CreateUserCommand{
		Username: "viewer", Password: "viewer-secret-123", Role: domain.RoleViewer, Request: request,
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestUserMutationsRejectReadOnly(t *testing.T) {
	repository := newMemoryRepository(
		domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin},
		domain.User{ID: 2, Username: "viewer", Role: domain.RoleViewer},
	)
	service := New(repository, &auditRecorder{}, true)
	_, err := service.Create(context.Background(), CreateUserCommand{
		Username: "operator", Password: "operator-secret-123", Role: domain.RoleOperator, Request: adminRequest(),
	})
	if !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = service.Update(context.Background(), UpdateUserCommand{
		ID: 2, Role: domain.RoleOperator, Request: adminRequest(),
	})
	if !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestUpdateUserAndAudit(t *testing.T) {
	repository := newMemoryRepository(
		domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin},
		domain.User{ID: 2, Username: "viewer", Role: domain.RoleViewer},
	)
	audit := &auditRecorder{}
	user, err := New(repository, audit, false).Update(context.Background(), UpdateUserCommand{
		ID: 2, Role: domain.RoleOperator, Disabled: true, Request: adminRequest(),
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if user.Role != domain.RoleOperator || !user.Disabled || user.PasswordHash != "" {
		t.Fatalf("updated user = %+v", user)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "user.update" {
		t.Fatalf("audit events = %+v", audit.events)
	}
	if audit.events[0].Detail["previousRole"] != domain.RoleViewer || audit.events[0].Detail["disabled"] != true {
		t.Fatalf("audit detail = %+v", audit.events[0].Detail)
	}
}

func TestUpdateUserRejectsSelfModification(t *testing.T) {
	repository := newMemoryRepository(domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin})
	_, err := New(repository, &auditRecorder{}, false).Update(context.Background(), UpdateUserCommand{
		ID: 1, Role: domain.RoleViewer, Disabled: true, Request: adminRequest(),
	})
	if !errors.Is(err, domain.ErrSelfModification) {
		t.Fatalf("Update() error = %v", err)
	}
	if repository.users[1].Role != domain.RoleAdmin || repository.users[1].Disabled {
		t.Fatalf("self user changed = %+v", repository.users[1])
	}
}

func TestUpdateUserErrors(t *testing.T) {
	repository := newMemoryRepository(domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin})
	_, err := New(repository, &auditRecorder{}, false).Update(context.Background(), UpdateUserCommand{
		ID: 99, Role: domain.RoleViewer, Request: adminRequest(),
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing user error = %v", err)
	}
	_, err = New(repository, &auditRecorder{}, false).Update(context.Background(), UpdateUserCommand{
		ID: 1, Role: domain.Role("root"), Request: adminRequest(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid role error = %v", err)
	}
}

func TestChangePasswordRehashesAndKeepsCurrentSession(t *testing.T) {
	currentHash, err := authapp.HashPassword("current-secret-123")
	if err != nil {
		t.Fatal(err)
	}
	repository := newMemoryRepository(domain.User{ID: 2, Username: "viewer", Role: domain.RoleViewer, PasswordHash: currentHash})
	repository.sessions[2] = []domain.UserSession{
		{ID: "current", TokenHash: "keep-me"},
		{ID: "other", TokenHash: "revoke-me"},
	}
	audit := &auditRecorder{}
	request := MutationContext{Session: domain.Session{TokenHash: "keep-me", User: domain.User{ID: 2, Username: "viewer", Role: domain.RoleViewer}}}
	if err := New(repository, audit, false).ChangePassword(context.Background(), ChangePasswordCommand{
		CurrentPassword: "current-secret-123", NewPassword: "new-secret-456", Request: request,
	}); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	if !authapp.VerifyPassword(repository.users[2].PasswordHash, "new-secret-456") {
		t.Fatal("new password was not persisted")
	}
	if len(repository.sessions[2]) != 1 || repository.sessions[2][0].ID != "current" {
		t.Fatalf("sessions = %+v", repository.sessions[2])
	}
	if len(audit.events) != 1 || audit.events[0].Action != "user.password.change" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestChangePasswordRejectsWrongCurrentPassword(t *testing.T) {
	currentHash, err := authapp.HashPassword("current-secret-123")
	if err != nil {
		t.Fatal(err)
	}
	repository := newMemoryRepository(domain.User{ID: 2, Username: "viewer", PasswordHash: currentHash})
	request := MutationContext{Session: domain.Session{User: domain.User{ID: 2, Username: "viewer"}}}
	if err := New(repository, &auditRecorder{}, false).ChangePassword(context.Background(), ChangePasswordCommand{
		CurrentPassword: "wrong-password", NewPassword: "new-secret-456", Request: request,
	}); !errors.Is(err, domain.ErrCurrentPassword) {
		t.Fatalf("ChangePassword() error = %v", err)
	}
}

func TestResetPasswordRevokesAllSessionsAndAudits(t *testing.T) {
	repository := newMemoryRepository(
		domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin},
		domain.User{ID: 2, Username: "viewer", Role: domain.RoleViewer},
	)
	repository.sessions[2] = []domain.UserSession{{ID: "one", TokenHash: "a"}, {ID: "two", TokenHash: "b"}}
	audit := &auditRecorder{}
	if err := New(repository, audit, false).ResetPassword(context.Background(), ResetPasswordCommand{
		ID: 2, Password: "new-secret-456", Request: adminRequest(),
	}); err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	if !authapp.VerifyPassword(repository.users[2].PasswordHash, "new-secret-456") || len(repository.sessions[2]) != 0 {
		t.Fatalf("user=%+v sessions=%+v", repository.users[2], repository.sessions[2])
	}
	if len(audit.events) != 1 || audit.events[0].Action != "user.password.reset" || audit.events[0].Detail["sessionsRevoked"] != true {
		t.Fatalf("audit events = %+v", audit.events)
	}
	if err := New(repository, audit, false).ResetPassword(context.Background(), ResetPasswordCommand{
		ID: 1, Password: "new-secret-456", Request: adminRequest(),
	}); !errors.Is(err, domain.ErrSelfModification) {
		t.Fatalf("self reset error = %v", err)
	}
}

func TestListAndRevokeSessions(t *testing.T) {
	repository := newMemoryRepository(domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin})
	repository.sessions[1] = []domain.UserSession{{ID: "current", TokenHash: "token-current"}, {ID: "other", TokenHash: "token-other"}}
	service := New(repository, &auditRecorder{}, false)
	session := domain.Session{TokenHash: "token-current", User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}
	result, err := service.ListSessions(context.Background(), 1, session)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if result.Total != 2 || !result.Items[0].Current || result.Items[0].TokenHash != "" {
		t.Fatalf("sessions = %+v", result)
	}
	audit := &auditRecorder{}
	service = New(repository, audit, false)
	if err := service.RevokeSession(context.Background(), RevokeSessionCommand{
		ID: 1, SessionID: "other", Request: MutationContext{Session: session},
	}); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}
	if len(repository.sessions[1]) != 1 || repository.sessions[1][0].ID != "current" {
		t.Fatalf("sessions = %+v", repository.sessions[1])
	}
	if len(audit.events) != 1 || audit.events[0].Action != "user.session.revoke" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestUserAuditFailurePropagates(t *testing.T) {
	repository := newMemoryRepository(domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin})
	want := errors.New("audit unavailable")
	audit := &auditRecorder{err: want}
	_, err := New(repository, audit, false).Create(context.Background(), CreateUserCommand{
		Username: "viewer", Password: "viewer-secret-123", Role: domain.RoleViewer, Request: adminRequest(),
	})
	if !errors.Is(err, want) {
		t.Fatalf("Create() error = %v", err)
	}
}
