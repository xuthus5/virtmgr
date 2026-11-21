package domain

type HostDetails struct {
	Host                 Host             `json:"host"`
	Counts               VMCounts         `json:"counts"`
	AllocatedVCPUs       uint64           `json:"allocatedVcpus"`
	AllocatedMemoryBytes uint64           `json:"allocatedMemoryBytes"`
	VMs                  []VM             `json:"vms"`
	StoragePools         []StoragePool    `json:"storagePools"`
	Networks             []VirtualNetwork `json:"networks"`
	Warnings             []string         `json:"warnings,omitempty"`
}

type HostCapabilities struct {
	Architecture   string         `json:"architecture"`
	Architectures  []string       `json:"architectures,omitempty"`
	MachineTypes   []string       `json:"machineTypes,omitempty"`
	Virtualization string         `json:"virtualization,omitempty"`
	UEFI           bool           `json:"uefi"`
	SecureBoot     bool           `json:"secureBoot"`
	TPM            bool           `json:"tpm"`
	NUMA           bool           `json:"numa"`
	NUMANodes      []HostNUMANode `json:"numaNodes,omitempty"`
	HugePages      []HostHugePage `json:"hugePages,omitempty"`
}

type HostNUMANode struct {
	ID              int    `json:"id"`
	CPUs            []uint `json:"cpus"`
	MemoryBytes     uint64 `json:"memoryBytes"`
	FreeMemoryBytes uint64 `json:"freeMemoryBytes"`
}

type HostHugePage struct {
	SizeBytes uint64 `json:"sizeBytes"`
	Total     int    `json:"total"`
	Free      int    `json:"free"`
	Reserved  int    `json:"reserved"`
	Surplus   int    `json:"surplus"`
}
