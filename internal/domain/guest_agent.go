package domain

import (
	"strings"
	"time"
)

type GuestAgentState string

const (
	GuestAgentStateUnknown       GuestAgentState = "unknown"
	GuestAgentStateNotConfigured GuestAgentState = "not_configured"
	GuestAgentStateDisconnected  GuestAgentState = "disconnected"
	GuestAgentStateConnected     GuestAgentState = "connected"
	GuestAgentStateUnresponsive  GuestAgentState = "unresponsive"
	GuestAgentStateError         GuestAgentState = "error"
)

type GuestAgentOSInfo struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name,omitempty"`
	PrettyName    string `json:"prettyName,omitempty"`
	Version       string `json:"version,omitempty"`
	KernelRelease string `json:"kernelRelease,omitempty"`
	Arch          string `json:"arch,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
}

type GuestAgentIPAddress struct {
	Address string `json:"address"`
	Family  string `json:"family"`
	Prefix  int    `json:"prefix,omitempty"`
}

type GuestAgentNetworkInterface struct {
	Name      string                `json:"name"`
	MAC       string                `json:"mac,omitempty"`
	Addresses []GuestAgentIPAddress `json:"addresses"`
}

type GuestAgentFilesystem struct {
	Device     string `json:"device,omitempty"`
	MountPoint string `json:"mountPoint"`
	Type       string `json:"type,omitempty"`
	TotalBytes uint64 `json:"totalBytes,omitempty"`
	UsedBytes  uint64 `json:"usedBytes,omitempty"`
	FreeBytes  uint64 `json:"freeBytes,omitempty"`
}

type GuestAgentCapabilities struct {
	Hostname bool `json:"hostname"`
	Network  bool `json:"network"`
	OSInfo   bool `json:"osInfo"`
	FSInfo   bool `json:"fsInfo"`
	Shutdown bool `json:"shutdown"`
	Reboot   bool `json:"reboot"`
	TimeSync bool `json:"timeSync"`
	Trim     bool `json:"trim"`
	Quiesce  bool `json:"quiesce"`
}

type GuestAgentInfo struct {
	State             GuestAgentState              `json:"state"`
	Connected         bool                         `json:"connected"`
	ChannelConfigured bool                         `json:"channelConfigured"`
	Version           string                       `json:"version,omitempty"`
	Hostname          string                       `json:"hostname,omitempty"`
	LastSeenAt        *time.Time                   `json:"lastSeenAt,omitempty"`
	LastError         string                       `json:"lastError,omitempty"`
	Stale             bool                         `json:"stale"`
	OS                GuestAgentOSInfo             `json:"os"`
	Interfaces        []GuestAgentNetworkInterface `json:"interfaces"`
	Filesystems       []GuestAgentFilesystem       `json:"filesystems"`
	Capabilities      GuestAgentCapabilities       `json:"capabilities"`
	ProbedAt          time.Time                    `json:"probedAt"`
}

type GuestAgentAction string

const (
	GuestAgentActionShutdown GuestAgentAction = "shutdown"
	GuestAgentActionReboot   GuestAgentAction = "reboot"
	GuestAgentActionSyncTime GuestAgentAction = "sync-time"
	GuestAgentActionTrim     GuestAgentAction = "trim"
)

type GuestAgentActionSpec struct {
	Action       GuestAgentAction `json:"action"`
	Mount        string           `json:"mount,omitempty"`
	MinimumBytes uint64           `json:"minimumBytes,omitempty"`
}

func ValidGuestAgentAction(value string) (GuestAgentAction, bool) {
	action := GuestAgentAction(strings.ToLower(strings.TrimSpace(value)))
	switch action {
	case GuestAgentActionShutdown, GuestAgentActionReboot, GuestAgentActionSyncTime, GuestAgentActionTrim:
		return action, true
	default:
		return "", false
	}
}

func (spec GuestAgentActionSpec) SupportedBy(capabilities GuestAgentCapabilities) bool {
	switch spec.Action {
	case GuestAgentActionShutdown:
		return capabilities.Shutdown
	case GuestAgentActionReboot:
		return capabilities.Reboot
	case GuestAgentActionSyncTime:
		return capabilities.TimeSync
	case GuestAgentActionTrim:
		return capabilities.Trim
	default:
		return false
	}
}
