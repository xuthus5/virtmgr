package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *Store {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "virtmgr.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return NewStore(db)
}

func TestMigrateIsIdempotent(t *testing.T) {
	store := openTestDB(t)
	if err := Migrate(context.Background(), store.db); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
	for _, table := range []string{"users", "sessions", "tasks", "audit_events", "settings", "login_attempts", "vm_templates", "snapshot_policies"} {
		var name string
		if err := store.db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}
}
