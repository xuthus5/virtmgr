package domain

import (
	"strings"
	"time"
)

const (
	DataPlaneOperationBackup  = "vm.backup"
	DataPlaneOperationMigrate = "vm.disk.migrate"
	DataPlaneOperationImport  = "vm.import"
	DataPlaneOperationConvert = "storage.volume.convert"
	DataPlaneOperationFlatten = "storage.volume.flatten"
)

const (
	DataPlaneStatusRunning        = "running"
	DataPlaneStatusSucceeded      = "succeeded"
	DataPlaneStatusFailed         = "failed"
	DataPlaneStatusRecovered      = "recovered"
	DataPlaneStatusNeedsAttention = "needs_attention"
)

const (
	DataPlaneRecoveryCompleted      = "completed"
	DataPlaneRecoveryRolledBack     = "rolled_back"
	DataPlaneRecoveryCleaned        = "cleaned"
	DataPlaneRecoveryNoop           = "noop"
	DataPlaneRecoveryNeedsAttention = "needs_attention"
)

// DataPlaneOperation is the durable journal entry for a mutation that can
// leave libvirt snapshots, overlay disks or target volumes behind if the
// process exits before the operation is finalized.
type DataPlaneOperation struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	VMUUID       string    `json:"vmUuid,omitempty"`
	VMName       string    `json:"vmName,omitempty"`
	TaskID       string    `json:"taskId,omitempty"`
	BackupID     string    `json:"backupId,omitempty"`
	Phase        string    `json:"phase"`
	Online       bool      `json:"online"`
	SourcePool   string    `json:"sourcePool,omitempty"`
	SourceVolume string    `json:"sourceVolume,omitempty"`
	SourcePath   string    `json:"sourcePath,omitempty"`
	TargetPool   string    `json:"targetPool,omitempty"`
	TargetName   string    `json:"targetName,omitempty"`
	TargetPath   string    `json:"targetPath,omitempty"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// DataPlaneRecovery describes the outcome of reconciling one interrupted
// operation against the current libvirt state.
type DataPlaneRecovery struct {
	Status          string     `json:"status"`
	Message         string     `json:"message"`
	CompletedBackup *VMBackup  `json:"completedBackup,omitempty"`
	RecoveredAt     *time.Time `json:"recoveredAt,omitempty"`
}

func VMLockKey(uuid string) string {
	return "vm/" + strings.TrimSpace(uuid)
}

func VMNameLockKey(name string) string {
	return "vm-name/" + strings.ToLower(strings.TrimSpace(name))
}

func StorageVolumeLockKey(pool, volume string) string {
	return "storage/" + strings.TrimSpace(pool) + "/" + strings.TrimSpace(volume)
}

func DataPlaneOperationTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case DataPlaneStatusSucceeded, DataPlaneStatusFailed, DataPlaneStatusRecovered, DataPlaneStatusNeedsAttention:
		return true
	default:
		return false
	}
}
