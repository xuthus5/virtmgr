package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

type Repository interface {
	CreateTask(ctx context.Context, task domain.Task) error
	UpdateTask(ctx context.Context, task domain.Task) error
}

type MetadataRepository interface {
	SetTaskMetadata(ctx context.Context, id string, metadata map[string]any) error
}

type Recorder struct {
	repository Repository
	now        func() time.Time
	newID      func() (string, error)
}

func New(repository Repository) *Recorder {
	return &Recorder{repository: repository, now: time.Now, newID: generateTaskID}
}

func (r *Recorder) Start(ctx context.Context, kind, targetKind, targetID, actor, step string) (string, error) {
	kind = strings.TrimSpace(kind)
	targetKind = strings.TrimSpace(targetKind)
	targetID = strings.TrimSpace(targetID)
	actor = strings.TrimSpace(actor)
	step = strings.TrimSpace(step)
	if kind == "" || targetKind == "" || targetID == "" || actor == "" {
		return "", domain.ErrInvalidInput
	}
	id, err := r.newID()
	if err != nil {
		return "", fmt.Errorf("generate task id: %w", err)
	}
	now := r.now().UTC()
	if step == "" {
		step = "任务已开始"
	}
	task := domain.Task{
		ID: id, Kind: kind, TargetKind: targetKind, TargetID: targetID,
		Status: domain.TaskStatusRunning, Progress: 0, Step: step, Actor: actor,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := r.repository.CreateTask(ctx, task); err != nil {
		return "", fmt.Errorf("create task: %w", err)
	}
	return id, nil
}

func (r *Recorder) Progress(ctx context.Context, id string, progress int, step string) error {
	return r.update(ctx, id, domain.TaskStatusRunning, progress, step, "")
}

func (r *Recorder) SetMetadata(ctx context.Context, id string, metadata map[string]any) error {
	repository, ok := r.repository.(MetadataRepository)
	if !ok {
		return nil
	}
	if strings.TrimSpace(id) == "" {
		return domain.ErrInvalidInput
	}
	if err := repository.SetTaskMetadata(ctx, id, metadata); err != nil {
		return fmt.Errorf("set task metadata %s: %w", id, err)
	}
	return nil
}

func (r *Recorder) Succeed(ctx context.Context, id, step string) error {
	return r.update(ctx, id, domain.TaskStatusSucceeded, 100, step, "")
}

func (r *Recorder) Fail(ctx context.Context, id string, progress int, step string, cause error) error {
	message := "任务执行失败"
	if cause != nil {
		message = cause.Error()
	}
	return r.update(ctx, id, domain.TaskStatusFailed, progress, step, message)
}

func (r *Recorder) update(ctx context.Context, id string, status domain.TaskStatus, progress int, step, taskError string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.ErrInvalidInput
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	task := domain.Task{
		ID: id, Status: status, Progress: progress, Step: strings.TrimSpace(step), Error: strings.TrimSpace(taskError),
		UpdatedAt: r.now().UTC(),
	}
	if err := r.repository.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("update task %s: %w", id, err)
	}
	return nil
}

func generateTaskID() (string, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "task_" + hex.EncodeToString(random[:]), nil
}
