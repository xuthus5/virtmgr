package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
	authapp "virtmgr/internal/modules/auth/app"
)

func TestAPITokenLifecycleAndPrincipal(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	user, err := store.CreateUser(ctx, "operator", "hash", domain.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(time.Hour)
	token := domain.APIToken{ID: "apitok_1", Name: "automation", Scopes: []domain.APITokenScope{"read", "write"}, CreatedAt: now, ExpiresAt: &expires}
	hash := authapp.HashToken("vmt_secret")
	if _, err := store.CreateAPIToken(ctx, user.ID, token, hash); err != nil {
		t.Fatalf("CreateAPIToken() error = %v", err)
	}
	items, err := store.ListAPITokensByUser(ctx, user.ID)
	if err != nil || len(items) != 1 || items[0].ID != "apitok_1" || items[0].Expired {
		t.Fatalf("tokens = %+v error = %v", items, err)
	}
	principal, err := store.APITokenPrincipalByHash(ctx, hash, now)
	if err != nil {
		t.Fatalf("APITokenPrincipalByHash() error = %v", err)
	}
	if principal.User.Username != "operator" || principal.User.PasswordHash != "" || len(principal.Token.Scopes) != 2 {
		t.Fatalf("principal = %+v", principal)
	}
	if err := store.TouchAPIToken(ctx, token.ID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	items, err = store.ListAPITokensByUser(ctx, user.ID)
	if err != nil || items[0].LastUsedAt == nil || !items[0].LastUsedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("touched token = %+v error = %v", items, err)
	}
	if err := store.RevokeAPIToken(ctx, user.ID, token.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.APITokenPrincipalByHash(ctx, hash, now); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("revoked principal error = %v", err)
	}
	if err := store.RevokeAPIToken(ctx, user.ID, token.ID, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second revoke error = %v", err)
	}
	if _, err := store.APITokenPrincipalByHash(ctx, authapp.HashToken("wrong"), now); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("wrong token error = %v", err)
	}
}

func TestAPITokenExpiryAndUserDisable(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	user, err := store.CreateUser(ctx, "viewer", "hash", domain.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	expired := now.Add(-time.Second)
	if _, err := store.CreateAPIToken(ctx, user.ID, domain.APIToken{ID: "expired", Name: "expired", Scopes: []domain.APITokenScope{"read"}, CreatedAt: now.Add(-time.Hour), ExpiresAt: &expired}, authapp.HashToken("vmt_expired")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.APITokenPrincipalByHash(ctx, authapp.HashToken("vmt_expired"), now); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired token error = %v", err)
	}
	if _, err := store.CreateAPIToken(ctx, user.ID, domain.APIToken{ID: "active", Name: "active", Scopes: []domain.APITokenScope{"read"}, CreatedAt: now}, authapp.HashToken("vmt_active")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateUser(ctx, user.ID, domain.RoleViewer, true); err != nil {
		t.Fatal(err)
	}
	principal, err := store.APITokenPrincipalByHash(ctx, authapp.HashToken("vmt_active"), now)
	if err != nil || !principal.User.Disabled {
		t.Fatalf("disabled principal = %+v error = %v", principal, err)
	}
}
