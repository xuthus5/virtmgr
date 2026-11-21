package app

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
)

type placementTestRepository struct {
	fakeVMRepository
	calls int
	uuid  string
	spec  domain.VMPlacementSpec
	err   error
}

func (r *placementTestRepository) UpdateVMPlacement(_ context.Context, uuid string, spec domain.VMPlacementSpec) (domain.VM, error) {
	r.calls++
	r.uuid = uuid
	r.spec = spec
	if r.err != nil {
		return domain.VM{}, r.err
	}
	for _, vm := range r.vms {
		if vm.UUID == uuid {
			placement := domain.VMPlacement(spec)
			vm.Placement = &placement
			return vm, nil
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func placementTestVM(state domain.VMState) domain.VM {
	return domain.VM{
		UUID: "uuid-placement", Name: "api-01", State: state, Persistent: true,
		VCPUs: 4, MemoryBytes: 4 << 30, AllowedActions: domain.AllowedVMActions(state),
	}
}

func TestUpdatePlacementNormalizesAndAudits(t *testing.T) {
	repository := &placementTestRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{placementTestVM(domain.VMStateShutoff)}}}
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false)
	updated, err := service.UpdatePlacement(context.Background(), UpdatePlacementCommand{
		UUID: " uuid-placement ",
		Spec: domain.VMPlacementSpec{
			HugePageSize: "2MiB", EmulatorPin: "8",
			NUMACells: []domain.VMNUMACell{{ID: 1, Cpuset: "2-3", MemoryMiB: 2048}, {ID: 0, Cpuset: "0-1", MemoryMiB: 2048}},
			VCPUPins:  []domain.VMCpuPin{{VCPU: 1, Cpuset: "3"}, {VCPU: 0, Cpuset: "2"}},
		},
		Request: MutationContext{Session: operatorSession()},
	})
	if err != nil {
		t.Fatalf("UpdatePlacement() error = %v", err)
	}
	if repository.calls != 1 || repository.uuid != "uuid-placement" || repository.spec.NUMACells[0].ID != 0 || repository.spec.VCPUPins[0].VCPU != 0 {
		t.Fatalf("repository call = %+v", repository)
	}
	if updated.Placement == nil || updated.Placement.HugePageSize != "2MiB" {
		t.Fatalf("updated placement = %+v", updated.Placement)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.placement.update" || audit.events[0].Result != "success" || audit.events[0].Detail["numaCells"] != 2 {
		t.Fatalf("audit = %+v", audit.events)
	}
}

func TestUpdatePlacementRejectsUnsafeMutations(t *testing.T) {
	viewer := operatorSession()
	viewer.User.Role = domain.RoleViewer
	transient := placementTestVM(domain.VMStateShutoff)
	transient.Persistent = false
	tests := []struct {
		name     string
		readOnly bool
		vm       domain.VM
		session  domain.Session
		spec     domain.VMPlacementSpec
		want     error
	}{
		{name: "viewer", vm: placementTestVM(domain.VMStateShutoff), session: viewer, want: domain.ErrForbidden},
		{name: "read only", readOnly: true, vm: placementTestVM(domain.VMStateShutoff), session: operatorSession(), want: domain.ErrReadOnly},
		{name: "transient", vm: transient, session: operatorSession(), want: domain.ErrActionNotAllowed},
		{name: "running", vm: placementTestVM(domain.VMStateRunning), session: operatorSession(), want: domain.ErrActionNotAllowed},
		{name: "bad hugepage", vm: placementTestVM(domain.VMStateShutoff), session: operatorSession(), spec: domain.VMPlacementSpec{HugePageSize: "3MiB"}, want: domain.ErrInvalidInput},
		{name: "bad numa", vm: placementTestVM(domain.VMStateShutoff), session: operatorSession(), spec: domain.VMPlacementSpec{NUMACells: []domain.VMNUMACell{{ID: 0, Cpuset: "0-1", MemoryMiB: 1024}}}, want: domain.ErrInvalidInput},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &placementTestRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{test.vm}}}
			audit := &fakeAuditRecorder{}
			_, err := New(repository, audit, test.readOnly).UpdatePlacement(context.Background(), UpdatePlacementCommand{
				UUID: test.vm.UUID, Spec: test.spec, Request: MutationContext{Session: test.session},
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if repository.calls != 0 || len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("repository calls = %d audit = %+v", repository.calls, audit.events)
			}
		})
	}
}
