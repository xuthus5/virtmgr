package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
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
