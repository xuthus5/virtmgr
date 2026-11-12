package authhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"virtmgr/internal/domain"
	authapp "virtmgr/internal/modules/auth/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/sqlite"

	"github.com/go-chi/chi/v5"
)

func newTestHandler(t *testing.T) (*Handler, *authapp.Service) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	store := sqlite.NewStore(db)
	service := authapp.New(store, store, time.Hour)
	if err := service.EnsureAdmin(context.Background(), "admin", "secret"); err != nil {
		t.Fatalf("EnsureAdmin() error = %v", err)
	}
	return New(service, false, time.Hour, true), service
}

func TestLoginGetLogout(t *testing.T) {
	handler, _ := newTestHandler(t)
	router := chi.NewRouter()
	router.Route("/api/v1", handler.Mount)

	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "secret"})
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewReader(loginBody))
	loginRequest.RemoteAddr = "192.0.2.40:1234"
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
	cookies := loginRecorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != CookieName || !cookies[0].HttpOnly {
		t.Fatalf("cookies = %+v", cookies)
	}
	var loginResponse sessionResponse
	if err := json.Unmarshal(loginRecorder.Body.Bytes(), &loginResponse); err != nil {
		t.Fatal(err)
	}
	if loginResponse.User.Username != "admin" || loginResponse.CSRFToken == "" || !loginResponse.ReadOnly {
		t.Fatalf("login response = %+v", loginResponse)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	getRequest.AddCookie(cookies[0])
	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("get session status = %d body=%s", getRecorder.Code, getRecorder.Body.String())
	}

	logoutRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/session", nil)
	logoutRequest.AddCookie(cookies[0])
	logoutRecorder := httptest.NewRecorder()
	router.ServeHTTP(logoutRecorder, logoutRequest)
	if logoutRecorder.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", logoutRecorder.Code)
	}
}

type mutableReadOnly struct{ value bool }

func (m *mutableReadOnly) ReadOnly() bool { return m.value }

func TestSessionUsesDynamicReadOnlyMode(t *testing.T) {
	base, service := newTestHandler(t)
	mode := &mutableReadOnly{value: true}
	handler := New(service, false, time.Hour, true, WithReadOnlyMode(mode))
	session := domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}
	if !handler.response(session).ReadOnly {
		t.Fatal("initial session is not read-only")
	}
	mode.value = false
	if handler.response(session).ReadOnly {
		t.Fatal("session did not observe dynamic read-only change")
	}
	if !base.response(session).ReadOnly {
		t.Fatal("static fallback behavior changed")
	}
}

func TestSessionRequiresCookie(t *testing.T) {
	handler, _ := newTestHandler(t)
	router := chi.NewRouter()
	router.Route("/api/v1", handler.Mount)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestLoginRejectsBadPassword(t *testing.T) {
	handler, _ := newTestHandler(t)
	router := chi.NewRouter()
	router.Route("/api/v1", handler.Mount)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestLoginRateLimitReturnsTooManyRequests(t *testing.T) {
	handler, _ := newTestHandler(t)
	router := chi.NewRouter()
	router.Route("/api/v1", handler.Mount)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	for attempt := 0; attempt < 5; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewReader(body))
		request.RemoteAddr = "192.0.2.50:1234"
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d", attempt+1, recorder.Code)
		}
	}
	body, _ = json.Marshal(map[string]string{"username": "admin", "password": "secret"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewReader(body))
	request.RemoteAddr = "192.0.2.50:1234"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRequireSession(t *testing.T) {
	handler, _ := newTestHandler(t)
	called := false
	protected := handler.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if _, ok := SessionFromContext(r.Context()); !ok {
			t.Fatal("session missing from context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	protected.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusUnauthorized || called {
		t.Fatalf("status = %d called=%v", recorder.Code, called)
	}
}

func TestRequireCSRF(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := domain.Session{User: domain.User{ID: 1, Username: "admin"}, CSRFToken: "csrf-token"}
	protected := handler.RequireCSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	base := httptest.NewRequest(http.MethodPost, "/", nil)
	base = base.WithContext(authctx.WithSession(base.Context(), session))

	recorder := httptest.NewRecorder()
	protected.ServeHTTP(recorder, base)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d", recorder.Code)
	}

	wrong := httptest.NewRequest(http.MethodPost, "/", nil)
	wrong.Header.Set("X-CSRF-Token", "wrong")
	wrong = wrong.WithContext(authctx.WithSession(wrong.Context(), session))
	recorder = httptest.NewRecorder()
	protected.ServeHTTP(recorder, wrong)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("wrong csrf status = %d", recorder.Code)
	}

	valid := httptest.NewRequest(http.MethodPost, "/", nil)
	valid.Header.Set("X-CSRF-Token", "csrf-token")
	valid = valid.WithContext(authctx.WithSession(valid.Context(), session))
	recorder = httptest.NewRecorder()
	protected.ServeHTTP(recorder, valid)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("valid csrf status = %d", recorder.Code)
	}
}
