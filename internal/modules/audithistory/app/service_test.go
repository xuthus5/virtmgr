package app

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
)

type fakeRepository struct {
	result   domain.AuditEventList
	err      error
	received domain.AuditEventQuery
	called   bool
}

func (f *fakeRepository) ListAuditEvents(_ context.Context, query domain.AuditEventQuery) (domain.AuditEventList, error) {
	f.called = true
	f.received = query
	return f.result, f.err
}

func adminSession() domain.Session {
	return domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}
}

func TestListNormalizesQuery(t *testing.T) {
	repository := &fakeRepository{result: domain.AuditEventList{Total: 1, Items: []domain.AuditEvent{{ID: 1}}}}
	result, err := New(repository).List(context.Background(), adminSession(), domain.AuditEventQuery{
		Actor: " admin ", Action: " vm.start ", Result: " success ", TargetKind: " vm ", Page: 0, PageSize: 500,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("result = %+v", result)
	}
	want := domain.AuditEventQuery{
		Actor: "admin", Action: "vm.start", Result: "success", TargetKind: "vm", Page: 1, PageSize: domain.MaxAuditPageSize,
	}
	if repository.received != want {
		t.Fatalf("repository query = %+v, want %+v", repository.received, want)
	}
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
