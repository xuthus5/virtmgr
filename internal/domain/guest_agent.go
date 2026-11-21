package domain

import (
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

var guestAgentUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.@\\-]{1,128}$`)

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
	Hostname         bool `json:"hostname"`
	Network          bool `json:"network"`
	OSInfo           bool `json:"osInfo"`
	FSInfo           bool `json:"fsInfo"`
	Shutdown         bool `json:"shutdown"`
	Reboot           bool `json:"reboot"`
	TimeSync         bool `json:"timeSync"`
	Trim             bool `json:"trim"`
	Quiesce          bool `json:"quiesce"`
	FileAccess       bool `json:"fileAccess"`
	PasswordReset    bool `json:"passwordReset"`
	SSHKeys          bool `json:"sshKeys"`
	Processes        bool `json:"processes"`
	Users            bool `json:"users"`
	Timezone         bool `json:"timezone"`
	NetworkRoutes    bool `json:"networkRoutes"`
	FilesystemResize bool `json:"filesystemResize"`
	Packages         bool `json:"packages"`
}

type GuestAgentPackage struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	Architecture   string `json:"architecture,omitempty"`
	Summary        string `json:"summary,omitempty"`
	Upgradable     bool   `json:"upgradable"`
	UpdateVersion  string `json:"updateVersion,omitempty"`
	SecurityUpdate bool   `json:"securityUpdate"`
}

type GuestAgentPackageInventory struct {
	Manager         string              `json:"manager"`
	Items           []GuestAgentPackage `json:"items"`
	Total           int                 `json:"total"`
	Upgradable      int                 `json:"upgradable"`
	SecurityUpdates int                 `json:"securityUpdates"`
	Truncated       bool                `json:"truncated"`
	Warnings        []string            `json:"warnings"`
	CheckedAt       time.Time           `json:"checkedAt"`
}

type GuestAgentFilesystemResizeSpec struct {
	MountPoint string `json:"mountPoint"`
}

type GuestAgentFilesystemResizeResult struct {
	MountPoint         string `json:"mountPoint"`
	Device             string `json:"device,omitempty"`
	Filesystem         string `json:"filesystem"`
	PreviousTotalBytes uint64 `json:"previousTotalBytes"`
	TotalBytes         uint64 `json:"totalBytes"`
	FreeBytes          uint64 `json:"freeBytes"`
}

type GuestAgentDiagnostics struct {
	CheckedAt       time.Time              `json:"checkedAt"`
	FailedServices  []GuestAgentService    `json:"failedServices"`
	ServiceLogs     []GuestAgentServiceLog `json:"serviceLogs"`
	KernelWarnings  []string               `json:"kernelWarnings"`
	SystemErrors    []string               `json:"systemErrors"`
	OOMEvents       []string               `json:"oomEvents"`
	Recommendations []string               `json:"recommendations"`
	CommandErrors   []string               `json:"commandErrors"`
}

type GuestAgentServiceLog struct {
	Unit      string `json:"unit"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

type GuestAgentRoute struct {
	Destination string `json:"destination,omitempty"`
	Gateway     string `json:"gateway,omitempty"`
	Interface   string `json:"interface,omitempty"`
	Family      string `json:"family,omitempty"`
	Metric      int    `json:"metric,omitempty"`
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
	Timezone          string                       `json:"timezone,omitempty"`
	Routes            []GuestAgentRoute            `json:"routes"`
	Capabilities      GuestAgentCapabilities       `json:"capabilities"`
	DiagnosticCode    string                       `json:"diagnosticCode,omitempty"`
	DiagnosticHint    string                       `json:"diagnosticHint,omitempty"`
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

type GuestAgentFileReadSpec struct {
	Path     string `json:"path"`
	Offset   uint64 `json:"offset,omitempty"`
	MaxBytes uint64 `json:"maxBytes,omitempty"`
}

type GuestAgentFileContent struct {
	Path       string `json:"path"`
	DataBase64 string `json:"dataBase64"`
	Offset     uint64 `json:"offset"`
	Size       uint64 `json:"size"`
	EOF        bool   `json:"eof"`
	SHA256     string `json:"sha256,omitempty"`
}

type GuestAgentFileWriteSpec struct {
	Path       string `json:"path"`
	DataBase64 string `json:"dataBase64"`
	Mode       string `json:"mode,omitempty"`
	Overwrite  bool   `json:"overwrite,omitempty"`
}

type GuestAgentFileWriteResult struct {
	Path         string `json:"path"`
	BytesWritten uint64 `json:"bytesWritten"`
	SHA256       string `json:"sha256,omitempty"`
}

type GuestAgentPasswordSpec struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type GuestAgentSSHKeySpec struct {
	Username string   `json:"username"`
	Keys     []string `json:"keys"`
}

type GuestAgentAuthorizedKeys struct {
	Username string   `json:"username"`
	Keys     []string `json:"keys"`
}

type GuestAgentProcess struct {
	PID     int64  `json:"pid"`
	PPID    int64  `json:"ppid,omitempty"`
	User    string `json:"user,omitempty"`
	State   string `json:"state,omitempty"`
	Command string `json:"command"`
	Args    string `json:"args,omitempty"`
}

type GuestAgentProcessSignalSpec struct {
	PID    int64  `json:"pid"`
	Signal string `json:"signal"`
}

type GuestAgentService struct {
	Unit        string `json:"unit"`
	Load        string `json:"load"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Description string `json:"description,omitempty"`
}

func ValidGuestAgentProcessSignal(value string) (string, bool) {
	signal := strings.ToLower(strings.TrimSpace(value))
	switch signal {
	case "term", "kill":
		return signal, true
	default:
		return "", false
	}
}

type GuestAgentUser struct {
	Username  string `json:"username"`
	Domain    string `json:"domain,omitempty"`
	LoginTime int64  `json:"loginTime,omitempty"`
}

type GuestAgentCloudInit struct {
	Installed         bool      `json:"installed"`
	State             string    `json:"state"`
	Status            string    `json:"status,omitempty"`
	ExtendedStatus    string    `json:"extendedStatus,omitempty"`
	Stage             string    `json:"stage,omitempty"`
	Datasource        string    `json:"datasource,omitempty"`
	LastUpdate        string    `json:"lastUpdate,omitempty"`
	Errors            []string  `json:"errors"`
	RecoverableErrors []string  `json:"recoverableErrors"`
	Log               string    `json:"log,omitempty"`
	LogPath           string    `json:"logPath,omitempty"`
	LogTruncated      bool      `json:"logTruncated"`
	CheckedAt         time.Time `json:"checkedAt"`
}

func ValidGuestAgentUsername(value string) bool {
	return guestAgentUsernamePattern.MatchString(strings.TrimSpace(value))
}

func ParseSSHAuthorizedKey(value string) (ssh.PublicKey, bool) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 16<<10 || strings.ContainsRune(value, '\x00') {
		return nil, false
	}
	key, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(value))
	if err != nil || key == nil || strings.TrimSpace(string(rest)) != "" {
		return nil, false
	}
	return key, true
}

type GuestAgentPolicy struct {
	VMUUID              string          `json:"vmUuid"`
	Expected            bool            `json:"expected"`
	LastState           GuestAgentState `json:"lastState,omitempty"`
	LastError           string          `json:"lastError,omitempty"`
	ConsecutiveFailures int             `json:"consecutiveFailures"`
	LastCheckedAt       *time.Time      `json:"lastCheckedAt,omitempty"`
	LastHealthyAt       *time.Time      `json:"lastHealthyAt,omitempty"`
	UpdatedBy           string          `json:"updatedBy,omitempty"`
	UpdatedAt           time.Time       `json:"updatedAt"`
}

type GuestAgentPolicySpec struct {
	Expected bool `json:"expected"`
}

type GuestAgentHealthSample struct {
	ID        int64           `json:"id"`
	VMUUID    string          `json:"vmUuid"`
	SampledAt time.Time       `json:"sampledAt"`
	State     GuestAgentState `json:"state"`
	LatencyMS int64           `json:"latencyMs"`
	Connected bool            `json:"connected"`
	Error     string          `json:"error,omitempty"`
}

type GuestAgentHealthHistory struct {
	Items               []GuestAgentHealthSample `json:"items"`
	Total               int                      `json:"total"`
	ConnectivityPercent float64                  `json:"connectivityPercent"`
}

type GuestAgentSnapshotRecord struct {
	VMUUID       string    `json:"vmUuid"`
	SnapshotName string    `json:"snapshotName"`
	Quiesced     bool      `json:"quiesced"`
	CreatedAt    time.Time `json:"createdAt"`
}
