package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

const backupColumns = `b.id, b.vm_uuid, b.vm_name, b.repository_id, COALESCE(r.name, ''), b.target_pool,
	b.name, b.status, b.format_version, b.manifest_volume, b.config_volume, b.config_bytes,
	b.total_bytes, b.disk_count, b.pinned, b.verify_status, b.verified_at, b.verify_message,
	b.error, b.created_by, b.created_at, b.completed_at`

func (s *Store) ListBackupRepositories(ctx context.Context) ([]domain.BackupRepository, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, pool_name, is_default, created_at, updated_at
		FROM backup_repositories
		ORDER BY is_default DESC, name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, fmt.Errorf("list backup repositories: %w", err)
	}
	defer rows.Close()
	items := make([]domain.BackupRepository, 0)
	for rows.Next() {
		item, err := scanBackupRepository(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup repositories: %w", err)
	}
	return items, nil
}

func (s *Store) GetBackupRepository(ctx context.Context, id string) (domain.BackupRepository, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, type, pool_name, is_default, created_at, updated_at
		FROM backup_repositories WHERE id = ?`, id)
	item, err := scanBackupRepository(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.BackupRepository{}, domain.ErrNotFound
	}
	return item, err
}

func (s *Store) BackupRepositoryByPool(ctx context.Context, poolName string) (domain.BackupRepository, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, type, pool_name, is_default, created_at, updated_at
		FROM backup_repositories WHERE pool_name = ?`, poolName)
	item, err := scanBackupRepository(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.BackupRepository{}, domain.ErrNotFound
	}
	return item, err
}

func (s *Store) DefaultBackupRepository(ctx context.Context) (domain.BackupRepository, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, type, pool_name, is_default, created_at, updated_at
		FROM backup_repositories ORDER BY is_default DESC, created_at ASC LIMIT 1`)
	item, err := scanBackupRepository(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.BackupRepository{}, domain.ErrNotFound
	}
	return item, err
}

func (s *Store) CreateBackupRepository(ctx context.Context, item domain.BackupRepository) (domain.BackupRepository, error) {
	now := item.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	updated := item.UpdatedAt
	if updated.IsZero() {
		updated = now
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.BackupRepository{}, fmt.Errorf("begin backup repository create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if item.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE backup_repositories SET is_default = 0, updated_at = ? WHERE is_default = 1`, now.Unix()); err != nil {
			return domain.BackupRepository{}, fmt.Errorf("clear default backup repository: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO backup_repositories(id, name, type, pool_name, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, item.ID, item.Name, item.Type, item.PoolName, boolInt(item.IsDefault), now.Unix(), updated.Unix()); err != nil {
		if isUniqueConstraint(err) {
			return domain.BackupRepository{}, domain.ErrConflict
		}
		return domain.BackupRepository{}, fmt.Errorf("create backup repository: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.BackupRepository{}, fmt.Errorf("commit backup repository create: %w", err)
	}
	return s.GetBackupRepository(ctx, item.ID)
}

func (s *Store) UpdateBackupRepository(ctx context.Context, item domain.BackupRepository) (domain.BackupRepository, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.BackupRepository{}, fmt.Errorf("begin backup repository update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if item.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE backup_repositories SET is_default = 0, updated_at = ? WHERE is_default = 1 AND id <> ?`, now.Unix(), item.ID); err != nil {
			return domain.BackupRepository{}, fmt.Errorf("clear previous default backup repository: %w", err)
		}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE backup_repositories SET name = ?, pool_name = ?, is_default = ?, updated_at = ? WHERE id = ?`,
		item.Name, item.PoolName, boolInt(item.IsDefault), now.Unix(), item.ID)
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.BackupRepository{}, domain.ErrConflict
		}
		return domain.BackupRepository{}, fmt.Errorf("update backup repository: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.BackupRepository{}, fmt.Errorf("read updated backup repository count: %w", err)
	}
	if affected == 0 {
		return domain.BackupRepository{}, domain.ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return domain.BackupRepository{}, fmt.Errorf("commit backup repository update: %w", err)
	}
	return s.GetBackupRepository(ctx, item.ID)
}

func (s *Store) DeleteBackupRepository(ctx context.Context, id string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM backups WHERE repository_id = ?`, id).Scan(&count); err != nil {
		return fmt.Errorf("count repository backups: %w", err)
	}
	if count > 0 {
		return domain.ErrConflict
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM backup_schedules WHERE repository_id = ?`, id).Scan(&count); err != nil {
		return fmt.Errorf("count repository schedules: %w", err)
	}
	if count > 0 {
		return domain.ErrConflict
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM backup_repositories WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete backup repository: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted backup repository count: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) ListBackups(ctx context.Context, query domain.BackupQuery) (domain.BackupList, error) {
	query = query.Normalized()
	where, args := backupWhere(query)
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM backups b WHERE `+where, args...).Scan(&total); err != nil {
		return domain.BackupList{}, fmt.Errorf("count backups: %w", err)
	}
	args = append(args, query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+backupColumns+`
		FROM backups b LEFT JOIN backup_repositories r ON r.id = b.repository_id
		WHERE `+where+`
		ORDER BY b.created_at DESC, b.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return domain.BackupList{}, fmt.Errorf("list backups: %w", err)
	}
	defer rows.Close()
	items := make([]domain.Backup, 0)
	for rows.Next() {
		item, err := scanBackup(rows)
		if err != nil {
			return domain.BackupList{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.BackupList{}, fmt.Errorf("iterate backups: %w", err)
	}
	for index := range items {
		disks, err := s.listBackupDisks(ctx, items[index].ID)
		if err != nil {
			return domain.BackupList{}, err
		}
		items[index].Disks = disks
	}
	return domain.BackupList{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

func backupWhere(query domain.BackupQuery) (string, []any) {
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 3)
	if query.VMUUID != "" {
		clauses = append(clauses, "b.vm_uuid = ?")
		args = append(args, query.VMUUID)
	}
	if query.Status != "" {
		clauses = append(clauses, "b.status = ?")
		args = append(args, string(query.Status))
	}
	if query.RepositoryID != "" {
		clauses = append(clauses, "b.repository_id = ?")
		args = append(args, query.RepositoryID)
	}
	return strings.Join(clauses, " AND "), args
}

func (s *Store) GetBackup(ctx context.Context, id string) (domain.Backup, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+backupColumns+`
		FROM backups b LEFT JOIN backup_repositories r ON r.id = b.repository_id
		WHERE b.id = ?`, id)
	item, err := scanBackup(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Backup{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Backup{}, err
	}
	disks, err := s.listBackupDisks(ctx, id)
	if err != nil {
		return domain.Backup{}, err
	}
	item.Disks = disks
	return item, nil
}

func (s *Store) listBackupDisks(ctx context.Context, backupID string) ([]domain.BackupDisk, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sequence, target, source_volume, artifact_volume, format, capacity_bytes, sha256
		FROM backup_disks WHERE backup_id = ? ORDER BY sequence ASC`, backupID)
	if err != nil {
		return nil, fmt.Errorf("list backup disks: %w", err)
	}
	defer rows.Close()
	items := make([]domain.BackupDisk, 0)
	for rows.Next() {
		var item domain.BackupDisk
		if err := rows.Scan(&item.Sequence, &item.Target, &item.SourceVolume, &item.Artifact, &item.Format, &item.CapacityBytes, &item.SHA256); err != nil {
			return nil, fmt.Errorf("scan backup disk: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup disks: %w", err)
	}
	return items, nil
}

func (s *Store) CreateBackup(ctx context.Context, item domain.Backup) (domain.Backup, error) {
	now := item.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Backup{}, fmt.Errorf("begin backup create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO backups(
			id, vm_uuid, vm_name, repository_id, target_pool, name, status, format_version,
			manifest_volume, config_volume, config_bytes, total_bytes, disk_count, pinned,
			verify_status, verified_at, verify_message, error, created_by, created_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.VMUUID, item.VMName, item.RepositoryID, item.TargetPool, item.Name, string(item.Status),
		item.FormatVersion, item.ManifestVolume, item.ConfigVolume, item.ConfigBytes, item.TotalBytes,
		item.DiskCount, boolInt(item.Pinned), string(item.VerifyStatus), timePtrUnix(item.VerifiedAt),
		item.VerifyMessage, item.Error, item.CreatedBy, now.UTC().Unix(), timePtrUnix(item.CompletedAt)); err != nil {
		if isUniqueConstraint(err) {
			return domain.Backup{}, domain.ErrConflict
		}
		return domain.Backup{}, fmt.Errorf("create backup: %w", err)
	}
	if err := insertBackupDisks(ctx, tx, item.ID, item.Disks); err != nil {
		return domain.Backup{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Backup{}, fmt.Errorf("commit backup create: %w", err)
	}
	return s.GetBackup(ctx, item.ID)
}

func (s *Store) UpdateBackup(ctx context.Context, item domain.Backup) (domain.Backup, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Backup{}, fmt.Errorf("begin backup update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	completedAt := timePtrUnix(item.CompletedAt)
	verifiedAt := timePtrUnix(item.VerifiedAt)
	result, err := tx.ExecContext(ctx, `
		UPDATE backups SET
			status = ?, format_version = ?, manifest_volume = ?, config_volume = ?,
			config_bytes = ?, total_bytes = ?, disk_count = ?, pinned = ?, verify_status = ?,
			verified_at = ?, verify_message = ?, error = ?, completed_at = ?
		WHERE id = ?`,
		string(item.Status), item.FormatVersion, item.ManifestVolume, item.ConfigVolume, item.ConfigBytes,
		item.TotalBytes, item.DiskCount, boolInt(item.Pinned), string(item.VerifyStatus), verifiedAt,
		item.VerifyMessage, item.Error, completedAt, item.ID)
	if err != nil {
		return domain.Backup{}, fmt.Errorf("update backup: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.Backup{}, fmt.Errorf("read updated backup count: %w", err)
	}
	if affected == 0 {
		return domain.Backup{}, domain.ErrNotFound
	}
	if item.Disks != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM backup_disks WHERE backup_id = ?`, item.ID); err != nil {
			return domain.Backup{}, fmt.Errorf("replace backup disks: %w", err)
		}
		if err := insertBackupDisks(ctx, tx, item.ID, item.Disks); err != nil {
			return domain.Backup{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.Backup{}, fmt.Errorf("commit backup update: %w", err)
	}
	return s.GetBackup(ctx, item.ID)
}

func insertBackupDisks(ctx context.Context, tx *sql.Tx, backupID string, disks []domain.BackupDisk) error {
	for _, disk := range disks {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO backup_disks(backup_id, sequence, target, source_volume, artifact_volume, format, capacity_bytes, sha256)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, backupID, disk.Sequence, disk.Target, disk.SourceVolume, disk.Artifact, disk.Format, disk.CapacityBytes, disk.SHA256); err != nil {
			return fmt.Errorf("create backup disk: %w", err)
		}
	}
	return nil
}

func (s *Store) DeleteBackup(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted backup count: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) MarkRunningBackupsInterrupted(ctx context.Context, now time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE backups
		SET status = 'interrupted', error = '服务重启时备份任务未完成', completed_at = ?
		WHERE status IN ('running', 'deleting')`, now.UTC().Unix())
	if err != nil {
		return 0, fmt.Errorf("mark running backups interrupted: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read interrupted backup count: %w", err)
	}
	return affected, nil
}

func (s *Store) BackupSummary(ctx context.Context, vmUUID string) (domain.BackupSummary, error) {
	where := "1 = 1"
	args := make([]any, 0, 1)
	if vmUUID = strings.TrimSpace(vmUUID); vmUUID != "" {
		where = "vm_uuid = ?"
		args = append(args, vmUUID)
	}
	var summary domain.BackupSummary
	var lastCompleted sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN pinned = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(total_bytes), 0), MAX(completed_at)
		FROM backups WHERE `+where, args...).Scan(
		&summary.Total, &summary.Completed, &summary.Failed, &summary.Running,
		&summary.Pinned, &summary.TotalBytes, &lastCompleted,
	); err != nil {
		return domain.BackupSummary{}, fmt.Errorf("summarize backups: %w", err)
	}
	if lastCompleted.Valid {
		value := time.Unix(lastCompleted.Int64, 0).UTC()
		summary.LastCompletedAt = &value
	}
	return summary, nil
}

func (s *Store) ListCompletedBackups(ctx context.Context, vmUUID string, limit int) ([]domain.Backup, error) {
	if limit < 1 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+backupColumns+`
		FROM backups b LEFT JOIN backup_repositories r ON r.id = b.repository_id
		WHERE b.vm_uuid = ? AND b.status = 'completed'
		ORDER BY b.completed_at DESC, b.created_at DESC LIMIT ?`, vmUUID, limit)
	if err != nil {
		return nil, fmt.Errorf("list completed backups: %w", err)
	}
	defer rows.Close()
	items := make([]domain.Backup, 0)
	for rows.Next() {
		item, err := scanBackup(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate completed backups: %w", err)
	}
	return items, nil
}

func (s *Store) ListBackupSchedules(ctx context.Context, vmUUID string) ([]domain.BackupSchedule, error) {
	query := `SELECT ` + backupScheduleColumns + ` FROM backup_schedules s LEFT JOIN backup_repositories r ON r.id = s.repository_id`
	args := make([]any, 0, 1)
	if strings.TrimSpace(vmUUID) != "" {
		query += ` WHERE s.vm_uuid = ?`
		args = append(args, strings.TrimSpace(vmUUID))
	}
	query += ` ORDER BY s.enabled DESC, s.next_run_at ASC, s.id ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list backup schedules: %w", err)
	}
	defer rows.Close()
	items := make([]domain.BackupSchedule, 0)
	for rows.Next() {
		item, err := scanBackupSchedule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup schedules: %w", err)
	}
	return items, nil
}

func (s *Store) GetBackupSchedule(ctx context.Context, id int64) (domain.BackupSchedule, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+backupScheduleColumns+`
		FROM backup_schedules s LEFT JOIN backup_repositories r ON r.id = s.repository_id
		WHERE s.id = ?`, id)
	item, err := scanBackupSchedule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.BackupSchedule{}, domain.ErrNotFound
	}
	return item, err
}

func (s *Store) CreateBackupSchedule(ctx context.Context, item domain.BackupSchedule) (domain.BackupSchedule, error) {
	now := item.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	updated := item.UpdatedAt
	if updated.IsZero() {
		updated = now
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO backup_schedules(
			vm_uuid, repository_id, target_pool, name, interval_hours, schedule_mode, daily_time, retention_count,
			retention_days, enabled, last_run_at, next_run_at, last_backup_id, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.VMUUID, item.RepositoryID, item.TargetPool, item.Name, item.IntervalHours, item.ScheduleMode, item.DailyTime,
		item.RetentionCount, item.RetentionDays, boolInt(item.Enabled), timePtrUnix(item.LastRunAt),
		item.NextRunAt.UTC().Unix(), item.LastBackupID, item.LastError, now.Unix(), updated.Unix())
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.BackupSchedule{}, domain.ErrConflict
		}
		return domain.BackupSchedule{}, fmt.Errorf("create backup schedule: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.BackupSchedule{}, fmt.Errorf("read backup schedule id: %w", err)
	}
	return s.GetBackupSchedule(ctx, id)
}

func (s *Store) UpdateBackupSchedule(ctx context.Context, item domain.BackupSchedule) (domain.BackupSchedule, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE backup_schedules SET
			repository_id = ?, target_pool = ?, name = ?, interval_hours = ?, schedule_mode = ?, daily_time = ?, retention_count = ?,
			retention_days = ?, enabled = ?, last_run_at = ?, next_run_at = ?, last_backup_id = ?,
			last_error = ?, updated_at = ?
		WHERE id = ?`,
		item.RepositoryID, item.TargetPool, item.Name, item.IntervalHours, item.ScheduleMode, item.DailyTime, item.RetentionCount,
		item.RetentionDays, boolInt(item.Enabled), timePtrUnix(item.LastRunAt), item.NextRunAt.UTC().Unix(),
		item.LastBackupID, item.LastError, time.Now().UTC().Unix(), item.ID)
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.BackupSchedule{}, domain.ErrConflict
		}
		return domain.BackupSchedule{}, fmt.Errorf("update backup schedule: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.BackupSchedule{}, fmt.Errorf("read updated backup schedule count: %w", err)
	}
	if affected == 0 {
		return domain.BackupSchedule{}, domain.ErrNotFound
	}
	return s.GetBackupSchedule(ctx, item.ID)
}

func (s *Store) DeleteBackupSchedule(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM backup_schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete backup schedule: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted backup schedule count: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) ListDueBackupSchedules(ctx context.Context, now time.Time, limit int) ([]domain.BackupSchedule, error) {
	if limit < 1 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+backupScheduleColumns+`
		FROM backup_schedules s LEFT JOIN backup_repositories r ON r.id = s.repository_id
		WHERE s.enabled = 1 AND s.next_run_at <= ?
		ORDER BY s.next_run_at ASC, s.id ASC LIMIT ?`, now.UTC().Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("list due backup schedules: %w", err)
	}
	defer rows.Close()
	items := make([]domain.BackupSchedule, 0)
	for rows.Next() {
		item, err := scanBackupSchedule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due backup schedules: %w", err)
	}
	return items, nil
}

const backupScheduleColumns = `s.id, s.vm_uuid, s.repository_id, COALESCE(r.name, ''), s.target_pool,
	s.name, s.interval_hours, s.schedule_mode, s.daily_time, s.retention_count, s.retention_days, s.enabled, s.last_run_at,
	s.next_run_at, s.last_backup_id, s.last_error, s.created_at, s.updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBackupRepository(scanner rowScanner) (domain.BackupRepository, error) {
	var item domain.BackupRepository
	var isDefault int
	var createdAt int64
	var updatedAt int64
	if err := scanner.Scan(&item.ID, &item.Name, &item.Type, &item.PoolName, &isDefault, &createdAt, &updatedAt); err != nil {
		return domain.BackupRepository{}, err
	}
	item.IsDefault = isDefault != 0
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return item, nil
}

func scanBackup(scanner rowScanner) (domain.Backup, error) {
	var item domain.Backup
	var pinned int
	var verifiedAt sql.NullInt64
	var createdAt int64
	var completedAt sql.NullInt64
	if err := scanner.Scan(
		&item.ID, &item.VMUUID, &item.VMName, &item.RepositoryID, &item.RepositoryName, &item.TargetPool,
		&item.Name, &item.Status, &item.FormatVersion, &item.ManifestVolume, &item.ConfigVolume,
		&item.ConfigBytes, &item.TotalBytes, &item.DiskCount, &pinned, &item.VerifyStatus,
		&verifiedAt, &item.VerifyMessage, &item.Error, &item.CreatedBy, &createdAt, &completedAt,
	); err != nil {
		return domain.Backup{}, err
	}
	item.Pinned = pinned != 0
	if verifiedAt.Valid {
		value := time.Unix(verifiedAt.Int64, 0).UTC()
		item.VerifiedAt = &value
	}
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	if completedAt.Valid {
		value := time.Unix(completedAt.Int64, 0).UTC()
		item.CompletedAt = &value
	}
	return item, nil
}

func scanBackupSchedule(scanner rowScanner) (domain.BackupSchedule, error) {
	var item domain.BackupSchedule
	var enabled int
	var lastRunAt sql.NullInt64
	var nextRunAt int64
	var createdAt int64
	var updatedAt int64
	if err := scanner.Scan(
		&item.ID, &item.VMUUID, &item.RepositoryID, &item.RepositoryName, &item.TargetPool,
		&item.Name, &item.IntervalHours, &item.ScheduleMode, &item.DailyTime, &item.RetentionCount, &item.RetentionDays, &enabled,
		&lastRunAt, &nextRunAt, &item.LastBackupID, &item.LastError, &createdAt, &updatedAt,
	); err != nil {
		return domain.BackupSchedule{}, err
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
