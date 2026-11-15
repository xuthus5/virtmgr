package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
	authapp "virtmgr/internal/modules/auth/app"
)

type memoryTokenRepository struct {
	created   domain.APIToken
	hash      string
	userID    int64
	principal domain.APITokenPrincipal
	touched   string
	revoked   string
}

func (m *memoryTokenRepository) CreateAPIToken(_ context.Context, userID int64, token domain.APIToken, tokenHash string) (domain.APIToken, error) {
	m.userID = userID
	m.created = token
	m.hash = tokenHash
	return token, nil
}

func (m *memoryTokenRepository) ListAPITokensByUser(context.Context, int64) ([]domain.APIToken, error) {
	return []domain.APIToken{m.created}, nil
}

func (m *memoryTokenRepository) RevokeAPIToken(_ context.Context, _ int64, id string, _ time.Time) error {
	m.revoked = id
	return nil
}

func (m *memoryTokenRepository) APITokenPrincipalByHash(context.Context, string, time.Time) (domain.APITokenPrincipal, error) {
	return m.principal, nil
}

func (m *memoryTokenRepository) TouchAPIToken(_ context.Context, id string, _ time.Time) error {
	m.touched = id
	return nil
}

type captureAudit struct{ events []domain.AuditEvent }

func (c *captureAudit) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	c.events = append(c.events, event)
	return nil
}

func tokenSession(role domain.Role) domain.Session {
	return domain.Session{User: domain.User{ID: 7, Username: "alice", Role: role}, AuthMode: domain.AuthModeSession}
}

func TestCreateAPITokenStoresHashAndReturnsSecretOnce(t *testing.T) {
	repository := &memoryTokenRepository{}
	audit := &captureAudit{}
	service := New(repository, audit)
	service.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
	service.newID = func() (string, error) { return "apitok_test", nil }
	service.newSecret = func() (string, error) { return "vmt_test-secret", nil }
	result, err := service.Create(context.Background(), domain.APITokenCreateSpec{
		Name: " automation ", Scopes: []domain.APITokenScope{"write"}, ExpiresInDays: 30,
	}, MutationContext{Session: tokenSession(domain.RoleViewer), RequestID: "req-1", SourceIP: "127.0.0.1"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Token != "vmt_test-secret" || repository.hash != authapp.HashToken("vmt_test-secret") || repository.hash == result.Token {
		t.Fatalf("secret/hash = %q %q", result.Token, repository.hash)
	}
	if repository.userID != 7 || result.APIToken.Name != "automation" || len(result.APIToken.Scopes) != 2 || result.APIToken.Scopes[1] != domain.APITokenScopeWrite {
		t.Fatalf("token = %+v user = %d", result.APIToken, repository.userID)
	}
	if result.APIToken.ExpiresAt == nil || !result.APIToken.ExpiresAt.Equal(time.Unix(1_700_000_000, 0).UTC().AddDate(0, 0, 30)) {
		t.Fatalf("expiry = %v", result.APIToken.ExpiresAt)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "api_token.create" {
		t.Fatalf("audit = %+v", audit.events)
	}
	if _, leaked := audit.events[0].Detail["token"]; leaked {
		t.Fatalf("audit leaked token: %+v", audit.events[0].Detail)
	}
}

func TestCreateAPITokenRejectsAPITokenSessionAndInvalidSpec(t *testing.T) {
	service := New(&memoryTokenRepository{}, nil)
	session := tokenSession(domain.RoleAdmin)
	session.AuthMode = domain.AuthModeToken
	if _, err := service.Create(context.Background(), domain.APITokenCreateSpec{Name: "bad", Scopes: []domain.APITokenScope{"read"}}, MutationContext{Session: session}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("API token create error = %v", err)
	}
	for _, spec := range []domain.APITokenCreateSpec{
		{Name: "", Scopes: []domain.APITokenScope{"read"}},
		{Name: "bad", Scopes: nil},
		{Name: "bad", Scopes: []domain.APITokenScope{"admin"}},
		{Name: "bad", Scopes: []domain.APITokenScope{"read"}, ExpiresInDays: 3651},
	} {
		if _, err := service.Create(context.Background(), spec, MutationContext{Session: tokenSession(domain.RoleAdmin)}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("spec %+v error = %v", spec, err)
		}
	}
}

func TestAPITokenCannotListOrRevokeTokens(t *testing.T) {
	service := New(&memoryTokenRepository{}, nil)
	session := tokenSession(domain.RoleAdmin)
	session.AuthMode = domain.AuthModeToken
	if _, err := service.List(context.Background(), session); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("List() error = %v", err)
	}
	if err := service.Revoke(context.Background(), "apitok_other", MutationContext{Session: session}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Revoke() error = %v", err)
	}
}

func TestAuthenticateAPITokenBuildsScopedSessionAndTouchesLastUsed(t *testing.T) {
	repository := &memoryTokenRepository{principal: domain.APITokenPrincipal{
		Token: domain.APIToken{ID: "apitok_1", Scopes: []domain.APITokenScope{"read"}},
		User:  domain.User{ID: 3, Username: "api", Role: domain.RoleOperator},
	}}
	service := New(repository, nil)
	session, err := service.Authenticate(context.Background(), "vmt_secret")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if !session.IsAPIToken() || session.TokenID != "apitok_1" || !session.HasScope("read") || session.HasScope("write") || session.User.Username != "api" {
		t.Fatalf("session = %+v", session)
	}
	if repository.touched != "apitok_1" {
		t.Fatalf("touched = %q", repository.touched)
	}
}

func TestNormalizeAPITokenScopes(t *testing.T) {
	scopes, ok := domain.NormalizeAPITokenScopes([]domain.APITokenScope{" WRITE ", "read", "write"})
	if !ok || len(scopes) != 2 || scopes[0] != domain.APITokenScopeRead || scopes[1] != domain.APITokenScopeWrite {
		t.Fatalf("scopes = %+v ok=%v", scopes, ok)
	}
	if _, ok := domain.NormalizeAPITokenScopes(nil); ok {
		t.Fatal("empty scopes accepted")
	}
}
