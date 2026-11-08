package domain

import "time"

// VMTemplateSpec contains reusable defaults for creating a virtual machine.
// Instance-specific values such as the VM name and disk path are intentionally
// excluded so a template can be applied repeatedly.
type VMTemplateSpec struct {
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	VCPUs             uint       `json:"vcpus"`
	MemoryMiB         uint64     `json:"memoryMiB"`
	DiskFormat        string     `json:"diskFormat"`
	Network           string     `json:"network"`
	ISOPath           string     `json:"isoPath,omitempty"`
	Firmware          VMFirmware `json:"firmware"`
	SecureBoot        bool       `json:"secureBoot"`
	CloudInit         bool       `json:"cloudInit"`
	CloudInitUserData string     `json:"cloudInitUserData,omitempty"`
	Autostart         bool       `json:"autostart"`
	Start             bool       `json:"start"`
}

type VMTemplate struct {
	ID                int64      `json:"id"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	VCPUs             uint       `json:"vcpus"`
	MemoryMiB         uint64     `json:"memoryMiB"`
	DiskFormat        string     `json:"diskFormat"`
	Network           string     `json:"network"`
	ISOPath           string     `json:"isoPath,omitempty"`
	Firmware          VMFirmware `json:"firmware"`
	SecureBoot        bool       `json:"secureBoot"`
	CloudInit         bool       `json:"cloudInit"`
	CloudInitUserData string     `json:"cloudInitUserData,omitempty"`
	Autostart         bool       `json:"autostart"`
	Start             bool       `json:"start"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type VMTemplateList struct {
	Items []VMTemplate `json:"items"`
	Total int          `json:"total"`
}
