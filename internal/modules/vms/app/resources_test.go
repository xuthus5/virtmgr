package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

type resourceRepository struct {
	fakeVMRepository
	uuid  string
	spec  domain.VMResourcesSpec
	calls int
	err   error
}

func (r *resourceRepository) UpdateVMResources(_ context.Context, uuid string, spec domain.VMResourcesSpec) (domain.VM, error) {
	r.calls++
	r.uuid = uuid
	r.spec = spec
	if r.err != nil {
		return domain.VM{}, r.err
	}
	for _, vm := range r.vms {
		if vm.UUID != uuid {
			continue
		}
		vm.VCPUs = spec.VCPUs
		vm.MaxVCPUs = max(vm.MaxVCPUs, spec.VCPUs)
		vm.MemoryBytes = spec.MemoryMiB * 1024 * 1024
		vm.MaxMemoryBytes = max(vm.MaxMemoryBytes, vm.MemoryBytes)
		return vm, nil
	}
	return domain.VM{}, domain.ErrNotFound
}

func resourceVM(state domain.VMState) domain.VM {
	return domain.VM{
		UUID: "uuid-a", Name: "api-01", State: state, Persistent: true,
		VCPUs: 2, MaxVCPUs: 4, MemoryBytes: 2 << 30, MaxMemoryBytes: 4 << 30,
		AllowedActions: domain.AllowedVMActions(state),
	}
}

func TestUpdateResourcesRecordsLiveUpdate(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &resourceRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{resourceVM(domain.VMStateRunning)}}}
	service := New(repository, audit, false)

	updated, err := service.UpdateResources(context.Background(), UpdateResourcesCommand{
		UUID: " uuid-a ", Spec: domain.VMResourcesSpec{VCPUs: 4, MemoryMiB: 3072},
		Request: MutationContext{Session: operatorSession(), RequestID: "req-resources", SourceIP: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("UpdateResources() error = %v", err)
	}
	if updated.VCPUs != 4 || updated.MemoryBytes != 3<<30 {
		t.Fatalf("updated VM = %+v", updated)
	}
	if repository.calls != 1 || repository.uuid != "uuid-a" || repository.spec.VCPUs != 4 || repository.spec.MemoryMiB != 3072 {
		t.Fatalf("repository call = %+v", repository)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	event := audit.events[0]
	if event.Action != "vm.resources.update" || event.TargetKind != "vm" || event.TargetID != "uuid-a" || event.Result != "success" {
		t.Fatalf("audit event = %+v", event)
	}
	if event.Detail["previousVCPUs"] != uint(2) || event.Detail["previousMemoryMiB"] != uint64(2048) || event.Detail["vcpus"] != uint(4) || event.Detail["memoryMiB"] != uint64(3072) {
		t.Fatalf("audit detail = %+v", event.Detail)
	}
}

func TestUpdateResourcesAllowsShutoffExpansion(t *testing.T) {
	repository := &resourceRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{resourceVM(domain.VMStateShutoff)}}}
	service := New(repository, &fakeAuditRecorder{}, false)

	updated, err := service.UpdateResources(context.Background(), UpdateResourcesCommand{
		UUID: "uuid-a", Spec: domain.VMResourcesSpec{VCPUs: 8, MemoryMiB: 8192},
		Request: MutationContext{Session: operatorSession()},
	})
	if err != nil {
		t.Fatalf("UpdateResources() error = %v", err)
	}
	if updated.VCPUs != 8 || updated.MaxVCPUs != 8 || updated.MemoryBytes != 8<<30 || updated.MaxMemoryBytes != 8<<30 {
		t.Fatalf("updated VM = %+v", updated)
	}
}

func TestUpdateResourcesRejectsActiveExpansionBeyondMaximum(t *testing.T) {
	for _, spec := range []domain.VMResourcesSpec{
		{VCPUs: 5, MemoryMiB: 2048},
		{VCPUs: 2, MemoryMiB: 4097},
	} {
		t.Run("too large", func(t *testing.T) {
			repository := &resourceRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{resourceVM(domain.VMStateRunning)}}}
			audit := &fakeAuditRecorder{}
			_, err := New(repository, audit, false).UpdateResources(context.Background(), UpdateResourcesCommand{
				UUID: "uuid-a", Spec: spec, Request: MutationContext{Session: operatorSession()},
			})
			if !errors.Is(err, domain.ErrActionNotAllowed) {
				t.Fatalf("error = %v, want action not allowed", err)
			}
			if repository.calls != 0 {
				t.Fatalf("backend calls = %d", repository.calls)
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestUpdateResourcesRejectsInvalidMutations(t *testing.T) {
	viewer := operatorSession()
	viewer.User.Role = domain.RoleViewer
	transient := resourceVM(domain.VMStateRunning)
	transient.Persistent = false
	shuttingDown := resourceVM(domain.VMStateShutdown)

	tests := []struct {
		name     string
		readOnly bool
		vm       domain.VM
		session  domain.Session
		spec     domain.VMResourcesSpec
		want     error
	}{
		{name: "viewer", vm: resourceVM(domain.VMStateRunning), session: viewer, spec: domain.VMResourcesSpec{VCPUs: 2, MemoryMiB: 2048}, want: domain.ErrForbidden},
		{name: "read only", readOnly: true, vm: resourceVM(domain.VMStateRunning), session: operatorSession(), spec: domain.VMResourcesSpec{VCPUs: 2, MemoryMiB: 2048}, want: domain.ErrReadOnly},
		{name: "transient", vm: transient, session: operatorSession(), spec: domain.VMResourcesSpec{VCPUs: 2, MemoryMiB: 2048}, want: domain.ErrActionNotAllowed},
		{name: "shutting down", vm: shuttingDown, session: operatorSession(), spec: domain.VMResourcesSpec{VCPUs: 2, MemoryMiB: 2048}, want: domain.ErrActionNotAllowed},
		{name: "zero vcpus", vm: resourceVM(domain.VMStateRunning), session: operatorSession(), spec: domain.VMResourcesSpec{MemoryMiB: 2048}, want: domain.ErrInvalidInput},
		{name: "too many vcpus", vm: resourceVM(domain.VMStateRunning), session: operatorSession(), spec: domain.VMResourcesSpec{VCPUs: 256, MemoryMiB: 2048}, want: domain.ErrInvalidInput},
		{name: "too little memory", vm: resourceVM(domain.VMStateRunning), session: operatorSession(), spec: domain.VMResourcesSpec{VCPUs: 2, MemoryMiB: 127}, want: domain.ErrInvalidInput},
		{name: "too much memory", vm: resourceVM(domain.VMStateRunning), session: operatorSession(), spec: domain.VMResourcesSpec{VCPUs: 2, MemoryMiB: 1024*1024 + 1}, want: domain.ErrInvalidInput},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &resourceRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{test.vm}}}
			audit := &fakeAuditRecorder{}
			_, err := New(repository, audit, test.readOnly).UpdateResources(context.Background(), UpdateResourcesCommand{
				UUID: "uuid-a", Spec: test.spec, Request: MutationContext{Session: test.session},
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if repository.calls != 0 {
				t.Fatalf("backend calls = %d", repository.calls)
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" || audit.events[0].Action != "vm.resources.update" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestUpdateResourcesAuditsBackendFailureAndFailsClosed(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &resourceRepository{
		fakeVMRepository: fakeVMRepository{vms: []domain.VM{resourceVM(domain.VMStateRunning)}},
		err:              errors.New("libvirt failed"),
	}
	_, err := New(repository, audit, false).UpdateResources(context.Background(), UpdateResourcesCommand{
		UUID: "uuid-a", Spec: domain.VMResourcesSpec{VCPUs: 3, MemoryMiB: 3072},
		Request: MutationContext{Session: operatorSession()},
	})
	if err == nil || len(audit.events) != 1 || audit.events[0].Result != "failure" || audit.events[0].Detail["error"] == nil {
		t.Fatalf("error = %v audit = %+v", err, audit.events)
	}

	repository = &resourceRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{resourceVM(domain.VMStateRunning)}}}
	_, err = New(repository, &fakeAuditRecorder{err: errors.New("audit unavailable")}, false).UpdateResources(context.Background(), UpdateResourcesCommand{
		UUID: "uuid-a", Spec: domain.VMResourcesSpec{VCPUs: 3, MemoryMiB: 3072},
		Request: MutationContext{Session: operatorSession()},
	})
	if err == nil || !strings.Contains(err.Error(), "record VM resources audit") {
		t.Fatalf("error = %v", err)
	}
}
