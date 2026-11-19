package dataplane

import (
	"context"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

type JournalStore interface {
	CreateDataPlaneOperation(ctx context.Context, operation domain.DataPlaneOperation) error
	UpdateDataPlaneOperationPhase(ctx context.Context, id, phase, targetPath string, at time.Time) error
	FinishDataPlaneOperation(ctx context.Context, id, status, message string, at time.Time) error
}

type Journal struct {
	store JournalStore
	now   func() time.Time
}

func NewJournal(store JournalStore) *Journal {
	return &Journal{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (j *Journal) Start(ctx context.Context, operation domain.DataPlaneOperation) error {
	if j == nil || j.store == nil {
		return nil
	}
	operation.ID = strings.TrimSpace(operation.ID)
	operation.Kind = strings.TrimSpace(operation.Kind)
	if operation.ID == "" || operation.Kind == "" {
		return domain.ErrInvalidInput
	}
	now := j.now()
	if operation.CreatedAt.IsZero() {
		operation.CreatedAt = now
	}
	if operation.UpdatedAt.IsZero() {
		operation.UpdatedAt = now
	}
	if operation.Status == "" {
		operation.Status = domain.DataPlaneStatusRunning
	}
	return j.store.CreateDataPlaneOperation(ctx, operation)
}

func (j *Journal) Phase(ctx context.Context, id, phase, targetPath string) {
	if j == nil || j.store == nil || id == "" {
		return
	}
	_ = j.store.UpdateDataPlaneOperationPhase(ctx, id, phase, targetPath, j.now())
}

func (j *Journal) Finish(ctx context.Context, id, status, message string) {
	if j == nil || j.store == nil || id == "" {
		return
	}
	_ = j.store.FinishDataPlaneOperation(ctx, id, status, message, j.now())
}
