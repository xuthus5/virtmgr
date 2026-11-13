package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestExportDatabaseCreatesConsistentCopy(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	if err := store.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: time.Unix(1_700_000_000, 0), Actor: "admin", Role: domain.RoleAdmin,
		Action: "vm.start", TargetKind: "vm", TargetID: "vm-a", Result: "success",
	}); err != nil {
		t.Fatalf("RecordAudit() error = %v", err)
	}
	destination := filepath.Join(t.TempDir(), "export.db")
	if err := os.WriteFile(destination, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.ExportDatabase(ctx, destination); err != nil {
		t.Fatalf("ExportDatabase() error = %v", err)
	}
	info, err := os.Stat(destination)
	if err != nil || info.Size() == 0 {
		t.Fatalf("export stat = %+v, %v", info, err)
	}

	exported, err := sql.Open("sqlite", "file:"+destination+"?mode=ro")
	if err != nil {
		t.Fatalf("open export error = %v", err)
	}
	defer exported.Close()
	var count int
	if err := exported.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&count); err != nil {
		t.Fatalf("query export error = %v", err)
	}
	if count != 1 {
		t.Fatalf("exported audit count = %d", count)
	}
}

func TestExportDatabaseRequiresDestination(t *testing.T) {
	if err := openTestDB(t).ExportDatabase(context.Background(), ""); err == nil {
		t.Fatal("ExportDatabase() error = nil")
	}
}
