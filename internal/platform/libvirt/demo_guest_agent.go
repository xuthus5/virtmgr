package libvirtbackend

import (
	"context"
	"net"
	"time"

	"virtmgr/internal/domain"
)

func (d *DemoBackend) GetGuestAgentInfo(_ context.Context, uuid string, _ bool) (domain.GuestAgentInfo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.GuestAgentInfo{}, domain.ErrNotFound
	}
	xmlText := d.xmlConfig[uuid]
	if xmlText == "" {
		xmlText = demoVMDomainXML(vm)
	}
	info := domain.GuestAgentInfo{
		State:             domain.GuestAgentStateUnknown,
		ChannelConfigured: hasGuestAgentChannelXML(xmlText),
		Interfaces:        []domain.GuestAgentNetworkInterface{},
		Filesystems:       []domain.GuestAgentFilesystem{},
		ProbedAt:          time.Now().UTC(),
	}
	if !info.ChannelConfigured {
		info.State = domain.GuestAgentStateNotConfigured
		info.LastError = "虚拟机配置中没有 org.qemu.guest_agent.0 通道"
		return info, nil
	}
	if vm.State != domain.VMStateRunning || !vm.AgentConnected {
		info.State = domain.GuestAgentStateDisconnected
		info.LastError = "虚拟机未运行，或客户机代理尚未启动"
		return info, nil
	}
	now := time.Now().UTC()
	info.State = domain.GuestAgentStateConnected
	info.Connected = true
	info.Version = "8.2.2-demo"
	info.Hostname = vm.Hostname
	info.LastSeenAt = &now
	info.OS = domain.GuestAgentOSInfo{
		ID: "ubuntu", Name: "Ubuntu", PrettyName: "Ubuntu 24.04.1 LTS", Version: "24.04",
		KernelRelease: "6.8.0-51-generic", Arch: "x86_64", Hostname: vm.Hostname,
	}
	info.Capabilities = domain.GuestAgentCapabilities{
		Hostname: true, Network: true, OSInfo: true, FSInfo: true,
		Shutdown: true, Reboot: true, TimeSync: true, Trim: true, Quiesce: true,
	}
	for _, networkInterface := range vm.Interfaces {
		item := domain.GuestAgentNetworkInterface{Name: "eth0", MAC: networkInterface.MAC, Addresses: []domain.GuestAgentIPAddress{}}
		for _, address := range vm.IPAddresses {
			ip := net.ParseIP(address)
			if ip == nil {
				continue
			}
			family := "ipv6"
			prefix := 64
			if ip.To4() != nil {
				family = "ipv4"
				prefix = 24
			}
			item.Addresses = append(item.Addresses, domain.GuestAgentIPAddress{Address: address, Family: family, Prefix: prefix})
		}
		info.Interfaces = append(info.Interfaces, item)
	}
	for _, disk := range vm.Disks {
		if disk.Device != "disk" || disk.ReadOnly {
			continue
		}
		info.Filesystems = append(info.Filesystems, domain.GuestAgentFilesystem{
			Device: disk.Source, MountPoint: "/", Type: "ext4",
			TotalBytes: disk.CapacityBytes, UsedBytes: disk.AllocationBytes,
			FreeBytes: disk.CapacityBytes - minUint64(disk.AllocationBytes, disk.CapacityBytes),
		})
	}
	return info, nil
}

func (d *DemoBackend) PerformGuestAgentAction(_ context.Context, uuid string, spec domain.GuestAgentActionSpec) error {
	info, err := d.GetGuestAgentInfo(context.Background(), uuid, true)
	if err != nil {
		return err
	}
	if info.State != domain.GuestAgentStateConnected || !spec.SupportedBy(info.Capabilities) {
		return domain.ErrGuestAgent
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.ErrNotFound
	}
	switch spec.Action {
	case domain.GuestAgentActionShutdown:
		vm.State = domain.VMStateShutoff
		vm.StateReason = 0
		vm.AgentConnected = false
		d.publish("domain.shutdown", "info", "vm", uuid, vm.Name, "客户机代理已完成关机")
	case domain.GuestAgentActionReboot:
		vm.State = domain.VMStateRunning
		vm.StateReason = 0
		d.publish("domain.started", "info", "vm", uuid, vm.Name, "客户机代理已完成重启")
	case domain.GuestAgentActionSyncTime:
		d.publish("guest-agent.time-synced", "info", "vm", uuid, vm.Name, "客户机时间已与宿主机同步")
	case domain.GuestAgentActionTrim:
		d.publish("guest-agent.trimmed", "info", "vm", uuid, vm.Name, "客户机已执行 TRIM")
	default:
		return domain.ErrInvalidInput
	}
	vm.AllowedActions = domain.AllowedVMActions(vm.State)
	d.vms[uuid] = vm
	return nil
}

func (d *DemoBackend) EnsureGuestAgentChannel(_ context.Context, uuid string) (domain.VM, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	if vm.State != domain.VMStateShutoff && vm.State != domain.VMStateCrashed {
		return domain.VM{}, domain.ErrActionNotAllowed
	}
	xmlText := d.xmlConfig[uuid]
	if xmlText == "" {
		xmlText = demoVMDomainXML(vm)
	}
	updated, err := addGuestAgentChannelXML(xmlText)
	if err != nil {
		return domain.VM{}, err
	}
	d.xmlConfig[uuid] = updated
	d.publish("domain.updated", "info", "vm", uuid, vm.Name, "已为虚拟机补充 QEMU Guest Agent 通道")
	return cloneVM(vm), nil
}

func minUint64(left, right uint64) uint64 {
	if left < right {
		return left
	}
	return right
}
