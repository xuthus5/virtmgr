package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"virtmgr/internal/domain"
)

func (s *Store) CreateTask(ctx context.Context, task domain.Task) error {
	createdAt := task.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := task.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	metadata, err := encodeTaskMetadata(task.Metadata)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tasks(id, kind, target_kind, target_id, status, progress, step, error, metadata_json, actor, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.Kind, task.TargetKind, task.TargetID, string(task.Status), task.Progress,
		task.Step, task.Error, metadata, task.Actor, createdAt.UTC().Unix(), updatedAt.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	return nil
}

func (s *Store) SetTaskMetadata(ctx context.Context, id string, metadata map[string]any) error {
	encoded, err := encodeTaskMetadata(metadata)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE tasks SET metadata_json = ? WHERE id = ?`, encoded, id)
	if err != nil {
		return fmt.Errorf("update task metadata: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated task metadata: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateTask(ctx context.Context, task domain.Task) error {
	updatedAt := task.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET status = ?, progress = ?, step = ?, error = ?, updated_at = ?
		WHERE id = ?`,
		string(task.Status), task.Progress, task.Step, task.Error, updatedAt.UTC().Unix(), task.ID,
	)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated task: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) GetTask(ctx context.Context, id string) (domain.Task, error) {
	var task domain.Task
	var status string
	var metadataJSON string
	var createdAt int64
	var updatedAt int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, kind, target_kind, target_id, status, progress, step, error, metadata_json, actor, created_at, updated_at
		FROM tasks WHERE id = ?`, id).Scan(
		&task.ID, &task.Kind, &task.TargetKind, &task.TargetID, &status, &task.Progress,
		&task.Step, &task.Error, &metadataJSON, &task.Actor, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return domain.Task{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Task{}, fmt.Errorf("get task: %w", err)
	}
	task.Status = domain.TaskStatus(status)
	task.Metadata = decodeTaskMetadata(metadataJSON)
	task.CreatedAt = time.Unix(createdAt, 0).UTC()
	task.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return task, nil
}

func (s *Store) ListTasks(ctx context.Context, input domain.TaskQuery) (domain.TaskList, error) {
	query := input.Normalized()
	where := ""
	args := make([]any, 0, 1)
	if query.Status != "" {
		where = " WHERE status = ?"
		args = append(args, string(query.Status))
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`+where, args...).Scan(&total); err != nil {
		return domain.TaskList{}, fmt.Errorf("count tasks: %w", err)
	}
	offset := (query.Page - 1) * query.PageSize
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, kind, target_kind, target_id, status, progress, step, error, metadata_json, actor, created_at, updated_at
		FROM tasks`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?`, append(args, query.PageSize, offset)...)
	if err != nil {
		return domain.TaskList{}, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	items := make([]domain.Task, 0, query.PageSize)
	for rows.Next() {
		var task domain.Task
		var status string
		var metadataJSON string
		var createdAt int64
		var updatedAt int64
		if err := rows.Scan(
			&task.ID, &task.Kind, &task.TargetKind, &task.TargetID, &status, &task.Progress,
			&task.Step, &task.Error, &metadataJSON, &task.Actor, &createdAt, &updatedAt,
		); err != nil {
			return domain.TaskList{}, fmt.Errorf("scan task: %w", err)
		}
		task.Status = domain.TaskStatus(status)
		task.Metadata = decodeTaskMetadata(metadataJSON)
		task.CreatedAt = time.Unix(createdAt, 0).UTC()
		task.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		items = append(items, task)
	}
	if err := rows.Err(); err != nil {
		return domain.TaskList{}, fmt.Errorf("iterate tasks: %w", err)
	}
	return domain.TaskList{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

func encodeTaskMetadata(metadata map[string]any) (string, error) {
	if len(metadata) == 0 {
		return "{}", nil
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode task metadata: %w", err)
	}
	return string(encoded), nil
}

func decodeTaskMetadata(encoded string) map[string]any {
	if encoded == "" || encoded == "{}" {
		return nil
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(encoded), &metadata); err != nil {
		return nil
	}
	return metadata
}

func (s *Store) MarkRunningTasksInterrupted(ctx context.Context, now time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET status = ?, step = ?, error = ?, updated_at = ?
		WHERE status IN (?, ?)`,
		string(domain.TaskStatusInterrupted), "服务重启，任务已中断", "VirtMgr 进程在任务完成前退出",
		now.UTC().Unix(), string(domain.TaskStatusQueued), string(domain.TaskStatusRunning),
	)
	if err != nil {
		return 0, fmt.Errorf("mark interrupted tasks: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count interrupted tasks: %w", err)
	}
	return affected, nil
}
