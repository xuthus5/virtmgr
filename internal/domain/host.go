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
	Architecture   string   `json:"architecture"`
	Architectures  []string `json:"architectures,omitempty"`
	MachineTypes   []string `json:"machineTypes,omitempty"`
	Virtualization string   `json:"virtualization,omitempty"`
	UEFI           bool     `json:"uefi"`
	SecureBoot     bool     `json:"secureBoot"`
	TPM            bool     `json:"tpm"`
}
