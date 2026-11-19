package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"virtmgr/internal/domain"
	dataplaneapp "virtmgr/internal/modules/dataplane/app"
	"virtmgr/internal/platform/authctx"

	"github.com/go-chi/chi/v5"
)

type handlerStore struct {
	items []domain.DataPlaneOperation
}

func (s *handlerStore) ListPendingDataPlaneOperations(context.Context) ([]domain.DataPlaneOperation, error) {
	return nil, nil
}

func (s *handlerStore) ListDataPlaneOperations(context.Context, int) ([]domain.DataPlaneOperation, error) {
	return append([]domain.DataPlaneOperation(nil), s.items...), nil
}

func (s *handlerStore) ListUnresolvedDataPlaneOperations(context.Context) ([]domain.DataPlaneOperation, error) {
	return nil, nil
}

func (s *handlerStore) GetDataPlaneOperation(_ context.Context, id string) (domain.DataPlaneOperation, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.DataPlaneOperation{}, domain.ErrNotFound
}

func (s *handlerStore) FinishDataPlaneOperation(context.Context, string, string, string, time.Time) error {
	return nil
}

func (s *handlerStore) ResolveDataPlaneOperation(_ context.Context, id, status, message string, _ time.Time) error {
	for index := range s.items {
		if s.items[index].ID == id {
			s.items[index].Status = status
			s.items[index].Error = message
		}
	}
	return nil
}

func (s *handlerStore) UpdateInterruptedTaskDetail(context.Context, string, string, string, time.Time) error {
	return nil
}

type handlerBackend struct{}

func (handlerBackend) RecoverDataPlaneOperation(context.Context, domain.DataPlaneOperation) (domain.DataPlaneRecovery, error) {
	return domain.DataPlaneRecovery{Status: domain.DataPlaneRecoveryRolledBack, Message: "isolated volume removed"}, nil
}

func newDataPlaneRouter(store *handlerStore) *chi.Mux {
	service := dataplaneapp.New(store, nil, handlerBackend{}, nil, nil)
	router := chi.NewRouter()
	New(service).Mount(router, func(next http.Handler) http.Handler { return next })
	return router
}

func dataPlaneRequest(method, target string, role *domain.Role) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	if role == nil {
		return request
	}
	return request.WithContext(authctx.WithSession(request.Context(), domain.Session{
		User: domain.User{ID: 1, Username: "user", Role: *role},
	}))
}

func TestListRequiresAdmin(t *testing.T) {
	router := newDataPlaneRouter(&handlerStore{items: []domain.DataPlaneOperation{{ID: "op-1"}}})
	admin := domain.RoleAdmin
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, dataPlaneRequest(http.MethodGet, "/dataplane/operations", &admin))
	if recorder.Code != http.StatusOK {
		t.Fatalf("admin status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result domain.DataPlaneOperationList
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Items[0].ID != "op-1" {
		t.Fatalf("list result = %+v", result)
	}

	for name, role := range map[string]*domain.Role{"viewer": func() *domain.Role { value := domain.RoleViewer; return &value }(), "anonymous": nil} {
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, dataPlaneRequest(http.MethodGet, "/dataplane/operations", role))
		want := http.StatusForbidden
		if role == nil {
			want = http.StatusUnauthorized
		}
		if recorder.Code != want {
			t.Fatalf("%s status = %d body=%s, want %d", name, recorder.Code, recorder.Body.String(), want)
		}
	}
}

func TestMutationRequiresAdmin(t *testing.T) {
	store := &handlerStore{items: []domain.DataPlaneOperation{{
		ID: "op-1", VMUUID: "vm-1", Status: domain.DataPlaneStatusNeedsAttention,
	}}}
	router := newDataPlaneRouter(store)
	viewer := domain.RoleViewer
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, dataPlaneRequest(http.MethodPost, "/dataplane/operations/op-1/recheck", &viewer))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if store.items[0].Status != domain.DataPlaneStatusNeedsAttention {
		t.Fatalf("viewer changed operation = %+v", store.items[0])
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, dataPlaneRequest(http.MethodPost, "/dataplane/operations/op-1/unblock", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAdminRecheckReturnsRecoveryResult(t *testing.T) {
	store := &handlerStore{items: []domain.DataPlaneOperation{{
		ID: "op-1", VMUUID: "vm-1", Status: domain.DataPlaneStatusNeedsAttention,
	}}}
	router := newDataPlaneRouter(store)
	admin := domain.RoleAdmin
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, dataPlaneRequest(http.MethodPost, "/dataplane/operations/op-1/recheck", &admin))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result domain.DataPlaneOperationResult
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Operation.Status != domain.DataPlaneStatusRecovered || result.Recovery.Status != domain.DataPlaneRecoveryRolledBack {
		t.Fatalf("result = %+v", result)
	}
}
