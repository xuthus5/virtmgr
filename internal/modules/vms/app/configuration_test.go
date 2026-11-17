package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

const configurationTestXML = `<domain type='kvm'>
  <name>api-01</name>
  <uuid>uuid-a</uuid>
  <os><type arch='x86_64'>hvm</type><boot dev='hd'/></os>
  <cpu mode='host-model'><topology sockets='1' cores='2' threads='1'/></cpu>
  <devices>
    <disk type='file' device='disk'><source file='/images/api.qcow2'/><target dev='vda' bus='virtio'/></disk>
    <disk type='file' device='cdrom'><source file='/images/install.iso'/><target dev='sda' bus='sata'/><readonly/></disk>
    <interface type='network'><mac address='52:54:00:01:12:99'/><source network='default'/><model type='virtio'/></interface>
  </devices>
</domain>`

type configurationRepository struct {
	*fakeVMRepository
	xml            string
	getXMLErr      error
	defineXMLErr   error
	defineCalls    int
	definedXML     string
	diskQoSSpec    domain.VMDiskQoSSpec
	networkQoSSpec domain.VMNetworkQoSSpec
	qosErr         error
}

func (r *configurationRepository) GetVMXML(_ context.Context, uuid string, format domain.VMXMLFormat) (domain.VMXML, error) {
	if r.getXMLErr != nil {
		return domain.VMXML{}, r.getXMLErr
	}
	xmlText := r.xml
	if xmlText == "" {
		xmlText = configurationTestXML
	}
	return domain.VMXML{UUID: uuid, Name: "api-01", Format: format, XML: xmlText}, nil
}

func (r *configurationRepository) DefineVMXML(_ context.Context, uuid, xmlText string) (domain.VMXML, error) {
	r.defineCalls++
	r.definedXML = xmlText
	if r.defineXMLErr != nil {
		return domain.VMXML{}, r.defineXMLErr
	}
	return domain.VMXML{UUID: uuid, Name: "api-01", Format: domain.VMXMLFormatConfig, XML: xmlText}, nil
}

func (r *configurationRepository) UpdateVMDiskQoS(_ context.Context, uuid, target string, spec domain.VMDiskQoSSpec) (domain.VM, error) {
	if r.qosErr != nil {
		return domain.VM{}, r.qosErr
	}
	r.diskQoSSpec = spec
	for _, vm := range r.vms {
		if vm.UUID != uuid {
			continue
		}
		for index := range vm.Disks {
			if vm.Disks[index].Target == target {
				qos := domain.DiskQoS(spec)
				vm.Disks[index].QoS = &qos
				return vm, nil
			}
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func (r *configurationRepository) UpdateVMNetworkQoS(_ context.Context, uuid, mac string, spec domain.VMNetworkQoSSpec) (domain.VM, error) {
	if r.qosErr != nil {
		return domain.VM{}, r.qosErr
	}
	r.networkQoSSpec = spec
	for _, vm := range r.vms {
		if vm.UUID != uuid {
			continue
		}
		for index := range vm.Interfaces {
			if vm.Interfaces[index].MAC == mac {
				qos := domain.NetworkQoS(spec)
				vm.Interfaces[index].QoS = &qos
				return vm, nil
			}
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func configurationVM(state domain.VMState) domain.VM {
	return domain.VM{
		UUID: "uuid-a", Name: "api-01", State: state, Persistent: true, VCPUs: 2, MaxVCPUs: 4,
		BootDevices: []domain.VMBootDevice{domain.VMBootDeviceDisk}, CPUModel: "host-model", CPUSockets: 1, CPUCores: 2, CPUThreads: 1,
		Disks: []domain.Disk{
			{Device: "disk", Target: "vda", Bus: "virtio", Source: "/images/api.qcow2"},
			{Device: "cdrom", Target: "sda", Bus: "sata", Source: "/images/install.iso", ReadOnly: true},
		},
		Interfaces: []domain.NetworkInterface{{MAC: "52:54:00:01:12:99", Type: "network", Source: "default"}},
	}
}

func TestUpdateBootOrderPersistsAndAudits(t *testing.T) {
	repository := &configurationRepository{fakeVMRepository: &fakeVMRepository{vms: []domain.VM{configurationVM(domain.VMStateRunning)}}}
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false)
	updated, err := service.UpdateBootOrder(context.Background(), BootCommand{
		UUID: " uuid-a ", Spec: domain.VMBootSpec{Devices: []domain.VMBootDevice{domain.VMBootDeviceCDROM, domain.VMBootDeviceDisk, domain.VMBootDeviceNetwork}},
		Request: MutationContext{Session: operatorSession(), RequestID: "req-boot"},
	})
	if err != nil {
		t.Fatalf("UpdateBootOrder() error = %v", err)
	}
	if repository.defineCalls != 1 || len(updated.BootDevices) != 3 || updated.BootDevices[0] != domain.VMBootDeviceCDROM {
		t.Fatalf("defineCalls=%d updated boot=%+v", repository.defineCalls, updated.BootDevices)
	}
	for _, want := range []string{`<boot dev="cdrom">`, `<boot dev="hd">`, `<boot dev="network">`} {
		if !strings.Contains(repository.definedXML, want) {
			t.Fatalf("defined XML missing %q:\n%s", want, repository.definedXML)
		}
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.boot.update" || audit.events[0].Result != "success" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestUpdateBootOrderRejectsUnavailableDeviceAndPermissions(t *testing.T) {
	vm := configurationVM(domain.VMStateShutoff)
	vm.Interfaces = nil
	request := MutationContext{Session: operatorSession()}
	command := BootCommand{UUID: "uuid-a", Spec: domain.VMBootSpec{Devices: []domain.VMBootDevice{domain.VMBootDeviceNetwork}}, Request: request}
	repository := &configurationRepository{fakeVMRepository: &fakeVMRepository{vms: []domain.VM{vm}}}
	service := New(repository, &fakeAuditRecorder{}, false)
	if _, err := service.UpdateBootOrder(context.Background(), command); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("missing boot device error = %v", err)
	}
	viewer := command
	viewer.Request.Session.User.Role = domain.RoleViewer
	if _, err := service.UpdateBootOrder(context.Background(), viewer); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("viewer error = %v", err)
	}
	readOnly := New(repository, &fakeAuditRecorder{}, true)
	if _, err := readOnly.UpdateBootOrder(context.Background(), command); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only error = %v", err)
	}
	vm.Persistent = false
	repository.vms = []domain.VM{vm}
	command.Spec.Devices = []domain.VMBootDevice{domain.VMBootDeviceDisk}
	if _, err := service.UpdateBootOrder(context.Background(), command); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("non-persistent error = %v", err)
	}
	if repository.defineCalls != 0 {
		t.Fatalf("rejected request defined XML %d times", repository.defineCalls)
	}
}

func TestUpdateCPUConfigPersistsModelAndTopology(t *testing.T) {
	vm := configurationVM(domain.VMStateShutoff)
	vm.VCPUs = 4
	repository := &configurationRepository{fakeVMRepository: &fakeVMRepository{vms: []domain.VM{vm}}}
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false)
	updated, err := service.UpdateCPUConfig(context.Background(), CPUCommand{
		UUID: "uuid-a", Spec: domain.VMCPUConfigSpec{Model: "Cascadelake-Server", Sockets: 2, Cores: 2, Threads: 1},
		Request: MutationContext{Session: operatorSession()},
	})
	if err != nil {
		t.Fatalf("UpdateCPUConfig() error = %v", err)
	}
	if updated.CPUModel != "Cascadelake-Server" || updated.CPUSockets != 2 || updated.CPUCores != 2 || updated.CPUThreads != 1 {
		t.Fatalf("updated CPU = %+v", updated)
	}
	if repository.defineCalls != 1 || !strings.Contains(repository.definedXML, "Cascadelake-Server") || !strings.Contains(repository.definedXML, `sockets="2"`) {
		t.Fatalf("defined XML = %s", repository.definedXML)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.cpu.update" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestUpdateCPUConfigRejectsUnsafeChanges(t *testing.T) {
	request := MutationContext{Session: operatorSession()}
	for _, test := range []struct {
		name     string
		state    domain.VMState
		spec     domain.VMCPUConfigSpec
		readOnly bool
		want     error
	}{
		{name: "running", state: domain.VMStateRunning, spec: domain.VMCPUConfigSpec{Model: "host-model", Sockets: 1, Cores: 2, Threads: 1}, want: domain.ErrActionNotAllowed},
		{name: "topology mismatch", state: domain.VMStateShutoff, spec: domain.VMCPUConfigSpec{Model: "host-model", Sockets: 2, Cores: 2, Threads: 1}, want: domain.ErrInvalidInput},
		{name: "invalid model", state: domain.VMStateShutoff, spec: domain.VMCPUConfigSpec{Model: "bad model", Sockets: 1, Cores: 2, Threads: 1}, want: domain.ErrInvalidInput},
		{name: "read only", state: domain.VMStateShutoff, spec: domain.VMCPUConfigSpec{Model: "host-model", Sockets: 1, Cores: 2, Threads: 1}, readOnly: true, want: domain.ErrReadOnly},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &configurationRepository{fakeVMRepository: &fakeVMRepository{vms: []domain.VM{configurationVM(test.state)}}}
			service := New(repository, &fakeAuditRecorder{}, test.readOnly)
			if _, err := service.UpdateCPUConfig(context.Background(), CPUCommand{UUID: "uuid-a", Spec: test.spec, Request: request}); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if repository.defineCalls != 0 {
				t.Fatalf("rejected request defined XML %d times", repository.defineCalls)
			}
		})
	}
}

func TestUpdateDiskQoSPersistsAndValidates(t *testing.T) {
	repository := &configurationRepository{fakeVMRepository: &fakeVMRepository{vms: []domain.VM{configurationVM(domain.VMStateRunning)}}}
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false)
	spec := domain.VMDiskQoSSpec{TotalBytesSec: 20 << 20, ReadIOPS: 1000, WriteIOPS: 500}
	updated, err := service.UpdateDiskQoS(context.Background(), DiskQoSCommand{UUID: "uuid-a", Target: " vda ", Spec: spec, Request: MutationContext{Session: operatorSession()}})
	if err != nil {
		t.Fatalf("UpdateDiskQoS() error = %v", err)
	}
	if repository.diskQoSSpec != spec || updated.Disks[0].QoS == nil || updated.Disks[0].QoS.ReadIOPS != 1000 {
		t.Fatalf("spec=%+v updated=%+v", repository.diskQoSSpec, updated.Disks)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.disk.qos.update" {
		t.Fatalf("audit events = %+v", audit.events)
	}
	if _, err := service.UpdateDiskQoS(context.Background(), DiskQoSCommand{UUID: "uuid-a", Target: "sda", Spec: spec, Request: MutationContext{Session: operatorSession()}}); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("cdrom QoS error = %v", err)
	}
	if _, err := service.UpdateDiskQoS(context.Background(), DiskQoSCommand{UUID: "uuid-a", Target: "vda", Spec: domain.VMDiskQoSSpec{TotalIOPS: maxQoSOperationsPerSecond + 1}, Request: MutationContext{Session: operatorSession()}}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("oversized QoS error = %v", err)
	}
}

func TestUpdateNetworkQoSPersistsAndValidates(t *testing.T) {
	repository := &configurationRepository{fakeVMRepository: &fakeVMRepository{vms: []domain.VM{configurationVM(domain.VMStateRunning)}}}
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false)
	spec := domain.VMNetworkQoSSpec{InboundAverageBytes: 10 << 20, InboundPeakBytes: 20 << 20, OutboundAverageBytes: 5 << 20}
	updated, err := service.UpdateNetworkQoS(context.Background(), NetworkQoSCommand{
		UUID: "uuid-a", MAC: " 52:54:00:01:12:99 ", Spec: spec, Request: MutationContext{Session: operatorSession()},
	})
	if err != nil {
		t.Fatalf("UpdateNetworkQoS() error = %v", err)
	}
	if repository.networkQoSSpec != spec || updated.Interfaces[0].QoS == nil || updated.Interfaces[0].QoS.InboundPeakBytes != 20<<20 {
		t.Fatalf("spec=%+v updated=%+v", repository.networkQoSSpec, updated.Interfaces)
	}
	for _, invalid := range []domain.VMNetworkQoSSpec{
		{InboundPeakBytes: 1 << 20},
		{InboundAverageBytes: 20 << 20, InboundPeakBytes: 10 << 20},
		{OutboundBurstBytes: 1 << 20},
	} {
		if _, err := service.UpdateNetworkQoS(context.Background(), NetworkQoSCommand{UUID: "uuid-a", MAC: "52:54:00:01:12:99", Spec: invalid, Request: MutationContext{Session: operatorSession()}}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("invalid QoS %+v error = %v", invalid, err)
		}
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.network.qos.update" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}
