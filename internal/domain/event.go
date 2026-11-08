package domain

import "time"

type SystemEvent struct {
	ID         uint64         `json:"id"`
	OccurredAt time.Time      `json:"occurredAt"`
	Source     string         `json:"source"`
	Kind       string         `json:"kind"`
	Severity   string         `json:"severity"`
	TargetKind string         `json:"targetKind"`
	TargetID   string         `json:"targetId"`
	TargetName string         `json:"targetName,omitempty"`
	Message    string         `json:"message"`
	Detail     map[string]any `json:"detail,omitempty"`
}

type SystemEventList struct {
	Items []SystemEvent `json:"items"`
	Total int           `json:"total"`
}
