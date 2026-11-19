package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

const maxDatabaseRestoreBytes = int64(1 << 30)

var requiredDatabaseTables = []string{
	"users", "sessions", "tasks", "audit_events", "settings", "metric_samples", "data_plane_operations",
}

type databaseRestoreManifest struct {
	Filename      string    `json:"filename"`
	SizeBytes     int64     `json:"sizeBytes"`
	SchemaVersion int       `json:"schemaVersion"`
	UploadedAt    time.Time `json:"uploadedAt"`
	UploadedBy    string    `json:"uploadedBy"`
}

type databaseRestoreResult struct {
	State         string    `json:"state"`
	Filename      string    `json:"filename"`
	SizeBytes     int64     `json:"sizeBytes"`
	SchemaVersion int       `json:"schemaVersion"`
	AppliedAt     time.Time `json:"appliedAt"`
	Message       string    `json:"message"`
}

func databaseRestorePaths(databasePath string) (staged, manifest, result string) {
	return databasePath + ".restore-staged", databasePath + ".restore-manifest.json", databasePath + ".restore-result.json"
}

func (s *Store) StageDatabaseRestore(ctx context.Context, databasePath string, reader io.Reader, size int64, filename, actor string) (domain.DatabaseRestoreStatus, error) {
	if databasePath == "" || databasePath == ":memory:" || reader == nil || size <= 0 {
		return domain.DatabaseRestoreStatus{}, domain.ErrInvalidInput
	}
	if size > maxDatabaseRestoreBytes {
		return domain.DatabaseRestoreStatus{}, domain.ErrTooLarge
	}
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "" || filename == "." || len(filename) > 255 {
		return domain.DatabaseRestoreStatus{}, domain.ErrInvalidInput
	}
	staged, manifestPath, resultPath := databaseRestorePaths(databasePath)
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o750); err != nil {
		return domain.DatabaseRestoreStatus{}, fmt.Errorf("create database directory for restore: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(databasePath), ".virtmgr-restore-upload-*")
	if err != nil {
		return domain.DatabaseRestoreStatus{}, fmt.Errorf("create database restore staging file: %w", err)
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return domain.DatabaseRestoreStatus{}, fmt.Errorf("secure database restore staging file: %w", err)
	}
	written, err := io.CopyN(temp, reader, size)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return domain.DatabaseRestoreStatus{}, errors.Join(domain.ErrInvalidInput, errors.New("database upload ended before declared size"))
		}
		return domain.DatabaseRestoreStatus{}, fmt.Errorf("write database restore upload: %w", err)
	}
	if written != size {
		return domain.DatabaseRestoreStatus{}, domain.ErrInvalidInput
	}
	if err := temp.Sync(); err != nil {
		return domain.DatabaseRestoreStatus{}, fmt.Errorf("sync database restore upload: %w", err)
	}
	if err := temp.Close(); err != nil {
		return domain.DatabaseRestoreStatus{}, fmt.Errorf("close database restore upload: %w", err)
	}
	schemaVersion, err := validateDatabaseRestore(ctx, tempPath)
	if err != nil {
		return domain.DatabaseRestoreStatus{}, err
	}
	if err := os.Rename(tempPath, staged); err != nil {
		return domain.DatabaseRestoreStatus{}, fmt.Errorf("publish staged database: %w", err)
	}
	committed = true
	manifest := databaseRestoreManifest{
		Filename: filename, SizeBytes: size, SchemaVersion: schemaVersion,
		UploadedAt: time.Now().UTC(), UploadedBy: strings.TrimSpace(actor),
	}
	if err := writeJSONFile(manifestPath, manifest); err != nil {
		_ = os.Remove(staged)
		return domain.DatabaseRestoreStatus{}, err
	}
	_ = os.Remove(resultPath)
	uploadedAt := manifest.UploadedAt
	return domain.DatabaseRestoreStatus{
		State: domain.DatabaseRestorePendingRestart, Filename: filename, SizeBytes: size,
		SchemaVersion: schemaVersion, UploadedAt: &uploadedAt,
		Message: "数据库已通过完整性校验，重启 VirtMgr 后生效",
	}, nil
}

func (s *Store) DatabaseRestoreStatus(databasePath string) (domain.DatabaseRestoreStatus, error) {
	if databasePath == "" || databasePath == ":memory:" {
		return domain.DatabaseRestoreStatus{State: domain.DatabaseRestoreNone}, nil
	}
	staged, manifestPath, resultPath := databaseRestorePaths(databasePath)
	manifest, err := readRestoreManifest(manifestPath)
	if err == nil {
		if info, statErr := os.Stat(staged); statErr == nil && info.Mode().IsRegular() {
			uploadedAt := manifest.UploadedAt
			return domain.DatabaseRestoreStatus{
				State: domain.DatabaseRestorePendingRestart, Filename: manifest.Filename,
				SizeBytes: manifest.SizeBytes, SchemaVersion: manifest.SchemaVersion, UploadedAt: &uploadedAt,
				Message: "数据库已通过完整性校验，重启 VirtMgr 后生效",
			}, nil
		}
	}
	result, resultErr := readRestoreResult(resultPath)
	if resultErr == nil {
		status := domain.DatabaseRestoreStatus{
			State: result.State, Filename: result.Filename, SizeBytes: result.SizeBytes,
			SchemaVersion: result.SchemaVersion, Message: result.Message,
		}
		if !result.AppliedAt.IsZero() {
			appliedAt := result.AppliedAt
			status.AppliedAt = &appliedAt
		}
		return status, nil
	}
	return domain.DatabaseRestoreStatus{State: domain.DatabaseRestoreNone}, nil
}

func (s *Store) CancelDatabaseRestore(databasePath string) (domain.DatabaseRestoreStatus, error) {
	if databasePath == "" || databasePath == ":memory:" {
		return domain.DatabaseRestoreStatus{State: domain.DatabaseRestoreNone}, nil
	}
	staged, manifestPath, resultPath := databaseRestorePaths(databasePath)
	for _, path := range []string{staged, staged + "-wal", staged + "-shm", manifestPath, resultPath} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return domain.DatabaseRestoreStatus{}, fmt.Errorf("cancel staged database restore: %w", err)
		}
	}
	return domain.DatabaseRestoreStatus{State: domain.DatabaseRestoreNone}, nil
}

// ApplyPendingDatabaseRestore must run before sqlite.Open. It validates the
// staged copy again, keeps a timestamped copy of the current database, and
// atomically swaps files without touching a live SQLite connection.
func ApplyPendingDatabaseRestore(ctx context.Context, databasePath string) (domain.DatabaseRestoreStatus, error) {
	if databasePath == "" || databasePath == ":memory:" {
		return domain.DatabaseRestoreStatus{State: domain.DatabaseRestoreNone}, nil
	}
	staged, manifestPath, resultPath := databaseRestorePaths(databasePath)
	manifest, err := readRestoreManifest(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return domain.DatabaseRestoreStatus{State: domain.DatabaseRestoreNone}, nil
	}
	if err != nil {
		return domain.DatabaseRestoreStatus{State: domain.DatabaseRestoreFailed, Message: "恢复清单损坏"}, err
	}
	schemaVersion, validationErr := validateDatabaseRestore(ctx, staged)
	if validationErr != nil {
		_ = os.Remove(staged)
		_ = os.Remove(manifestPath)
		result := databaseRestoreResult{
			State: domain.DatabaseRestoreFailed, Filename: manifest.Filename, SizeBytes: manifest.SizeBytes,
			AppliedAt: time.Now().UTC(), Message: "恢复数据库未通过启动前校验：" + validationErr.Error(),
		}
		_ = writeJSONFile(resultPath, result)
		return restoreStatusFromResult(result), nil
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o750); err != nil {
		return domain.DatabaseRestoreStatus{}, fmt.Errorf("create database directory before restore: %w", err)
	}
	backupPath := ""
	if _, statErr := os.Stat(databasePath); statErr == nil {
		backupPath = databasePath + ".pre-restore-" + time.Now().UTC().Format("20060102-150405.000000000") + ".bak"
		if err := os.Rename(databasePath, backupPath); err != nil {
			return domain.DatabaseRestoreStatus{}, fmt.Errorf("backup current database before restore: %w", err)
		}
		for _, suffix := range []string{"-wal", "-shm"} {
			if _, err := os.Stat(databasePath + suffix); err == nil {
				_ = os.Rename(databasePath+suffix, backupPath+suffix)
			}
		}
	}
	restoreCurrent := func() {
		_ = os.Remove(databasePath)
		if backupPath != "" {
			_ = os.Rename(backupPath, databasePath)
			for _, suffix := range []string{"-wal", "-shm"} {
				if _, err := os.Stat(backupPath + suffix); err == nil {
					_ = os.Rename(backupPath+suffix, databasePath+suffix)
				}
			}
		}
	}
	if err := os.Rename(staged, databasePath); err != nil {
		restoreCurrent()
		return domain.DatabaseRestoreStatus{}, fmt.Errorf("activate restored database: %w", err)
	}
	_ = os.Remove(staged + "-wal")
	_ = os.Remove(staged + "-shm")
	_ = os.Remove(manifestPath)
	appliedAt := time.Now().UTC()
	result := databaseRestoreResult{
		State: domain.DatabaseRestoreApplied, Filename: manifest.Filename, SizeBytes: manifest.SizeBytes,
		SchemaVersion: schemaVersion, AppliedAt: appliedAt, Message: "数据库恢复已应用",
	}
	if backupPath != "" {
		result.Message += "，原数据库备份为 " + filepath.Base(backupPath)
	}
	if err := writeJSONFile(resultPath, result); err != nil {
		return domain.DatabaseRestoreStatus{}, err
	}
	return restoreStatusFromResult(result), nil
}

func validateDatabaseRestore(ctx context.Context, path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, errors.Join(domain.ErrInvalidInput, fmt.Errorf("open staged database: %w", err))
	}
	header := make([]byte, 16)
	_, readErr := io.ReadFull(file, header)
	_ = file.Close()
	if readErr != nil || !bytes.Equal(header, []byte("SQLite format 3\x00")) {
		return 0, errors.Join(domain.ErrInvalidInput, errors.New("uploaded file is not a SQLite database"))
	}
	db, err := openRestoreDatabase(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()
	version, err := detectDatabaseSchemaVersion(ctx, db)
	if err != nil {
		return 0, errors.Join(domain.ErrInvalidInput, err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		return 0, err
	}
	latest := migrations[len(migrations)-1].version
	if version > latest {
		return 0, errors.Join(domain.ErrInvalidInput, fmt.Errorf("database schema version %d is newer than supported version %d", version, latest))
	}
	if err := Migrate(ctx, db); err != nil {
		return 0, errors.Join(domain.ErrInvalidInput, err)
	}
	if err := checkDatabaseIntegrity(ctx, db); err != nil {
		return 0, err
	}
	if err := checkRequiredDatabaseTables(ctx, db); err != nil {
		return 0, err
	}
	return detectDatabaseSchemaVersion(ctx, db)
}

func openRestoreDatabase(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(DELETE)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, errors.Join(domain.ErrInvalidInput, fmt.Errorf("open staged database: %w", err))
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, errors.Join(domain.ErrInvalidInput, fmt.Errorf("ping staged database: %w", err))
	}
	return db, nil
}

func detectDatabaseSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	if version < 1 {
		return 0, errors.New("database does not contain an applied VirtMgr schema")
	}
	return version, nil
}

func checkDatabaseIntegrity(ctx context.Context, db *sql.DB) error {
	var result string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return errors.Join(domain.ErrInvalidInput, fmt.Errorf("check database integrity: %w", err))
	}
	if !strings.EqualFold(strings.TrimSpace(result), "ok") {
		return errors.Join(domain.ErrInvalidInput, fmt.Errorf("database integrity check failed: %s", result))
	}
	return nil
}

func checkRequiredDatabaseTables(ctx context.Context, db *sql.DB) error {
	for _, table := range requiredDatabaseTables {
		var name string
		err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Join(domain.ErrInvalidInput, fmt.Errorf("database is missing required table %s", table))
		}
		if err != nil {
			return errors.Join(domain.ErrInvalidInput, fmt.Errorf("inspect required table %s: %w", table, err))
		}
	}
	return nil
}

func readRestoreManifest(path string) (databaseRestoreManifest, error) {
	var manifest databaseRestoreManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func readRestoreResult(path string) (databaseRestoreResult, error) {
	var result databaseRestoreResult
	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	return result, nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode restore metadata: %w", err)
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), ".virtmgr-restore-metadata-*")
	if err != nil {
		return fmt.Errorf("create restore metadata file: %w", err)
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("secure restore metadata file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write restore metadata file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync restore metadata file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close restore metadata file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("publish restore metadata file: %w", err)
	}
	committed = true
	return nil
}

func restoreStatusFromResult(result databaseRestoreResult) domain.DatabaseRestoreStatus {
	status := domain.DatabaseRestoreStatus{
		State: result.State, Filename: result.Filename, SizeBytes: result.SizeBytes,
		SchemaVersion: result.SchemaVersion, Message: result.Message,
	}
	if !result.AppliedAt.IsZero() {
		appliedAt := result.AppliedAt
		status.AppliedAt = &appliedAt
	}
	return status
}
