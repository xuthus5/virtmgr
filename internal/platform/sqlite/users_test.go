package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestUserAndSessionStore(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	if count, err := store.CountUsers(ctx); err != nil || count != 0 {
		t.Fatalf("CountUsers() = %d, %v", count, err)
	}
	user, err := store.CreateUser(ctx, "admin", "hash", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	loaded, err := store.UserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("UserByUsername() error = %v", err)
	}
	if loaded.ID != user.ID || loaded.Username != "admin" || loaded.Role != domain.RoleAdmin {
		t.Fatalf("loaded user = %+v", loaded)
	}
	expires := time.Now().Add(time.Hour)
	if err := store.CreateSession(ctx, "token-hash", "csrf", user.ID, expires); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	session, err := store.SessionByTokenHash(ctx, "token-hash")
	if err != nil {
		t.Fatalf("SessionByTokenHash() error = %v", err)
	}
	if session.CSRFToken != "csrf" || session.User.Username != "admin" {
		t.Fatalf("session = %+v", session)
	}
	if err := store.DeleteSession(ctx, "token-hash"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if _, err := store.SessionByTokenHash(ctx, "token-hash"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("deleted session error = %v", err)
	}
}

func TestSessionExpires(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	user, err := store.CreateUser(ctx, "admin", "hash", domain.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(ctx, "expired", "csrf", user.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SessionByTokenHash(ctx, "expired"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired session error = %v", err)
	}
}

func TestListAndUpdateUsers(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	viewer, err := store.CreateUser(ctx, "viewer", "viewer-hash", domain.RoleViewer)
	if err != nil {
		t.Fatalf("CreateUser(viewer) error = %v", err)
	}
	admin, err := store.CreateUser(ctx, "Admin", "admin-hash", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser(admin) error = %v", err)
	}

	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(users) != 2 || users[0].Username != "Admin" || users[1].Username != "viewer" {
		t.Fatalf("users = %+v", users)
	}
	if users[0].PasswordHash != "" {
		t.Fatalf("list exposed password hash: %+v", users[0])
	}

	loaded, err := store.UserByID(ctx, viewer.ID)
	if err != nil || loaded.Username != "viewer" || loaded.Role != domain.RoleViewer {
		t.Fatalf("UserByID() = %+v, %v", loaded, err)
	}

	updated, err := store.UpdateUser(ctx, viewer.ID, domain.RoleOperator, true)
	if err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}
	if updated.Role != domain.RoleOperator || !updated.Disabled {
		t.Fatalf("updated user = %+v", updated)
	}

	if err := store.CreateSession(ctx, "viewer-session", "csrf", viewer.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := store.SessionByTokenHash(ctx, "viewer-session"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("disabled user session error = %v", err)
	}
	if admin.Role != domain.RoleAdmin {
		t.Fatalf("admin role changed: %+v", admin)
	}
	if _, err := store.UpdateUser(ctx, 9999, domain.RoleViewer, false); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing user update error = %v", err)
	}
}
