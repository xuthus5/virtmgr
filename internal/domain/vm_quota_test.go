package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestSetVMResourceQuotaXMLPreservesPlacement(t *testing.T) {
	input := `<domain><name>quota-vm</name><memory unit="MiB">2048</memory><vcpu>2</vcpu><cputune><vcpupin vcpu="0" cpuset="0"/><shares>512</shares></cputune><devices></devices></domain>`
	updated, err := SetVMResourceQuotaXML(input, VMResourceQuota{
		CPUShares: 2048, CPUQuota: 50_000, CPUPeriod: 100_000,
		MemoryHardLimitMiB: 4096, MemorySoftLimitMiB: 3072,
	})
	if err != nil {
		t.Fatalf("SetVMResourceQuotaXML() error = %v", err)
	}
	for _, fragment := range []string{`<shares>2048</shares>`, `<period>100000</period>`, `<quota>50000</quota>`, `<hard_limit unit="MiB">4096</hard_limit>`, `vcpupin vcpu="0" cpuset="0"`} {
		if !strings.Contains(updated, fragment) {
			t.Fatalf("updated XML missing %q:\n%s", fragment, updated)
		}
	}
	quota, err := VMResourceQuotaFromXML(updated)
	if err != nil || quota == nil || quota.CPUShares != 2048 || quota.MemorySoftLimitMiB != 3072 {
		t.Fatalf("quota = %+v, error = %v", quota, err)
	}
	cleared, err := SetVMResourceQuotaXML(updated, VMResourceQuota{})
	if err != nil || strings.Contains(cleared, "<memtune>") || strings.Contains(cleared, "<quota>") {
		t.Fatalf("quota was not cleared: %v\n%s", err, cleared)
	}
	if !strings.Contains(cleared, "vcpupin") {
		t.Fatalf("placement was removed:\n%s", cleared)
	}
}

func TestNormalizeVMResourceQuotaRejectsInvalidMemoryRelationships(t *testing.T) {
	for name, quota := range map[string]VMResourceQuota{
		"period without quota":      {CPUPeriod: 100_000},
		"hard below memory":         {MemoryHardLimitMiB: 1024},
		"soft above hard":           {MemoryHardLimitMiB: 2048, MemorySoftLimitMiB: 4096},
		"min guarantee unsupported": {MemoryMinGuaranteeMiB: 4096},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeVMResourceQuota(quota, 2048); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestSetVMXMLHardwareReplacesRNGAndWatchdog(t *testing.T) {
	input := `<domain><name>hardware-vm</name><devices><disk type="file" device="disk"/><rng model="virtio"/></devices></domain>`
	updated, err := SetVMXMLHardware(input, VMHardware{RNG: true, Watchdog: &VMWatchdog{Model: VMWatchdogModelIB700, Action: VMWatchdogActionPoweroff}})
	if err != nil {
		t.Fatalf("SetVMXMLHardware() error = %v", err)
	}
	if strings.Count(updated, "<rng ") != 1 || !strings.Contains(updated, `/dev/urandom`) || !strings.Contains(updated, `watchdog model="ib700" action="poweroff"`) {
		t.Fatalf("unexpected hardware XML:\n%s", updated)
	}
	cleared, err := SetVMXMLHardware(updated, VMHardware{})
	if err != nil || strings.Contains(cleared, "<rng") || strings.Contains(cleared, "<watchdog") {
		t.Fatalf("hardware was not cleared: %v\n%s", err, cleared)
	}
	if _, err := SetVMXMLHardware(input, VMHardware{Watchdog: &VMWatchdog{Model: "bad", Action: "reset"}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid watchdog error = %v", err)
	}
}
