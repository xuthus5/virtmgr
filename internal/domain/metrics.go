package domain

type MetricsCounters struct {
	CPUTimeNanos     uint64
	CPUCapacity      uint64
	MemoryUsedBytes  uint64
	MemoryTotalBytes uint64
	NetworkRxBytes   uint64
	NetworkTxBytes   uint64
	Disks            []MetricsDiskCounters
	Interfaces       []MetricsInterfaceCounters
}

type MetricsDiskCounters struct {
	Target          string
	ReadBytes       uint64
	WriteBytes      uint64
	ReadOperations  uint64
	WriteOperations uint64
}

type MetricsInterfaceCounters struct {
	Name    string
	RxBytes uint64
	TxBytes uint64
}

type ResourceMetrics struct {
	Timestamp                    int64                     `json:"timestamp"`
	CPUPercent                   float64                   `json:"cpuPercent"`
	MemoryUsedBytes              uint64                    `json:"memoryUsedBytes"`
	MemoryTotalBytes             uint64                    `json:"memoryTotalBytes"`
	NetworkRxBytes               uint64                    `json:"networkRxBytes"`
	NetworkTxBytes               uint64                    `json:"networkTxBytes"`
	NetworkRxBytesPerSecond      uint64                    `json:"networkRxBytesPerSecond"`
	NetworkTxBytesPerSecond      uint64                    `json:"networkTxBytesPerSecond"`
	DiskReadBytes                uint64                    `json:"diskReadBytes"`
	DiskWriteBytes               uint64                    `json:"diskWriteBytes"`
	DiskReadOperations           uint64                    `json:"diskReadOperations"`
	DiskWriteOperations          uint64                    `json:"diskWriteOperations"`
	DiskReadBytesPerSecond       uint64                    `json:"diskReadBytesPerSecond"`
	DiskWriteBytesPerSecond      uint64                    `json:"diskWriteBytesPerSecond"`
	DiskReadOperationsPerSecond  uint64                    `json:"diskReadOperationsPerSecond"`
	DiskWriteOperationsPerSecond uint64                    `json:"diskWriteOperationsPerSecond"`
	Interfaces                   []NetworkInterfaceMetrics `json:"interfaces"`
}

type NetworkInterfaceMetrics struct {
	Name             string `json:"name"`
	RxBytes          uint64 `json:"rxBytes"`
	TxBytes          uint64 `json:"txBytes"`
	RxBytesPerSecond uint64 `json:"rxBytesPerSecond"`
	TxBytesPerSecond uint64 `json:"txBytesPerSecond"`
}

type HostMetrics struct {
	HostName string `json:"hostName"`
	ResourceMetrics
}

type VMMetrics struct {
	UUID string `json:"uuid"`
	ResourceMetrics
}
