package releasecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

const (
	defaultEndpoint     = "https://api.github.com/repos/xuthus5/virtmgr/releases/latest"
	defaultRepository   = "https://github.com/xuthus5/virtmgr"
	maxReleaseBodyBytes = 512 << 10
	maxReleaseNotesSize = 32 << 10
)

type GitHubClient struct {
	httpClient *http.Client
	endpoint   string
	repository string
	userAgent  string
}

func NewGitHubClient() *GitHubClient {
	return &GitHubClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		endpoint:   defaultEndpoint,
		repository: defaultRepository,
		userAgent:  "virtmgr-release-check",
	}
}

type githubRelease struct {
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	HTMLURL     string     `json:"html_url"`
	Body        string     `json:"body"`
	PublishedAt *time.Time `json:"published_at"`
	Draft       bool       `json:"draft"`
	Prerelease  bool       `json:"prerelease"`
}

func (c *GitHubClient) LatestRelease(ctx context.Context) (domain.ReleaseInfo, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return domain.ReleaseInfo{}, fmt.Errorf("create release check request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", c.userAgent)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return domain.ReleaseInfo{}, fmt.Errorf("request latest release: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotFound {
			return domain.ReleaseInfo{}, fmt.Errorf("latest release: %w", domain.ErrNotFound)
		}
		return domain.ReleaseInfo{}, fmt.Errorf("latest release returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseBodyBytes+1))
	if err != nil {
		return domain.ReleaseInfo{}, fmt.Errorf("read latest release: %w", err)
	}
	if len(body) > maxReleaseBodyBytes {
		return domain.ReleaseInfo{}, fmt.Errorf("latest release response is too large")
	}
	var parsed githubRelease
	if err := json.Unmarshal(body, &parsed); err != nil {
		return domain.ReleaseInfo{}, fmt.Errorf("decode latest release: %w", err)
	}
	version := strings.TrimSpace(parsed.TagName)
	if version == "" || parsed.Draft {
		return domain.ReleaseInfo{}, fmt.Errorf("latest release has no usable tag")
	}
	releaseURL := strings.TrimSpace(parsed.HTMLURL)
	if !strings.HasPrefix(releaseURL, c.repository+"/releases/") {
		releaseURL = c.repository + "/releases/latest"
	}
	return domain.ReleaseInfo{
		Version:     version,
		Name:        strings.TrimSpace(parsed.Name),
		URL:         releaseURL,
		Notes:       truncateUTF8(parsed.Body, maxReleaseNotesSize),
		PublishedAt: parsed.PublishedAt,
	}, nil
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return strings.ToValidUTF8(value[:limit], "")
}
