package app

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
)

type diskResizeRepository struct {
	fakeVMRepository
	uuid   string
	target string
	spec   domain.VMDiskResizeSpec
	calls  int
	err    error
}

func (r *diskResizeRepository) ResizeVMDisk(_ context.Context, uuid, target string, spec domain.VMDiskResizeSpec) (domain.VM, error) {
	r.calls++
	r.uuid = uuid
	r.target = target
	r.spec = spec
	if r.err != nil {
		return domain.VM{}, r.err
	}
	for _, vm := range r.vms {
		if vm.UUID != uuid {
			continue
		}
		for index := range vm.Disks {
			if vm.Disks[index].Target == target {
				vm.Disks[index].CapacityBytes = spec.CapacityBytes
				return vm, nil
			}
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func diskResizeVM() domain.VM {
	return domain.VM{
		UUID: "uuid-a", Name: "api-01", State: domain.VMStateRunning, Persistent: true,
		Disks: []domain.Disk{{
			Device: "disk", Target: "vda", Bus: "virtio", Format: "qcow2",
			Source: "/var/lib/libvirt/images/api-01.qcow2", CapacityBytes: 20 << 30, AllocationBytes: 5 << 30,
		}},
	}
}

func TestResizeDiskUpdatesCapacityAndRecordsAudit(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &diskResizeRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{diskResizeVM()}}}
	service := New(repository, audit, false)

	updated, err := service.ResizeDisk(context.Background(), ResizeDiskCommand{
		UUID: " uuid-a ", Target: " vda ", Spec: domain.VMDiskResizeSpec{CapacityBytes: 40 << 30},
		Request: MutationContext{Session: operatorSession(), RequestID: "req-disk", SourceIP: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("ResizeDisk() error = %v", err)
	}
	if updated.Disks[0].CapacityBytes != 40<<30 {
		t.Fatalf("updated disk = %+v", updated.Disks[0])
	}
	if repository.calls != 1 || repository.uuid != "uuid-a" || repository.target != "vda" || repository.spec.CapacityBytes != 40<<30 {
		t.Fatalf("repository call = %+v", repository)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	event := audit.events[0]
	if event.Action != "vm.disk.resize" || event.TargetKind != "vm" || event.TargetID != "uuid-a" || event.Result != "success" {
		t.Fatalf("audit event = %+v", event)
	}
	if event.Detail["target"] != "vda" || event.Detail["previousCapacityBytes"] != uint64(20<<30) || event.Detail["capacityBytes"] != uint64(40<<30) {
		t.Fatalf("audit detail = %+v", event.Detail)
	}
}

func TestResizeDiskRejectsInvalidMutations(t *testing.T) {
	tests := []struct {
		name      string
		session   domain.Session
		readOnly  bool
		mutateVM  func(*domain.VM)
		target    string
		capacity  uint64
		wantError error
	}{
		{name: "viewer", session: viewerSession(), target: "vda", capacity: 40 << 30, wantError: domain.ErrForbidden},
		{name: "read only", session: operatorSession(), readOnly: true, target: "vda", capacity: 40 << 30, wantError: domain.ErrReadOnly},
		{name: "shrink", session: operatorSession(), target: "vda", capacity: 19 << 30, wantError: domain.ErrInvalidInput},
		{name: "too small", session: operatorSession(), target: "vda", capacity: 1, wantError: domain.ErrInvalidInput},
		{name: "missing disk", session: operatorSession(), target: "vdb", capacity: 40 << 30, wantError: domain.ErrNotFound},
		{name: "temporary domain", session: operatorSession(), mutateVM: func(vm *domain.VM) { vm.Persistent = false }, target: "vda", capacity: 40 << 30, wantError: domain.ErrActionNotAllowed},
		{name: "cdrom", session: operatorSession(), mutateVM: func(vm *domain.VM) { vm.Disks[0].Device = "cdrom"; vm.Disks[0].ReadOnly = true }, target: "vda", capacity: 40 << 30, wantError: domain.ErrActionNotAllowed},
		{name: "unmanaged", session: operatorSession(), mutateVM: func(vm *domain.VM) { vm.Disks[0].CapacityBytes = 0 }, target: "vda", capacity: 40 << 30, wantError: domain.ErrDiskResize},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			vm := diskResizeVM()
			if test.mutateVM != nil {
				test.mutateVM(&vm)
			}
			repository := &diskResizeRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{vm}}}
			audit := &fakeAuditRecorder{}
			_, err := New(repository, audit, test.readOnly).ResizeDisk(context.Background(), ResizeDiskCommand{
				UUID: "uuid-a", Target: test.target, Spec: domain.VMDiskResizeSpec{CapacityBytes: test.capacity},
				Request: MutationContext{Session: test.session},
			})
			if !errors.Is(err, test.wantError) {
				t.Fatalf("error = %v, want %v", err, test.wantError)
			}
			if repository.calls != 0 {
				t.Fatalf("backend calls = %d", repository.calls)
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}

	repository := &diskResizeRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{diskResizeVM()}}, err: errors.New("resize failed")}
	_, err := New(repository, &fakeAuditRecorder{}, false).ResizeDisk(context.Background(), ResizeDiskCommand{
		UUID: "uuid-a", Target: "vda", Spec: domain.VMDiskResizeSpec{CapacityBytes: 40 << 30}, Request: MutationContext{Session: operatorSession()},
	})
	if err == nil || repository.calls != 1 {
		t.Fatalf("backend failure = %v calls=%d", err, repository.calls)
	}
}

func viewerSession() domain.Session {
	session := operatorSession()
	session.User.Role = domain.RoleViewer
	return session
}
