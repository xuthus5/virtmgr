package usershttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"virtmgr/internal/domain"
	usersapp "virtmgr/internal/modules/users/app"
	"virtmgr/internal/platform/authctx"

	"github.com/go-chi/chi/v5"
)

type handlerRepository struct {
	users map[int64]domain.User
	next  int64
}

func (f *handlerRepository) ListUsers(context.Context) ([]domain.User, error) {
	users := make([]domain.User, 0, len(f.users))
	for _, user := range f.users {
		users = append(users, user)
	}
	return users, nil
}

func (f *handlerRepository) UserByID(_ context.Context, id int64) (domain.User, error) {
	user, ok := f.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}

func (f *handlerRepository) UserByUsername(_ context.Context, username string) (domain.User, error) {
	for _, user := range f.users {
		if user.Username == username {
			return user, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

func (f *handlerRepository) CreateUser(_ context.Context, username, passwordHash string, role domain.Role) (domain.User, error) {
	f.next++
	user := domain.User{ID: f.next, Username: username, PasswordHash: passwordHash, Role: role}
	f.users[user.ID] = user
	return user, nil
}

func (f *handlerRepository) UpdateUser(_ context.Context, id int64, role domain.Role, disabled bool) (domain.User, error) {
	user, ok := f.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	user.Role = role
	user.Disabled = disabled
	f.users[id] = user
	return user, nil
}

func allowAll(next http.Handler) http.Handler { return next }

func newTestRouter() (*chi.Mux, *handlerRepository) {
	repository := &handlerRepository{users: map[int64]domain.User{
		1: {ID: 1, Username: "admin", Role: domain.RoleAdmin},
	}, next: 1}
	router := chi.NewRouter()
	New(usersapp.New(repository, nil, false)).Mount(router, allowAll)
	return router, repository
}

func withSession(request *http.Request, role domain.Role) *http.Request {
	return request.WithContext(authctx.WithSession(request.Context(), domain.Session{
		User: domain.User{ID: 1, Username: "admin", Role: role},
	}))
}

func TestUserRoutes(t *testing.T) {
	router, _ := newTestRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, withSession(httptest.NewRequest(http.MethodGet, "/users", nil), domain.RoleAdmin))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var list domain.UserList
	if err := json.NewDecoder(recorder.Body).Decode(&list); err != nil || list.Total != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}

	create := withSession(httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"username":"viewer","password":"viewer-secret-123","role":"viewer"}`)), domain.RoleAdmin)
	create.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, create)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var created domain.User
	if err := json.NewDecoder(recorder.Body).Decode(&created); err != nil || created.Username != "viewer" || created.ID != 2 {
		t.Fatalf("created user = %+v, %v", created, err)
	}

	update := withSession(httptest.NewRequest(http.MethodPatch, "/users/2", strings.NewReader(`{"role":"operator","disabled":true}`)), domain.RoleAdmin)
	update.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, update)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var updated domain.User
	if err := json.NewDecoder(recorder.Body).Decode(&updated); err != nil || updated.Role != domain.RoleOperator || !updated.Disabled {
		t.Fatalf("updated user = %+v, %v", updated, err)
	}
}

func TestUserRoutesRequireAdmin(t *testing.T) {
	router, _ := newTestRouter()
	for _, request := range []*http.Request{
		withSession(httptest.NewRequest(http.MethodGet, "/users", nil), domain.RoleViewer),
		withSession(httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"username":"viewer","password":"viewer-secret-123","role":"viewer"}`)), domain.RoleOperator),
	} {
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("%s %s status = %d body=%s", request.Method, request.URL.Path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestUserRoutesRequireSessionAndValidateID(t *testing.T) {
	router, _ := newTestRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/users", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	request := withSession(httptest.NewRequest(http.MethodPatch, "/users/nope", strings.NewReader(`{"role":"viewer","disabled":false}`)), domain.RoleAdmin)
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid id status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestUserRouteRejectsSelfModification(t *testing.T) {
	router, _ := newTestRouter()
	request := withSession(httptest.NewRequest(http.MethodPatch, "/users/1", strings.NewReader(`{"role":"viewer","disabled":true}`)), domain.RoleAdmin)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
