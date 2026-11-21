package releasecheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

func TestGitHubClientLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("User-Agent is missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name":"v0.4.0",
			"name":"VirtMgr 0.4.0",
			"html_url":"https://github.com/xuthus5/virtmgr/releases/tag/v0.4.0",
			"body":"## Changes\n- About page\n<script>alert(1)</script>",
			"published_at":"2026-11-20T12:00:00Z",
			"draft":false
		}`))
	}))
	defer server.Close()

	client := NewGitHubClient()
	client.endpoint = server.URL
	release, err := client.LatestRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "v0.4.0" || release.Name != "VirtMgr 0.4.0" {
		t.Fatalf("release = %+v", release)
	}
	if !strings.Contains(release.URL, "/releases/tag/v0.4.0") || release.PublishedAt == nil {
		t.Fatalf("release metadata = %+v", release)
	}
	if !strings.Contains(release.Notes, "<script>") {
		t.Fatalf("release notes = %q", release.Notes)
	}
}

func TestGitHubClientRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "http error", status: http.StatusTooManyRequests, body: `{}`},
		{name: "missing tag", status: http.StatusOK, body: `{"name":"release"}`},
		{name: "draft", status: http.StatusOK, body: `{"tag_name":"v1.0.0","draft":true}`},
		{name: "invalid json", status: http.StatusOK, body: `{`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			client := NewGitHubClient()
			client.endpoint = server.URL
			if _, err := client.LatestRelease(context.Background()); err == nil {
				t.Fatal("expected release check error")
			}
		})
	}
}

func TestGitHubClientReportsMissingRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer server.Close()
	client := NewGitHubClient()
	client.endpoint = server.URL
	if _, err := client.LatestRelease(context.Background()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}
