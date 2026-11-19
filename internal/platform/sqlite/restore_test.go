package sqlite

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"virtmgr/internal/domain"
)

func TestDatabaseRestoreStagesValidatesAndAppliesBeforeOpen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	sourceDB, err := Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, sourceDB); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceDB.ExecContext(ctx, `INSERT INTO settings(key, value, updated_at) VALUES ('restore-marker', 'ok', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	targetPath := filepath.Join(dir, "target", "virtmgr.db")
	targetDB, err := Open(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, targetDB); err != nil {
		t.Fatal(err)
	}
	store := NewStore(targetDB)
	status, err := store.StageDatabaseRestore(ctx, targetPath, bytes.NewReader(body), int64(len(body)), "virtmgr-backup.db", "admin")
	if err != nil {
		t.Fatalf("StageDatabaseRestore() error = %v", err)
	}
	if status.State != domain.DatabaseRestorePendingRestart || status.SchemaVersion < 1 {
		t.Fatalf("staged status = %+v", status)
	}
	if err := targetDB.Close(); err != nil {
		t.Fatal(err)
	}

	applied, err := ApplyPendingDatabaseRestore(ctx, targetPath)
	if err != nil {
		t.Fatalf("ApplyPendingDatabaseRestore() error = %v", err)
	}
	if applied.State != domain.DatabaseRestoreApplied {
		t.Fatalf("applied status = %+v", applied)
	}
	restoredDB, err := Open(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restoredDB.Close() }()
	var value string
	if err := restoredDB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'restore-marker'`).Scan(&value); err != nil {
		t.Fatalf("read restored marker: %v", err)
	}
	if value != "ok" {
		t.Fatalf("restored marker = %q", value)
	}
	if status, err := store.DatabaseRestoreStatus(targetPath); err != nil || status.State != domain.DatabaseRestoreApplied {
		t.Fatalf("DatabaseRestoreStatus() = %+v, %v", status, err)
	}
}

func TestDatabaseRestoreRejectsInvalidAndCanBeCancelled(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "virtmgr.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	invalid := []byte("not a sqlite database")
	if _, err := store.StageDatabaseRestore(ctx, path, bytes.NewReader(invalid), int64(len(invalid)), "broken.db", "admin"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid import error = %v, want ErrInvalidInput", err)
	}

	sourcePath := filepath.Join(t.TempDir(), "valid.db")
	sourceDB, err := Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, sourceDB); err != nil {
		t.Fatal(err)
	}
	_ = sourceDB.Close()
	body, _ := os.ReadFile(sourcePath)
	if _, err := store.StageDatabaseRestore(ctx, path, bytes.NewReader(body), int64(len(body)), "valid.db", "admin"); err != nil {
		t.Fatal(err)
	}
	if status, err := store.DatabaseRestoreStatus(path); err != nil || status.State != domain.DatabaseRestorePendingRestart {
		t.Fatalf("pending status = %+v, %v", status, err)
	}
	if status, err := store.CancelDatabaseRestore(path); err != nil || status.State != domain.DatabaseRestoreNone {
		t.Fatalf("cancel status = %+v, %v", status, err)
	}
}
