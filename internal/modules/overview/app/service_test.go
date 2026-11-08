package app

import (
	"context"
	"testing"

	"virtmgr/internal/domain"
)

type fakeRepository struct {
	host domain.Host
	vms  []domain.VM
	err  error
}

func (f fakeRepository) Host(context.Context) (domain.Host, error) { return f.host, f.err }
func (f fakeRepository) ListVMs(context.Context) ([]domain.VM, error) {
	return f.vms, f.err
}

func TestOverviewCounts(t *testing.T) {
	service := New(fakeRepository{
		host: domain.Host{Name: "host", CPUs: 8},
		vms: []domain.VM{
			{Name: "z", State: domain.VMStateRunning, VCPUs: 4, MaxMemoryBytes: 8 << 30},
			{Name: "a", State: domain.VMStatePaused, VCPUs: 2, MaxMemoryBytes: 4 << 30},
			{Name: "m", State: domain.VMStateShutoff},
			{Name: "x", State: domain.VMStateCrashed},
			{Name: "b", State: domain.VMStateRunning},
		},
	})
	overview, err := service.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if overview.Counts != (domain.VMCounts{Total: 5, Running: 2, Paused: 1, Shutoff: 1, Failed: 1}) {
		t.Fatalf("counts = %+v", overview.Counts)
	}
	if overview.VMs[0].Name != "a" {
		t.Fatalf("vms not sorted: %+v", overview.VMs)
	}
	if overview.AllocatedVCPUs != 6 || overview.AllocatedMemoryBytes != 12<<30 {
		t.Fatalf("allocated resources = %d vCPU, %d bytes", overview.AllocatedVCPUs, overview.AllocatedMemoryBytes)
	}
}
