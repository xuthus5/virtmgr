package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"virtmgr/internal/domain"
	authapp "virtmgr/internal/modules/auth/app"
)

const (
	maxTokenNameRunes = 64
	maxTokenTTLDays   = 3650
)

type Repository interface {
	CreateAPIToken(ctx context.Context, userID int64, token domain.APIToken, tokenHash string) (domain.APIToken, error)
	ListAPITokensByUser(ctx context.Context, userID int64) ([]domain.APIToken, error)
	RevokeAPIToken(ctx context.Context, userID int64, id string, revokedAt time.Time) error
	APITokenPrincipalByHash(ctx context.Context, tokenHash string, now time.Time) (domain.APITokenPrincipal, error)
	TouchAPIToken(ctx context.Context, id string, usedAt time.Time) error
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type Service struct {
	repository Repository
	audit      AuditRecorder
	now        func() time.Time
	newID      func() (string, error)
	newSecret  func() (string, error)
}

type Option func(*Service)

func New(repository Repository, audit AuditRecorder, options ...Option) *Service {
	service := &Service{repository: repository, audit: audit, now: func() time.Time { return time.Now().UTC() }, newID: newTokenID, newSecret: newTokenSecret}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Create(ctx context.Context, spec domain.APITokenCreateSpec, request MutationContext) (domain.APITokenCreated, error) {
	if !request.Session.IsAPITokenManagementAllowed() {
		return domain.APITokenCreated{}, domain.ErrForbidden
	}
	spec.Name = strings.TrimSpace(spec.Name)
	if spec.Name == "" || !utf8.ValidString(spec.Name) || utf8.RuneCountInString(spec.Name) > maxTokenNameRunes || containsControl(spec.Name) {
		return domain.APITokenCreated{}, domain.ErrInvalidInput
	}
	scopes, ok := domain.NormalizeAPITokenScopes(spec.Scopes)
	if !ok {
		return domain.APITokenCreated{}, domain.ErrInvalidInput
	}
	if spec.ExpiresInDays < 0 || spec.ExpiresInDays > maxTokenTTLDays {
		return domain.APITokenCreated{}, domain.ErrInvalidInput
	}
	id, err := s.newID()
	if err != nil {
		return domain.APITokenCreated{}, err
	}
	secret, err := s.newSecret()
	if err != nil {
		return domain.APITokenCreated{}, err
	}
	now := s.now().UTC()
	token := domain.APIToken{ID: id, Name: spec.Name, Scopes: scopes, CreatedAt: now}
	if spec.ExpiresInDays > 0 {
		expiresAt := now.AddDate(0, 0, spec.ExpiresInDays)
		token.ExpiresAt = &expiresAt
	}
	created, err := s.repository.CreateAPIToken(ctx, request.Session.User.ID, token, authapp.HashToken(secret))
	if err != nil {
		return domain.APITokenCreated{}, err
	}
	if err := s.recordAudit(ctx, request, "api_token.create", created.ID, "success", tokenDetail(created)); err != nil {
		return domain.APITokenCreated{}, fmt.Errorf("record API token create audit: %w", err)
	}
	return domain.APITokenCreated{Token: secret, APIToken: created}, nil
}

func (s *Service) List(ctx context.Context, session domain.Session) (domain.APITokenList, error) {
	if !session.IsAPITokenManagementAllowed() {
		return domain.APITokenList{}, domain.ErrForbidden
	}
	items, err := s.repository.ListAPITokensByUser(ctx, session.User.ID)
	if err != nil {
		return domain.APITokenList{}, err
	}
	if items == nil {
		items = []domain.APIToken{}
	}
	return domain.APITokenList{Items: items, Total: len(items)}, nil
}

func (s *Service) Revoke(ctx context.Context, id string, request MutationContext) error {
	if !request.Session.IsAPITokenManagementAllowed() {
		return domain.ErrForbidden
	}
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 64 {
		return domain.ErrInvalidInput
	}
	now := s.now().UTC()
	if err := s.repository.RevokeAPIToken(ctx, request.Session.User.ID, id, now); err != nil {
		return err
	}
	if err := s.recordAudit(ctx, request, "api_token.revoke", id, "success", map[string]any{"id": id}); err != nil {
		return fmt.Errorf("record API token revoke audit: %w", err)
	}
	return nil
}

func (s *Service) Authenticate(ctx context.Context, secret string) (domain.Session, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" || len(secret) > 256 {
		return domain.Session{}, domain.ErrUnauthorized
	}
	now := s.now().UTC()
	principal, err := s.repository.APITokenPrincipalByHash(ctx, authapp.HashToken(secret), now)
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) || errors.Is(err, domain.ErrNotFound) {
			return domain.Session{}, domain.ErrUnauthorized
		}
		return domain.Session{}, err
	}
	if principal.User.Disabled || !principal.Token.Active(now) {
		return domain.Session{}, domain.ErrUnauthorized
	}
	scopes := make([]string, 0, len(principal.Token.Scopes))
	for _, scope := range principal.Token.Scopes {
		scopes = append(scopes, string(scope))
	}
	_ = s.repository.TouchAPIToken(ctx, principal.Token.ID, now)
	session := domain.Session{
		User: principal.User, AuthMode: domain.AuthModeToken, TokenID: principal.Token.ID, Scopes: scopes,
	}
	if principal.Token.ExpiresAt != nil {
		session.ExpiresAt = *principal.Token.ExpiresAt
	}
	return session, nil
}

func (s *Service) recordAudit(ctx context.Context, request MutationContext, action, targetID, result string, detail map[string]any) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: s.now().UTC(), RequestID: request.RequestID, Actor: request.Session.User.Username,
		Role: request.Session.User.Role, Action: action, TargetKind: "api_token", TargetID: targetID,
		SourceIP: request.SourceIP, Result: result, Detail: detail,
	})
}

func tokenDetail(token domain.APIToken) map[string]any {
	scopes := make([]string, 0, len(token.Scopes))
	for _, scope := range token.Scopes {
		scopes = append(scopes, string(scope))
	}
	detail := map[string]any{"name": token.Name, "scopes": scopes}
	if token.ExpiresAt != nil {
		detail["expiresAt"] = token.ExpiresAt.Format(time.RFC3339)
	}
	return detail
}

func newTokenID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate API token id: %w", err)
	}
	return "apitok_" + hex.EncodeToString(raw[:]), nil
}

func newTokenSecret() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate API token secret: %w", err)
	}
	return "vmt_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func containsControl(value string) bool {
	for _, char := range value {
		if unicode.IsControl(char) {
			return true
		}
	}
	return false
}
