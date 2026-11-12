package app

import (
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type fakeRepository struct {
	result        domain.AuditEventList
	exported      []domain.AuditEvent
	err           error
	received      domain.AuditEventQuery
	exportedQuery domain.AuditEventQuery
	called        bool
}

func (f *fakeRepository) ListAuditEvents(_ context.Context, query domain.AuditEventQuery) (domain.AuditEventList, error) {
	f.called = true
	f.received = query
	return f.result, f.err
}

func (f *fakeRepository) ExportAuditEvents(_ context.Context, query domain.AuditEventQuery) ([]domain.AuditEvent, error) {
	f.exportedQuery = query
	return f.exported, f.err
}

func adminSession() domain.Session {
	return domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}
}

func TestListNormalizesQuery(t *testing.T) {
	repository := &fakeRepository{result: domain.AuditEventList{Total: 1, Items: []domain.AuditEvent{{ID: 1}}}}
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	result, err := New(repository).List(context.Background(), adminSession(), domain.AuditEventQuery{
		Actor: " admin ", Action: " vm.start ", Result: " success ", TargetKind: " vm ", TargetID: " uuid-a ",
		From: from, To: to, Page: 0, PageSize: 500,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("result = %+v", result)
	}
	want := domain.AuditEventQuery{
		Actor: "admin", Action: "vm.start", Result: "success", TargetKind: "vm", TargetID: "uuid-a",
		From: from, To: to, Page: 1, PageSize: domain.MaxAuditPageSize,
	}
	if repository.received != want {
		t.Fatalf("repository query = %+v, want %+v", repository.received, want)
	}
}

func TestListRejectsInvalidTimeRange(t *testing.T) {
	repository := &fakeRepository{}
	from := time.Now().UTC()
	_, err := New(repository).List(context.Background(), adminSession(), domain.AuditEventQuery{From: from, To: from.Add(-time.Minute)})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("error = %v", err)
	}
	if repository.called {
		t.Fatal("invalid query reached repository")
	}
}

func TestExportProducesSafeCSVAndAudits(t *testing.T) {
	repository := &fakeRepository{exported: []domain.AuditEvent{{
		ID: 1, OccurredAt: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC),
		RequestID: "req-1", Actor: "=cmd", Role: domain.RoleAdmin, Action: "vm.create",
		TargetKind: "vm", TargetID: "@target", SourceIP: "127.0.0.1", Result: "success",
		Detail: map[string]any{"name": "+payload"},
	}}}
	audit := &auditRecorder{}
	result, err := New(repository, WithAuditRecorder(audit)).Export(context.Background(), ExportCommand{
		Session: adminSession(), Format: domain.AuditExportFormatCSV,
		Query: domain.AuditEventQuery{TargetID: " target "}, RequestID: "export-req", SourceIP: "10.0.0.1",
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if result.EventCount != 1 || !strings.HasPrefix(result.ContentType, "text/csv") {
		t.Fatalf("result = %+v", result)
	}
	records, err := csv.NewReader(strings.NewReader(string(result.Data))).ReadAll()
	if err != nil || len(records) != 2 {
		t.Fatalf("records = %+v, error = %v", records, err)
	}
	if !strings.HasPrefix(records[1][3], "'=") || !strings.HasPrefix(records[1][7], "'@") || !strings.Contains(records[1][10], "\"name\":\"+payload\"") {
		t.Fatalf("CSV formula cells were not escaped: %+v", records[1])
	}
	if repository.exportedQuery.TargetID != "target" || len(audit.events) != 1 || audit.events[0].Action != "audit.export" || audit.events[0].Detail["eventCount"] != 1 {
		t.Fatalf("query = %+v, audit = %+v", repository.exportedQuery, audit.events)
	}
}

func TestExportRejectsInvalidRequests(t *testing.T) {
	repository := &fakeRepository{}
	if _, err := New(repository).Export(context.Background(), ExportCommand{Session: domain.Session{User: domain.User{Role: domain.RoleViewer}}, Format: domain.AuditExportFormatJSON}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("viewer error = %v", err)
	}
	if _, err := New(repository).Export(context.Background(), ExportCommand{Session: adminSession(), Format: "xml"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("format error = %v", err)
	}
}

type auditRecorder struct{ events []domain.AuditEvent }

func (a *auditRecorder) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	a.events = append(a.events, event)
	return nil
}

func TestListRequiresAdmin(t *testing.T) {
	for _, role := range []domain.Role{domain.RoleViewer, domain.RoleOperator} {
		repository := &fakeRepository{}
		_, err := New(repository).List(context.Background(), domain.Session{User: domain.User{ID: 2, Role: role}}, domain.AuditEventQuery{})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("role %s error = %v", role, err)
		}
		if repository.called {
			t.Fatalf("role %s reached repository", role)
		}
	}
}

func TestListRejectsInvalidResult(t *testing.T) {
	repository := &fakeRepository{}
	_, err := New(repository).List(context.Background(), adminSession(), domain.AuditEventQuery{Result: "unknown"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("error = %v", err)
	}
	if repository.called {
		t.Fatal("invalid query reached repository")
	}
}

func TestListPropagatesRepositoryError(t *testing.T) {
	want := errors.New("database unavailable")
	_, err := New(&fakeRepository{err: want}).List(context.Background(), adminSession(), domain.AuditEventQuery{})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}
