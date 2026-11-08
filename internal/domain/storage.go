package domain

type StoragePoolState string

const (
	StoragePoolStateActive       StoragePoolState = "active"
	StoragePoolStateInactive     StoragePoolState = "inactive"
	StoragePoolStateDegraded     StoragePoolState = "degraded"
	StoragePoolStateInaccessible StoragePoolState = "inaccessible"
	StoragePoolStateUnknown      StoragePoolState = "unknown"
)

type StoragePoolAction string

const (
	StoragePoolActionStart   StoragePoolAction = "start"
	StoragePoolActionStop    StoragePoolAction = "stop"
	StoragePoolActionRefresh StoragePoolAction = "refresh"
)

func ValidStoragePoolAction(value string) (StoragePoolAction, bool) {
	action := StoragePoolAction(value)
	switch action {
	case StoragePoolActionStart, StoragePoolActionStop, StoragePoolActionRefresh:
		return action, true
	default:
		return "", false
	}
}

type StoragePool struct {
	Name            string           `json:"name"`
	UUID            string           `json:"uuid"`
	Type            string           `json:"type"`
	State           StoragePoolState `json:"state"`
	Autostart       bool             `json:"autostart"`
	Persistent      bool             `json:"persistent"`
	TargetPath      string           `json:"targetPath,omitempty"`
	CapacityBytes   uint64           `json:"capacityBytes"`
	AllocationBytes uint64           `json:"allocationBytes"`
	AvailableBytes  uint64           `json:"availableBytes"`
	VolumeCount     int              `json:"volumeCount"`
}

type StoragePoolCreateSpec struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	TargetPath string `json:"targetPath"`
	Autostart  bool   `json:"autostart"`
	Start      bool   `json:"start"`
}

type StorageVolume struct {
	Name            string `json:"name"`
	Path            string `json:"path,omitempty"`
	Type            string `json:"type"`
	Format          string `json:"format,omitempty"`
	CapacityBytes   uint64 `json:"capacityBytes"`
	AllocationBytes uint64 `json:"allocationBytes"`
}

type StorageVolumeCreateSpec struct {
	Name          string `json:"name"`
	Format        string `json:"format"`
	CapacityBytes uint64 `json:"capacityBytes"`
}

func CanManageStorage(role Role) bool {
	return role == RoleOperator || role == RoleAdmin
}

type StoragePoolList struct {
	Items []StoragePool `json:"items"`
	Total int           `json:"total"`
}

type StorageVolumeList struct {
	Pool  string          `json:"pool"`
	Items []StorageVolume `json:"items"`
	Total int             `json:"total"`
}
