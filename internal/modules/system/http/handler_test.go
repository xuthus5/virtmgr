package systemhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"virtmgr/internal/domain"
	systemapp "virtmgr/internal/modules/system/app"
	"virtmgr/internal/platform/authctx"

	"github.com/go-chi/chi/v5"
)

type restoreRepository struct{}

func (restoreRepository) ExportDatabase(context.Context, string) error { return nil }

func (restoreRepository) StageDatabaseRestore(context.Context, string, io.Reader, int64, string, string) (domain.DatabaseRestoreStatus, error) {
	return domain.DatabaseRestoreStatus{}, nil
}

func (restoreRepository) DatabaseRestoreStatus(string) (domain.DatabaseRestoreStatus, error) {
	return domain.DatabaseRestoreStatus{State: domain.DatabaseRestorePendingRestart, Filename: "backup.db"}, nil
}

func (restoreRepository) CancelDatabaseRestore(string) (domain.DatabaseRestoreStatus, error) {
	return domain.DatabaseRestoreStatus{State: domain.DatabaseRestoreNone}, nil
}

func systemRequest(method, target string, role *domain.Role) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	if role == nil {
		return request
	}
	return request.WithContext(authctx.WithSession(request.Context(), domain.Session{
		User: domain.User{ID: 1, Username: "user", Role: *role},
	}))
}

func TestDatabaseRestoreStatusRequiresAdmin(t *testing.T) {
	router := chi.NewRouter()
	New(systemapp.New(restoreRepository{}, nil, systemapp.WithDatabasePath("/tmp/virtmgr.db"))).Mount(router, func(next http.Handler) http.Handler { return next })

	viewer := domain.RoleViewer
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, systemRequest(http.MethodGet, "/system/database-restore", &viewer))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, systemRequest(http.MethodGet, "/system/database-restore", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDatabaseRestoreStatusReturnsAdminView(t *testing.T) {
	router := chi.NewRouter()
	New(systemapp.New(restoreRepository{}, nil, systemapp.WithDatabasePath("/tmp/virtmgr.db"))).Mount(router, func(next http.Handler) http.Handler { return next })
	admin := domain.RoleAdmin
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, systemRequest(http.MethodGet, "/system/database-restore", &admin))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var status domain.DatabaseRestoreStatus
	if err := json.NewDecoder(recorder.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.State != domain.DatabaseRestorePendingRestart || status.Filename != "backup.db" {
		t.Fatalf("status = %+v", status)
	}
}
