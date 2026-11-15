package libvirtbackend

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net"
	"sort"
	"strings"
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
		info.DiagnosticCode = "channel_missing"
		info.DiagnosticHint = "关机后可为持久域补充 QGA 通道；新建虚拟机默认会挂载该通道。"
		return info, nil
	}
	if vm.State != domain.VMStateRunning || !vm.AgentConnected {
		info.State = domain.GuestAgentStateDisconnected
		info.LastError = "虚拟机未运行，或客户机代理尚未启动"
		info.DiagnosticCode = "agent_not_running"
		info.DiagnosticHint = "启动虚拟机并确认 qemu-guest-agent 服务已运行。"
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
		FileAccess: true, PasswordReset: true, SSHKeys: true, Processes: true, Users: true,
		Timezone: true, NetworkRoutes: true,
	}
	info.Timezone = "Asia/Shanghai"
	info.Routes = []domain.GuestAgentRoute{
		{Destination: "0.0.0.0/0", Gateway: "192.168.122.1", Interface: "eth0", Family: "ipv4", Metric: 100},
		{Destination: "192.168.122.0/24", Interface: "eth0", Family: "ipv4", Metric: 100},
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
	vm.AgentConfigured = true
	vm.AgentState = domain.GuestAgentStateDisconnected
	d.vms[uuid] = vm
	d.publish("domain.updated", "info", "vm", uuid, vm.Name, "已为虚拟机补充 QEMU Guest Agent 通道")
	return cloneVM(vm), nil
}

func minUint64(left, right uint64) uint64 {
	if left < right {
		return left
	}
	return right
}

func (d *DemoBackend) ReadGuestAgentFile(ctx context.Context, uuid string, spec domain.GuestAgentFileReadSpec) (domain.GuestAgentFileContent, error) {
	if _, err := d.demoGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool { return capabilities.FileAccess }); err != nil {
		return domain.GuestAgentFileContent{}, err
	}
	spec.Path = strings.TrimSpace(spec.Path)
	if spec.Path == "" || !strings.HasPrefix(spec.Path, "/") || spec.MaxBytes > 1<<20 {
		return domain.GuestAgentFileContent{}, domain.ErrInvalidInput
	}
	if spec.MaxBytes == 0 {
		spec.MaxBytes = 1 << 20
	}
	d.mu.RLock()
	content, ok := d.guestFiles[spec.Path]
	d.mu.RUnlock()
	if !ok {
		return domain.GuestAgentFileContent{}, domain.ErrNotFound
	}
	if spec.Offset > uint64(len(content)) {
		spec.Offset = uint64(len(content))
	}
	content = content[spec.Offset:]
	eof := true
	if uint64(len(content)) > spec.MaxBytes {
		content = content[:spec.MaxBytes]
		eof = false
	}
	content = append([]byte(nil), content...)
	sum := sha256.Sum256(content)
	return domain.GuestAgentFileContent{
		Path: spec.Path, DataBase64: base64.StdEncoding.EncodeToString(content), Offset: spec.Offset,
		Size: uint64(len(content)), EOF: eof, SHA256: hex.EncodeToString(sum[:]),
	}, nil
}

func (d *DemoBackend) WriteGuestAgentFile(ctx context.Context, uuid string, spec domain.GuestAgentFileWriteSpec) (domain.GuestAgentFileWriteResult, error) {
	if _, err := d.demoGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool { return capabilities.FileAccess }); err != nil {
		return domain.GuestAgentFileWriteResult{}, err
	}
	spec.Path = strings.TrimSpace(spec.Path)
	spec.Mode = strings.ToLower(strings.TrimSpace(spec.Mode))
	if spec.Mode == "" {
		spec.Mode = "replace"
	}
	if spec.Path == "" || !strings.HasPrefix(spec.Path, "/") || (spec.Mode != "replace" && spec.Mode != "append") || (spec.Mode == "replace" && !spec.Overwrite) {
		return domain.GuestAgentFileWriteResult{}, domain.ErrInvalidInput
	}
	data, err := base64.StdEncoding.DecodeString(spec.DataBase64)
	if err != nil || len(data) > 1<<20 {
		return domain.GuestAgentFileWriteResult{}, domain.ErrInvalidInput
	}
	d.mu.Lock()
	if spec.Mode == "append" {
		d.guestFiles[spec.Path] = append(d.guestFiles[spec.Path], data...)
	} else {
		d.guestFiles[spec.Path] = append([]byte(nil), data...)
	}
	d.mu.Unlock()
	sum := sha256.Sum256(data)
	return domain.GuestAgentFileWriteResult{Path: spec.Path, BytesWritten: uint64(len(data)), SHA256: hex.EncodeToString(sum[:])}, nil
}

func (d *DemoBackend) ResetGuestAgentPassword(ctx context.Context, uuid string, spec domain.GuestAgentPasswordSpec) error {
	if !domain.ValidGuestAgentUsername(spec.Username) || len(spec.Password) == 0 || len(spec.Password) > 256 {
		return domain.ErrInvalidInput
	}
	_, err := d.demoGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool { return capabilities.PasswordReset })
	return err
}

func (d *DemoBackend) ListGuestAgentAuthorizedKeys(ctx context.Context, uuid, username string) (domain.GuestAgentAuthorizedKeys, error) {
	if !domain.ValidGuestAgentUsername(username) {
		return domain.GuestAgentAuthorizedKeys{}, domain.ErrInvalidInput
	}
	if _, err := d.demoGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool { return capabilities.SSHKeys }); err != nil {
		return domain.GuestAgentAuthorizedKeys{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return domain.GuestAgentAuthorizedKeys{Username: username, Keys: append([]string(nil), d.guestSSHKeys[username]...)}, nil
}

func (d *DemoBackend) AddGuestAgentAuthorizedKeys(ctx context.Context, uuid string, spec domain.GuestAgentSSHKeySpec) error {
	if _, err := d.demoGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool { return capabilities.SSHKeys }); err != nil {
		return err
	}
	if !domain.ValidGuestAgentUsername(spec.Username) || len(spec.Keys) == 0 || len(spec.Keys) > 16 {
		return domain.ErrInvalidInput
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	existing := make(map[string]struct{}, len(d.guestSSHKeys[spec.Username]))
	for _, key := range d.guestSSHKeys[spec.Username] {
		existing[strings.TrimSpace(key)] = struct{}{}
	}
	for _, key := range spec.Keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := existing[key]; ok {
			continue
		}
		d.guestSSHKeys[spec.Username] = append(d.guestSSHKeys[spec.Username], key)
		existing[key] = struct{}{}
	}
	return nil
}

func (d *DemoBackend) RemoveGuestAgentAuthorizedKeys(ctx context.Context, uuid string, spec domain.GuestAgentSSHKeySpec) error {
	if _, err := d.demoGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool { return capabilities.SSHKeys }); err != nil {
		return err
	}
	if !domain.ValidGuestAgentUsername(spec.Username) || len(spec.Keys) == 0 || len(spec.Keys) > 16 {
		return domain.ErrInvalidInput
	}
	remove := make(map[string]struct{}, len(spec.Keys))
	for _, key := range spec.Keys {
		remove[strings.TrimSpace(key)] = struct{}{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	keys := d.guestSSHKeys[spec.Username]
	kept := keys[:0]
	for _, key := range keys {
		if _, ok := remove[strings.TrimSpace(key)]; !ok {
			kept = append(kept, key)
		}
	}
	d.guestSSHKeys[spec.Username] = append([]string(nil), kept...)
	return nil
}

func (d *DemoBackend) ListGuestAgentProcesses(ctx context.Context, uuid string) ([]domain.GuestAgentProcess, error) {
	if _, err := d.demoGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool { return capabilities.Processes }); err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]domain.GuestAgentProcess(nil), d.guestProcesses...), nil
}

func (d *DemoBackend) SignalGuestAgentProcess(ctx context.Context, uuid string, spec domain.GuestAgentProcessSignalSpec) error {
	if _, err := d.demoGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool { return capabilities.Processes }); err != nil {
		return err
	}
	if _, ok := domain.ValidGuestAgentProcessSignal(spec.Signal); !ok || spec.PID <= 1 {
		return domain.ErrInvalidInput
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for index, process := range d.guestProcesses {
		if process.PID == spec.PID {
			d.guestProcesses = append(d.guestProcesses[:index], d.guestProcesses[index+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (d *DemoBackend) ListGuestAgentUsers(ctx context.Context, uuid string) ([]domain.GuestAgentUser, error) {
	if _, err := d.demoGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool { return capabilities.Users }); err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	users := append([]domain.GuestAgentUser(nil), d.guestUsers...)
	sort.Slice(users, func(i, j int) bool { return users[i].LoginTime > users[j].LoginTime })
	return users, nil
}

func (d *DemoBackend) demoGuestAgentCapability(ctx context.Context, uuid string, allowed func(domain.GuestAgentCapabilities) bool) (domain.GuestAgentInfo, error) {
	info, err := d.GetGuestAgentInfo(ctx, uuid, true)
	if err != nil {
		return domain.GuestAgentInfo{}, err
	}
	if info.State != domain.GuestAgentStateConnected || !allowed(info.Capabilities) {
		return info, domain.ErrGuestAgent
	}
	return info, nil
}
