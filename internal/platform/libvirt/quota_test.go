package libvirtbackend

import (
	"testing"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func TestVMResourceQuotaLiveParametersClearToDefaults(t *testing.T) {
	scheduler := vmResourceQuotaSchedulerParameters(domain.VMResourceQuota{})
	if !scheduler.CpuSharesSet || scheduler.CpuShares != defaultLiveCPUShares {
		t.Fatalf("CPU shares = set %v value %d", scheduler.CpuSharesSet, scheduler.CpuShares)
	}
	if !scheduler.GlobalPeriodSet || scheduler.GlobalPeriod != defaultLiveCPUPeriod {
		t.Fatalf("CPU period = set %v value %d", scheduler.GlobalPeriodSet, scheduler.GlobalPeriod)
	}
	if !scheduler.GlobalQuotaSet || scheduler.GlobalQuota != -1 {
		t.Fatalf("CPU quota = set %v value %d", scheduler.GlobalQuotaSet, scheduler.GlobalQuota)
	}

	memory := vmResourceQuotaMemoryParameters(domain.VMResourceQuota{})
	if !memory.HardLimitSet || memory.HardLimit != libvirt.DOMAIN_MEMORY_PARAM_UNLIMITED {
		t.Fatalf("memory hard limit = set %v value %d", memory.HardLimitSet, memory.HardLimit)
	}
	if !memory.SoftLimitSet || memory.SoftLimit != libvirt.DOMAIN_MEMORY_PARAM_UNLIMITED {
		t.Fatalf("memory soft limit = set %v value %d", memory.SoftLimitSet, memory.SoftLimit)
	}
	if memory.MinGuaranteeSet {
		t.Fatal("QEMU live memory quota must not set min guarantee")
	}
}

func TestVMResourceQuotaLiveParametersUseConfiguredValues(t *testing.T) {
	scheduler := vmResourceQuotaSchedulerParameters(domain.VMResourceQuota{
		CPUShares: 512, CPUQuota: 50_000, CPUPeriod: 100_000,
	})
	if scheduler.CpuShares != 512 || scheduler.GlobalPeriod != 100_000 || scheduler.GlobalQuota != 50_000 {
		t.Fatalf("scheduler = %+v", scheduler)
	}

	memory := vmResourceQuotaMemoryParameters(domain.VMResourceQuota{
		MemoryHardLimitMiB: 4096, MemorySoftLimitMiB: 3072,
	})
	if memory.HardLimit != 4096<<10 || memory.SoftLimit != 3072<<10 {
		t.Fatalf("memory = %+v", memory)
	}
}
