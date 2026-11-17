package libvirtbackend

import (
	"context"
	"strings"

	"virtmgr/internal/domain"
)

func (d *DemoBackend) UpdateVMDiskQoS(_ context.Context, uuid, target string, spec domain.VMDiskQoSSpec) (domain.VM, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	for index := range vm.Disks {
		if vm.Disks[index].Target != target || vm.Disks[index].Device != "disk" || vm.Disks[index].ReadOnly {
			continue
		}
		qos := domain.DiskQoS(spec)
		if spec == (domain.VMDiskQoSSpec{}) {
			vm.Disks[index].QoS = nil
		} else {
			vm.Disks[index].QoS = &qos
		}
		d.vms[uuid] = vm
		return cloneVM(vm), nil
	}
	return domain.VM{}, domain.ErrNotFound
}

func (d *DemoBackend) UpdateVMNetworkQoS(_ context.Context, uuid, mac string, spec domain.VMNetworkQoSSpec) (domain.VM, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	for index := range vm.Interfaces {
		if !strings.EqualFold(vm.Interfaces[index].MAC, mac) {
			continue
		}
		qos := domain.NetworkQoS(spec)
		if spec == (domain.VMNetworkQoSSpec{}) {
			vm.Interfaces[index].QoS = nil
		} else {
			vm.Interfaces[index].QoS = &qos
		}
		d.vms[uuid] = vm
		return cloneVM(vm), nil
	}
	return domain.VM{}, domain.ErrNotFound
}
