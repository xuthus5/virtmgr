package domain

import "time"

const (
	UpdateStatusAvailable   = "update_available"
	UpdateStatusUpToDate    = "up_to_date"
	UpdateStatusAhead       = "ahead"
	UpdateStatusUnknown     = "unknown"
	UpdateStatusNoRelease   = "no_release"
	UpdateStatusUnavailable = "unavailable"
)

// ReleaseInfo is the minimal, sanitized release metadata exposed by an update
// source. Release bodies are display-only text and must never be interpreted as
// HTML by clients.
type ReleaseInfo struct {
	Version     string     `json:"version"`
	Name        string     `json:"name,omitempty"`
	URL         string     `json:"url,omitempty"`
	Notes       string     `json:"notes,omitempty"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
}

type UpdateCheck struct {
	CurrentVersion  string     `json:"currentVersion"`
	LatestVersion   string     `json:"latestVersion,omitempty"`
	Status          string     `json:"status"`
	UpdateAvailable bool       `json:"updateAvailable"`
	ReleaseName     string     `json:"releaseName,omitempty"`
	ReleaseNotes    string     `json:"releaseNotes,omitempty"`
	ReleaseURL      string     `json:"releaseUrl,omitempty"`
	PublishedAt     *time.Time `json:"publishedAt,omitempty"`
	CheckedAt       time.Time  `json:"checkedAt"`
	Cached          bool       `json:"cached"`
	Message         string     `json:"message,omitempty"`
}
