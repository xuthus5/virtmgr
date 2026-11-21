package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

func TestUpdateResourceQuotaRecordsLiveUpdate(t *testing.T) {
	repository := &configurationRepository{fakeVMRepository: &fakeVMRepository{vms: []domain.VM{resourceVM(domain.VMStateRunning)}}}
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false)
	updated, err := service.UpdateResourceQuota(context.Background(), UpdateResourceQuotaCommand{
		UUID: "uuid-a", Quota: domain.VMResourceQuota{CPUShares: 1024, CPUQuota: 50_000, CPUPeriod: 100_000, MemoryHardLimitMiB: 4096, MemorySoftLimitMiB: 3072},
		Request: MutationContext{Session: operatorSession()},
	})
	if err != nil {
		t.Fatalf("UpdateResourceQuota() error = %v", err)
	}
	if updated.ResourceQuota == nil || repository.quotaSpec.CPUShares != 1024 || repository.quotaSpec.MemoryHardLimitMiB != 4096 {
		t.Fatalf("quota = %+v repository = %+v", updated.ResourceQuota, repository.quotaSpec)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.quota.update" || audit.events[0].Result != "success" {
		t.Fatalf("audit = %+v", audit.events)
	}
}

func TestUpdateResourceQuotaValidationAndState(t *testing.T) {
	for name, test := range map[string]struct {
		vm    domain.VM
		quota domain.VMResourceQuota
		want  error
	}{
		"hard below memory": {vm: resourceVM(domain.VMStateRunning), quota: domain.VMResourceQuota{MemoryHardLimitMiB: 1024}, want: domain.ErrInvalidInput},
		"shutting down":     {vm: resourceVM(domain.VMStateShutdown), quota: domain.VMResourceQuota{}, want: domain.ErrActionNotAllowed},
	} {
		t.Run(name, func(t *testing.T) {
			repository := &configurationRepository{fakeVMRepository: &fakeVMRepository{vms: []domain.VM{test.vm}}}
			audit := &fakeAuditRecorder{}
			_, err := New(repository, audit, false).UpdateResourceQuota(context.Background(), UpdateResourceQuotaCommand{
				UUID: "uuid-a", Quota: test.quota, Request: MutationContext{Session: operatorSession()},
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("audit = %+v", audit.events)
			}
		})
	}
}

func TestUpdateHardwareRewritesPersistentDevices(t *testing.T) {
	vm := resourceVM(domain.VMStateShutoff)
	repository := &configurationRepository{fakeVMRepository: &fakeVMRepository{vms: []domain.VM{vm}}}
	audit := &fakeAuditRecorder{}
	updated, err := New(repository, audit, false).UpdateHardware(context.Background(), UpdateHardwareCommand{
		UUID: "uuid-a", Hardware: domain.VMHardware{RNG: true, Watchdog: &domain.VMWatchdog{Model: domain.VMWatchdogModelIB700, Action: domain.VMWatchdogActionPoweroff}},
		Request: MutationContext{Session: operatorSession()},
	})
	if err != nil {
		t.Fatalf("UpdateHardware() error = %v", err)
	}
	if !updated.RNG || updated.Watchdog == nil || updated.Watchdog.Model != domain.VMWatchdogModelIB700 {
		t.Fatalf("updated VM = %+v", updated)
	}
	if !strings.Contains(repository.definedXML, "/dev/urandom") || !strings.Contains(repository.definedXML, `watchdog model="ib700" action="poweroff"`) {
		t.Fatalf("defined XML = %s", repository.definedXML)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.hardware.update" {
		t.Fatalf("audit = %+v", audit.events)
	}
}

func TestUpdateHardwareRequiresShutoff(t *testing.T) {
	repository := &configurationRepository{fakeVMRepository: &fakeVMRepository{vms: []domain.VM{resourceVM(domain.VMStateRunning)}}}
	audit := &fakeAuditRecorder{}
	_, err := New(repository, audit, false).UpdateHardware(context.Background(), UpdateHardwareCommand{
		UUID: "uuid-a", Hardware: domain.VMHardware{RNG: true}, Request: MutationContext{Session: operatorSession()},
	})
	if !errors.Is(err, domain.ErrActionNotAllowed) || repository.defineCalls != 0 {
		t.Fatalf("error = %v, define calls = %d", err, repository.defineCalls)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("audit = %+v", audit.events)
	}
}
