package app

import (
	"context"
	"fmt"
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

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
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
	exporter DatabaseExporter
	audit    AuditRecorder
	tempDir  string
	now      func() time.Time
}

func New(exporter DatabaseExporter, audit AuditRecorder) *Service {
	return &Service{
		exporter: exporter,
		audit:    audit,
		tempDir:  os.TempDir(),
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Info() domain.SystemInfo {
	return domain.SystemInfo{Build: domain.BuildInfo{
		Version: strings.TrimSpace(version.Version),
		Commit:  strings.TrimSpace(version.Commit),
		Date:    strings.TrimSpace(version.Date),
	}}
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
