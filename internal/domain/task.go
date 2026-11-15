package domain

import (
	"strings"
	"time"
)

type TaskStatus string

const (
	TaskStatusQueued      TaskStatus = "queued"
	TaskStatusRunning     TaskStatus = "running"
	TaskStatusSucceeded   TaskStatus = "succeeded"
	TaskStatusFailed      TaskStatus = "failed"
	TaskStatusInterrupted TaskStatus = "interrupted"
	TaskStatusCancelled   TaskStatus = "cancelled"
)

type Task struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	TargetKind  string         `json:"targetKind"`
	TargetID    string         `json:"targetId"`
	Status      TaskStatus     `json:"status"`
	Progress    int            `json:"progress"`
	Step        string         `json:"step"`
	Error       string         `json:"error,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Actor       string         `json:"actor"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	Cancellable bool           `json:"cancellable"`
}

type TaskQuery struct {
	Status   TaskStatus
	Page     int
	PageSize int
}

type TaskList struct {
	Items    []Task `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

const (
	DefaultTaskPageSize = 50
	MaxTaskPageSize     = 200
)

func ValidTaskStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusQueued, TaskStatusRunning, TaskStatusSucceeded, TaskStatusFailed, TaskStatusInterrupted, TaskStatusCancelled:
		return true
	default:
		return false
	}
}

func CancellableTaskKind(kind string) bool {
	switch kind {
	case "vm.clone", "vm.import", "vm.backup", "vm.backup.verify", "vm.backup.restore", "vm.backup.delete", "storage.volume.upload":
		return true
	default:
		return false
	}
}

func TaskCancellable(task Task) bool {
	return (task.Status == TaskStatusQueued || task.Status == TaskStatusRunning) && CancellableTaskKind(task.Kind)
}

func (q TaskQuery) Normalized() TaskQuery {
	q.Status = TaskStatus(strings.TrimSpace(string(q.Status)))
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = DefaultTaskPageSize
	}
	if q.PageSize > MaxTaskPageSize {
		q.PageSize = MaxTaskPageSize
	}
	return q
}
