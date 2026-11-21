package app

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"virtmgr/internal/domain"
	"virtmgr/internal/version"
)

const updateCacheTTL = 10 * time.Minute

var releaseVersionPattern = regexp.MustCompile(`^[vV]?(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$`)

type ReleaseProvider interface {
	LatestRelease(ctx context.Context) (domain.ReleaseInfo, error)
}

func WithReleaseProvider(provider ReleaseProvider) Option {
	return func(service *Service) { service.releaseProvider = provider }
}

// CheckUpdate returns sanitized release metadata and a conservative update
// decision. Failures are represented as data so the About page remains usable
// when the server cannot reach GitHub.
func (s *Service) CheckUpdate(ctx context.Context, refresh bool) domain.UpdateCheck {
	current := strings.TrimSpace(version.Version)
	if current == "" {
		current = "dev"
	}
	now := s.now().UTC()
	if !refresh {
		s.updateMu.Lock()
		if s.updateCacheValid && now.Sub(s.updateCheckedAt) < updateCacheTTL {
			cached := s.updateCache
			cached.Cached = true
			s.updateMu.Unlock()
			return cached
		}
		s.updateMu.Unlock()
	}

	result := domain.UpdateCheck{
		CurrentVersion: current,
		Status:         domain.UpdateStatusUnavailable,
		CheckedAt:      now,
		Message:        "暂时无法连接更新服务，请稍后重试。",
	}
	if s.releaseProvider == nil {
		return result
	}
	release, err := s.releaseProvider.LatestRelease(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			result.CheckedAt = s.now().UTC()
			result.Status = domain.UpdateStatusNoRelease
			result.Message = "项目尚未发布稳定版本，可前往 Releases 页面查看后续发布。"
			s.updateMu.Lock()
			s.updateCache = result
			s.updateCheckedAt = result.CheckedAt
			s.updateCacheValid = true
			s.updateMu.Unlock()
		}
		return result
	}
	result.CheckedAt = s.now().UTC()
	result.LatestVersion = release.Version
	result.ReleaseName = release.Name
	result.ReleaseNotes = release.Notes
	result.ReleaseURL = release.URL
	result.PublishedAt = release.PublishedAt

	status, updateAvailable, message := compareReleaseVersions(current, release.Version)
	result.Status = status
	result.UpdateAvailable = updateAvailable
	result.Message = message
	if status != domain.UpdateStatusUnknown {
		s.updateMu.Lock()
		s.updateCache = result
		s.updateCheckedAt = result.CheckedAt
		s.updateCacheValid = true
		s.updateMu.Unlock()
	}
	return result
}

func compareReleaseVersions(current, latest string) (string, bool, string) {
	currentParts, currentOK := parseReleaseVersion(current)
	latestParts, latestOK := parseReleaseVersion(latest)
	if !currentOK || !latestOK {
		return domain.UpdateStatusUnknown, false, "当前构建或发布版本无法比较，请直接查看发布页。"
	}
	for index := range currentParts {
		switch {
		case currentParts[index] < latestParts[index]:
			return domain.UpdateStatusAvailable, true, "发现新版本，可前往发布页查看说明并安排升级。"
		case currentParts[index] > latestParts[index]:
			return domain.UpdateStatusAhead, false, "当前构建高于最新稳定发布版本。"
		}
	}
	return domain.UpdateStatusUpToDate, false, "当前版本已是最新稳定发布版本。"
}

func parseReleaseVersion(value string) ([3]int, bool) {
	var result [3]int
	matches := releaseVersionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) != 4 {
		return result, false
	}
	for index := 1; index <= 3; index++ {
		parsed, err := strconv.Atoi(matches[index])
		if err != nil {
			return result, false
		}
		result[index-1] = parsed
	}
	return result, true
}
