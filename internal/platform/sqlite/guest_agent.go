package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"virtmgr/internal/domain"
)

func (s *Store) ListGuestAgentPolicies(ctx context.Context) ([]domain.GuestAgentPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT vm_uuid, expected, last_state, last_error, consecutive_failures,
			last_checked_at, last_healthy_at, updated_by, updated_at
		FROM guest_agent_policies ORDER BY vm_uuid ASC`)
	if err != nil {
		return nil, fmt.Errorf("list guest agent policies: %w", err)
	}
	defer rows.Close()
	items := make([]domain.GuestAgentPolicy, 0)
	for rows.Next() {
		item, err := scanGuestAgentPolicy(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate guest agent policies: %w", err)
	}
	return items, nil
}

func (s *Store) GetGuestAgentPolicy(ctx context.Context, vmUUID string) (domain.GuestAgentPolicy, error) {
	item, err := scanGuestAgentPolicy(s.db.QueryRowContext(ctx, `
		SELECT vm_uuid, expected, last_state, last_error, consecutive_failures,
			last_checked_at, last_healthy_at, updated_by, updated_at
		FROM guest_agent_policies WHERE vm_uuid = ?`, vmUUID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.GuestAgentPolicy{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.GuestAgentPolicy{}, fmt.Errorf("get guest agent policy: %w", err)
	}
	return item, nil
}

func (s *Store) UpsertGuestAgentPolicy(ctx context.Context, policy domain.GuestAgentPolicy) (domain.GuestAgentPolicy, error) {
	updatedAt := policy.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO guest_agent_policies(
			vm_uuid, expected, last_state, last_error, consecutive_failures,
			last_checked_at, last_healthy_at, updated_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(vm_uuid) DO UPDATE SET
			expected = excluded.expected,
			last_state = excluded.last_state,
			last_error = excluded.last_error,
			consecutive_failures = excluded.consecutive_failures,
			last_checked_at = excluded.last_checked_at,
			last_healthy_at = excluded.last_healthy_at,
			updated_by = excluded.updated_by,
			updated_at = excluded.updated_at`,
		policy.VMUUID, boolInt(policy.Expected), policy.LastState, policy.LastError, policy.ConsecutiveFailures,
		timePtrUnix(policy.LastCheckedAt), timePtrUnix(policy.LastHealthyAt), policy.UpdatedBy, updatedAt.UTC().Unix())
	if err != nil {
		return domain.GuestAgentPolicy{}, fmt.Errorf("upsert guest agent policy: %w", err)
	}
	return s.GetGuestAgentPolicy(ctx, policy.VMUUID)
}

func (s *Store) DeleteGuestAgentPolicy(ctx context.Context, vmUUID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM guest_agent_policies WHERE vm_uuid = ?`, vmUUID)
	if err != nil {
		return fmt.Errorf("delete guest agent policy: %w", err)
	}
	return nil
}

func (s *Store) InsertGuestAgentHealthSample(ctx context.Context, sample domain.GuestAgentHealthSample) error {
	sampledAt := sample.SampledAt
	if sampledAt.IsZero() {
		sampledAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO guest_agent_health_samples(vm_uuid, sampled_at, state, latency_ms, connected, error)
		VALUES (?, ?, ?, ?, ?, ?)`,
		sample.VMUUID, sampledAt.UTC().Unix(), sample.State, sample.LatencyMS, boolInt(sample.Connected), sample.Error)
	if err != nil {
		return fmt.Errorf("insert guest agent health sample: %w", err)
	}
	return nil
}

func (s *Store) ListGuestAgentHealthSamples(ctx context.Context, vmUUID string, since time.Time, limit int) ([]domain.GuestAgentHealthSample, error) {
	if limit < 1 {
		limit = 288
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, vm_uuid, sampled_at, state, latency_ms, connected, error
		FROM guest_agent_health_samples
		WHERE vm_uuid = ? AND sampled_at >= ?
		ORDER BY sampled_at DESC, id DESC LIMIT ?`, vmUUID, since.UTC().Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("list guest agent health samples: %w", err)
	}
	defer rows.Close()
	items := make([]domain.GuestAgentHealthSample, 0)
	for rows.Next() {
		var item domain.GuestAgentHealthSample
		var sampledAt int64
		var connected int
		if err := rows.Scan(&item.ID, &item.VMUUID, &sampledAt, &item.State, &item.LatencyMS, &connected, &item.Error); err != nil {
			return nil, fmt.Errorf("scan guest agent health sample: %w", err)
		}
		item.SampledAt = time.Unix(sampledAt, 0).UTC()
		item.Connected = connected != 0
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate guest agent health samples: %w", err)
	}
	return items, nil
}

func (s *Store) DeleteGuestAgentHealthSamplesBefore(ctx context.Context, before time.Time) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM guest_agent_health_samples WHERE sampled_at < ?`, before.UTC().Unix()); err != nil {
		return fmt.Errorf("delete guest agent health samples: %w", err)
	}
	return nil
}

func (s *Store) UpsertGuestAgentSnapshotRecord(ctx context.Context, record domain.GuestAgentSnapshotRecord) error {
	createdAt := record.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO guest_agent_snapshot_records(vm_uuid, snapshot_name, quiesced, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(vm_uuid, snapshot_name) DO UPDATE SET
			quiesced = excluded.quiesced, created_at = excluded.created_at`,
		record.VMUUID, record.SnapshotName, boolInt(record.Quiesced), createdAt.UTC().Unix())
	if err != nil {
		return fmt.Errorf("upsert guest agent snapshot record: %w", err)
	}
	return nil
}

func (s *Store) ListGuestAgentSnapshotRecords(ctx context.Context, vmUUID string) ([]domain.GuestAgentSnapshotRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT vm_uuid, snapshot_name, quiesced, created_at
		FROM guest_agent_snapshot_records WHERE vm_uuid = ?`, vmUUID)
	if err != nil {
		return nil, fmt.Errorf("list guest agent snapshot records: %w", err)
	}
	defer rows.Close()
	items := make([]domain.GuestAgentSnapshotRecord, 0)
	for rows.Next() {
		var item domain.GuestAgentSnapshotRecord
		var quiesced int
		var createdAt int64
		if err := rows.Scan(&item.VMUUID, &item.SnapshotName, &quiesced, &createdAt); err != nil {
			return nil, fmt.Errorf("scan guest agent snapshot record: %w", err)
		}
		item.Quiesced = quiesced != 0
		item.CreatedAt = time.Unix(createdAt, 0).UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate guest agent snapshot records: %w", err)
	}
	return items, nil
}

func (s *Store) DeleteGuestAgentSnapshotRecord(ctx context.Context, vmUUID, snapshotName string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM guest_agent_snapshot_records WHERE vm_uuid = ? AND snapshot_name = ?`, vmUUID, snapshotName)
	if err != nil {
		return fmt.Errorf("delete guest agent snapshot record: %w", err)
	}
	return nil
}

func (s *Store) DeleteGuestAgentVMData(ctx context.Context, vmUUID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin guest agent VM cleanup: %w", err)
	}
	for _, statement := range []string{
		`DELETE FROM guest_agent_policies WHERE vm_uuid = ?`,
		`DELETE FROM guest_agent_health_samples WHERE vm_uuid = ?`,
		`DELETE FROM guest_agent_snapshot_records WHERE vm_uuid = ?`,
	} {
		if _, err := tx.ExecContext(ctx, statement, vmUUID); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("clean guest agent VM data: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit guest agent VM cleanup: %w", err)
	}
	return nil
}

type guestAgentPolicyScanner interface {
	Scan(dest ...any) error
}

func scanGuestAgentPolicy(scanner guestAgentPolicyScanner) (domain.GuestAgentPolicy, error) {
	var item domain.GuestAgentPolicy
	var expected int
	var lastChecked sql.NullInt64
	var lastHealthy sql.NullInt64
	var updatedAt int64
	if err := scanner.Scan(
		&item.VMUUID, &expected, &item.LastState, &item.LastError, &item.ConsecutiveFailures,
		&lastChecked, &lastHealthy, &item.UpdatedBy, &updatedAt,
	); err != nil {
		return domain.GuestAgentPolicy{}, err
	}
	item.Expected = expected != 0
	if lastChecked.Valid {
		value := time.Unix(lastChecked.Int64, 0).UTC()
		item.LastCheckedAt = &value
	}
	if lastHealthy.Valid {
		value := time.Unix(lastHealthy.Int64, 0).UTC()
		item.LastHealthyAt = &value
	}
	item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return item, nil
}
