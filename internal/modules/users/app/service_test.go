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
	next      int64
	createErr error
	updateErr error
}

func newMemoryRepository(users ...domain.User) *memoryRepository {
	repository := &memoryRepository{users: map[int64]domain.User{}}
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
