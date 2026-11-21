package domain

import "time"

const BackupPackageFormatVersion = "virtmgr.backup-package/v1"

type BackupManifest struct {
	FormatVersion string                 `json:"formatVersion"`
	BackupID      string                 `json:"backupId,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
	ToolVersion   string                 `json:"toolVersion,omitempty"`
	Source        *BackupManifestSource  `json:"source,omitempty"`
	Config        *BackupManifestConfig  `json:"config,omitempty"`
	Restore       *BackupManifestRestore `json:"restore,omitempty"`
	VMUUID        string                 `json:"vmUuid,omitempty"`
	VMName        string                 `json:"vmName,omitempty"`
	ConfigVolume  string                 `json:"configVolume,omitempty"`
	ConfigSHA256  string                 `json:"configSha256,omitempty"`
	Disks         []VMBackupDisk         `json:"disks"`
	Online        bool                   `json:"online,omitempty"`
	Quiesced      bool                   `json:"quiesced,omitempty"`
	Consistent    bool                   `json:"consistent,omitempty"`
	ContainsTPM   bool                   `json:"containsTpm,omitempty"`
	TPMStateSaved bool                   `json:"tpmStateSaved,omitempty"`
}

type BackupManifestSource struct {
	VMUUID     string  `json:"vmUuid"`
	VMName     string  `json:"vmName"`
	State      VMState `json:"state"`
	Persistent bool    `json:"persistent"`
}

type BackupManifestConfig struct {
	Artifact  string `json:"artifact"`
	SizeBytes uint64 `json:"sizeBytes"`
	SHA256    string `json:"sha256"`
}

type BackupManifestRestore struct {
	DefaultTargetPool string `json:"defaultTargetPool"`
	RequiresNewMAC    bool   `json:"requiresNewMac"`
	ContainsMemory    bool   `json:"containsMemory"`
}

type BackupPackageManifest struct {
	FormatVersion string              `json:"formatVersion"`
	ExportedAt    time.Time           `json:"exportedAt"`
	Backup        Backup              `json:"backup"`
	ConfigPath    string              `json:"configPath"`
	Disks         []BackupPackageDisk `json:"disks"`
}

type BackupPackageDisk struct {
	BackupDisk
	ArchivePath string `json:"archivePath"`
	SizeBytes   uint64 `json:"sizeBytes"`
}
