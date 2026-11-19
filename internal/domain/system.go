package domain

import "time"

type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

type SystemInfo struct {
	Build BuildInfo `json:"build"`
}

const (
	DatabaseRestoreNone           = "none"
	DatabaseRestorePendingRestart = "pending_restart"
	DatabaseRestoreApplied        = "applied"
	DatabaseRestoreFailed         = "failed"
)

type DatabaseRestoreStatus struct {
	State         string     `json:"state"`
	Filename      string     `json:"filename,omitempty"`
	SizeBytes     int64      `json:"sizeBytes,omitempty"`
	SchemaVersion int        `json:"schemaVersion,omitempty"`
	UploadedAt    *time.Time `json:"uploadedAt,omitempty"`
	AppliedAt     *time.Time `json:"appliedAt,omitempty"`
	Message       string     `json:"message,omitempty"`
}
