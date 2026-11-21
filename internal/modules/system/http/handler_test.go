package systemhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"virtmgr/internal/domain"
	systemapp "virtmgr/internal/modules/system/app"
	"virtmgr/internal/platform/authctx"

	"github.com/go-chi/chi/v5"
)

type restoreRepository struct{}

type tlsStatusProvider struct{ host string }

type updateReleaseProvider struct {
	calls int
}

func (provider *updateReleaseProvider) LatestRelease(context.Context) (domain.ReleaseInfo, error) {
	provider.calls++
	return domain.ReleaseInfo{
		Version: "v0.4.0", Name: "VirtMgr 0.4.0", URL: "https://github.com/xuthus5/virtmgr/releases/tag/v0.4.0",
		Notes: "release notes",
	}, nil
}

func (provider *tlsStatusProvider) Status(host string) domain.TLSInfo {
	provider.host = host
	return domain.TLSInfo{Enabled: true, RequestHost: host, RequestHostMatched: false, Warnings: []string{"SAN mismatch"}}
}

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

func TestSystemInfoIncludesTLSStatusForRequestHost(t *testing.T) {
	provider := &tlsStatusProvider{}
	router := chi.NewRouter()
	New(systemapp.New(restoreRepository{}, nil, systemapp.WithTLSStatus(provider))).Mount(router, func(next http.Handler) http.Handler { return next })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "https://[fd00::10]:8443/system/info", nil)
	request.Host = "[fd00::10]:8443"
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if provider.host != "[fd00::10]:8443" {
		t.Fatalf("TLS request host = %q", provider.host)
	}
	if !strings.Contains(recorder.Body.String(), `"tls":{"enabled":true`) || !strings.Contains(recorder.Body.String(), `"requestHostMatched":false`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestSystemUpdateReturnsReleaseMetadata(t *testing.T) {
	provider := &updateReleaseProvider{}
	router := chi.NewRouter()
	New(systemapp.New(restoreRepository{}, nil, systemapp.WithReleaseProvider(provider))).Mount(router, func(next http.Handler) http.Handler { return next })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, systemRequest(http.MethodGet, "/system/update?refresh=true", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result domain.UpdateCheck
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.LatestVersion != "v0.4.0" || result.ReleaseName != "VirtMgr 0.4.0" || provider.calls != 1 {
		t.Fatalf("result = %+v calls=%d", result, provider.calls)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", recorder.Header().Get("Cache-Control"))
	}
}
