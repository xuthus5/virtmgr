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
