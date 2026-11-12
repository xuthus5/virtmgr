package metrics

import (
	"context"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type fakeSource struct {
	host      domain.MetricsCounters
	vm        domain.MetricsCounters
	hostErr   error
	vmErr     error
	lastUUID  string
	hostCalls int
}

func (f *fakeSource) HostMetrics(context.Context) (domain.MetricsCounters, error) {
	f.hostCalls++
	return f.host, f.hostErr
}

func (f *fakeSource) VMMetrics(_ context.Context, uuid string) (domain.MetricsCounters, error) {
	f.lastUUID = uuid
	return f.vm, f.vmErr
}

func TestServiceComputesHostRatesFromPreviousSample(t *testing.T) {
	source := &fakeSource{host: domain.MetricsCounters{
		CPUTimeNanos: 10 * uint64(time.Second), CPUCapacity: 8,
		MemoryUsedBytes: 4 << 30, MemoryTotalBytes: 16 << 30,
		NetworkRxBytes: 1000, NetworkTxBytes: 2000,
	}}
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	service := New(source, WithClock(func() time.Time { return now }))

	first, err := service.HostNamed(context.Background(), "node-01")
	if err != nil {
		t.Fatalf("HostNamed() error = %v", err)
	}
	if first.HostName != "node-01" || first.CPUPercent != 0 || first.NetworkRxBytesPerSecond != 0 {
		t.Fatalf("first sample = %+v", first)
	}
	if first.Timestamp != now.UnixMilli() || first.MemoryUsedBytes != 4<<30 || first.MemoryTotalBytes != 16<<30 {
		t.Fatalf("first resource metrics = %+v", first.ResourceMetrics)
	}

	now = now.Add(2 * time.Second)
	source.host = domain.MetricsCounters{
		CPUTimeNanos: 14 * uint64(time.Second), CPUCapacity: 8,
		MemoryUsedBytes: 5 << 30, MemoryTotalBytes: 16 << 30,
		NetworkRxBytes: 3000, NetworkTxBytes: 5000,
	}
	second, err := service.HostNamed(context.Background(), "node-01")
	if err != nil {
		t.Fatalf("HostNamed() second error = %v", err)
	}
	if second.CPUPercent != 25 {
		t.Fatalf("CPUPercent = %v, want 25", second.CPUPercent)
	}
	if second.NetworkRxBytesPerSecond != 1000 || second.NetworkTxBytesPerSecond != 1500 {
		t.Fatalf("network rates = rx %d tx %d", second.NetworkRxBytesPerSecond, second.NetworkTxBytesPerSecond)
	}
	if second.MemoryUsedBytes != 5<<30 {
		t.Fatalf("memory used = %d", second.MemoryUsedBytes)
	}
	if source.hostCalls != 2 {
		t.Fatalf("host calls = %d", source.hostCalls)
	}
}

func TestServiceComputesVMRatesAndMasksCounterReset(t *testing.T) {
	source := &fakeSource{vm: domain.MetricsCounters{
		CPUTimeNanos: 2 * uint64(time.Second), CPUCapacity: 2,
		NetworkRxBytes: 5000, NetworkTxBytes: 4000,
	}}
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	service := New(source, WithClock(func() time.Time { return now }))
	const uuid = "22222222-2222-4222-8222-222222222222"

	if _, err := service.VM(context.Background(), uuid); err != nil {
		t.Fatalf("VM() error = %v", err)
	}
	if source.lastUUID != uuid {
		t.Fatalf("uuid = %q", source.lastUUID)
	}

	now = now.Add(4 * time.Second)
	source.vm = domain.MetricsCounters{
		CPUTimeNanos: 6 * uint64(time.Second), CPUCapacity: 2,
		NetworkRxBytes: 100, NetworkTxBytes: 200,
	}
	result, err := service.VM(context.Background(), uuid)
	if err != nil {
		t.Fatalf("VM() second error = %v", err)
	}
	if result.UUID != uuid || result.CPUPercent != 50 {
		t.Fatalf("result = %+v", result)
	}
	if result.NetworkRxBytesPerSecond != 0 || result.NetworkTxBytesPerSecond != 0 {
		t.Fatalf("counter reset rates = %d/%d", result.NetworkRxBytesPerSecond, result.NetworkTxBytesPerSecond)
	}
}

func TestServiceComputesDiskAndPerInterfaceRates(t *testing.T) {
	source := &fakeSource{vm: domain.MetricsCounters{Disks: []domain.MetricsDiskCounters{{
		Target: "vda", ReadBytes: 1000, WriteBytes: 2000, ReadOperations: 10, WriteOperations: 20,
	}}, Interfaces: []domain.MetricsInterfaceCounters{
		{Name: "vnet0", RxBytes: 4000, TxBytes: 3000},
		{Name: "vnet1", RxBytes: 100, TxBytes: 200},
	}}}
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	service := New(source, WithClock(func() time.Time { return now }))
	if _, err := service.VM(context.Background(), "vm-a"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	source.vm = domain.MetricsCounters{Disks: []domain.MetricsDiskCounters{{
		Target: "vda", ReadBytes: 5000, WriteBytes: 6000, ReadOperations: 30, WriteOperations: 50,
	}}, Interfaces: []domain.MetricsInterfaceCounters{
		{Name: "vnet0", RxBytes: 8000, TxBytes: 5000},
		{Name: "vnet1", RxBytes: 300, TxBytes: 600},
	}}
	result, err := service.VM(context.Background(), "vm-a")
	if err != nil {
		t.Fatal(err)
	}
	if result.DiskReadBytes != 5000 || result.DiskWriteBytes != 6000 || result.DiskReadOperations != 30 || result.DiskWriteOperations != 50 {
		t.Fatalf("disk counters = %+v", result.ResourceMetrics)
	}
	if result.DiskReadBytesPerSecond != 2000 || result.DiskWriteBytesPerSecond != 2000 || result.DiskReadOperationsPerSecond != 10 || result.DiskWriteOperationsPerSecond != 15 {
		t.Fatalf("disk rates = %+v", result.ResourceMetrics)
	}
	if len(result.Interfaces) != 2 || result.Interfaces[0].RxBytesPerSecond != 2000 || result.Interfaces[0].TxBytesPerSecond != 1000 || result.Interfaces[1].RxBytesPerSecond != 100 || result.Interfaces[1].TxBytesPerSecond != 200 {
		t.Fatalf("interfaces = %+v", result.Interfaces)
	}
}

func TestServiceClampsCPUAndPropagatesErrors(t *testing.T) {
	source := &fakeSource{host: domain.MetricsCounters{CPUTimeNanos: uint64(time.Second), CPUCapacity: 1}}
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	service := New(source, WithClock(func() time.Time { return now }))
	if _, err := service.Host(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	source.host.CPUTimeNanos = 10 * uint64(time.Second)
	result, err := service.Host(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.CPUPercent != 100 {
		t.Fatalf("CPUPercent = %v, want clamped 100", result.CPUPercent)
	}
	if _, err := New(&fakeSource{hostErr: domain.ErrUnavailable}).Host(context.Background()); err != domain.ErrUnavailable {
		t.Fatalf("host error = %v", err)
	}
	if _, err := New(&fakeSource{vmErr: domain.ErrNotFound}).VM(context.Background(), "missing"); err != domain.ErrNotFound {
		t.Fatalf("vm error = %v", err)
	}
}
