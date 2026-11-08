package app

import (
	"context"
	"testing"

	"virtmgr/internal/domain"
)

type fakeRepository struct {
	host     domain.Host
	devices  []domain.HostDevice
	vms      []domain.VM
	pools    []domain.StoragePool
	networks []domain.VirtualNetwork
	err      error
}

type fakeMetrics struct {
	result   domain.HostMetrics
	err      error
	hostName string
}

func (f *fakeMetrics) HostNamed(_ context.Context, hostName string) (domain.HostMetrics, error) {
	f.hostName = hostName
	return f.result, f.err
}

func (f fakeRepository) Host(context.Context) (domain.Host, error) { return f.host, f.err }
func (f fakeRepository) ListHostDevices(context.Context) ([]domain.HostDevice, error) {
	return f.devices, f.err
}
func (f fakeRepository) ListVMs(context.Context) ([]domain.VM, error) {
	return f.vms, f.err
}
func (f fakeRepository) ListStoragePools(context.Context) ([]domain.StoragePool, error) {
	return f.pools, f.err
}
func (f fakeRepository) ListNetworks(context.Context) ([]domain.VirtualNetwork, error) {
	return f.networks, f.err
}

func TestHostDetailsAggregatesInfrastructure(t *testing.T) {
	service := New(fakeRepository{
		host: domain.Host{Name: "kvm-node-01", CPUs: 16, MemoryBytes: 64 << 30},
		vms: []domain.VM{
			{Name: "web-02", State: domain.VMStateRunning, VCPUs: 2, MaxMemoryBytes: 4 << 30},
			{Name: "api-01", State: domain.VMStateCrashed, VCPUs: 4, MaxMemoryBytes: 8 << 30},
			{Name: "db-01", State: domain.VMStatePaused, VCPUs: 8, MaxMemoryBytes: 16 << 30},
		},
		pools: []domain.StoragePool{
			{Name: "zpool", State: domain.StoragePoolStateActive},
			{Name: "archive", State: domain.StoragePoolStateDegraded},
		},
		networks: []domain.VirtualNetwork{{Name: "lab", Active: true}, {Name: "default", Active: true}},
	})
	details, err := service.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if details.Host.Name != "kvm-node-01" || details.Counts != (domain.VMCounts{Total: 3, Running: 1, Paused: 1, Failed: 1}) {
		t.Fatalf("details = %+v", details)
	}
	if details.AllocatedVCPUs != 14 || details.AllocatedMemoryBytes != 28<<30 {
		t.Fatalf("allocation = %d vCPU %d bytes", details.AllocatedVCPUs, details.AllocatedMemoryBytes)
	}
	if details.VMs[0].Name != "api-01" || details.StoragePools[0].Name != "archive" || details.Networks[0].Name != "default" {
		t.Fatalf("sorting failed: vms=%+v pools=%+v networks=%+v", details.VMs, details.StoragePools, details.Networks)
	}
	if len(details.Warnings) != 2 || details.Warnings[0] != "1 台虚拟机处于异常状态" {
		t.Fatalf("warnings = %+v", details.Warnings)
	}
}

func TestHostDetailsPropagatesErrors(t *testing.T) {
	service := New(fakeRepository{err: domain.ErrUnavailable})
	if _, err := service.Get(context.Background()); err != domain.ErrUnavailable {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestHostDevicesSortsByTypeAndName(t *testing.T) {
	service := New(fakeRepository{devices: []domain.HostDevice{
		{Name: "usb_device_02", Type: "usb_device"},
		{Name: "pci_0000_00_1f_6", Type: "pci"},
		{Name: "pci_0000_00_02_0", Type: "pci"},
		{Name: "net_eth0", Type: "net"},
	}})
	devices, err := service.Devices(context.Background())
	if err != nil {
		t.Fatalf("Devices() error = %v", err)
	}
	if devices.Total != 4 || len(devices.Items) != 4 {
		t.Fatalf("devices = %+v", devices)
	}
	got := []string{devices.Items[0].Name, devices.Items[1].Name, devices.Items[2].Name, devices.Items[3].Name}
	want := []string{"net_eth0", "pci_0000_00_02_0", "pci_0000_00_1f_6", "usb_device_02"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestHostDevicesPropagatesErrors(t *testing.T) {
	service := New(fakeRepository{err: domain.ErrUnavailable})
	if _, err := service.Devices(context.Background()); err != domain.ErrUnavailable {
		t.Fatalf("Devices() error = %v", err)
	}
}

func TestHostMetricsUsesConfiguredProvider(t *testing.T) {
	metrics := &fakeMetrics{result: domain.HostMetrics{HostName: "node-01", ResourceMetrics: domain.ResourceMetrics{CPUPercent: 42}}}
	service := New(fakeRepository{host: domain.Host{Name: "node-01"}}, WithMetrics(metrics))
	result, err := service.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics() error = %v", err)
	}
	if metrics.hostName != "node-01" || result.CPUPercent != 42 || result.HostName != "node-01" {
		t.Fatalf("metrics = %+v, hostName = %q", result, metrics.hostName)
	}
}

func TestHostMetricsPropagatesErrorsAndRequiresProvider(t *testing.T) {
	if _, err := New(fakeRepository{}).Metrics(context.Background()); err != domain.ErrUnavailable {
		t.Fatalf("missing provider error = %v", err)
	}
	metrics := &fakeMetrics{err: domain.ErrUnavailable}
	if _, err := New(fakeRepository{host: domain.Host{Name: "node-01"}}, WithMetrics(metrics)).Metrics(context.Background()); err != domain.ErrUnavailable {
		t.Fatalf("provider error = %v", err)
	}
	if _, err := New(fakeRepository{err: domain.ErrUnavailable}, WithMetrics(metrics)).Metrics(context.Background()); err != domain.ErrUnavailable {
		t.Fatalf("repository error = %v", err)
	}
}
