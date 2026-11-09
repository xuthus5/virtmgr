package consoletoken

import (
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestIssueAndConsumeOnce(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	store := New(30 * time.Second)
	store.now = func() time.Time { return now }

	grant, err := store.Issue("vm-a", 7, domain.ConsoleKindSerial, "serial0")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if grant.Token == "" || grant.VMUUID != "vm-a" || grant.UserID != 7 || grant.Kind != domain.ConsoleKindSerial || grant.Device != "serial0" || !grant.ExpiresAt.Equal(now.Add(30*time.Second)) {
		t.Fatalf("grant = %+v", grant)
	}
	consumed, err := store.Consume(grant.Token)
	if err != nil || consumed.Token != grant.Token {
		t.Fatalf("Consume() = %+v, %v", consumed, err)
	}
	if _, err := store.Consume(grant.Token); !errors.Is(err, domain.ErrConsoleToken) {
		t.Fatalf("second Consume() error = %v", err)
	}
}

func TestConsumeRejectsExpiredAndUnknownTokens(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	store := New(time.Minute)
	store.now = func() time.Time { return now }
	grant, err := store.Issue("vm-a", 1, domain.ConsoleKindVNC, "")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := store.Consume(grant.Token); !errors.Is(err, domain.ErrConsoleToken) {
		t.Fatalf("expired Consume() error = %v", err)
	}
	if _, err := store.Consume("missing"); !errors.Is(err, domain.ErrConsoleToken) {
		t.Fatalf("unknown Consume() error = %v", err)
	}
}

func TestIssueRemovesExpiredGrants(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	store := New(time.Second)
	store.now = func() time.Time { return now }
	first, err := store.Issue("vm-a", 1, domain.ConsoleKindVNC, "")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	if _, err := store.Issue("vm-b", 2, domain.ConsoleKindVNC, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.grants[first.Token]; ok {
		t.Fatal("expired grant was not removed")
	}
}
