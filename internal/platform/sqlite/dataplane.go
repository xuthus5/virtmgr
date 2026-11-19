package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"virtmgr/internal/domain"
)

const dataPlaneOperationColumns = `id, kind, vm_uuid, vm_name, task_id, backup_id,
	phase, online, source_pool, source_volume, source_path, target_pool,
	target_name, target_path, status, error, created_at, updated_at`

func (s *Store) CreateDataPlaneOperation(ctx context.Context, operation domain.DataPlaneOperation) error {
	now := operation.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	updated := operation.UpdatedAt
	if updated.IsZero() {
		updated = now
	}
	status := operation.Status
	if status == "" {
		status = domain.DataPlaneStatusRunning
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO data_plane_operations(
			id, kind, vm_uuid, vm_name, task_id, backup_id, phase, online,
			source_pool, source_volume, source_path, target_pool, target_name,
			target_path, status, error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		operation.ID, operation.Kind, operation.VMUUID, operation.VMName,
		operation.TaskID, operation.BackupID, operation.Phase, boolInt(operation.Online),
		operation.SourcePool, operation.SourceVolume, operation.SourcePath,
		operation.TargetPool, operation.TargetName, operation.TargetPath,
		status, operation.Error, now.UTC().Unix(), updated.UTC().Unix(),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.ErrConflict
		}
		return fmt.Errorf("create data plane operation: %w", err)
	}
	return nil
}

func (s *Store) UpdateDataPlaneOperationPhase(ctx context.Context, id, phase, targetPath string, at time.Time) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE data_plane_operations
		SET phase = ?, target_path = CASE WHEN ? <> '' THEN ? ELSE target_path END, updated_at = ?
		WHERE id = ? AND status = ?`,
		phase, targetPath, targetPath, at.UTC().Unix(), id, domain.DataPlaneStatusRunning,
	)
	if err != nil {
		return fmt.Errorf("update data plane operation phase: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated data plane operation: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) FinishDataPlaneOperation(ctx context.Context, id, status, message string, at time.Time) error {
	if !domain.DataPlaneOperationTerminal(status) {
		return domain.ErrInvalidInput
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE data_plane_operations
		SET status = ?, error = ?, updated_at = ?
		WHERE id = ? AND status = ?`,
		status, message, at.UTC().Unix(), id, domain.DataPlaneStatusRunning,
	)
	if err != nil {
		return fmt.Errorf("finish data plane operation: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count finished data plane operation: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) ListPendingDataPlaneOperations(ctx context.Context) ([]domain.DataPlaneOperation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+dataPlaneOperationColumns+`
		FROM data_plane_operations
		WHERE status = ?
		ORDER BY created_at ASC, id ASC`, domain.DataPlaneStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("list pending data plane operations: %w", err)
	}
	defer rows.Close()
	items := make([]domain.DataPlaneOperation, 0)
	for rows.Next() {
		item, err := scanDataPlaneOperation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending data plane operations: %w", err)
	}
	return items, nil
}

func (s *Store) ListDataPlaneOperations(ctx context.Context, limit int) ([]domain.DataPlaneOperation, error) {
	if limit < 1 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+dataPlaneOperationColumns+`
		FROM data_plane_operations
		ORDER BY CASE status WHEN 'needs_attention' THEN 0 WHEN 'failed' THEN 1 WHEN 'running' THEN 2 ELSE 3 END,
			created_at DESC, id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list data plane operations: %w", err)
	}
	defer rows.Close()
	items := make([]domain.DataPlaneOperation, 0)
	for rows.Next() {
		item, err := scanDataPlaneOperation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan data plane operation: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate data plane operations: %w", err)
	}
	return items, nil
}

func (s *Store) ListUnresolvedDataPlaneOperations(ctx context.Context) ([]domain.DataPlaneOperation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+dataPlaneOperationColumns+`
		FROM data_plane_operations
		WHERE status IN (?, ?)
		ORDER BY created_at ASC, id ASC`, domain.DataPlaneStatusNeedsAttention, domain.DataPlaneStatusFailed)
	if err != nil {
		return nil, fmt.Errorf("list unresolved data plane operations: %w", err)
	}
	defer rows.Close()
	items := make([]domain.DataPlaneOperation, 0)
	for rows.Next() {
		item, err := scanDataPlaneOperation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan unresolved data plane operation: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unresolved data plane operations: %w", err)
	}
	return items, nil
}

func (s *Store) GetDataPlaneOperation(ctx context.Context, id string) (domain.DataPlaneOperation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+dataPlaneOperationColumns+`
		FROM data_plane_operations WHERE id = ?`, id)
	item, err := scanDataPlaneOperation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DataPlaneOperation{}, domain.ErrNotFound
	}
	return item, err
}

func (s *Store) ResolveDataPlaneOperation(ctx context.Context, id, status, message string, at time.Time) error {
	if !domain.DataPlaneOperationTerminal(status) || status == domain.DataPlaneStatusRunning {
		return domain.ErrInvalidInput
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE data_plane_operations
		SET status = ?, error = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?)`,
		status, message, at.UTC().Unix(), id,
		domain.DataPlaneStatusNeedsAttention, domain.DataPlaneStatusFailed,
	)
	if err != nil {
		return fmt.Errorf("resolve data plane operation: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count resolved data plane operation: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateInterruptedTaskDetail(ctx context.Context, id, step, taskError string, at time.Time) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET step = ?, error = ?, updated_at = ?
		WHERE id = ? AND status = ?`,
		step, taskError, at.UTC().Unix(), id, domain.TaskStatusInterrupted,
	)
	if err != nil {
		return fmt.Errorf("update interrupted task detail: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated interrupted task: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

type dataPlaneScanner interface {
	Scan(dest ...any) error
}

func scanDataPlaneOperation(scanner dataPlaneScanner) (domain.DataPlaneOperation, error) {
	var item domain.DataPlaneOperation
	var online int
	var createdAt int64
	var updatedAt int64
	if err := scanner.Scan(
		&item.ID, &item.Kind, &item.VMUUID, &item.VMName, &item.TaskID,
		&item.BackupID, &item.Phase, &online, &item.SourcePool, &item.SourceVolume,
		&item.SourcePath, &item.TargetPool, &item.TargetName, &item.TargetPath,
		&item.Status, &item.Error, &createdAt, &updatedAt,
	); err != nil {
		return domain.DataPlaneOperation{}, err
	}
	item.Online = online != 0
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return item, nil
}
