package domain

import (
	"strings"
	"time"
)

type VMState string

const (
	VMStateRunning     VMState = "running"
	VMStateBlocked     VMState = "blocked"
	VMStatePaused      VMState = "paused"
	VMStateShutdown    VMState = "shutting-down"
	VMStateCrashed     VMState = "crashed"
	VMStatePMSuspended VMState = "suspended"
	VMStateShutoff     VMState = "shutoff"
	VMStateUnknown     VMState = "unknown"
)

type VMAction string

const (
	VMActionStart    VMAction = "start"
	VMActionShutdown VMAction = "shutdown"
	VMActionReboot   VMAction = "reboot"
	VMActionReset    VMAction = "reset"
	VMActionPause    VMAction = "pause"
	VMActionResume   VMAction = "resume"
	VMActionDestroy  VMAction = "destroy"
)

func ValidVMAction(value string) (VMAction, bool) {
	action := VMAction(value)
	switch action {
	case VMActionStart, VMActionShutdown, VMActionReboot, VMActionReset, VMActionPause, VMActionResume, VMActionDestroy:
		return action, true
	default:
		return "", false
	}
}

func AllowedVMActions(state VMState) []VMAction {
	switch state {
	case VMStateRunning, VMStateBlocked:
		return []VMAction{VMActionShutdown, VMActionReboot, VMActionReset, VMActionPause, VMActionDestroy}
	case VMStatePaused:
		return []VMAction{VMActionResume, VMActionDestroy}
	case VMStateShutoff, VMStateCrashed:
		return []VMAction{VMActionStart}
	default:
		return []VMAction{}
	}
}

func ContainsVMAction(actions []VMAction, action VMAction) bool {
	for _, candidate := range actions {
		if candidate == action {
			return true
		}
	}
	return false
}

func CanOperateVMs(role Role) bool {
	return role == RoleOperator || role == RoleAdmin
}

func CanEditVMXML(role Role) bool {
	return role == RoleAdmin
}

type VMBatchActionSpec struct {
	Action VMAction `json:"action"`
	UUIDs  []string `json:"uuids"`
}

type VMBatchActionItem struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name,omitempty"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	VM      *VM    `json:"vm,omitempty"`
}

type VMBatchActionResult struct {
	Action    VMAction            `json:"action"`
	Items     []VMBatchActionItem `json:"items"`
	Succeeded int                 `json:"succeeded"`
	Failed    int                 `json:"failed"`
}

type VMFirmware string

const (
	VMFirmwareBIOS VMFirmware = "bios"
	VMFirmwareUEFI VMFirmware = "uefi"
)

func NormalizeVMFirmware(value string) (VMFirmware, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(VMFirmwareBIOS):
		return VMFirmwareBIOS, true
	case string(VMFirmwareUEFI), "efi":
		return VMFirmwareUEFI, true
	default:
		return "", false
	}
}

type VMDiskBus string

const (
	VMDiskBusVirtio VMDiskBus = "virtio"
	VMDiskBusSATA   VMDiskBus = "sata"
	VMDiskBusSCSI   VMDiskBus = "scsi"
)

func NormalizeVMDiskBus(value string) (VMDiskBus, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(VMDiskBusVirtio):
		return VMDiskBusVirtio, true
	case string(VMDiskBusSATA):
		return VMDiskBusSATA, true
	case string(VMDiskBusSCSI):
		return VMDiskBusSCSI, true
	default:
		return "", false
	}
}

type VMNICModel string

const (
	VMNICModelVirtio VMNICModel = "virtio"
	VMNICModelE1000  VMNICModel = "e1000"
)

func NormalizeVMNICModel(value string) (VMNICModel, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(VMNICModelVirtio):
		return VMNICModelVirtio, true
	case string(VMNICModelE1000):
		return VMNICModelE1000, true
	default:
		return "", false
	}
}

type VMBootOrder string

const (
	VMBootOrderAuto       VMBootOrder = "auto"
	VMBootOrderDiskFirst  VMBootOrder = "disk-first"
	VMBootOrderCDROMFirst VMBootOrder = "cdrom-first"
)

func NormalizeVMBootOrder(value string) (VMBootOrder, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(VMBootOrderAuto):
		return VMBootOrderAuto, true
	case string(VMBootOrderDiskFirst):
		return VMBootOrderDiskFirst, true
	case string(VMBootOrderCDROMFirst):
		return VMBootOrderCDROMFirst, true
	default:
		return "", false
	}
}

type VMCreateSpec struct {
	Name       string         `json:"name"`
	VCPUs      uint           `json:"vcpus"`
	MemoryMiB  uint64         `json:"memoryMiB"`
	DiskPath   string         `json:"diskPath"`
	DiskFormat string         `json:"diskFormat"`
	DiskBus    VMDiskBus      `json:"diskBus,omitempty"`
	Network    string         `json:"network"`
	NICModel   VMNICModel     `json:"nicModel,omitempty"`
	ISOPath    string         `json:"isoPath,omitempty"`
	BootOrder  VMBootOrder    `json:"bootOrder,omitempty"`
	Firmware   VMFirmware     `json:"firmware"`
	SecureBoot bool           `json:"secureBoot"`
	CloudInit  *CloudInitSpec `json:"cloudInit,omitempty"`
	Autostart  bool           `json:"autostart"`
	Start      bool           `json:"start"`
}

type VMResourcesSpec struct {
	VCPUs     uint   `json:"vcpus"`
	MemoryMiB uint64 `json:"memoryMiB"`
}

type VMDiskResizeSpec struct {
	CapacityBytes uint64 `json:"capacityBytes"`
}

type CloudInitSpec struct {
	UserData string `json:"userData"`
	Pool     string `json:"pool"`
}

type VMCloneSpec struct {
	Name       string `json:"name"`
	TargetPool string `json:"targetPool,omitempty"`
	Autostart  bool   `json:"autostart"`
	Start      bool   `json:"start"`
}

type VMBackupSpec struct {
	BackupID   string `json:"backupId,omitempty"`
	Name       string `json:"name"`
	TargetPool string `json:"targetPool"`
}

type VMBackupDisk struct {
	SourceVolume  string `json:"sourceVolume"`
	BackupVolume  string `json:"backupVolume"`
	Target        string `json:"target,omitempty"`
	Format        string `json:"format"`
	CapacityBytes uint64 `json:"capacityBytes"`
	SHA256        string `json:"sha256,omitempty"`
}

type VMBackup struct {
	BackupID       string         `json:"backupId"`
	FormatVersion  string         `json:"formatVersion"`
	CreatedAt      time.Time      `json:"createdAt"`
	VMUUID         string         `json:"vmUuid"`
	VMName         string         `json:"vmName"`
	TargetPool     string         `json:"targetPool"`
	Name           string         `json:"name"`
	ManifestVolume string         `json:"manifestVolume"`
	ConfigVolume   string         `json:"configVolume"`
	ConfigBytes    uint64         `json:"configBytes"`
	Disks          []VMBackupDisk `json:"disks"`
	TotalBytes     uint64         `json:"totalBytes"`
}

type VMSnapshot struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	CreatedAt   int64   `json:"createdAt"`
	State       VMState `json:"state"`
	Current     bool    `json:"current"`
	Parent      string  `json:"parent,omitempty"`
}

type VMSnapshotCreateSpec struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type VMSnapshotList struct {
	Items []VMSnapshot `json:"items"`
	Total int          `json:"total"`
}

type Disk struct {
	Type            string `json:"type,omitempty"`
	Device          string `json:"device"`
	Target          string `json:"target"`
	Bus             string `json:"bus"`
	Format          string `json:"format,omitempty"`
	Source          string `json:"source,omitempty"`
	ReadOnly        bool   `json:"readOnly"`
	CapacityBytes   uint64 `json:"capacityBytes"`
	AllocationBytes uint64 `json:"allocationBytes"`
}

type NetworkInterface struct {
	Type    string `json:"type"`
	MAC     string `json:"mac,omitempty"`
	Source  string `json:"source,omitempty"`
	Model   string `json:"model,omitempty"`
	Address string `json:"address,omitempty"`
}

type Graphics struct {
	Type   string `json:"type"`
	Listen string `json:"listen,omitempty"`
	Port   int    `json:"port,omitempty"`
}

type SerialConsole struct {
	Device     string `json:"device,omitempty"`
	Label      string `json:"label"`
	TargetPort int    `json:"targetPort"`
	TargetType string `json:"targetType,omitempty"`
}

type PassthroughDevice struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Vendor    string `json:"vendor,omitempty"`
	VendorID  string `json:"vendorId,omitempty"`
	Product   string `json:"product,omitempty"`
	ProductID string `json:"productId,omitempty"`
	Address   string `json:"address,omitempty"`
	Driver    string `json:"driver,omitempty"`
}

type VM struct {
	UUID           string              `json:"uuid"`
	Name           string              `json:"name"`
	State          VMState             `json:"state"`
	StateReason    int                 `json:"stateReason"`
	Persistent     bool                `json:"persistent"`
	Autostart      bool                `json:"autostart"`
	VCPUs          uint                `json:"vcpus"`
	MaxVCPUs       uint                `json:"maxVcpus"`
	MemoryBytes    uint64              `json:"memoryBytes"`
	MaxMemoryBytes uint64              `json:"maxMemoryBytes"`
	CPUTimeNanos   uint64              `json:"cpuTimeNanos"`
	OSType         string              `json:"osType,omitempty"`
	Arch           string              `json:"arch,omitempty"`
	Machine        string              `json:"machine,omitempty"`
	Firmware       VMFirmware          `json:"firmware"`
	SecureBoot     bool                `json:"secureBoot"`
	CloudInit      bool                `json:"cloudInit"`
	Disks          []Disk              `json:"disks"`
	Interfaces     []NetworkInterface  `json:"interfaces"`
	Graphics       []Graphics          `json:"graphics"`
	Serials        []SerialConsole     `json:"serials"`
	Passthrough    []PassthroughDevice `json:"passthrough"`
	IPAddresses    []string            `json:"ipAddresses"`
	Hostname       string              `json:"hostname,omitempty"`
	AgentConnected bool                `json:"agentConnected"`
	AllowedActions []VMAction          `json:"allowedActions"`
}

type VMXMLFormat string

const (
	VMXMLFormatLive   VMXMLFormat = "live"
	VMXMLFormatConfig VMXMLFormat = "config"
)

type VMXML struct {
	UUID   string      `json:"uuid"`
	Name   string      `json:"name"`
	Format VMXMLFormat `json:"format"`
	XML    string      `json:"xml"`
}

const VMExportFormatVersion = "virtmgr.vm-export/v1"

type VMExport struct {
	FormatVersion string         `json:"formatVersion"`
	ExportedAt    time.Time      `json:"exportedAt"`
	VM            VM             `json:"vm"`
	ConfigXML     VMXML          `json:"configXml"`
	LiveXML       *VMXML         `json:"liveXml,omitempty"`
	Snapshots     VMSnapshotList `json:"snapshots"`
	Disclaimer    string         `json:"disclaimer"`
}

func ValidVMXMLFormat(value string) (VMXMLFormat, bool) {
	switch VMXMLFormat(value) {
	case VMXMLFormatLive, VMXMLFormatConfig:
		return VMXMLFormat(value), true
	default:
		return "", false
	}
}

type Host struct {
	Name           string `json:"name"`
	LibvirtVersion string `json:"libvirtVersion"`
	CPUModel       string `json:"cpuModel"`
	CPUs           uint   `json:"cpus"`
	CPUSockets     uint32 `json:"cpuSockets"`
	CPUCores       uint32 `json:"cpuCores"`
	CPUThreads     uint32 `json:"cpuThreads"`
	CPUMHz         uint   `json:"cpuMhz"`
	MemoryBytes    uint64 `json:"memoryBytes"`
}

type VMCounts struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Paused  int `json:"paused"`
	Shutoff int `json:"shutoff"`
	Failed  int `json:"failed"`
}

type Overview struct {
	Host                 Host     `json:"host"`
	Counts               VMCounts `json:"counts"`
	AllocatedVCPUs       uint64   `json:"allocatedVcpus"`
	AllocatedMemoryBytes uint64   `json:"allocatedMemoryBytes"`
	VMs                  []VM     `json:"vms"`
	Warnings             []string `json:"warnings,omitempty"`
}

type VMSummaryList struct {
	Items []VM `json:"items"`
	Total int  `json:"total"`
}
