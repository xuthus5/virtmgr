package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"virtmgr/internal/domain"
)

const snapshotPolicyColumns = `
	id, vm_uuid, name, interval_minutes, retention, enabled,
	last_run_at, next_run_at, last_snapshot, last_error, created_at, updated_at`

func (s *Store) ListSnapshotPolicies(ctx context.Context, vmUUID string) ([]domain.SnapshotPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT`+snapshotPolicyColumns+`
		FROM snapshot_policies WHERE vm_uuid = ?
		ORDER BY name COLLATE NOCASE ASC, id ASC`, vmUUID)
	if err != nil {
		return nil, fmt.Errorf("list snapshot policies: %w", err)
	}
	defer rows.Close()
	items := make([]domain.SnapshotPolicy, 0)
	for rows.Next() {
		item, err := scanSnapshotPolicy(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate snapshot policies: %w", err)
	}
	return items, nil
}

func (s *Store) GetSnapshotPolicy(ctx context.Context, id int64) (domain.SnapshotPolicy, error) {
	row := s.db.QueryRowContext(ctx, `SELECT`+snapshotPolicyColumns+` FROM snapshot_policies WHERE id = ?`, id)
	item, err := scanSnapshotPolicy(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SnapshotPolicy{}, domain.ErrNotFound
	}
	return item, err
}

func (s *Store) CreateSnapshotPolicy(ctx context.Context, policy domain.SnapshotPolicy) (domain.SnapshotPolicy, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO snapshot_policies(
			vm_uuid, name, interval_minutes, retention, enabled,
			last_run_at, next_run_at, last_snapshot, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		policy.VMUUID, policy.Name, policy.IntervalMinutes, policy.Retention, boolInt(policy.Enabled),
		timePtrUnix(policy.LastRunAt), policy.NextRunAt.UTC().Unix(), policy.LastSnapshot, policy.LastError,
		policy.CreatedAt.UTC().Unix(), policy.UpdatedAt.UTC().Unix(),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.SnapshotPolicy{}, domain.ErrConflict
		}
		return domain.SnapshotPolicy{}, fmt.Errorf("create snapshot policy: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.SnapshotPolicy{}, fmt.Errorf("read snapshot policy id: %w", err)
	}
	return s.GetSnapshotPolicy(ctx, id)
}

func (s *Store) UpdateSnapshotPolicy(ctx context.Context, policy domain.SnapshotPolicy) (domain.SnapshotPolicy, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE snapshot_policies SET
			name = ?, interval_minutes = ?, retention = ?, enabled = ?,
			last_run_at = ?, next_run_at = ?, last_snapshot = ?, last_error = ?, updated_at = ?
		WHERE id = ?`,
		policy.Name, policy.IntervalMinutes, policy.Retention, boolInt(policy.Enabled),
		timePtrUnix(policy.LastRunAt), policy.NextRunAt.UTC().Unix(), policy.LastSnapshot, policy.LastError,
		policy.UpdatedAt.UTC().Unix(), policy.ID,
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.SnapshotPolicy{}, domain.ErrConflict
		}
		return domain.SnapshotPolicy{}, fmt.Errorf("update snapshot policy: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.SnapshotPolicy{}, fmt.Errorf("read updated snapshot policy count: %w", err)
	}
	if affected == 0 {
		return domain.SnapshotPolicy{}, domain.ErrNotFound
	}
	return s.GetSnapshotPolicy(ctx, policy.ID)
}

func (s *Store) DeleteSnapshotPolicy(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM snapshot_policies WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete snapshot policy: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted snapshot policy count: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) ListDueSnapshotPolicies(ctx context.Context, now time.Time, limit int) ([]domain.SnapshotPolicy, error) {
	if limit < 1 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT`+snapshotPolicyColumns+`
		FROM snapshot_policies
		WHERE enabled = 1 AND next_run_at <= ?
		ORDER BY next_run_at ASC, id ASC
		LIMIT ?`, now.UTC().Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("list due snapshot policies: %w", err)
	}
	defer rows.Close()
	items := make([]domain.SnapshotPolicy, 0)
	for rows.Next() {
		item, err := scanSnapshotPolicy(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due snapshot policies: %w", err)
	}
	return items, nil
}

type snapshotPolicyScanner interface {
	Scan(dest ...any) error
}

func scanSnapshotPolicy(scanner snapshotPolicyScanner) (domain.SnapshotPolicy, error) {
	var item domain.SnapshotPolicy
	var enabled int
	var lastRunAt sql.NullInt64
	var nextRunAt int64
	var createdAt int64
	var updatedAt int64
	if err := scanner.Scan(
		&item.ID, &item.VMUUID, &item.Name, &item.IntervalMinutes, &item.Retention, &enabled,
		&lastRunAt, &nextRunAt, &item.LastSnapshot, &item.LastError, &createdAt, &updatedAt,
	); err != nil {
		return domain.SnapshotPolicy{}, err
	}
	item.Enabled = enabled != 0
	if lastRunAt.Valid {
		value := time.Unix(lastRunAt.Int64, 0).UTC()
		item.LastRunAt = &value
	}
	item.NextRunAt = time.Unix(nextRunAt, 0).UTC()
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return item, nil
}

func timePtrUnix(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Unix()
}
