package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

type UserRepository interface {
	CountUsers(ctx context.Context) (int, error)
	CreateUser(ctx context.Context, username, passwordHash string, role domain.Role) (domain.User, error)
	UserByUsername(ctx context.Context, username string) (domain.User, error)
}

type SessionRepository interface {
	CreateSession(ctx context.Context, tokenHash, csrfToken string, userID int64, expiresAt time.Time) error
	SessionByTokenHash(ctx context.Context, tokenHash string) (domain.Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
}

type Service struct {
	users    UserRepository
	sessions SessionRepository
	ttl      time.Duration
	now      func() time.Time
}

type LoginResult struct {
	Token   string
	Session domain.Session
}

func New(users UserRepository, sessions SessionRepository, ttl time.Duration) *Service {
	return &Service{users: users, sessions: sessions, ttl: ttl, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) EnsureAdmin(ctx context.Context, username, password string) error {
	count, err := s.users.CountUsers(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("admin username must not be empty")
	}
	if password == "" {
		return fmt.Errorf("VIRTMGR_ADMIN_PASSWORD is required for the first start")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.users.CreateUser(ctx, username, hash, domain.RoleAdmin)
	return err
}

func (s *Service) Login(ctx context.Context, username, password string) (LoginResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return LoginResult{}, domain.ErrInvalidCredentials
	}
	user, err := s.users.UserByUsername(ctx, username)
	if err != nil || user.Disabled || !VerifyPassword(user.PasswordHash, password) {
		return LoginResult{}, domain.ErrInvalidCredentials
	}
	token, err := randomToken()
	if err != nil {
		return LoginResult{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return LoginResult{}, err
	}
	expiresAt := s.now().Add(s.ttl)
	if err := s.sessions.CreateSession(ctx, HashToken(token), csrf, user.ID, expiresAt); err != nil {
		return LoginResult{}, err
	}
	user.PasswordHash = ""
	return LoginResult{
		Token: token,
		Session: domain.Session{
			CSRFToken: csrf,
			User:      user,
			ExpiresAt: expiresAt,
		},
	}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (domain.Session, error) {
	if strings.TrimSpace(token) == "" {
		return domain.Session{}, domain.ErrUnauthorized
	}
	return s.sessions.SessionByTokenHash(ctx, HashToken(token))
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return s.sessions.DeleteSession(ctx, HashToken(token))
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
