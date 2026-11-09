package domain

import (
	"strings"
	"time"
)

// VMConfigVersionSource identifies why a persistent domain definition was archived.
type VMConfigVersionSource string

const (
	VMConfigSourceManualXML      VMConfigVersionSource = "manual_xml"
	VMConfigSourceResources      VMConfigVersionSource = "resources"
	VMConfigSourcePassthrough    VMConfigVersionSource = "passthrough"
	VMConfigSourceSnapshotRevert VMConfigVersionSource = "snapshot_revert"
	VMConfigSourceExternal       VMConfigVersionSource = "external"
	VMConfigSourceCreate         VMConfigVersionSource = "create"
	VMConfigSourceClone          VMConfigVersionSource = "clone"
	VMConfigSourceBackupRestore  VMConfigVersionSource = "backup_restore"
	VMConfigSourceRollback       VMConfigVersionSource = "rollback"
	VMConfigSourcePreRollback    VMConfigVersionSource = "pre_rollback"
	VMConfigSourceDiskResize     VMConfigVersionSource = "disk_resize"
	VMConfigSourceBaseline       VMConfigVersionSource = "baseline"
)

func ValidVMConfigVersionSource(value VMConfigVersionSource) bool {
	switch value {
	case VMConfigSourceManualXML, VMConfigSourceResources, VMConfigSourcePassthrough,
		VMConfigSourceSnapshotRevert, VMConfigSourceExternal, VMConfigSourceCreate,
		VMConfigSourceClone, VMConfigSourceBackupRestore, VMConfigSourceRollback,
		VMConfigSourcePreRollback, VMConfigSourceDiskResize, VMConfigSourceBaseline:
		return true
	default:
		return false
	}
}

const (
	// VMConfigVersionRetention is the default number of unpinned versions kept per VM.
	VMConfigVersionRetention = 100
	// MaxVMConfigVersionXMLBytes bounds both stored and restored XML payloads.
	MaxVMConfigVersionXMLBytes = 512 << 10
)

type VMConfigVersion struct {
	VMUUID        string                `json:"vmUuid"`
	VersionNo     int                   `json:"versionNo"`
	XMLSHA256     string                `json:"xmlSha256"`
	SizeBytes     int                   `json:"sizeBytes"`
	Source        VMConfigVersionSource `json:"source"`
	SourceVersion int                   `json:"sourceVersion,omitempty"`
	Actor         string                `json:"actor"`
	Role          Role                  `json:"role"`
	RequestID     string                `json:"requestId,omitempty"`
	SourceIP      string                `json:"sourceIp,omitempty"`
	Note          string                `json:"note,omitempty"`
	Protected     bool                  `json:"protected"`
	CreatedAt     time.Time             `json:"createdAt"`
	IsCurrent     bool                  `json:"isCurrent"`
}

type VMConfigVersionDetail struct {
	VMConfigVersion
	XML string `json:"xml"`
}

type VMConfigVersionList struct {
	Items           []VMConfigVersion `json:"items"`
	Total           int               `json:"total"`
	CurrentSHA256   string            `json:"currentSha256"`
	CurrentCaptured bool              `json:"currentCaptured"`
}

type VMConfigCapture struct {
	Source        VMConfigVersionSource
	SourceVersion int
	Actor         string
	Role          Role
	RequestID     string
	SourceIP      string
	Note          string
}

func (capture VMConfigCapture) Normalized() VMConfigCapture {
	capture.Actor = strings.TrimSpace(capture.Actor)
	capture.RequestID = strings.TrimSpace(capture.RequestID)
	capture.SourceIP = strings.TrimSpace(capture.SourceIP)
	capture.Note = strings.TrimSpace(capture.Note)
	if capture.Actor == "" {
		capture.Actor = "system"
	}
	if capture.Role == "" {
		capture.Role = RoleAdmin
	}
	if len(capture.Note) > 1024 {
		capture.Note = capture.Note[:1024]
	}
	return capture
}
