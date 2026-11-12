package metrics

import (
	"context"
	"math"
	"sync"
	"time"

	"virtmgr/internal/domain"
)

type Source interface {
	HostMetrics(ctx context.Context) (domain.MetricsCounters, error)
	VMMetrics(ctx context.Context, uuid string) (domain.MetricsCounters, error)
}

type previousSample struct {
	at       time.Time
	counters domain.MetricsCounters
}

type Service struct {
	source   Source
	now      func() time.Time
	mu       sync.Mutex
	previous map[string]previousSample
}

type Option func(*Service)

func WithClock(now func() time.Time) Option {
	return func(service *Service) {
		if now != nil {
			service.now = now
		}
	}
}

func New(source Source, options ...Option) *Service {
	service := &Service{
		source:   source,
		now:      time.Now,
		previous: make(map[string]previousSample),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Host(ctx context.Context) (domain.HostMetrics, error) {
	counters, err := s.source.HostMetrics(ctx)
	if err != nil {
		return domain.HostMetrics{}, err
	}
	return domain.HostMetrics{
		HostName:        "host",
		ResourceMetrics: s.sample("host", counters),
	}, nil
}

func (s *Service) HostNamed(ctx context.Context, hostName string) (domain.HostMetrics, error) {
	counters, err := s.source.HostMetrics(ctx)
	if err != nil {
		return domain.HostMetrics{}, err
	}
	return domain.HostMetrics{
		HostName:        hostName,
		ResourceMetrics: s.sample("host", counters),
	}, nil
}

func (s *Service) VM(ctx context.Context, uuid string) (domain.VMMetrics, error) {
	counters, err := s.source.VMMetrics(ctx, uuid)
	if err != nil {
		return domain.VMMetrics{}, err
	}
	return domain.VMMetrics{
		UUID:            uuid,
		ResourceMetrics: s.sample("vm:"+uuid, counters),
	}, nil
}

func (s *Service) sample(key string, counters domain.MetricsCounters) domain.ResourceMetrics {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()

	result := domain.ResourceMetrics{
		Timestamp:        now.UnixMilli(),
		MemoryUsedBytes:  counters.MemoryUsedBytes,
		MemoryTotalBytes: counters.MemoryTotalBytes,
		NetworkRxBytes:   counters.NetworkRxBytes,
		NetworkTxBytes:   counters.NetworkTxBytes,
		Interfaces:       make([]domain.NetworkInterfaceMetrics, 0, len(counters.Interfaces)),
	}
	previous, ok := s.previous[key]
	s.previous[key] = previousSample{at: now, counters: counters}
	for _, currentDisk := range counters.Disks {
		result.DiskReadBytes += currentDisk.ReadBytes
		result.DiskWriteBytes += currentDisk.WriteBytes
		result.DiskReadOperations += currentDisk.ReadOperations
		result.DiskWriteOperations += currentDisk.WriteOperations
	}
	for _, currentInterface := range counters.Interfaces {
		result.Interfaces = append(result.Interfaces, domain.NetworkInterfaceMetrics{
			Name: currentInterface.Name, RxBytes: currentInterface.RxBytes, TxBytes: currentInterface.TxBytes,
		})
	}
	if !ok || !now.After(previous.at) {
		return result
	}

	elapsed := now.Sub(previous.at).Seconds()
	if elapsed <= 0 {
		return result
	}
	result.CPUPercent = cpuPercent(counters, previous.counters, elapsed)
	result.NetworkRxBytesPerSecond = counterRate(previous.counters.NetworkRxBytes, counters.NetworkRxBytes, elapsed)
	result.NetworkTxBytesPerSecond = counterRate(previous.counters.NetworkTxBytes, counters.NetworkTxBytes, elapsed)
	previousDisks := make(map[string]domain.MetricsDiskCounters, len(previous.counters.Disks))
	for _, disk := range previous.counters.Disks {
		previousDisks[disk.Target] = disk
	}
	for _, currentDisk := range counters.Disks {
		previousDisk, exists := previousDisks[currentDisk.Target]
		if !exists {
			continue
		}
		result.DiskReadBytesPerSecond += counterRate(previousDisk.ReadBytes, currentDisk.ReadBytes, elapsed)
		result.DiskWriteBytesPerSecond += counterRate(previousDisk.WriteBytes, currentDisk.WriteBytes, elapsed)
		result.DiskReadOperationsPerSecond += counterRate(previousDisk.ReadOperations, currentDisk.ReadOperations, elapsed)
		result.DiskWriteOperationsPerSecond += counterRate(previousDisk.WriteOperations, currentDisk.WriteOperations, elapsed)
	}
	previousInterfaces := make(map[string]domain.MetricsInterfaceCounters, len(previous.counters.Interfaces))
	for _, previousInterface := range previous.counters.Interfaces {
		previousInterfaces[previousInterface.Name] = previousInterface
	}
	for index := range result.Interfaces {
		currentInterface := counters.Interfaces[index]
		previousInterface, exists := previousInterfaces[currentInterface.Name]
		if !exists {
			continue
		}
		result.Interfaces[index].RxBytesPerSecond = counterRate(previousInterface.RxBytes, currentInterface.RxBytes, elapsed)
		result.Interfaces[index].TxBytesPerSecond = counterRate(previousInterface.TxBytes, currentInterface.TxBytes, elapsed)
	}
	return result
}

func cpuPercent(current, previous domain.MetricsCounters, elapsedSeconds float64) float64 {
	if current.CPUTimeNanos <= previous.CPUTimeNanos {
		return 0
	}
	capacity := current.CPUCapacity
	if capacity == 0 {
		capacity = 1
	}
	used := float64(current.CPUTimeNanos-previous.CPUTimeNanos) / float64(time.Second)
	value := used / (elapsedSeconds * float64(capacity)) * 100
	value = math.Max(0, math.Min(100, value))
	return math.Round(value*100) / 100
}

func counterRate(previous, current uint64, elapsedSeconds float64) uint64 {
	if current <= previous || elapsedSeconds <= 0 {
		return 0
	}
	return uint64(math.Round(float64(current-previous) / elapsedSeconds))
}
