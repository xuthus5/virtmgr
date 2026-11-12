package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

func (s *Store) RecordAudit(ctx context.Context, event domain.AuditEvent) error {
	occurredAt := event.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	detail := event.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("encode audit detail: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO audit_events(
			occurred_at, request_id, actor, role, action, target_kind, target_id, source_ip, result, detail_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		occurredAt.UTC().Unix(), event.RequestID, event.Actor, event.Role, event.Action,
		event.TargetKind, event.TargetID, event.SourceIP, event.Result, string(detailJSON),
	)
	if err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}

func (s *Store) ListAuditEvents(ctx context.Context, input domain.AuditEventQuery) (domain.AuditEventList, error) {
	query := input.Normalized()
	where, args := auditEventWhere(query)

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`+where, args...).Scan(&total); err != nil {
		return domain.AuditEventList{}, fmt.Errorf("count audit events: %w", err)
	}

	offset := int64(query.Page-1) * int64(query.PageSize)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, occurred_at, request_id, actor, role, action, target_kind, target_id, source_ip, result, detail_json
		FROM audit_events`+where+`
		ORDER BY occurred_at DESC, id DESC
		LIMIT ? OFFSET ?`, append(args, query.PageSize, offset)...)
	if err != nil {
		return domain.AuditEventList{}, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()
	items, err := scanAuditEvents(rows)
	if err != nil {
		return domain.AuditEventList{}, err
	}
	return domain.AuditEventList{
		Items: items, Total: total, Page: query.Page, PageSize: query.PageSize,
	}, nil
}

func (s *Store) ExportAuditEvents(ctx context.Context, input domain.AuditEventQuery) ([]domain.AuditEvent, error) {
	query := input.Normalized()
	where, args := auditEventWhere(query)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, occurred_at, request_id, actor, role, action, target_kind, target_id, source_ip, result, detail_json
		FROM audit_events`+where+`
		ORDER BY occurred_at DESC, id DESC
		LIMIT ?`, append(args, domain.MaxAuditExportEvents+1)...)
	if err != nil {
		return nil, fmt.Errorf("export audit events: %w", err)
	}
	defer rows.Close()
	return scanAuditEvents(rows)
}

func auditEventWhere(query domain.AuditEventQuery) (string, []any) {
	clauses := make([]string, 0, 7)
	args := make([]any, 0, 7)
	if query.Actor != "" {
		clauses = append(clauses, "actor = ?")
		args = append(args, query.Actor)
	}
	if query.Action != "" {
		clauses = append(clauses, "action = ?")
		args = append(args, query.Action)
	}
	if query.TargetKind != "" {
		clauses = append(clauses, "target_kind = ?")
		args = append(args, query.TargetKind)
	}
	if query.TargetID != "" {
		clauses = append(clauses, "target_id = ?")
		args = append(args, query.TargetID)
	}
	if query.Result != "" {
		clauses = append(clauses, "result = ?")
		args = append(args, query.Result)
	}
	if !query.From.IsZero() {
		clauses = append(clauses, "occurred_at >= ?")
		args = append(args, query.From.UTC().Unix())
	}
	if !query.To.IsZero() {
		clauses = append(clauses, "occurred_at <= ?")
		args = append(args, query.To.UTC().Unix())
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanAuditEvents(rows *sql.Rows) ([]domain.AuditEvent, error) {
	items := make([]domain.AuditEvent, 0)
	for rows.Next() {
		var event domain.AuditEvent
		var occurredAt int64
		var role string
		var detailJSON string
		if err := rows.Scan(
			&event.ID, &occurredAt, &event.RequestID, &event.Actor, &role, &event.Action,
			&event.TargetKind, &event.TargetID, &event.SourceIP, &event.Result, &detailJSON,
		); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		event.OccurredAt = time.Unix(occurredAt, 0).UTC()
		event.Role = domain.Role(role)
		if strings.TrimSpace(detailJSON) != "" {
			if err := json.Unmarshal([]byte(detailJSON), &event.Detail); err != nil {
				return nil, fmt.Errorf("decode audit detail for event %d: %w", event.ID, err)
			}
		}
		if event.Detail == nil {
			event.Detail = map[string]any{}
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	return items, nil
}
