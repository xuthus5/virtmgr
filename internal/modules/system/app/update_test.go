package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
	"virtmgr/internal/version"
)

type fakeReleaseProvider struct {
	release domain.ReleaseInfo
	err     error
	calls   int
}

func (provider *fakeReleaseProvider) LatestRelease(context.Context) (domain.ReleaseInfo, error) {
	provider.calls++
	return provider.release, provider.err
}

func TestCheckUpdateComparesVersionsAndCaches(t *testing.T) {
	previous := version.Version
	version.Version = "v0.3.1-32-g18b7ca3"
	defer func() { version.Version = previous }()

	provider := &fakeReleaseProvider{release: domain.ReleaseInfo{Version: "v0.3.2", URL: "https://example.test/release"}}
	service := New(nil, nil, WithReleaseProvider(provider))
	now := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	result := service.CheckUpdate(context.Background(), false)
	if result.Status != domain.UpdateStatusAvailable || !result.UpdateAvailable || result.LatestVersion != "v0.3.2" || result.Cached {
		t.Fatalf("first result = %+v", result)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d", provider.calls)
	}

	now = now.Add(time.Minute)
	cached := service.CheckUpdate(context.Background(), false)
	if !cached.Cached || cached.LatestVersion != "v0.3.2" || provider.calls != 1 {
		t.Fatalf("cached result = %+v calls=%d", cached, provider.calls)
	}

	refreshed := service.CheckUpdate(context.Background(), true)
	if refreshed.Cached || provider.calls != 2 {
		t.Fatalf("refreshed result = %+v calls=%d", refreshed, provider.calls)
	}
}

func TestCheckUpdateHandlesUnavailableAndUnknownVersions(t *testing.T) {
	previous := version.Version
	version.Version = "dev"
	defer func() { version.Version = previous }()

	failed := New(nil, nil, WithReleaseProvider(&fakeReleaseProvider{err: errors.New("offline")}))
	result := failed.CheckUpdate(context.Background(), true)
	if result.Status != domain.UpdateStatusUnavailable || result.UpdateAvailable || result.Message == "" {
		t.Fatalf("failed result = %+v", result)
	}

	unknown := New(nil, nil, WithReleaseProvider(&fakeReleaseProvider{release: domain.ReleaseInfo{Version: "v0.4.0"}}))
	result = unknown.CheckUpdate(context.Background(), true)
	if result.Status != domain.UpdateStatusUnknown || result.UpdateAvailable {
		t.Fatalf("unknown result = %+v", result)
	}

	noRelease := New(nil, nil, WithReleaseProvider(&fakeReleaseProvider{err: domain.ErrNotFound}))
	result = noRelease.CheckUpdate(context.Background(), true)
	if result.Status != domain.UpdateStatusNoRelease || result.UpdateAvailable {
		t.Fatalf("no release result = %+v", result)
	}
}

func TestCompareReleaseVersions(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		status  string
		update  bool
	}{
		{current: "v0.3.1", latest: "v0.3.2", status: domain.UpdateStatusAvailable, update: true},
		{current: "v0.4.0", latest: "v0.4.0", status: domain.UpdateStatusUpToDate},
		{current: "v0.5.0-12-gabcdef", latest: "v0.4.9", status: domain.UpdateStatusAhead},
		{current: "dev", latest: "v0.4.0", status: domain.UpdateStatusUnknown},
		{current: "v0.4.0", latest: "nightly", status: domain.UpdateStatusUnknown},
	}
	for _, test := range tests {
		status, update, _ := compareReleaseVersions(test.current, test.latest)
		if status != test.status || update != test.update {
			t.Errorf("compare(%q, %q) = %s/%v", test.current, test.latest, status, update)
		}
	}
}
