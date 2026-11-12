package app

import (
	"context"
	"errors"
	"strconv"
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
	attempts []memoryLoginAttempt
}

type memoryLoginAttempt struct {
	username    string
	sourceIP    string
	success     bool
	attemptedAt time.Time
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
func (m *memorySessionRepository) RecordLoginAttempt(_ context.Context, username, sourceIP string, success bool, attemptedAt time.Time) error {
	m.attempts = append(m.attempts, memoryLoginAttempt{username: username, sourceIP: sourceIP, success: success, attemptedAt: attemptedAt})
	return nil
}
func (m *memorySessionRepository) CountRecentFailedLoginAttempts(_ context.Context, username, sourceIP string, since time.Time) (int, error) {
	count := 0
	for _, attempt := range m.attempts {
		if !attempt.success && attempt.username == username && attempt.sourceIP == sourceIP && !attempt.attemptedAt.Before(since) {
			count++
		}
	}
	return count, nil
}
func (m *memorySessionRepository) CountRecentFailedLoginAttemptsByUsername(_ context.Context, username string, since time.Time) (int, error) {
	count := 0
	for _, attempt := range m.attempts {
		if !attempt.success && attempt.username == username && !attempt.attemptedAt.Before(since) {
			count++
		}
	}
	return count, nil
}
func (m *memorySessionRepository) ClearFailedLoginAttempts(_ context.Context, username string) error {
	filtered := m.attempts[:0]
	for _, attempt := range m.attempts {
		if attempt.username != username || attempt.success {
			filtered = append(filtered, attempt)
		}
	}
	m.attempts = filtered
	return nil
}
func (m *memorySessionRepository) DeleteLoginAttemptsBefore(_ context.Context, before time.Time) error {
	filtered := m.attempts[:0]
	for _, attempt := range m.attempts {
		if !attempt.attemptedAt.Before(before) {
			filtered = append(filtered, attempt)
		}
	}
	m.attempts = filtered
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
	result, err := service.Login(context.Background(), "admin", "secret", "192.0.2.10")
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
	if _, err := service.Login(context.Background(), "admin", "wrong", "192.0.2.10"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v", err)
	}
}

func TestLoginRateLimitsFailuresPerSourceAndAccount(t *testing.T) {
	service, _, _ := newTestService(t)
	if err := service.EnsureAdmin(context.Background(), "admin", "secret"); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < loginFailureLimitPerPeer; attempt++ {
		if _, err := service.Login(context.Background(), "admin", "wrong", "192.0.2.20"); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("attempt %d error = %v", attempt+1, err)
		}
	}
	if _, err := service.Login(context.Background(), "admin", "secret", "192.0.2.20"); !errors.Is(err, domain.ErrTooManyAttempts) {
		t.Fatalf("rate-limited login error = %v", err)
	}
	if _, err := service.Login(context.Background(), "admin", "secret", "192.0.2.21"); err != nil {
		t.Fatalf("login from another source error = %v", err)
	}

	for attempt := 0; attempt < loginFailureLimitPerUser; attempt++ {
		sourceIP := "198.51.100." + strconv.Itoa(attempt)
		if _, err := service.Login(context.Background(), "admin", "wrong", sourceIP); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("account attempt %d error = %v", attempt+1, err)
		}
	}
	if _, err := service.Login(context.Background(), "admin", "secret", "203.0.113.1"); !errors.Is(err, domain.ErrTooManyAttempts) {
		t.Fatalf("account-wide rate-limited login error = %v", err)
	}
}

func TestLoginSuccessClearsFailures(t *testing.T) {
	service, _, _ := newTestService(t)
	if err := service.EnsureAdmin(context.Background(), "admin", "secret"); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < loginFailureLimitPerPeer-1; attempt++ {
		_, _ = service.Login(context.Background(), "admin", "wrong", "192.0.2.30")
	}
	if _, err := service.Login(context.Background(), "admin", "secret", "192.0.2.30"); err != nil {
		t.Fatalf("successful login error = %v", err)
	}
	for attempt := 0; attempt < loginFailureLimitPerPeer-1; attempt++ {
		if _, err := service.Login(context.Background(), "admin", "wrong", "192.0.2.30"); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("post-success attempt %d error = %v", attempt+1, err)
		}
	}
}

// Compile-time check for the session repository shape used by production code.
var _ SessionRepository = (*memorySessionRepository)(nil)

type memorySessionUserLookup interface {
	UserByUsername(context.Context, string) (domain.User, error)
}

var _ memorySessionUserLookup = (*memoryUserRepository)(nil)
