package domain

type MetricsCounters struct {
	CPUTimeNanos     uint64
	CPUCapacity      uint64
	MemoryUsedBytes  uint64
	MemoryTotalBytes uint64
	NetworkRxBytes   uint64
	NetworkTxBytes   uint64
}

type ResourceMetrics struct {
	Timestamp               int64   `json:"timestamp"`
	CPUPercent              float64 `json:"cpuPercent"`
	MemoryUsedBytes         uint64  `json:"memoryUsedBytes"`
	MemoryTotalBytes        uint64  `json:"memoryTotalBytes"`
	NetworkRxBytes          uint64  `json:"networkRxBytes"`
	NetworkTxBytes          uint64  `json:"networkTxBytes"`
	NetworkRxBytesPerSecond uint64  `json:"networkRxBytesPerSecond"`
	NetworkTxBytesPerSecond uint64  `json:"networkTxBytesPerSecond"`
}

type HostMetrics struct {
	HostName string `json:"hostName"`
	ResourceMetrics
}

type VMMetrics struct {
	UUID string `json:"uuid"`
	ResourceMetrics
}
