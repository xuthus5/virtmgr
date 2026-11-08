package settingshttp

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"virtmgr/internal/domain"
	settingsapp "virtmgr/internal/modules/settings/app"
	"virtmgr/internal/platform/authctx"

	"github.com/go-chi/chi/v5"
)

type testMode struct{ readOnly bool }

func (m *testMode) ReadOnly() bool { return m.readOnly }
func (m *testMode) SetReadOnly(_ context.Context, readOnly bool) error {
	m.readOnly = readOnly
	return nil
}

func newTestRouter(mode *testMode, role domain.Role, csrfOK bool) *chi.Mux {
	router := chi.NewRouter()
	router.Use(func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			r = r.WithContext(authctx.WithSession(r.Context(), domain.Session{
				User: domain.User{ID: 1, Username: "admin", Role: role},
			}))
			next.ServeHTTP(w, r)
		})
	})
	handler := New(settingsapp.New(mode, nil))
	handler.Mount(router, func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			if !csrfOK {
				w.WriteHeader(stdhttp.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	return router
}

func TestGetAndUpdateSettings(t *testing.T) {
	mode := &testMode{readOnly: true}
	router := newTestRouter(mode, domain.RoleAdmin, true)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/settings", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("get status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var settings domain.Settings
	if err := json.NewDecoder(recorder.Body).Decode(&settings); err != nil || !settings.ReadOnly {
		t.Fatalf("settings = %+v error=%v", settings, err)
	}

	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPut, "/settings", strings.NewReader(`{"readOnly":false}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("put status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := json.NewDecoder(recorder.Body).Decode(&settings); err != nil || settings.ReadOnly || mode.readOnly {
		t.Fatalf("updated settings = %+v mode=%v error=%v", settings, mode.readOnly, err)
	}
}

func TestUpdateSettingsRequiresCSRFAdminAndBody(t *testing.T) {
	mode := &testMode{}
	for _, test := range []struct {
		name   string
		router *chi.Mux
		body   string
		status int
	}{
		{name: "csrf", router: newTestRouter(mode, domain.RoleAdmin, false), body: `{"readOnly":false}`, status: stdhttp.StatusForbidden},
		{name: "viewer", router: newTestRouter(mode, domain.RoleViewer, true), body: `{"readOnly":false}`, status: stdhttp.StatusForbidden},
		{name: "missing", router: newTestRouter(mode, domain.RoleAdmin, true), body: `{}`, status: stdhttp.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(stdhttp.MethodPut, "/settings", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			test.router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
