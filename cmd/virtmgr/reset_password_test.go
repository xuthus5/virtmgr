package main

import (
	"context"
	"testing"
	"time"

	"virtmgr/internal/domain"
	authapp "virtmgr/internal/modules/auth/app"
	"virtmgr/internal/platform/sqlite"
)

func TestResetLocalPasswordUpdatesHashRevokesSessionsAndAudits(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewStore(db)
	oldHash, err := authapp.HashPassword("old-password-123")
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(ctx, "recovery-admin", oldHash, domain.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(ctx, "session-token-hash", "csrf-token", user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if err := resetLocalPassword(ctx, store, user.Username, "new-password-456"); err != nil {
		t.Fatalf("resetLocalPassword() error = %v", err)
	}
	loaded, err := store.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !authapp.VerifyPassword(loaded.PasswordHash, "new-password-456") {
		t.Fatal("new password does not verify")
	}
	if authapp.VerifyPassword(loaded.PasswordHash, "old-password-123") {
		t.Fatal("old password still verifies")
	}
	sessions, err := store.ListSessionsByUser(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %d, want 0", len(sessions))
	}
	events, err := store.ListAuditEvents(ctx, domain.AuditEventQuery{Action: "user.password.recovery"})
	if err != nil {
		t.Fatal(err)
	}
	if events.Total != 1 || events.Items[0].Actor != "local-cli" || events.Items[0].Result != "success" {
		t.Fatalf("audit events = %+v", events)
	}
}

func TestValidRecoveryPassword(t *testing.T) {
	for _, test := range []struct {
		name     string
		password string
		want     bool
	}{
		{name: "minimum", password: "123456789012", want: true},
		{name: "too short", password: "12345678901", want: false},
		{name: "contains NUL", password: "12345678901\x00", want: false},
		{name: "too long", password: string(make([]byte, recoveryMaxPasswordLength+1)), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := validRecoveryPassword(test.password); got != test.want {
				t.Fatalf("validRecoveryPassword() = %v, want %v", got, test.want)
			}
		})
	}
}
