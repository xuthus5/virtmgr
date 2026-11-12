package libvirtbackend

import (
	"context"
	"strings"
	"testing"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func TestHostMetricHelpers(t *testing.T) {
	cpu := &libvirt.NodeCPUStats{UserSet: true, User: 10, KernelSet: true, Kernel: 20, IowaitSet: true, Iowait: 5}
	if got := hostCPUTimeNanos(cpu); got != 35 {
		t.Fatalf("hostCPUTimeNanos() = %d", got)
	}
	memory := &libvirt.NodeMemoryStats{TotalSet: true, Total: 100, AvailableSet: true, Available: 40}
	if got := hostMemoryUsed(memory); got != 60 {
		t.Fatalf("hostMemoryUsed(available) = %d", got)
	}
	memory.AvailableSet = false
	memory.FreeSet = true
	memory.Free = 25
	if got := hostMemoryUsed(memory); got != 75 {
		t.Fatalf("hostMemoryUsed(free) = %d", got)
	}
	stats := []libvirt.DomainMemoryStat{{Tag: int32(libvirt.DOMAIN_MEMORY_STAT_RSS), Val: 1234}}
	if got := domainMemoryValue(stats, libvirt.DOMAIN_MEMORY_STAT_RSS); got != 1234 {
		t.Fatalf("domainMemoryValue() = %d", got)
	}
}

func TestParseHostNetworkCounters(t *testing.T) {
	input := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 9999 1 0 0 0 0 0 0 9999 1 0 0 0 0 0 0
  eth0: 1000 1 0 0 0 0 0 0 2000 2 0 0 0 0 0 0
  vnet0: 5000 1 0 0 0 0 0 0 6000 2 0 0 0 0 0 0
virbr0: 7000 1 0 0 0 0 0 0 8000 2 0 0 0 0 0 0
  eth1: 3000 1 0 0 0 0 0 0 4000 2 0 0 0 0 0 0
`
	rx, tx := parseHostNetworkCounters(strings.NewReader(input))
	if rx != 4000 || tx != 6000 {
		t.Fatalf("parseHostNetworkCounters() = rx %d tx %d, want 4000/6000", rx, tx)
	}
}

func TestParseDomainInterfaceTargets(t *testing.T) {
	xmlText := `<domain><devices><interface type='network'><target dev='vnet7'/></interface><interface type='bridge'><target dev='vnet8'/></interface></devices></domain>`
	targets := parseDomainInterfaceTargets(xmlText)
	if len(targets) != 2 || targets[0] != "vnet7" || targets[1] != "vnet8" {
		t.Fatalf("targets = %v", targets)
	}
	if targets := parseDomainInterfaceTargets("not xml"); targets != nil {
		t.Fatalf("invalid XML targets = %v", targets)
	}
}

func TestDemoBackendMetrics(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	hostFirst, err := backend.HostMetrics(ctx)
	if err != nil {
		t.Fatalf("HostMetrics() error = %v", err)
	}
	hostSecond, err := backend.HostMetrics(ctx)
	if err != nil {
		t.Fatalf("HostMetrics() second error = %v", err)
	}
	if hostFirst.CPUTimeNanos >= hostSecond.CPUTimeNanos || hostFirst.MemoryTotalBytes != 64<<30 || hostFirst.CPUCapacity != 16 {
		t.Fatalf("host metrics = first %+v second %+v", hostFirst, hostSecond)
	}

	const uuid = "22222222-2222-4222-8222-222222222222"
	first, err := backend.VMMetrics(ctx, uuid)
	if err != nil {
		t.Fatalf("VMMetrics() error = %v", err)
	}
	second, err := backend.VMMetrics(ctx, uuid)
	if err != nil {
		t.Fatalf("VMMetrics() second error = %v", err)
	}
	if first.CPUTimeNanos >= second.CPUTimeNanos || first.CPUCapacity != 8 || first.MemoryTotalBytes != 16<<30 || first.MemoryUsedBytes == 0 {
		t.Fatalf("vm metrics = first %+v second %+v", first, second)
	}
	if len(first.Disks) == 0 || first.Disks[0].ReadBytes == 0 || len(first.Interfaces) == 0 || first.Interfaces[0].RxBytes == 0 {
		t.Fatalf("vm runtime counters = %+v", first)
	}
	if _, err := backend.VMMetrics(ctx, "missing"); err != domain.ErrNotFound {
		t.Fatalf("missing metrics error = %v", err)
	}
}
