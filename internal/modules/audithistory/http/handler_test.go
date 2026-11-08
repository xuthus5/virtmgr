package audithistoryhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"virtmgr/internal/domain"
	audithistoryapp "virtmgr/internal/modules/audithistory/app"
	"virtmgr/internal/platform/authctx"

	"github.com/go-chi/chi/v5"
)

type handlerRepository struct {
	result   domain.AuditEventList
	received domain.AuditEventQuery
}

func (f *handlerRepository) ListAuditEvents(_ context.Context, query domain.AuditEventQuery) (domain.AuditEventList, error) {
	f.received = query
	return f.result, nil
}

func withSession(request *http.Request, role domain.Role) *http.Request {
	return request.WithContext(authctx.WithSession(request.Context(), domain.Session{
		User: domain.User{ID: 1, Username: "admin", Role: role},
	}))
}

func TestAuditHistoryRoute(t *testing.T) {
	repository := &handlerRepository{result: domain.AuditEventList{
		Items: []domain.AuditEvent{{ID: 1, Action: "vm.start"}}, Total: 1, Page: 2, PageSize: 20,
	}}
	router := chi.NewRouter()
	New(audithistoryapp.New(repository)).Mount(router)
	request := withSession(httptest.NewRequest(http.MethodGet, "/audit-events?actor=admin&action=vm.start&result=success&targetKind=vm&page=2&pageSize=20", nil), domain.RoleAdmin)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result domain.AuditEventList
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Items[0].Action != "vm.start" {
		t.Fatalf("result = %+v", result)
	}
	want := domain.AuditEventQuery{Actor: "admin", Action: "vm.start", Result: "success", TargetKind: "vm", Page: 2, PageSize: 20}
	if repository.received != want {
		t.Fatalf("query = %+v, want %+v", repository.received, want)
	}
}

func TestAuditHistoryRouteRequiresAdmin(t *testing.T) {
	router := chi.NewRouter()
	New(audithistoryapp.New(&handlerRepository{})).Mount(router)
	request := withSession(httptest.NewRequest(http.MethodGet, "/audit-events", nil), domain.RoleViewer)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAuditHistoryRouteRequiresSession(t *testing.T) {
	router := chi.NewRouter()
	New(audithistoryapp.New(&handlerRepository{})).Mount(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/audit-events", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAuditHistoryRouteRejectsInvalidPagination(t *testing.T) {
	router := chi.NewRouter()
	New(audithistoryapp.New(&handlerRepository{})).Mount(router)
	request := withSession(httptest.NewRequest(http.MethodGet, "/audit-events?page=invalid", nil), domain.RoleAdmin)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
