package snapshotpolicieshttp

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"virtmgr/internal/domain"
	snapshotpoliciesapp "virtmgr/internal/modules/snapshotpolicies/app"
	"virtmgr/internal/platform/authctx"

	"github.com/go-chi/chi/v5"
)

type handlerPolicyRepository struct {
	policies map[int64]domain.SnapshotPolicy
	next     int64
}

func newHandlerPolicyRepository() *handlerPolicyRepository {
	return &handlerPolicyRepository{policies: map[int64]domain.SnapshotPolicy{}}
}

func (r *handlerPolicyRepository) ListSnapshotPolicies(_ context.Context, vmUUID string) ([]domain.SnapshotPolicy, error) {
	items := make([]domain.SnapshotPolicy, 0)
	for _, policy := range r.policies {
		if policy.VMUUID == vmUUID {
			items = append(items, policy)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (r *handlerPolicyRepository) GetSnapshotPolicy(_ context.Context, id int64) (domain.SnapshotPolicy, error) {
	policy, ok := r.policies[id]
	if !ok {
		return domain.SnapshotPolicy{}, domain.ErrNotFound
	}
	return policy, nil
}

func (r *handlerPolicyRepository) CreateSnapshotPolicy(_ context.Context, policy domain.SnapshotPolicy) (domain.SnapshotPolicy, error) {
	r.next++
	policy.ID = r.next
	r.policies[policy.ID] = policy
	return policy, nil
}

func (r *handlerPolicyRepository) UpdateSnapshotPolicy(_ context.Context, policy domain.SnapshotPolicy) (domain.SnapshotPolicy, error) {
	if _, ok := r.policies[policy.ID]; !ok {
		return domain.SnapshotPolicy{}, domain.ErrNotFound
	}
	r.policies[policy.ID] = policy
	return policy, nil
}

func (r *handlerPolicyRepository) DeleteSnapshotPolicy(_ context.Context, id int64) error {
	if _, ok := r.policies[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.policies, id)
	return nil
}

func (r *handlerPolicyRepository) ListDueSnapshotPolicies(context.Context, time.Time, int) ([]domain.SnapshotPolicy, error) {
	return nil, nil
}

type handlerVMRepository struct {
	snapshots map[string][]domain.VMSnapshot
	next      int64
}

func (r *handlerVMRepository) GetVM(_ context.Context, uuid string) (domain.VM, error) {
	return domain.VM{UUID: uuid, Name: "api-01"}, nil
}

func (r *handlerVMRepository) ListSnapshots(_ context.Context, uuid string) ([]domain.VMSnapshot, error) {
	return append([]domain.VMSnapshot(nil), r.snapshots[uuid]...), nil
}

func (r *handlerVMRepository) CreateSnapshot(_ context.Context, uuid string, spec domain.VMSnapshotCreateSpec) (domain.VMSnapshot, error) {
	r.next++
	snapshot := domain.VMSnapshot{Name: spec.Name, Description: spec.Description, CreatedAt: r.next, State: domain.VMStateShutoff, Current: true}
	r.snapshots[uuid] = append(r.snapshots[uuid], snapshot)
	return snapshot, nil
}

func (r *handlerVMRepository) DeleteSnapshot(_ context.Context, uuid, name string) error {
	snapshots := r.snapshots[uuid]
	for index, snapshot := range snapshots {
		if snapshot.Name == name {
			r.snapshots[uuid] = append(snapshots[:index], snapshots[index+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func newHandlerRouterWithSession(session *domain.Session, csrfOK bool) (*chi.Mux, *handlerPolicyRepository) {
	repository := newHandlerPolicyRepository()
	router := chi.NewRouter()
	router.Use(func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			if session != nil {
				r = r.WithContext(authctx.WithSession(r.Context(), *session))
			}
			next.ServeHTTP(w, r)
		})
	})
	handler := New(snapshotpoliciesapp.New(repository, &handlerVMRepository{snapshots: map[string][]domain.VMSnapshot{}}, nil, false))
	handler.Mount(router, func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			if !csrfOK {
				w.WriteHeader(stdhttp.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	return router, repository
}

func operatorHandlerSession() *domain.Session {
	return &domain.Session{User: domain.User{ID: 1, Username: "operator", Role: domain.RoleOperator}}
}

func TestSnapshotPolicyRoutesLifecycle(t *testing.T) {
	router, _ := newHandlerRouterWithSession(operatorHandlerSession(), true)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/vms/vm-a/snapshot-policies", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("initial list status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var list domain.SnapshotPolicyList
	if err := json.NewDecoder(recorder.Body).Decode(&list); err != nil || list.Total != 0 {
		t.Fatalf("initial list = %+v, %v", list, err)
	}

	request := httptest.NewRequest(stdhttp.MethodPost, "/vms/vm-a/snapshot-policies", strings.NewReader(`{"name":"nightly","intervalMinutes":60,"retention":3,"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("create status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var created domain.SnapshotPolicy
	if err := json.NewDecoder(recorder.Body).Decode(&created); err != nil || created.ID != 1 || created.Name != "nightly" {
		t.Fatalf("created = %+v, %v", created, err)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/vms/vm-a/snapshot-policies", nil))
	if err := json.NewDecoder(recorder.Body).Decode(&list); err != nil || list.Total != 1 {
		t.Fatalf("list = %+v, %v status=%d", list, err, recorder.Code)
	}

	request = httptest.NewRequest(stdhttp.MethodPatch, "/snapshot-policies/1", strings.NewReader(`{"name":"weekly","intervalMinutes":120,"retention":5,"enabled":false}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("update status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var updated domain.SnapshotPolicy
	if err := json.NewDecoder(recorder.Body).Decode(&updated); err != nil || updated.Name != "weekly" || updated.Enabled {
		t.Fatalf("updated = %+v, %v", updated, err)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/snapshot-policies/1/run", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("run status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result domain.SnapshotPolicyRunResult
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil || result.Snapshot == nil || result.Policy.LastSnapshot != result.Snapshot.Name {
		t.Fatalf("run result = %+v, %v", result, err)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodDelete, "/snapshot-policies/1", nil))
	if recorder.Code != stdhttp.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/vms/vm-a/snapshot-policies", nil))
	if err := json.NewDecoder(recorder.Body).Decode(&list); err != nil || list.Total != 0 {
		t.Fatalf("final list = %+v, %v", list, err)
	}
}

func TestSnapshotPolicyRoutesRequireSessionCSRFAndValidateInput(t *testing.T) {
	router, repository := newHandlerRouterWithSession(nil, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/vms/vm-a/snapshot-policies", strings.NewReader(`{"name":"nightly","intervalMinutes":60,"retention":3,"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("anonymous create status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	router, repository = newHandlerRouterWithSession(operatorHandlerSession(), false)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPost, "/vms/vm-a/snapshot-policies", strings.NewReader(`{"name":"nightly","intervalMinutes":60,"retention":3,"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden || len(repository.policies) != 0 {
		t.Fatalf("csrf create status = %d policies=%+v", recorder.Code, repository.policies)
	}

	router, _ = newHandlerRouterWithSession(operatorHandlerSession(), true)
	for name, test := range map[string]struct {
		method string
		path   string
		body   string
		status int
	}{
		"invalid id":       {method: stdhttp.MethodPatch, path: "/snapshot-policies/nope", body: `{}`, status: stdhttp.StatusBadRequest},
		"zero id":          {method: stdhttp.MethodDelete, path: "/snapshot-policies/0", status: stdhttp.StatusBadRequest},
		"unknown field":    {method: stdhttp.MethodPost, path: "/vms/vm-a/snapshot-policies", body: `{"name":"nightly","intervalMinutes":60,"retention":3,"enabled":true,"extra":1}`, status: stdhttp.StatusBadRequest},
		"invalid interval": {method: stdhttp.MethodPost, path: "/vms/vm-a/snapshot-policies", body: `{"name":"nightly","intervalMinutes":1,"retention":3,"enabled":true}`, status: stdhttp.StatusBadRequest},
		"missing update":   {method: stdhttp.MethodPatch, path: "/snapshot-policies/99", body: `{"name":"nightly","intervalMinutes":60,"retention":3,"enabled":true}`, status: stdhttp.StatusNotFound},
		"missing run":      {method: stdhttp.MethodPost, path: "/snapshot-policies/99/run", status: stdhttp.StatusNotFound},
		"missing delete":   {method: stdhttp.MethodDelete, path: "/snapshot-policies/99", status: stdhttp.StatusNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status = %d want %d body=%s", recorder.Code, test.status, recorder.Body.String())
			}
		})
	}

	viewer := &domain.Session{User: domain.User{ID: 2, Username: "viewer", Role: domain.RoleViewer}}
	router, _ = newHandlerRouterWithSession(viewer, true)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPost, "/vms/vm-a/snapshot-policies", strings.NewReader(`{"name":"nightly","intervalMinutes":60,"retention":3,"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden {
		t.Fatalf("viewer create status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
