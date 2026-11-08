package domain

import (
	"strings"
	"time"
)

type AuditEvent struct {
	ID         int64          `json:"id"`
	OccurredAt time.Time      `json:"occurredAt"`
	RequestID  string         `json:"requestId"`
	Actor      string         `json:"actor"`
	Role       Role           `json:"role"`
	Action     string         `json:"action"`
	TargetKind string         `json:"targetKind"`
	TargetID   string         `json:"targetId"`
	SourceIP   string         `json:"sourceIp"`
	Result     string         `json:"result"`
	Detail     map[string]any `json:"detail,omitempty"`
}

type AuditEventQuery struct {
	Actor      string
	Action     string
	TargetKind string
	Result     string
	Page       int
	PageSize   int
}

type AuditEventList struct {
	Items    []AuditEvent `json:"items"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
}

const (
	DefaultAuditPageSize = 50
	MaxAuditPageSize     = 200
)

func (q AuditEventQuery) Normalized() AuditEventQuery {
	q.Actor = strings.TrimSpace(q.Actor)
	q.Action = strings.TrimSpace(q.Action)
	q.TargetKind = strings.TrimSpace(q.TargetKind)
	q.Result = strings.TrimSpace(q.Result)
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = DefaultAuditPageSize
	}
	if q.PageSize > MaxAuditPageSize {
		q.PageSize = MaxAuditPageSize
	}
	return q
}
