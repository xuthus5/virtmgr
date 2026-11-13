package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"virtmgr/internal/domain"
)

const alertRuleColumns = `id, name, metric, operator, threshold, duration_minutes, enabled,
	webhook_url, last_state, last_changed_at, last_notified_at, created_at, updated_at`

func (s *Store) ListAlertRules(ctx context.Context) ([]domain.AlertRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+alertRuleColumns+` FROM alert_rules ORDER BY name COLLATE NOCASE ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list alert rules: %w", err)
	}
	defer rows.Close()
	items := make([]domain.AlertRule, 0)
	for rows.Next() {
		item, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate alert rules: %w", err)
	}
	return items, nil
}

func (s *Store) GetAlertRule(ctx context.Context, id int64) (domain.AlertRule, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+alertRuleColumns+` FROM alert_rules WHERE id = ?`, id)
	item, err := scanAlertRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AlertRule{}, domain.ErrNotFound
	}
	return item, err
}

func (s *Store) CreateAlertRule(ctx context.Context, item domain.AlertRule) (domain.AlertRule, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO alert_rules(name, metric, operator, threshold, duration_minutes, enabled, webhook_url,
			last_state, last_changed_at, last_notified_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.Name, item.Metric, item.Operator, item.Threshold, item.DurationMinutes, boolInt(item.Enabled), item.WebhookURL,
		item.LastState, timePtrUnix(item.LastChangedAt), timePtrUnix(item.LastNotifiedAt), item.CreatedAt.UTC().Unix(), item.UpdatedAt.UTC().Unix())
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.AlertRule{}, domain.ErrConflict
		}
		return domain.AlertRule{}, fmt.Errorf("create alert rule: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.AlertRule{}, fmt.Errorf("read alert rule id: %w", err)
	}
	return s.GetAlertRule(ctx, id)
}

func (s *Store) UpdateAlertRule(ctx context.Context, item domain.AlertRule) (domain.AlertRule, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE alert_rules SET name = ?, metric = ?, operator = ?, threshold = ?, duration_minutes = ?,
			enabled = ?, webhook_url = ?, last_state = ?, last_changed_at = ?, last_notified_at = ?, updated_at = ?
		WHERE id = ?`,
		item.Name, item.Metric, item.Operator, item.Threshold, item.DurationMinutes, boolInt(item.Enabled), item.WebhookURL,
		item.LastState, timePtrUnix(item.LastChangedAt), timePtrUnix(item.LastNotifiedAt), item.UpdatedAt.UTC().Unix(), item.ID)
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.AlertRule{}, domain.ErrConflict
		}
		return domain.AlertRule{}, fmt.Errorf("update alert rule: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.AlertRule{}, fmt.Errorf("read updated alert rule count: %w", err)
	}
	if affected == 0 {
		return domain.AlertRule{}, domain.ErrNotFound
	}
	return s.GetAlertRule(ctx, item.ID)
}

func (s *Store) DeleteAlertRule(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete alert rule: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted alert rule count: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanAlertRule(scanner rowScanner) (domain.AlertRule, error) {
	var item domain.AlertRule
	var enabled int
	var lastChangedAt sql.NullInt64
	var lastNotifiedAt sql.NullInt64
	var createdAt int64
	var updatedAt int64
	if err := scanner.Scan(
		&item.ID, &item.Name, &item.Metric, &item.Operator, &item.Threshold, &item.DurationMinutes, &enabled,
		&item.WebhookURL, &item.LastState, &lastChangedAt, &lastNotifiedAt, &createdAt, &updatedAt,
	); err != nil {
		return domain.AlertRule{}, err
	}
	item.Enabled = enabled != 0
	if lastChangedAt.Valid {
		value := time.Unix(lastChangedAt.Int64, 0).UTC()
		item.LastChangedAt = &value
	}
	if lastNotifiedAt.Valid {
		value := time.Unix(lastNotifiedAt.Int64, 0).UTC()
		item.LastNotifiedAt = &value
	}
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return item, nil
}

func (s *Store) InsertMetricSample(ctx context.Context, sample domain.MetricSample) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO metric_samples(scope, target_id, sampled_at, cpu_percent, memory_used_bytes, memory_total_bytes,
			disk_read_bytes_per_second, disk_write_bytes_per_second, network_rx_bytes_per_second, network_tx_bytes_per_second)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sample.Scope, sample.TargetID, sample.SampledAt.UTC().Unix(), sample.CPUPercent,
		sample.MemoryUsedBytes, sample.MemoryTotalBytes, sample.DiskReadBytesPerSecond, sample.DiskWriteBytesPerSecond,
		sample.NetworkRxBytesPerSecond, sample.NetworkTxBytesPerSecond)
	if err != nil {
		return fmt.Errorf("insert metric sample: %w", err)
	}
	return nil
}

func (s *Store) ListMetricSamples(ctx context.Context, scope, targetID string, from, to time.Time, limit int) ([]domain.MetricSample, error) {
	if limit < 1 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, scope, target_id, sampled_at, cpu_percent, memory_used_bytes, memory_total_bytes,
			disk_read_bytes_per_second, disk_write_bytes_per_second, network_rx_bytes_per_second, network_tx_bytes_per_second
		FROM metric_samples
		WHERE scope = ? AND target_id = ? AND sampled_at >= ? AND sampled_at <= ?
		ORDER BY sampled_at ASC, id ASC LIMIT ?`,
		scope, targetID, from.UTC().Unix(), to.UTC().Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("list metric samples: %w", err)
	}
	defer rows.Close()
	items := make([]domain.MetricSample, 0)
	for rows.Next() {
		var item domain.MetricSample
		var sampledAt int64
		if err := rows.Scan(&item.ID, &item.Scope, &item.TargetID, &sampledAt, &item.CPUPercent,
			&item.MemoryUsedBytes, &item.MemoryTotalBytes, &item.DiskReadBytesPerSecond, &item.DiskWriteBytesPerSecond,
			&item.NetworkRxBytesPerSecond, &item.NetworkTxBytesPerSecond); err != nil {
			return nil, fmt.Errorf("scan metric sample: %w", err)
		}
		item.SampledAt = time.Unix(sampledAt, 0).UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metric samples: %w", err)
	}
	return items, nil
}

func (s *Store) DeleteMetricSamplesBefore(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM metric_samples WHERE sampled_at < ?`, before.UTC().Unix())
	if err != nil {
		return 0, fmt.Errorf("delete expired metric samples: %w", err)
	}
	return result.RowsAffected()
}
