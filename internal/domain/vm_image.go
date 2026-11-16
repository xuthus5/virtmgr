package domain

import (
	"strings"
	"time"
)

type VMImageCloneMode string

const (
	VMImageCloneModeFull   VMImageCloneMode = "full"
	VMImageCloneModeLinked VMImageCloneMode = "linked"
)

func NormalizeVMImageCloneMode(value string) (VMImageCloneMode, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(VMImageCloneModeFull):
		return VMImageCloneModeFull, true
	case string(VMImageCloneModeLinked):
		return VMImageCloneModeLinked, true
	default:
		return "", false
	}
}

type VMImage struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Pool            string    `json:"pool"`
	Volume          string    `json:"volume"`
	Format          string    `json:"format"`
	OSFamily        string    `json:"osFamily"`
	OSName          string    `json:"osName"`
	Architecture    string    `json:"architecture"`
	CapacityBytes   uint64    `json:"capacityBytes"`
	AllocationBytes uint64    `json:"allocationBytes"`
	SHA256          string    `json:"sha256,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type VMImageCreateSpec struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Pool         string `json:"pool"`
	Volume       string `json:"volume"`
	OSFamily     string `json:"osFamily"`
	OSName       string `json:"osName"`
	Architecture string `json:"architecture"`
}

type VMImageList struct {
	Items []VMImage `json:"items"`
	Total int       `json:"total"`
}
