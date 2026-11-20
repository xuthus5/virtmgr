package domain

import "time"

type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

type SystemInfo struct {
	Build BuildInfo `json:"build"`
	TLS   TLSInfo   `json:"tls"`
}

type TLSInfo struct {
	Enabled            bool       `json:"enabled"`
	Subject            string     `json:"subject,omitempty"`
	Issuer             string     `json:"issuer,omitempty"`
	NotBefore          *time.Time `json:"notBefore,omitempty"`
	NotAfter           *time.Time `json:"notAfter,omitempty"`
	DNSNames           []string   `json:"dnsNames,omitempty"`
	IPAddresses        []string   `json:"ipAddresses,omitempty"`
	RequestHost        string     `json:"requestHost,omitempty"`
	RequestHostMatched bool       `json:"requestHostMatched"`
	ExpiresInDays      int        `json:"expiresInDays,omitempty"`
	KeyFileMode        string     `json:"keyFileMode,omitempty"`
	LastReloadedAt     *time.Time `json:"lastReloadedAt,omitempty"`
	ReloadError        string     `json:"reloadError,omitempty"`
	Warnings           []string   `json:"warnings,omitempty"`
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
