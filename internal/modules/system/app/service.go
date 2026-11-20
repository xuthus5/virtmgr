package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"virtmgr/internal/domain"
	"virtmgr/internal/version"
)

type DatabaseExporter interface {
	ExportDatabase(ctx context.Context, destination string) error
}

type DatabaseRestorer interface {
	StageDatabaseRestore(ctx context.Context, databasePath string, reader io.Reader, size int64, filename, actor string) (domain.DatabaseRestoreStatus, error)
	DatabaseRestoreStatus(databasePath string) (domain.DatabaseRestoreStatus, error)
	CancelDatabaseRestore(databasePath string) (domain.DatabaseRestoreStatus, error)
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type TLSStatusProvider interface {
	Status(requestHost string) domain.TLSInfo
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type DatabaseExport struct {
	Path     string
	Filename string
	Size     int64
}

type Service struct {
	exporter     DatabaseExporter
	restorer     DatabaseRestorer
	audit        AuditRecorder
	tlsStatus    TLSStatusProvider
	tempDir      string
	databasePath string
	now          func() time.Time
}

type Option func(*Service)

func WithDatabasePath(path string) Option {
	return func(service *Service) { service.databasePath = path }
}

func WithTLSStatus(provider TLSStatusProvider) Option {
	return func(service *Service) { service.tlsStatus = provider }
}

func New(exporter DatabaseExporter, audit AuditRecorder, options ...Option) *Service {
	service := &Service{
		exporter: exporter,
		audit:    audit,
		tempDir:  os.TempDir(),
		now:      func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		option(service)
	}
	service.restorer, _ = exporter.(DatabaseRestorer)
	return service
}

func (s *Service) Info(requestHost string) domain.SystemInfo {
	info := domain.SystemInfo{Build: domain.BuildInfo{
		Version: strings.TrimSpace(version.Version),
		Commit:  strings.TrimSpace(version.Commit),
		Date:    strings.TrimSpace(version.Date),
	}}
	if s.tlsStatus != nil {
		info.TLS = s.tlsStatus.Status(requestHost)
	}
	return info
}

func (s *Service) ExportDatabase(ctx context.Context, request MutationContext) (DatabaseExport, error) {
	if request.Session.User.Role != domain.RoleAdmin {
		return DatabaseExport{}, domain.ErrForbidden
	}
	now := s.now().UTC()
	filename := "virtmgr-database-" + now.Format("20060102-150405") + ".db"
	path := filepath.Join(s.tempDir, "virtmgr-export-"+now.Format("20060102-150405.000000000")+".db")
	if err := s.exporter.ExportDatabase(ctx, path); err != nil {
		s.recordAudit(ctx, request, "failure", 0, err)
		return DatabaseExport{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		_ = os.Remove(path)
		s.recordAudit(ctx, request, "failure", 0, err)
		return DatabaseExport{}, fmt.Errorf("stat database export: %w", err)
	}
	if err := s.recordAudit(ctx, request, "success", info.Size(), nil); err != nil {
		_ = os.Remove(path)
		return DatabaseExport{}, err
	}
	return DatabaseExport{Path: path, Filename: filename, Size: info.Size()}, nil
}

func (s *Service) DatabaseRestoreStatus() (domain.DatabaseRestoreStatus, error) {
	if s.restorer == nil || s.databasePath == "" {
		return domain.DatabaseRestoreStatus{}, domain.ErrUnavailable
	}
	return s.restorer.DatabaseRestoreStatus(s.databasePath)
}

func (s *Service) StageDatabaseRestore(ctx context.Context, body io.Reader, size int64, filename string, request MutationContext) (domain.DatabaseRestoreStatus, error) {
	if request.Session.User.Role != domain.RoleAdmin {
		return domain.DatabaseRestoreStatus{}, domain.ErrForbidden
	}
	if s.restorer == nil || s.databasePath == "" {
		return domain.DatabaseRestoreStatus{}, domain.ErrUnavailable
	}
	status, err := s.restorer.StageDatabaseRestore(ctx, s.databasePath, body, size, filename, request.Session.User.Username)
	if err != nil {
		_ = s.recordDatabaseAudit(ctx, request, "system.database.restore.stage", "failure", map[string]any{
			"filename": filename, "sizeBytes": size, "error": err.Error(),
		})
		return domain.DatabaseRestoreStatus{}, err
	}
	if err := s.recordDatabaseAudit(ctx, request, "system.database.restore.stage", "success", map[string]any{
		"filename": status.Filename, "sizeBytes": status.SizeBytes, "schemaVersion": status.SchemaVersion,
	}); err != nil {
		return domain.DatabaseRestoreStatus{}, err
	}
	return status, nil
}

func (s *Service) CancelDatabaseRestore(ctx context.Context, request MutationContext) (domain.DatabaseRestoreStatus, error) {
	if request.Session.User.Role != domain.RoleAdmin {
		return domain.DatabaseRestoreStatus{}, domain.ErrForbidden
	}
	if s.restorer == nil || s.databasePath == "" {
		return domain.DatabaseRestoreStatus{}, domain.ErrUnavailable
	}
	status, err := s.restorer.CancelDatabaseRestore(s.databasePath)
	if err != nil {
		_ = s.recordDatabaseAudit(ctx, request, "system.database.restore.cancel", "failure", map[string]any{"error": err.Error()})
		return domain.DatabaseRestoreStatus{}, err
	}
	if err := s.recordDatabaseAudit(ctx, request, "system.database.restore.cancel", "success", nil); err != nil {
		return domain.DatabaseRestoreStatus{}, err
	}
	return status, nil
}

func (s *Service) recordDatabaseAudit(ctx context.Context, request MutationContext, action, result string, detail map[string]any) error {
	if s.audit == nil {
		return nil
	}
	if detail == nil {
		detail = map[string]any{}
	}
	return s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: s.now().UTC(), RequestID: request.RequestID,
		Actor: request.Session.User.Username, Role: request.Session.User.Role,
		Action: action, TargetKind: "system", TargetID: "database",
		SourceIP: request.SourceIP, Result: result, Detail: detail,
	})
}

func (s *Service) recordAudit(ctx context.Context, request MutationContext, result string, size int64, cause error) error {
	if s.audit == nil {
		return cause
	}
	detail := map[string]any{"sizeBytes": size}
	if cause != nil {
		detail["error"] = cause.Error()
	}
	if err := s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: s.now().UTC(), RequestID: request.RequestID,
		Actor: request.Session.User.Username, Role: request.Session.User.Role,
		Action: "system.database.export", TargetKind: "system", TargetID: "database",
		SourceIP: request.SourceIP, Result: result, Detail: detail,
	}); err != nil {
		return fmt.Errorf("record database export audit: %w", err)
	}
	return cause
}
