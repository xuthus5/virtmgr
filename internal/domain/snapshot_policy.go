package domain

import "time"

type SnapshotPolicy struct {
	ID              int64      `json:"id"`
	VMUUID          string     `json:"vmUuid"`
	Name            string     `json:"name"`
	IntervalMinutes int        `json:"intervalMinutes"`
	Retention       int        `json:"retention"`
	Enabled         bool       `json:"enabled"`
	LastRunAt       *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt       time.Time  `json:"nextRunAt"`
	LastSnapshot    string     `json:"lastSnapshot,omitempty"`
	LastError       string     `json:"lastError,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type SnapshotPolicySpec struct {
	Name            string `json:"name"`
	IntervalMinutes int    `json:"intervalMinutes"`
	Retention       int    `json:"retention"`
	Enabled         bool   `json:"enabled"`
}

type SnapshotPolicyList struct {
	Items []SnapshotPolicy `json:"items"`
	Total int              `json:"total"`
}

type SnapshotPolicyRunResult struct {
	Policy           SnapshotPolicy `json:"policy"`
	Snapshot         *VMSnapshot    `json:"snapshot,omitempty"`
	DeletedSnapshots []string       `json:"deletedSnapshots,omitempty"`
}
