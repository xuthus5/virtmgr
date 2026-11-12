package httpx

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

func TestErrorMappings(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{domain.ErrUnauthorized, http.StatusUnauthorized},
		{domain.ErrInvalidCredentials, http.StatusUnauthorized},
		{domain.ErrForbidden, http.StatusForbidden},
		{domain.ErrReadOnly, http.StatusForbidden},
		{domain.ErrCSRF, http.StatusForbidden},
		{domain.ErrActionNotAllowed, http.StatusConflict},
		{domain.ErrVolumeInUse, http.StatusConflict},
		{domain.ErrSelfModification, http.StatusConflict},
		{domain.ErrNotFound, http.StatusNotFound},
		{domain.ErrConflict, http.StatusConflict},
		{domain.ErrInvalidInput, http.StatusBadRequest},
		{domain.ErrTooLarge, http.StatusRequestEntityTooLarge},
		{domain.ErrUnavailable, http.StatusServiceUnavailable},
		{errors.New("boom"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		Error(recorder, request, test.err)
		if recorder.Code != test.status {
			t.Fatalf("Error(%v) status = %d, want %d", test.err, recorder.Code, test.status)
		}
	}
}

func TestJSONSetsNoStore(t *testing.T) {
	recorder := httptest.NewRecorder()
	JSON(recorder, http.StatusOK, map[string]string{"status": "ok"})
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestDecodeJSONRejectsTrailingData(t *testing.T) {
	for _, body := range []string{`{"name":"one"}{"name":"two"}`, `{"name":"one"} trailing`} {
		var value struct {
			Name string `json:"name"`
		}
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		if err := DecodeJSON(request, &value); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("DecodeJSON(%q) error = %v, want invalid input", body, err)
		}
	}

	var value struct {
		Name string `json:"name"`
	}
	request := httptest.NewRequest(http.MethodPost, "/", io.NopCloser(strings.NewReader("{\"name\":\"one\"}\n")))
	if err := DecodeJSON(request, &value); err != nil {
		t.Fatalf("DecodeJSON() error = %v", err)
	}
	if value.Name != "one" {
		t.Fatalf("value.Name = %q", value.Name)
	}
}

func TestRealIPIgnoresForwardedHeadersFromUntrustedPeer(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "198.51.100.10:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.9")
	request.Header.Set("X-Real-IP", "203.0.113.10")
	recorder := httptest.NewRecorder()
	RealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := SourceIP(r); got != "198.51.100.10" {
			t.Fatalf("SourceIP() = %q, want direct peer", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)
}

func TestRealIPUsesForwardedChainFromTrustedProxy(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("10.0.0.0/8")}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.5")
	recorder := httptest.NewRecorder()
	RealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := SourceIP(r); got != "203.0.113.9" {
			t.Fatalf("SourceIP() = %q, want forwarded client", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)
}

func TestRealIPFallsBackToXRealIPForTrustedProxy(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("::1/128")}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "[::1]:1234"
	request.Header.Set("X-Real-IP", "203.0.113.11")
	recorder := httptest.NewRecorder()
	RealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := SourceIP(r); got != "203.0.113.11" {
			t.Fatalf("SourceIP() = %q, want X-Real-IP client", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)
}
