package domain

import (
	"fmt"
	"strings"
	"time"
)

const (
	VMBackupFormatVersionV1 = "virtmgr.vm-backup/v1"
	VMBackupFormatVersionV2 = "virtmgr.vm-backup/v2"
)

const VMBackupFormatVersion = VMBackupFormatVersionV1

type BackupStatus string

const (
	BackupStatusRunning      BackupStatus = "running"
	BackupStatusCompleted    BackupStatus = "completed"
	BackupStatusFailed       BackupStatus = "failed"
	BackupStatusInterrupted  BackupStatus = "interrupted"
	BackupStatusDeleting     BackupStatus = "deleting"
	BackupStatusDeleteFailed BackupStatus = "delete_failed"
)

type BackupVerifyStatus string

const (
	BackupVerifyUnknown  BackupVerifyStatus = "unknown"
	BackupVerifyVerified BackupVerifyStatus = "verified"
	BackupVerifyFailed   BackupVerifyStatus = "failed"
)

type BackupArtifactCheck struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Exists        bool   `json:"exists"`
	Format        string `json:"format,omitempty"`
	CapacityBytes uint64 `json:"capacityBytes,omitempty"`
	ExpectedBytes uint64 `json:"expectedBytes,omitempty"`
	Message       string `json:"message,omitempty"`
}

type BackupVerification struct {
	BackupID  string                `json:"backupId"`
	Status    BackupVerifyStatus    `json:"status"`
	Deep      bool                  `json:"deep"`
	CheckedAt time.Time             `json:"checkedAt"`
	Message   string                `json:"message"`
	Artifacts []BackupArtifactCheck `json:"artifacts"`
}

type BackupRepository struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	PoolName  string    `json:"poolName"`
	IsDefault bool      `json:"isDefault"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type BackupRepositorySpec struct {
	Name      string `json:"name"`
	PoolName  string `json:"poolName"`
	IsDefault bool   `json:"isDefault"`
}

type BackupRepositoryList struct {
	Items []BackupRepository `json:"items"`
	Total int                `json:"total"`
}

type BackupDisk struct {
	Sequence      int    `json:"sequence"`
	Target        string `json:"target,omitempty"`
	SourceVolume  string `json:"sourceVolume"`
	Artifact      string `json:"artifact"`
	Format        string `json:"format"`
	CapacityBytes uint64 `json:"capacityBytes"`
	SHA256        string `json:"sha256,omitempty"`
}

type Backup struct {
	ID             string             `json:"id"`
	VMUUID         string             `json:"vmUuid"`
	VMName         string             `json:"vmName"`
	RepositoryID   string             `json:"repositoryId"`
	RepositoryName string             `json:"repositoryName,omitempty"`
	TargetPool     string             `json:"targetPool"`
	Name           string             `json:"name"`
	Status         BackupStatus       `json:"status"`
	FormatVersion  string             `json:"formatVersion"`
	ManifestVolume string             `json:"manifestVolume"`
	ConfigVolume   string             `json:"configVolume"`
	ConfigBytes    uint64             `json:"configBytes"`
	TotalBytes     uint64             `json:"totalBytes"`
	DiskCount      int                `json:"diskCount"`
	Pinned         bool               `json:"pinned"`
	VerifyStatus   BackupVerifyStatus `json:"verifyStatus"`
	VerifiedAt     *time.Time         `json:"verifiedAt,omitempty"`
	VerifyMessage  string             `json:"verifyMessage,omitempty"`
	Error          string             `json:"error,omitempty"`
	CreatedBy      string             `json:"createdBy"`
	CreatedAt      time.Time          `json:"createdAt"`
	CompletedAt    *time.Time         `json:"completedAt,omitempty"`
	Disks          []BackupDisk       `json:"disks,omitempty"`
}

type BackupList struct {
	Items    []Backup `json:"items"`
	Total    int      `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
}

type BackupSummary struct {
	Total           int        `json:"total"`
	Completed       int        `json:"completed"`
	Failed          int        `json:"failed"`
	Running         int        `json:"running"`
	Pinned          int        `json:"pinned"`
	TotalBytes      uint64     `json:"totalBytes"`
	LastCompletedAt *time.Time `json:"lastCompletedAt,omitempty"`
}

type BackupCreateSpec struct {
	Name         string `json:"name"`
	TargetPool   string `json:"targetPool"`
	RepositoryID string `json:"repositoryId,omitempty"`
}

type BackupQuery struct {
	VMUUID       string
	Status       BackupStatus
	RepositoryID string
	Page         int
	PageSize     int
}

const (
	DefaultBackupPageSize = 25
	MaxBackupPageSize     = 100
)

func (q BackupQuery) Normalized() BackupQuery {
	q.VMUUID = strings.TrimSpace(q.VMUUID)
	q.RepositoryID = strings.TrimSpace(q.RepositoryID)
	q.Status = BackupStatus(strings.TrimSpace(string(q.Status)))
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = DefaultBackupPageSize
	}
	if q.PageSize > MaxBackupPageSize {
		q.PageSize = MaxBackupPageSize
	}
	return q
}

func ValidBackupStatus(value BackupStatus) bool {
	switch value {
	case BackupStatusRunning, BackupStatusCompleted, BackupStatusFailed, BackupStatusInterrupted, BackupStatusDeleting, BackupStatusDeleteFailed:
		return true
	default:
		return false
	}
}

type VMRestoreSpec struct {
	Name                string `json:"name"`
	TargetPool          string `json:"targetPool"`
	Autostart           bool   `json:"autostart"`
	Start               bool   `json:"start"`
	PreservePassthrough bool   `json:"preservePassthrough"`
}

type BackupPinSpec struct {
	Pinned bool `json:"pinned"`
}

type BackupSchedule struct {
	ID             int64      `json:"id"`
	VMUUID         string     `json:"vmUuid"`
	VMName         string     `json:"vmName,omitempty"`
	RepositoryID   string     `json:"repositoryId"`
	RepositoryName string     `json:"repositoryName,omitempty"`
	TargetPool     string     `json:"targetPool"`
	Name           string     `json:"name"`
	IntervalHours  int        `json:"intervalHours"`
	RetentionCount int        `json:"retentionCount"`
	RetentionDays  int        `json:"retentionDays"`
	Enabled        bool       `json:"enabled"`
	LastRunAt      *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt      time.Time  `json:"nextRunAt"`
	LastBackupID   string     `json:"lastBackupId,omitempty"`
	LastError      string     `json:"lastError,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type BackupScheduleSpec struct {
	VMUUID         string `json:"vmUuid,omitempty"`
	RepositoryID   string `json:"repositoryId"`
	Name           string `json:"name"`
	IntervalHours  int    `json:"intervalHours"`
	RetentionCount int    `json:"retentionCount"`
	RetentionDays  int    `json:"retentionDays"`
	Enabled        bool   `json:"enabled"`
}

type BackupScheduleList struct {
	Items []BackupSchedule `json:"items"`
	Total int              `json:"total"`
}

func CanCreateBackups(role Role) bool {
	return role == RoleOperator || role == RoleAdmin
}

func CanDeleteBackups(role Role) bool {
	return role == RoleAdmin
}

func ValidateBackupName(value string) error {
	value = strings.TrimSpace(value)
	if len(value) < 2 || len(value) > 64 {
		return fmt.Errorf("%w: backup name must be between 2 and 64 characters", ErrInvalidInput)
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return fmt.Errorf("%w: backup name contains unsupported characters", ErrInvalidInput)
	}
	if value[0] == '.' || value[0] == '-' {
		return fmt.Errorf("%w: backup name must start with a letter or number", ErrInvalidInput)
	}
	return nil
}

func ValidateVMRestoreName(value string) error {
	return ValidateBackupName(strings.TrimSpace(value))
}
