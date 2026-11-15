package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"virtmgr/internal/domain"
	apitokensapp "virtmgr/internal/modules/apitokens/app"
	"virtmgr/internal/platform/authctx"
)

type stubRepository struct{}

func (stubRepository) CreateAPIToken(context.Context, int64, domain.APIToken, string) (domain.APIToken, error) {
	return domain.APIToken{}, nil
}

func (stubRepository) ListAPITokensByUser(context.Context, int64) ([]domain.APIToken, error) {
	return nil, nil
}

func (stubRepository) RevokeAPIToken(context.Context, int64, string, time.Time) error {
	return nil
}

func (stubRepository) APITokenPrincipalByHash(context.Context, string, time.Time) (domain.APITokenPrincipal, error) {
	return domain.APITokenPrincipal{}, domain.ErrUnauthorized
}

func (stubRepository) TouchAPIToken(context.Context, string, time.Time) error {
	return nil
}

func TestAPITokenEndpointsRejectTokenAuthentication(t *testing.T) {
	handler := New(apitokensapp.New(stubRepository{}, nil))
	tokenSession := domain.Session{
		User:     domain.User{ID: 7, Username: "automation", Role: domain.RoleAdmin},
		AuthMode: domain.AuthModeToken,
		TokenID:  "apitok_caller",
		Scopes:   []string{"read", "write"},
	}
	tests := []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
	}{
		{name: "list", call: handler.List},
		{name: "create", call: handler.Create},
		{name: "revoke", call: handler.Revoke},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/api-tokens", nil)
			request = request.WithContext(authctx.WithSession(request.Context(), tokenSession))
			recorder := httptest.NewRecorder()
			test.call(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestOpenAPIContractCoversTaskCancellationAndImages(t *testing.T) {
	var document struct {
		Components struct {
			Schemas struct {
				Task struct {
					Properties map[string]any `json:"properties"`
				} `json:"Task"`
			} `json:"schemas"`
		} `json:"components"`
		Paths map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(openAPIContract, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	for _, path := range []string{"/images", "/images/{id}", "/images/{id}/refresh", "/api-tokens", "/tasks/{id}/cancel"} {
		if _, ok := document.Paths[path]; !ok {
			t.Fatalf("OpenAPI path %q is missing", path)
		}
	}
	if _, ok := document.Components.Schemas.Task.Properties["cancellable"]; !ok {
		t.Fatal("Task schema is missing cancellable")
	}
	if _, ok := document.Components.Schemas.Task.Properties["actor"]; !ok {
		t.Fatal("Task schema is missing actor")
	}
	if _, ok := document.Components.Schemas.Task.Properties["createdAt"]; !ok {
		t.Fatal("Task schema is missing createdAt")
	}
}
