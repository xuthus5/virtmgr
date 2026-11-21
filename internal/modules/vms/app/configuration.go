package app

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"virtmgr/internal/domain"
)

type VMQoSRepository interface {
	UpdateVMDiskQoS(ctx context.Context, uuid, target string, spec domain.VMDiskQoSSpec) (domain.VM, error)
	UpdateVMNetworkQoS(ctx context.Context, uuid, mac string, spec domain.VMNetworkQoSSpec) (domain.VM, error)
}

type BootCommand struct {
	UUID    string
	Spec    domain.VMBootSpec
	Request MutationContext
}

type CPUCommand struct {
	UUID    string
	Spec    domain.VMCPUConfigSpec
	Request MutationContext
}

type DiskQoSCommand struct {
	UUID    string
	Target  string
	Spec    domain.VMDiskQoSSpec
	Request MutationContext
}

type NetworkQoSCommand struct {
	UUID    string
	MAC     string
	Spec    domain.VMNetworkQoSSpec
	Request MutationContext
}

type NetworkModelCommand struct {
	UUID    string
	MAC     string
	Spec    domain.VMNetworkModelSpec
	Request MutationContext
}

var cpuModelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.+-]{0,63}$`)

const (
	maxQoSBytesPerSecond      = uint64(1 << 40)
	maxQoSOperationsPerSecond = uint64(10_000_000)
)

func (s *Service) UpdateBootOrder(ctx context.Context, command BootCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	devices, ok := domain.NormalizeVMBootDevices(toStringSlice(command.Spec.Devices))
	if command.UUID == "" || !ok {
		return domain.VM{}, domain.ErrInvalidInput
	}
	command.Spec.Devices = devices
	detail := map[string]any{"devices": devices}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.boot.update", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.boot.update", command.UUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.boot.update", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["previousDevices"] = vm.BootDevices
	if !vm.Persistent {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.boot.update", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	available := map[domain.VMBootDevice]bool{
		domain.VMBootDeviceDisk:    hasBootableDisk(vm),
		domain.VMBootDeviceCDROM:   hasCDROM(vm),
		domain.VMBootDeviceNetwork: len(vm.Interfaces) > 0,
	}
	for _, device := range devices {
		if !available[device] {
			return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.boot.update", command.UUID, detail,
				fmt.Errorf("%w: boot device %s is not configured on this VM", domain.ErrInvalidInput, device))
		}
	}
	config, err := s.repository.GetVMXML(ctx, command.UUID, domain.VMXMLFormatConfig)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.boot.update", command.UUID, detail, err)
	}
	updatedXML, err := domain.SetVMXMLBootDevices(config.XML, devices)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.boot.update", command.UUID, detail, err)
	}
	if _, err := s.repository.DefineVMXML(ctx, command.UUID, updatedXML); err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.boot.update", command.UUID, detail, err)
	}
	updated, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.boot.update", command.UUID, detail, err)
	}
	updated.BootDevices = devices
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceManualXML, 0, "修改启动顺序后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.boot.update", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM boot order audit: %w", err)
	}
	return updated, nil
}

func (s *Service) UpdateCPUConfig(ctx context.Context, command CPUCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Spec.Model = strings.TrimSpace(command.Spec.Model)
	detail := map[string]any{
		"model": command.Spec.Model, "sockets": command.Spec.Sockets,
		"cores": command.Spec.Cores, "threads": command.Spec.Threads,
	}
	if command.UUID == "" || !cpuModelPattern.MatchString(command.Spec.Model) || command.Spec.Sockets == 0 || command.Spec.Cores == 0 || command.Spec.Threads == 0 {
		return domain.VM{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.cpu.update", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.cpu.update", command.UUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.cpu.update", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	detail["previousModel"] = vm.CPUModel
	detail["previousSockets"] = vm.CPUSockets
	detail["previousCores"] = vm.CPUCores
	detail["previousThreads"] = vm.CPUThreads
	if !vm.Persistent {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.cpu.update", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	if vm.State != domain.VMStateShutoff && vm.State != domain.VMStateCrashed {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.cpu.update", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	total := uint64(command.Spec.Sockets) * uint64(command.Spec.Cores) * uint64(command.Spec.Threads)
	if total != uint64(vm.VCPUs) || total > uint64(vm.MaxVCPUs) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.cpu.update", command.UUID, detail,
			fmt.Errorf("%w: CPU topology must equal the configured %d vCPUs", domain.ErrInvalidInput, vm.VCPUs))
	}
	config, err := s.repository.GetVMXML(ctx, command.UUID, domain.VMXMLFormatConfig)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.cpu.update", command.UUID, detail, err)
	}
	updatedXML, err := domain.SetVMXMLCPUConfig(config.XML, command.Spec)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.cpu.update", command.UUID, detail, err)
	}
	if _, err := s.repository.DefineVMXML(ctx, command.UUID, updatedXML); err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.cpu.update", command.UUID, detail, err)
	}
	updated, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.cpu.update", command.UUID, detail, err)
	}
	updated.CPUModel = command.Spec.Model
	updated.CPUSockets = command.Spec.Sockets
	updated.CPUCores = command.Spec.Cores
	updated.CPUThreads = command.Spec.Threads
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceManualXML, 0, "修改 CPU 型号与拓扑后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.cpu.update", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM CPU config audit: %w", err)
	}
	return updated, nil
}

func (s *Service) UpdateDiskQoS(ctx context.Context, command DiskQoSCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Target = strings.TrimSpace(command.Target)
	detail := diskQoSDetail(command.Spec)
	if command.UUID == "" || command.Target == "" || !validDiskQoS(command.Spec) {
		return domain.VM{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.qos.update", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.qos.update", command.UUID, detail, domain.ErrReadOnly)
	}
	repository, ok := s.repository.(VMQoSRepository)
	if !ok {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.qos.update", command.UUID, detail, domain.ErrUnavailable)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.qos.update", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	var disk *domain.Disk
	for index := range vm.Disks {
		if vm.Disks[index].Target == command.Target {
			disk = &vm.Disks[index]
			break
		}
	}
	if disk == nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.qos.update", command.UUID, detail, domain.ErrNotFound)
	}
	if disk.Device != "disk" || disk.ReadOnly {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.qos.update", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	detail["previous"] = disk.QoS
	updated, err := repository.UpdateVMDiskQoS(ctx, command.UUID, command.Target, command.Spec)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.qos.update", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceManualXML, 0, "修改磁盘 I/O 限制后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.disk.qos.update", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM disk QoS audit: %w", err)
	}
	return updated, nil
}

func (s *Service) UpdateNetworkQoS(ctx context.Context, command NetworkQoSCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.MAC = strings.ToLower(strings.TrimSpace(command.MAC))
	detail := networkQoSDetail(command.Spec)
	if command.UUID == "" || command.MAC == "" || !validNetworkQoS(command.Spec) {
		return domain.VM{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.qos.update", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.qos.update", command.UUID, detail, domain.ErrReadOnly)
	}
	repository, ok := s.repository.(VMQoSRepository)
	if !ok {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.qos.update", command.UUID, detail, domain.ErrUnavailable)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.qos.update", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	var networkInterface *domain.NetworkInterface
	for index := range vm.Interfaces {
		if strings.EqualFold(vm.Interfaces[index].MAC, command.MAC) {
			networkInterface = &vm.Interfaces[index]
			break
		}
	}
	if networkInterface == nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.qos.update", command.UUID, detail, domain.ErrNotFound)
	}
	detail["previous"] = networkInterface.QoS
	updated, err := repository.UpdateVMNetworkQoS(ctx, command.UUID, networkInterface.MAC, command.Spec)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.qos.update", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceManualXML, 0, "修改网卡带宽限制后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.network.qos.update", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM network QoS audit: %w", err)
	}
	return updated, nil
}

func (s *Service) UpdateNetworkModel(ctx context.Context, command NetworkModelCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.MAC = strings.ToLower(strings.TrimSpace(command.MAC))
	model, ok := domain.NormalizeVMNICModel(string(command.Spec.Model))
	detail := map[string]any{"mac": command.MAC, "model": model}
	if command.UUID == "" || command.MAC == "" || !ok || command.Spec.Model == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	command.Spec.Model = model
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.model.update", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.model.update", command.UUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.model.update", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	if !vm.Persistent || (vm.State != domain.VMStateShutoff && vm.State != domain.VMStateCrashed) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.model.update", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	var networkInterface *domain.NetworkInterface
	for index := range vm.Interfaces {
		if strings.EqualFold(vm.Interfaces[index].MAC, command.MAC) {
			networkInterface = &vm.Interfaces[index]
			break
		}
	}
	if networkInterface == nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.model.update", command.UUID, detail, domain.ErrNotFound)
	}
	detail["previousModel"] = networkInterface.Model
	config, err := s.repository.GetVMXML(ctx, command.UUID, domain.VMXMLFormatConfig)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.model.update", command.UUID, detail, err)
	}
	updatedXML, err := domain.SetVMNetworkModelXML(config.XML, command.MAC, command.Spec.Model)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.model.update", command.UUID, detail, err)
	}
	if _, err := s.repository.DefineVMXML(ctx, command.UUID, updatedXML); err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.model.update", command.UUID, detail, err)
	}
	updated, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.model.update", command.UUID, detail, err)
	}
	if networkInterface := findNetworkInterfaceByMAC(updated.Interfaces, command.MAC); networkInterface != nil {
		networkInterface.Model = string(command.Spec.Model)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceManualXML, 0, "修改网卡型号后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.network.model.update", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM network model audit: %w", err)
	}
	return updated, nil
}

func findNetworkInterfaceByMAC(interfaces []domain.NetworkInterface, mac string) *domain.NetworkInterface {
	for index := range interfaces {
		if strings.EqualFold(interfaces[index].MAC, mac) {
			return &interfaces[index]
		}
	}
	return nil
}

func toStringSlice(devices []domain.VMBootDevice) []string {
	result := make([]string, 0, len(devices))
	for _, device := range devices {
		result = append(result, string(device))
	}
	return result
}

func hasBootableDisk(vm domain.VM) bool {
	for _, disk := range vm.Disks {
		if disk.Device == "disk" && !disk.ReadOnly {
			return true
		}
	}
	return false
}

func hasCDROM(vm domain.VM) bool {
	for _, disk := range vm.Disks {
		if disk.Device == "cdrom" {
			return true
		}
	}
	return false
}

func validDiskQoS(spec domain.VMDiskQoSSpec) bool {
	values := []uint64{spec.TotalBytesSec, spec.ReadBytesSec, spec.WriteBytesSec}
	for _, value := range values {
		if value > maxQoSBytesPerSecond {
			return false
		}
	}
	for _, value := range []uint64{spec.TotalIOPS, spec.ReadIOPS, spec.WriteIOPS} {
		if value > maxQoSOperationsPerSecond {
			return false
		}
	}
	return true
}

func validNetworkQoS(spec domain.VMNetworkQoSSpec) bool {
	for _, value := range []uint64{
		spec.InboundAverageBytes, spec.InboundPeakBytes, spec.InboundBurstBytes,
		spec.OutboundAverageBytes, spec.OutboundPeakBytes, spec.OutboundBurstBytes,
	} {
		if value > maxQoSBytesPerSecond {
			return false
		}
	}
	if (spec.InboundPeakBytes > 0 || spec.InboundBurstBytes > 0) && spec.InboundAverageBytes == 0 {
		return false
	}
	if (spec.OutboundPeakBytes > 0 || spec.OutboundBurstBytes > 0) && spec.OutboundAverageBytes == 0 {
		return false
	}
	if spec.InboundPeakBytes > 0 && spec.InboundPeakBytes < spec.InboundAverageBytes {
		return false
	}
	if spec.OutboundPeakBytes > 0 && spec.OutboundPeakBytes < spec.OutboundAverageBytes {
		return false
	}
	return true
}

func diskQoSDetail(spec domain.VMDiskQoSSpec) map[string]any {
	return map[string]any{
		"totalBytesSec": spec.TotalBytesSec, "readBytesSec": spec.ReadBytesSec, "writeBytesSec": spec.WriteBytesSec,
		"totalIops": spec.TotalIOPS, "readIops": spec.ReadIOPS, "writeIops": spec.WriteIOPS,
	}
}

func networkQoSDetail(spec domain.VMNetworkQoSSpec) map[string]any {
	return map[string]any{
		"inboundAverageBytes": spec.InboundAverageBytes, "inboundPeakBytes": spec.InboundPeakBytes,
		"inboundBurstBytes": spec.InboundBurstBytes, "outboundAverageBytes": spec.OutboundAverageBytes,
		"outboundPeakBytes": spec.OutboundPeakBytes, "outboundBurstBytes": spec.OutboundBurstBytes,
	}
}

func (s *Service) UpdateResourceQuota(ctx context.Context, command UpdateResourceQuotaCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	detail := quotaDetail(command.Quota)
	if command.UUID == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.quota.update", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.quota.update", command.UUID, detail, domain.ErrReadOnly)
	}
	repository, ok := s.repository.(VMResourceQuotaRepository)
	if !ok {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.quota.update", command.UUID, detail, domain.ErrUnavailable)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.quota.update", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	if !vm.Persistent || vm.State == domain.VMStateShutdown || vm.State == domain.VMStateUnknown {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.quota.update", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	normalized, err := domain.NormalizeVMResourceQuota(command.Quota, vm.MemoryBytes/(1024*1024))
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.quota.update", command.UUID, detail, err)
	}
	detail = quotaDetail(normalized)
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	detail["previous"] = vm.ResourceQuota
	updated, err := repository.UpdateVMResourceQuota(ctx, command.UUID, normalized)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.quota.update", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceQuota, 0, "更新 CPU 与内存配额后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.quota.update", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM resource quota audit: %w", err)
	}
	return updated, nil
}

func (s *Service) UpdateHardware(ctx context.Context, command UpdateHardwareCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	detail := hardwareDetail(command.Hardware)
	if command.UUID == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.hardware.update", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.hardware.update", command.UUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.hardware.update", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	if !vm.Persistent || (vm.State != domain.VMStateShutoff && vm.State != domain.VMStateCrashed) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.hardware.update", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	normalized, err := domain.NormalizeVMHardware(command.Hardware)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.hardware.update", command.UUID, detail, err)
	}
	detail = hardwareDetail(normalized)
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	detail["previousRNG"] = vm.RNG
	detail["previousWatchdog"] = vm.Watchdog
	config, err := s.repository.GetVMXML(ctx, command.UUID, domain.VMXMLFormatConfig)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.hardware.update", command.UUID, detail, err)
	}
	updatedXML, err := domain.SetVMXMLHardware(config.XML, normalized)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.hardware.update", command.UUID, detail, err)
	}
	if _, err := s.repository.DefineVMXML(ctx, command.UUID, updatedXML); err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.hardware.update", command.UUID, detail, err)
	}
	updated, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.hardware.update", command.UUID, detail, err)
	}
	updated.RNG = normalized.RNG
	updated.Watchdog = normalized.Watchdog
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceHardware, 0, "更新 RNG 与 watchdog 后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.hardware.update", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM hardware audit: %w", err)
	}
	return updated, nil
}

func quotaDetail(quota domain.VMResourceQuota) map[string]any {
	return map[string]any{
		"cpuShares": quota.CPUShares, "cpuQuota": quota.CPUQuota, "cpuPeriod": quota.CPUPeriod,
		"memoryHardLimitMiB": quota.MemoryHardLimitMiB, "memorySoftLimitMiB": quota.MemorySoftLimitMiB,
	}
}

func hardwareDetail(hardware domain.VMHardware) map[string]any {
	return map[string]any{"rng": hardware.RNG, "watchdog": hardware.Watchdog}
}
