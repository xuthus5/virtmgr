package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type memoryUserRepository struct {
	users map[string]domain.User
	next  int64
}

func (m *memoryUserRepository) CountUsers(context.Context) (int, error) { return len(m.users), nil }
func (m *memoryUserRepository) CreateUser(_ context.Context, username, passwordHash string, role domain.Role) (domain.User, error) {
	m.next++
	user := domain.User{ID: m.next, Username: username, PasswordHash: passwordHash, Role: role}
	m.users[username] = user
	return user, nil
}
func (m *memoryUserRepository) UserByUsername(_ context.Context, username string) (domain.User, error) {
	user, ok := m.users[username]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}

type memorySessionRepository struct {
	sessions map[string]domain.Session
	users    *memoryUserRepository
}

func (m *memorySessionRepository) CreateSession(_ context.Context, tokenHash, csrfToken string, userID int64, expiresAt time.Time) error {
	var user domain.User
	for _, candidate := range m.users.users {
		if candidate.ID == userID {
			user = candidate
			break
		}
	}
	m.sessions[tokenHash] = domain.Session{TokenHash: tokenHash, CSRFToken: csrfToken, User: user, ExpiresAt: expiresAt}
	return nil
}
func (m *memorySessionRepository) SessionByTokenHash(_ context.Context, tokenHash string) (domain.Session, error) {
	session, ok := m.sessions[tokenHash]
	if !ok || session.ExpiresAt.Before(time.Now()) {
		return domain.Session{}, domain.ErrUnauthorized
	}
	return session, nil
}
func (m *memorySessionRepository) DeleteSession(_ context.Context, tokenHash string) error {
	delete(m.sessions, tokenHash)
	return nil
}

func newTestService(t *testing.T) (*Service, *memoryUserRepository, *memorySessionRepository) {
	t.Helper()
	users := &memoryUserRepository{users: map[string]domain.User{}}
	sessions := &memorySessionRepository{sessions: map[string]domain.Session{}, users: users}
	return New(users, sessions, time.Hour), users, sessions
}

func TestEnsureAdminSeedsOnlyOnce(t *testing.T) {
	service, users, _ := newTestService(t)
	if err := service.EnsureAdmin(context.Background(), "admin", "secret"); err != nil {
		t.Fatalf("EnsureAdmin() error = %v", err)
	}
	if len(users.users) != 1 {
		t.Fatalf("users = %d", len(users.users))
	}
	if err := service.EnsureAdmin(context.Background(), "other", "other-secret"); err != nil {
		t.Fatalf("second EnsureAdmin() error = %v", err)
	}
	if len(users.users) != 1 {
		t.Fatalf("users after second seed = %d", len(users.users))
	}
}

func TestEnsureAdminRequiresPassword(t *testing.T) {
	service, _, _ := newTestService(t)
	if err := service.EnsureAdmin(context.Background(), "admin", ""); err == nil {
		t.Fatal("EnsureAdmin() error = nil")
	}
}

func TestLoginAuthenticateLogout(t *testing.T) {
	service, _, sessions := newTestService(t)
	if err := service.EnsureAdmin(context.Background(), "admin", "secret"); err != nil {
		t.Fatal(err)
	}
	result, err := service.Login(context.Background(), "admin", "secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.Token == "" || result.Session.CSRFToken == "" || result.Session.User.Username != "admin" {
		t.Fatalf("unexpected login result: %+v", result)
	}
	authenticated, err := service.Authenticate(context.Background(), result.Token)
	if err != nil || authenticated.User.Username != "admin" {
		t.Fatalf("Authenticate() = %+v, %v", authenticated, err)
	}
	if err := service.Logout(context.Background(), result.Token); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := service.Authenticate(context.Background(), result.Token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("Authenticate after logout error = %v", err)
	}
	if len(sessions.sessions) != 0 {
		t.Fatalf("sessions = %d", len(sessions.sessions))
	}
}

func TestLoginRejectsBadPassword(t *testing.T) {
	service, _, _ := newTestService(t)
	if err := service.EnsureAdmin(context.Background(), "admin", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(context.Background(), "admin", "wrong"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v", err)
	}
}

// Compile-time check for the session repository shape used by production code.
var _ SessionRepository = (*memorySessionRepository)(nil)

type memorySessionUserLookup interface {
	UserByUsername(context.Context, string) (domain.User, error)
}

var _ memorySessionUserLookup = (*memoryUserRepository)(nil)
