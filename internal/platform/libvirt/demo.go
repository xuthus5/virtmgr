package libvirtbackend

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"virtmgr/internal/domain"
	"virtmgr/internal/version"
)

type DemoBackend struct {
	mu          sync.RWMutex
	events      EventSink
	vms         map[string]domain.VM
	xmlConfig   map[string]string
	snapshots   map[string][]domain.VMSnapshot
	pools       map[string]domain.StoragePool
	volumes     map[string][]domain.StorageVolume
	volumeData  map[string][]byte
	networks    map[string]domain.VirtualNetwork
	leases      map[string][]domain.DHCPLease
	metricCalls map[string]uint64
}

func NewDemo(sinks ...EventSink) *DemoBackend {
	backend := &DemoBackend{
		vms:         make(map[string]domain.VM),
		xmlConfig:   make(map[string]string),
		snapshots:   make(map[string][]domain.VMSnapshot),
		pools:       demoStoragePools(),
		volumes:     demoStorageVolumes(),
		volumeData:  make(map[string][]byte),
		networks:    make(map[string]domain.VirtualNetwork),
		leases:      demoNetworkLeases(),
		metricCalls: make(map[string]uint64),
	}
	if len(sinks) > 0 {
		backend.events = sinks[0]
	}
	for _, vm := range demoVMs() {
		backend.vms[vm.UUID] = vm
	}
	for _, network := range demoNetworks() {
		backend.networks[network.Name] = network
	}
	backend.seedDemoEvents()
	return backend
}

func (d *DemoBackend) Ping(context.Context) error { return nil }
func (d *DemoBackend) Close() error               { return nil }

func (d *DemoBackend) Host(context.Context) (domain.Host, error) {
	return domain.Host{
		Name:           "virtmgr-demo",
		LibvirtVersion: "12.0.0-demo",
		CPUModel:       "x86_64",
		CPUs:           16,
		CPUSockets:     1,
		CPUCores:       8,
		CPUThreads:     2,
		CPUMHz:         3200,
		MemoryBytes:    64 * 1024 * 1024 * 1024,
	}, nil
}

func (d *DemoBackend) HostMetrics(context.Context) (domain.MetricsCounters, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.metricCalls["host"]++
	call := d.metricCalls["host"]
	return domain.MetricsCounters{
		CPUTimeNanos:     call * uint64(800*time.Millisecond),
		CPUCapacity:      16,
		MemoryUsedBytes:  28*1024*1024*1024 + call*1024*1024,
		MemoryTotalBytes: 64 * 1024 * 1024 * 1024,
		NetworkRxBytes:   10*1024*1024*1024 + call*4*1024*1024,
		NetworkTxBytes:   5*1024*1024*1024 + call*2*1024*1024,
	}, nil
}

func (d *DemoBackend) ListHostDevices(context.Context) ([]domain.HostDevice, error) {
	return demoHostDevices(), nil
}

func demoHostDevices() []domain.HostDevice {
	return []domain.HostDevice{
		{
			Name: "pci_0000_00_02_0", Type: "pci", Parent: "computer",
			Vendor: "Intel Corporation", VendorID: "0x8086", Product: "Ethernet Controller X710", ProductID: "0x1572",
			Address: "0000:00:02.0", Driver: "i40e", Active: true, Persistent: true,
			Capabilities: []string{"pci", "net"},
		},
		{
			Name: "pci_0000_3b_00_0", Type: "pci", Parent: "computer",
			Vendor: "NVIDIA Corporation", VendorID: "0x10de", Product: "GA102 [GeForce RTX 3090]", ProductID: "0x2204",
			Address: "0000:3b:00.0", Driver: "nvidia", Active: true, Persistent: true,
			Capabilities: []string{"pci", "drm"},
		},
		{
			Name: "pci_0000_00_14_0", Type: "pci", Parent: "computer",
			Vendor: "Intel Corporation", VendorID: "0x8086", Product: "Tiger Lake-H USB 3.2 Gen 2x1 xHCI Host Controller", ProductID: "0x0b00",
			Address: "0000:00:14.0", Driver: "xhci_hcd", Active: true, Persistent: true,
			Capabilities: []string{"pci", "usb"},
		},
		{
			Name: "usb_device_046d_c52b_1_2", Type: "usb_device", Parent: "pci_0000_00_14_0",
			Vendor: "Logitech, Inc.", VendorID: "0x046d", Product: "Unifying Receiver", ProductID: "0xc52b",
			Address: "bus 1 · device 2", Driver: "usbhid", Active: true, Persistent: true,
			Capabilities: []string{"usb_device"},
		},
		{
			Name: "net_enp2s0", Type: "net", Parent: "pci_0000_00_02_0", Address: "enp2s0",
			Active: true, Persistent: true, Capabilities: []string{"net"},
		},
		{
			Name: "block_sda", Type: "storage", Parent: "computer", Address: "/dev/sda",
			Active: true, Persistent: true, Capabilities: []string{"storage"},
		},
	}
}

func (d *DemoBackend) ListVMs(context.Context) ([]domain.VM, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]domain.VM, 0, len(d.vms))
	for _, vm := range d.vms {
		result = append(result, cloneVM(vm))
	}
	return result, nil
}

func (d *DemoBackend) GetVM(_ context.Context, uuid string) (domain.VM, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	return cloneVM(vm), nil
}

func (d *DemoBackend) VMMetrics(_ context.Context, uuid string) (domain.MetricsCounters, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.MetricsCounters{}, domain.ErrNotFound
	}
	key := "vm:" + uuid
	d.metricCalls[key]++
	call := d.metricCalls[key]
	counters := domain.MetricsCounters{
		CPUCapacity:      uint64(vm.VCPUs),
		MemoryTotalBytes: vm.MemoryBytes,
		MemoryUsedBytes:  vm.MemoryBytes * 3 / 5,
	}
	if vm.State == domain.VMStateRunning || vm.State == domain.VMStateBlocked {
		counters.CPUTimeNanos = call * uint64(400*time.Millisecond)
		counters.NetworkRxBytes = call * 2 * 1024 * 1024
		counters.NetworkTxBytes = call * 1024 * 1024
	}
	return counters, nil
}

func (d *DemoBackend) GetVMXML(_ context.Context, uuid string, format domain.VMXMLFormat) (domain.VMXML, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.VMXML{}, domain.ErrNotFound
	}
	if format == domain.VMXMLFormatConfig {
		if xmlText := d.xmlConfig[uuid]; xmlText != "" {
			return domain.VMXML{UUID: vm.UUID, Name: vm.Name, Format: format, XML: xmlText}, nil
		}
	}
	return domain.VMXML{UUID: vm.UUID, Name: vm.Name, Format: format, XML: demoVMDomainXML(vm)}, nil
}

func (d *DemoBackend) DefineVMXML(_ context.Context, uuid, xmlText string) (domain.VMXML, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.VMXML{}, domain.ErrNotFound
	}
	if !vm.Persistent {
		return domain.VMXML{}, domain.ErrActionNotAllowed
	}
	identity, err := domain.ParseVMXMLIdentity(xmlText)
	if err != nil {
		return domain.VMXML{}, err
	}
	if identity.UUID != vm.UUID || identity.Name != vm.Name {
		return domain.VMXML{}, fmt.Errorf("%w: domain name and UUID must match the existing VM", domain.ErrInvalidInput)
	}
	xmlText = strings.TrimSpace(xmlText)
	d.xmlConfig[uuid] = xmlText
	return domain.VMXML{UUID: vm.UUID, Name: vm.Name, Format: domain.VMXMLFormatConfig, XML: xmlText}, nil
}

func (d *DemoBackend) UpdateVMResources(_ context.Context, uuid string, spec domain.VMResourcesSpec) (domain.VM, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	if !vm.Persistent {
		return domain.VM{}, domain.ErrActionNotAllowed
	}
	if vm.MaxVCPUs == 0 {
		vm.MaxVCPUs = vm.VCPUs
	}
	if vm.MaxMemoryBytes == 0 {
		vm.MaxMemoryBytes = vm.MemoryBytes
	}
	targetMemoryBytes := spec.MemoryMiB * 1024 * 1024
	live := vm.State == domain.VMStateRunning || vm.State == domain.VMStateBlocked ||
		vm.State == domain.VMStatePaused || vm.State == domain.VMStatePMSuspended
	switch vm.State {
	case domain.VMStateShutdown, domain.VMStateUnknown:
		return domain.VM{}, domain.ErrActionNotAllowed
	}
	if live {
		if spec.VCPUs > vm.MaxVCPUs || targetMemoryBytes > vm.MaxMemoryBytes {
			return domain.VM{}, domain.ErrActionNotAllowed
		}
	} else {
		if spec.VCPUs > vm.MaxVCPUs {
			vm.MaxVCPUs = spec.VCPUs
		}
		if targetMemoryBytes > vm.MaxMemoryBytes {
			vm.MaxMemoryBytes = targetMemoryBytes
		}
	}
	vm.VCPUs = spec.VCPUs
	vm.MemoryBytes = targetMemoryBytes
	d.vms[uuid] = vm
	d.publish("domain.resources", "info", "vm", uuid, vm.Name, "虚拟机 CPU / 内存配置已更新")
	return cloneVM(vm), nil
}

func (d *DemoBackend) ResizeVMDisk(_ context.Context, uuid, target string, spec domain.VMDiskResizeSpec) (domain.VM, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	if !vm.Persistent {
		return domain.VM{}, domain.ErrActionNotAllowed
	}
	diskIndex := -1
	for index, disk := range vm.Disks {
		if disk.Target != target {
			continue
		}
		if disk.Device != "disk" || disk.ReadOnly || strings.TrimSpace(disk.Source) == "" {
			return domain.VM{}, domain.ErrActionNotAllowed
		}
		diskIndex = index
		break
	}
	if diskIndex < 0 {
		return domain.VM{}, domain.ErrNotFound
	}
	disk := &vm.Disks[diskIndex]
	volume, found := d.demoVolumeByPathLocked(disk.Source)
	if !found {
		return domain.VM{}, domain.ErrDiskResize
	}
	if spec.CapacityBytes <= volume.CapacityBytes {
		return domain.VM{}, fmt.Errorf("%w: new capacity must exceed current capacity", domain.ErrInvalidInput)
	}
	volume.CapacityBytes = spec.CapacityBytes
	for poolName, volumes := range d.volumes {
		for index := range volumes {
			if volumes[index].Path == volume.Path {
				d.volumes[poolName][index] = volume
			}
		}
	}
	disk.CapacityBytes = volume.CapacityBytes
	disk.AllocationBytes = volume.AllocationBytes
	d.vms[uuid] = vm
	d.publish("domain.disk.resized", "info", "vm", uuid, vm.Name, "虚拟机磁盘 "+target+" 已扩容")
	return cloneVM(vm), nil
}

func (d *DemoBackend) SetVMAutostart(_ context.Context, uuid string, enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.ErrNotFound
	}
	if !vm.Persistent {
		return domain.ErrActionNotAllowed
	}
	vm.Autostart = enabled
	d.vms[uuid] = vm
	message := "虚拟机开机自启动已禁用"
	if enabled {
		message = "虚拟机开机自启动已启用"
	}
	d.publish("domain.autostart", "info", "vm", uuid, vm.Name, message)
	return nil
}

func (d *DemoBackend) AttachHostDevice(_ context.Context, uuid, deviceName string) (domain.VM, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	deviceName = strings.TrimSpace(deviceName)
	var hostDevice domain.HostDevice
	found := false
	for _, candidate := range demoHostDevices() {
		if candidate.Name == deviceName {
			hostDevice = candidate
			found = true
			break
		}
	}
	if !found {
		return domain.VM{}, domain.ErrNotFound
	}
	assignment, err := passthroughFromHostDevice(hostDevice)
	if err != nil {
		return domain.VM{}, err
	}
	for _, candidate := range d.vms {
		for _, attached := range candidate.Passthrough {
			if attached.Name == assignment.Name {
				return domain.VM{}, domain.ErrConflict
			}
		}
	}
	vm.Passthrough = append(append([]domain.PassthroughDevice(nil), vm.Passthrough...), assignment)
	d.vms[uuid] = vm
	d.publish("vm.device.attached", "info", "vm", uuid, vm.Name, "PCI/USB 设备 "+assignment.Name+" 已直通到虚拟机")
	return cloneVM(vm), nil
}

func (d *DemoBackend) DetachHostDevice(_ context.Context, uuid, deviceName string) (domain.VM, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	deviceName = strings.TrimSpace(deviceName)
	for index, attached := range vm.Passthrough {
		if attached.Name != deviceName {
			continue
		}
		vm.Passthrough = append(vm.Passthrough[:index], vm.Passthrough[index+1:]...)
		d.vms[uuid] = vm
		d.publish("vm.device.detached", "warning", "vm", uuid, vm.Name, "PCI/USB 设备 "+deviceName+" 已从虚拟机移除")
		return cloneVM(vm), nil
	}
	return domain.VM{}, domain.ErrNotFound
}

func (d *DemoBackend) CreateVM(_ context.Context, spec domain.VMCreateSpec) (domain.VM, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	firmware, ok := domain.NormalizeVMFirmware(string(spec.Firmware))
	if !ok || (spec.SecureBoot && firmware != domain.VMFirmwareUEFI) {
		return domain.VM{}, domain.ErrInvalidInput
	}
	for _, vm := range d.vms {
		if vm.Name == spec.Name {
			return domain.VM{}, domain.ErrConflict
		}
	}
	uuid, err := newDemoUUID()
	if err != nil {
		return domain.VM{}, err
	}
	state := domain.VMStateShutoff
	if spec.Start {
		state = domain.VMStateRunning
	}
	disks := []domain.Disk{{
		Device: "disk", Target: "vda", Bus: "virtio", Format: spec.DiskFormat,
		Source: spec.DiskPath,
	}}
	if volume, found := d.demoVolumeByPathLocked(spec.DiskPath); found {
		disks[0].CapacityBytes = volume.CapacityBytes
		disks[0].AllocationBytes = volume.AllocationBytes
	}
	if spec.ISOPath != "" {
		disks = append(disks, domain.Disk{
			Device: "cdrom", Target: "sda", Bus: "sata", Format: "raw",
			Source: spec.ISOPath, ReadOnly: true,
		})
	}
	if spec.CloudInit != nil {
		disks = append(disks, domain.Disk{
			Device: "cdrom", Target: "sdb", Bus: "sata", Format: "raw",
			Source: "/var/lib/libvirt/images/" + cloudInitSeedVolumeName(spec.Name), ReadOnly: true,
		})
	}
	vm := domain.VM{
		UUID: uuid, Name: spec.Name, State: state, Persistent: true, Autostart: spec.Autostart,
		VCPUs: spec.VCPUs, MaxVCPUs: spec.VCPUs, MemoryBytes: spec.MemoryMiB * 1024 * 1024, MaxMemoryBytes: spec.MemoryMiB * 1024 * 1024,
		OSType: "hvm", Arch: "x86_64", Machine: "pc-q35-10.2",
		Firmware: firmware, SecureBoot: spec.SecureBoot, CloudInit: spec.CloudInit != nil,
		Disks:          disks,
		Interfaces:     []domain.NetworkInterface{{Type: "network", MAC: demoMAC(len(d.vms) + 1), Source: spec.Network, Model: "virtio"}},
		Graphics:       []domain.Graphics{{Type: "vnc", Listen: "127.0.0.1", Port: 5900 + len(d.vms) + 1}},
		IPAddresses:    []string{},
		AllowedActions: domain.AllowedVMActions(state),
	}
	d.vms[uuid] = vm
	d.publish("domain.defined", "info", "vm", uuid, vm.Name, "虚拟机已定义")
	if spec.Start {
		d.publish("domain.started", "info", "vm", uuid, vm.Name, "虚拟机已启动")
	}
	return cloneVM(vm), nil
}

func (d *DemoBackend) CloneVM(_ context.Context, uuid string, spec domain.VMCloneSpec, progress VMCloneProgress) (domain.VM, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	source, ok := d.vms[uuid]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	if source.State != domain.VMStateShutoff && source.State != domain.VMStateCrashed {
		return domain.VM{}, domain.ErrActionNotAllowed
	}
	if !source.Persistent {
		return domain.VM{}, domain.ErrActionNotAllowed
	}
	for _, vm := range d.vms {
		if vm.Name == spec.Name {
			return domain.VM{}, domain.ErrConflict
		}
	}
	reportCloneProgress(progress, 10, "正在准备演示磁盘复制")
	clone := cloneVM(source)
	var err error
	clone.UUID, err = newDemoUUID()
	if err != nil {
		return domain.VM{}, err
	}
	clone.Name = spec.Name
	clone.Hostname = ""
	clone.IPAddresses = []string{}
	clone.AgentConnected = false
	clone.Autostart = spec.Autostart
	clone.State = domain.VMStateShutoff
	if spec.Start {
		clone.State = domain.VMStateRunning
	}
	clone.AllowedActions = domain.AllowedVMActions(clone.State)
	for index := range clone.Disks {
		disk := &clone.Disks[index]
		if disk.Device != "disk" || disk.ReadOnly {
			continue
		}
		poolName, targetPath, poolOK := d.demoPoolForPathLocked(disk.Source)
		if !poolOK {
			return domain.VM{}, fmt.Errorf("%w: demo disk is not in an active storage pool", domain.ErrInvalidInput)
		}
		format := strings.ToLower(strings.TrimSpace(disk.Format))
		if format == "" {
			format = "qcow2"
		}
		targetName := cloneVolumeName(spec.Name, index, disk.Source, format)
		for _, existing := range d.volumes[poolName] {
			if existing.Name == targetName {
				return domain.VM{}, domain.ErrConflict
			}
		}
		volume := domain.StorageVolume{
			Name: targetName, Path: filepath.Join(targetPath, targetName), Type: "file", Format: format,
		}
		for _, sourceVolume := range d.volumes[poolName] {
			if sourceVolume.Path == disk.Source {
				volume.CapacityBytes = sourceVolume.CapacityBytes
				volume.AllocationBytes = sourceVolume.AllocationBytes
				disk.CapacityBytes = sourceVolume.CapacityBytes
				disk.AllocationBytes = sourceVolume.AllocationBytes
				break
			}
		}
		d.volumes[poolName] = append(d.volumes[poolName], volume)
		disk.Source = volume.Path
		reportCloneProgress(progress, 30+index*40/maxInt(1, len(clone.Disks)), "正在复制 "+targetName)
	}
	for index := range clone.Interfaces {
		clone.Interfaces[index].MAC = demoMAC(len(d.vms) + index + 1)
	}
	maxGraphicsPort := 5900
	for _, vm := range d.vms {
		for _, graphics := range vm.Graphics {
			if graphics.Port > maxGraphicsPort {
				maxGraphicsPort = graphics.Port
			}
		}
	}
	for index := range clone.Graphics {
		clone.Graphics[index].Port = maxGraphicsPort + 1 + index
	}
	clone.VCPUs = source.VCPUs
	clone.MaxVCPUs = source.MaxVCPUs
	clone.MemoryBytes = source.MemoryBytes
	clone.MaxMemoryBytes = source.MaxMemoryBytes
	d.vms[clone.UUID] = clone
	reportCloneProgress(progress, 90, "正在注册克隆域")
	d.publish("domain.defined", "info", "vm", clone.UUID, clone.Name, "虚拟机克隆已定义")
	if spec.Start {
		d.publish("domain.started", "info", "vm", clone.UUID, clone.Name, "克隆虚拟机已启动")
	}
	reportCloneProgress(progress, 100, "克隆完成")
	return cloneVM(clone), nil
}

func (d *DemoBackend) BackupVM(ctx context.Context, uuid string, spec domain.VMBackupSpec, progress VMBackupProgress) (domain.VMBackup, error) {
	if err := ctx.Err(); err != nil {
		return domain.VMBackup{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	source, ok := d.vms[uuid]
	if !ok {
		return domain.VMBackup{}, domain.ErrNotFound
	}
	if (source.State != domain.VMStateShutoff && source.State != domain.VMStateCrashed) || !source.Persistent {
		return domain.VMBackup{}, domain.ErrActionNotAllowed
	}
	targetPool, ok := d.pools[spec.TargetPool]
	if !ok {
		return domain.VMBackup{}, domain.ErrNotFound
	}
	if targetPool.State != domain.StoragePoolStateActive {
		return domain.VMBackup{}, domain.ErrUnavailable
	}

	type demoArtifact struct {
		name          string
		format        string
		capacityBytes uint64
		allocation    uint64
		sourceVolume  string
	}
	artifacts := make([]demoArtifact, 0, len(source.Disks))
	for _, disk := range source.Disks {
		if disk.Device != "disk" || disk.ReadOnly || strings.TrimSpace(disk.Source) == "" {
			continue
		}
		sourceVolume, found := d.demoVolumeByPathLocked(disk.Source)
		if !found {
			return domain.VMBackup{}, fmt.Errorf("%w: demo disk is not in an active storage pool", domain.ErrInvalidInput)
		}
		format := strings.ToLower(strings.TrimSpace(sourceVolume.Format))
		if format == "" {
			format = "qcow2"
		}
		artifacts = append(artifacts, demoArtifact{
			name:          backupVolumeName(backupPrefix(spec), fmt.Sprintf("-disk-%02d%s", len(artifacts), backupExtension(sourceVolume.Name, format))),
			format:        format,
			capacityBytes: sourceVolume.CapacityBytes,
			allocation:    sourceVolume.AllocationBytes,
			sourceVolume:  sourceVolume.Name,
		})
	}
	configName := backupVolumeName(backupPrefix(spec), "-domain.xml")
	manifestName := backupVolumeName(backupPrefix(spec), "-manifest.json")
	plannedNames := []string{configName, manifestName}
	for _, artifact := range artifacts {
		plannedNames = append(plannedNames, artifact.name)
	}
	for _, name := range plannedNames {
		for _, existing := range d.volumes[spec.TargetPool] {
			if existing.Name == name {
				return domain.VMBackup{}, domain.ErrConflict
			}
		}
	}

	created := make([]string, 0, len(artifacts)+2)
	committed := false
	defer func() {
		if committed {
			return
		}
		d.removeDemoVolumesLocked(spec.TargetPool, created)
	}()
	backupDisks := make([]domain.VMBackupDisk, 0, len(artifacts))
	totalBytes := uint64(0)
	for index, artifact := range artifacts {
		if err := ctx.Err(); err != nil {
			return domain.VMBackup{}, err
		}
		reportBackupProgress(progress, 10+index*60/maxInt(1, len(artifacts)), fmt.Sprintf("正在复制磁盘 %d/%d", index+1, len(artifacts)))
		d.volumes[spec.TargetPool] = append(d.volumes[spec.TargetPool], domain.StorageVolume{
			Name: artifact.name, Path: filepath.Join(targetPool.TargetPath, artifact.name), Type: "file",
			Format: artifact.format, CapacityBytes: artifact.capacityBytes, AllocationBytes: artifact.allocation,
		})
		created = append(created, artifact.name)
		totalBytes += artifact.capacityBytes
		backupDisks = append(backupDisks, domain.VMBackupDisk{
			SourceVolume: artifact.sourceVolume, BackupVolume: artifact.name,
			Format: artifact.format, CapacityBytes: artifact.capacityBytes,
		})
	}

	configXML := d.xmlConfig[uuid]
	if configXML == "" {
		configXML = demoVMDomainXML(source)
	}
	configBytes := []byte(configXML)
	reportBackupProgress(progress, 78, "正在写入虚拟机配置")
	d.volumes[spec.TargetPool] = append(d.volumes[spec.TargetPool], domain.StorageVolume{
		Name: configName, Path: filepath.Join(targetPool.TargetPath, configName), Type: "file",
		Format: "raw", CapacityBytes: uint64(len(configBytes)), AllocationBytes: uint64(len(configBytes)),
	})
	created = append(created, configName)
	totalBytes += uint64(len(configBytes))

	createdAt := time.Now().UTC()
	configSum := sha256.Sum256(configBytes)
	backupID := spec.BackupID
	if backupID == "" {
		backupID = spec.Name
	}
	manifest, err := json.MarshalIndent(backupManifest{
		FormatVersion: domain.VMBackupFormatVersionV2,
		BackupID:      backupID,
		CreatedAt:     createdAt,
		ToolVersion:   version.Version,
		Source:        &backupManifestSource{VMUUID: uuid, VMName: source.Name, State: source.State, Persistent: source.Persistent},
		Config:        &backupManifestConfig{Artifact: configName, SizeBytes: uint64(len(configBytes)), SHA256: hex.EncodeToString(configSum[:])},
		Restore:       &backupManifestRestore{DefaultTargetPool: spec.TargetPool, RequiresNewMAC: true, ContainsMemory: false},
		Disks:         backupDisks,
	}, "", "  ")
	if err != nil {
		return domain.VMBackup{}, fmt.Errorf("encode demo backup manifest: %w", err)
	}
	manifest = append(manifest, '\n')
	reportBackupProgress(progress, 90, "正在写入备份清单")
	d.volumes[spec.TargetPool] = append(d.volumes[spec.TargetPool], domain.StorageVolume{
		Name: manifestName, Path: filepath.Join(targetPool.TargetPath, manifestName), Type: "file",
		Format: "raw", CapacityBytes: uint64(len(manifest)), AllocationBytes: uint64(len(manifest)),
	})
	created = append(created, manifestName)
	totalBytes += uint64(len(manifest))
	d.volumeData[spec.TargetPool+"/"+configName] = append([]byte(nil), configBytes...)
	d.volumeData[spec.TargetPool+"/"+manifestName] = append([]byte(nil), manifest...)

	committed = true
	reportBackupProgress(progress, 100, "备份完成")
	d.publish("vm.backup.created", "info", "vm", uuid, source.Name, "虚拟机 "+source.Name+" 备份完成")
	return domain.VMBackup{
		BackupID:       backupID,
		FormatVersion:  domain.VMBackupFormatVersionV2,
		CreatedAt:      createdAt,
		VMUUID:         uuid,
		VMName:         source.Name,
		TargetPool:     spec.TargetPool,
		Name:           spec.Name,
		ManifestVolume: manifestName,
		ConfigVolume:   configName,
		ConfigBytes:    uint64(len(configBytes)),
		Disks:          backupDisks,
		TotalBytes:     totalBytes,
	}, nil
}

func (d *DemoBackend) demoVolumeByPathLocked(path string) (domain.StorageVolume, bool) {
	for _, volumes := range d.volumes {
		for _, volume := range volumes {
			if volume.Path == path {
				return volume, true
			}
		}
	}
	return domain.StorageVolume{}, false
}

func (d *DemoBackend) VerifyVMBackup(_ context.Context, backup domain.Backup, deep bool) (domain.BackupVerification, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	manifestBytes, ok := d.volumeData[backup.TargetPool+"/"+backup.ManifestVolume]
	checks := make([]domain.BackupArtifactCheck, 0, len(backup.Disks)+2)
	checks = append(checks, domain.BackupArtifactCheck{Name: backup.ManifestVolume, Kind: "manifest", Exists: ok})
	if !ok {
		return domain.BackupVerification{BackupID: backup.ID, Status: domain.BackupVerifyFailed, Deep: deep, CheckedAt: time.Now().UTC(), Message: "备份清单不存在", Artifacts: checks}, nil
	}
	var manifest backupManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return domain.BackupVerification{BackupID: backup.ID, Status: domain.BackupVerifyFailed, Deep: deep, CheckedAt: time.Now().UTC(), Message: "备份清单无法解析", Artifacts: checks}, nil
	}
	normalizeBackupManifest(&manifest, backup)
	configBytes, configOK := d.volumeData[backup.TargetPool+"/"+manifest.ConfigVolume]
	configCheck := domain.BackupArtifactCheck{Name: manifest.ConfigVolume, Kind: "config", Exists: configOK}
	if configOK && manifest.ConfigSHA256 != "" {
		sum := sha256.Sum256(configBytes)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), manifest.ConfigSHA256) {
			configCheck.Message = "配置 SHA-256 与备份清单不一致"
		}
	}
	checks = append(checks, configCheck)
	failed := !configOK || configCheck.Message != ""
	for _, disk := range manifest.Disks {
		volume, found := d.demoVolumeLocked(backup.TargetPool, disk.BackupVolume)
		check := domain.BackupArtifactCheck{Name: disk.BackupVolume, Kind: "disk", Exists: found, Format: disk.Format, CapacityBytes: disk.CapacityBytes}
		if found && deep && volume.CapacityBytes < disk.CapacityBytes {
			check.Message = "备份磁盘容量小于清单记录"
		}
		if !found || check.Message != "" {
			failed = true
		}
		checks = append(checks, check)
	}
	status := domain.BackupVerifyVerified
	message := "备份清单、配置和磁盘产物完整"
	if failed {
		status = domain.BackupVerifyFailed
		message = "备份校验失败，请查看产物检查结果"
	}
	return domain.BackupVerification{BackupID: backup.ID, Status: status, Deep: deep, CheckedAt: time.Now().UTC(), Message: message, Artifacts: checks}, nil
}

func (d *DemoBackend) DeleteVMBackup(_ context.Context, backup domain.Backup) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	names := make([]string, 0, len(backup.Disks)+2)
	for _, disk := range backup.Disks {
		names = append(names, disk.Artifact)
	}
	names = append(names, backup.ConfigVolume, backup.ManifestVolume)
	d.removeDemoVolumesLocked(backup.TargetPool, names)
	for _, name := range names {
		delete(d.volumeData, backup.TargetPool+"/"+name)
	}
	d.publish("vm.backup.deleted", "warning", "vm", backup.VMUUID, backup.VMName, "虚拟机 "+backup.VMName+" 的备份已删除")
	return nil
}

func (d *DemoBackend) RestoreVMBackup(ctx context.Context, backup domain.Backup, spec domain.VMRestoreSpec, progress VMBackupProgress) (domain.VM, error) {
	if err := ctx.Err(); err != nil {
		return domain.VM{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	manifestBytes, ok := d.volumeData[backup.TargetPool+"/"+backup.ManifestVolume]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	var manifest backupManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return domain.VM{}, fmt.Errorf("%w: invalid backup manifest", domain.ErrInvalidInput)
	}
	normalizeBackupManifest(&manifest, backup)
	configBytes, ok := d.volumeData[backup.TargetPool+"/"+manifest.ConfigVolume]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	for _, vm := range d.vms {
		if vm.Name == spec.Name {
			return domain.VM{}, domain.ErrConflict
		}
	}
	targetPool, ok := d.pools[spec.TargetPool]
	if !ok {
		return domain.VM{}, domain.ErrNotFound
	}
	if targetPool.State != domain.StoragePoolStateActive {
		return domain.VM{}, domain.ErrUnavailable
	}
	sourceDisks, err := cloneDisksFromDomainXML(string(configBytes))
	if err != nil {
		return domain.VM{}, err
	}
	if len(sourceDisks) != len(manifest.Disks) {
		return domain.VM{}, fmt.Errorf("%w: backup disk count does not match config", domain.ErrInvalidInput)
	}
	replaced := make(map[string]string, len(sourceDisks))
	created := make([]string, 0, len(sourceDisks))
	for index, disk := range manifest.Disks {
		reportBackupProgress(progress, 10+index*60/maxInt(1, len(manifest.Disks)), fmt.Sprintf("正在恢复演示磁盘 %d/%d", index+1, len(manifest.Disks)))
		sourceVolume, found := d.demoVolumeLocked(backup.TargetPool, disk.BackupVolume)
		if !found {
			d.removeDemoVolumesLocked(spec.TargetPool, created)
			return domain.VM{}, domain.ErrNotFound
		}
		targetName := cloneVolumeName(spec.Name, index, disk.BackupVolume, disk.Format)
		if _, found := d.demoVolumeLocked(spec.TargetPool, targetName); found {
			d.removeDemoVolumesLocked(spec.TargetPool, created)
			return domain.VM{}, domain.ErrConflict
		}
		targetVolume := domain.StorageVolume{
			Name: targetName, Path: filepath.Join(targetPool.TargetPath, targetName), Type: "file",
			Format: disk.Format, CapacityBytes: sourceVolume.CapacityBytes, AllocationBytes: sourceVolume.AllocationBytes,
		}
		d.volumes[spec.TargetPool] = append(d.volumes[spec.TargetPool], targetVolume)
		created = append(created, targetName)
		replaced[sourceDisks[index].source] = targetVolume.Path
	}
	rewritten, err := rewriteRestoreDomainXML(string(configBytes), spec.Name, replaced, spec.PreservePassthrough)
	if err != nil {
		d.removeDemoVolumesLocked(spec.TargetPool, created)
		return domain.VM{}, err
	}
	base, ok := d.vms[manifest.VMUUID]
	if !ok {
		base = domain.VM{OSType: "hvm", Arch: "x86_64", Machine: "q35", Firmware: domain.VMFirmwareBIOS}
	}
	restored := cloneVM(base)
	restored.UUID, err = newDemoUUID()
	if err != nil {
		d.removeDemoVolumesLocked(spec.TargetPool, created)
		return domain.VM{}, err
	}
	restored.Name = spec.Name
	restored.State = domain.VMStateShutoff
	restored.Autostart = spec.Autostart
	restored.Hostname = ""
	restored.IPAddresses = []string{}
	restored.AgentConnected = false
	restored.Disks = make([]domain.Disk, 0, len(sourceDisks))
	for index, sourceDisk := range sourceDisks {
		targetName := created[index]
		volume, _ := d.demoVolumeLocked(spec.TargetPool, targetName)
		restored.Disks = append(restored.Disks, domain.Disk{
			Device: "disk", Target: sourceDisk.target, Bus: "virtio", Format: manifest.Disks[index].Format,
			Source: volume.Path, CapacityBytes: volume.CapacityBytes, AllocationBytes: volume.AllocationBytes,
		})
	}
	for index := range restored.Interfaces {
		restored.Interfaces[index].MAC = demoMAC(len(d.vms) + index + 1)
	}
	if !spec.PreservePassthrough {
		restored.Passthrough = nil
	}
	if spec.Start {
		restored.State = domain.VMStateRunning
	}
	restored.AllowedActions = domain.AllowedVMActions(restored.State)
	d.vms[restored.UUID] = restored
	d.xmlConfig[restored.UUID] = rewritten
	reportBackupProgress(progress, 100, "恢复完成")
	d.publish("vm.backup.restored", "info", "vm", restored.UUID, restored.Name, "虚拟机 "+restored.Name+" 已从备份恢复")
	return cloneVM(restored), nil
}

func (d *DemoBackend) demoVolumeLocked(poolName, name string) (domain.StorageVolume, bool) {
	for _, volume := range d.volumes[poolName] {
		if volume.Name == name {
			return volume, true
		}
	}
	return domain.StorageVolume{}, false
}

func (d *DemoBackend) removeDemoVolumesLocked(poolName string, names []string) {
	if len(names) == 0 {
		return
	}
	remove := make(map[string]bool, len(names))
	for _, name := range names {
		remove[name] = true
	}
	kept := d.volumes[poolName][:0]
	for _, volume := range d.volumes[poolName] {
		if !remove[volume.Name] {
			kept = append(kept, volume)
		}
	}
	d.volumes[poolName] = kept
}

func (d *DemoBackend) DeleteVM(_ context.Context, uuid string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.ErrNotFound
	}
	if vm.State != domain.VMStateShutoff && vm.State != domain.VMStateCrashed {
		return domain.ErrActionNotAllowed
	}
	delete(d.vms, uuid)
	delete(d.xmlConfig, uuid)
	delete(d.snapshots, uuid)
	d.publish("domain.undefined", "warning", "vm", uuid, vm.Name, "虚拟机定义已删除")
	return nil
}

func (d *DemoBackend) ListSnapshots(_ context.Context, uuid string) ([]domain.VMSnapshot, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, ok := d.vms[uuid]; !ok {
		return nil, domain.ErrNotFound
	}
	return append([]domain.VMSnapshot(nil), d.snapshots[uuid]...), nil
}

func (d *DemoBackend) CreateSnapshot(_ context.Context, uuid string, spec domain.VMSnapshotCreateSpec) (domain.VMSnapshot, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.VMSnapshot{}, domain.ErrNotFound
	}
	for _, snapshot := range d.snapshots[uuid] {
		if snapshot.Name == spec.Name {
			return domain.VMSnapshot{}, domain.ErrConflict
		}
	}
	for i := range d.snapshots[uuid] {
		d.snapshots[uuid][i].Current = false
	}
	snapshot := domain.VMSnapshot{
		Name: spec.Name, Description: spec.Description, CreatedAt: time.Now().UTC().Unix(),
		State: vm.State, Current: true,
	}
	d.snapshots[uuid] = append(d.snapshots[uuid], snapshot)
	d.publish("snapshot.created", "info", "vm_snapshot", uuid+"/"+snapshot.Name, vm.Name, "快照 "+snapshot.Name+" 已创建")
	return snapshot, nil
}

func (d *DemoBackend) RevertSnapshot(_ context.Context, uuid, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.ErrNotFound
	}
	index := -1
	for i := range d.snapshots[uuid] {
		if d.snapshots[uuid][i].Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		return domain.ErrNotFound
	}
	for i := range d.snapshots[uuid] {
		d.snapshots[uuid][i].Current = i == index
	}
	vm.State = d.snapshots[uuid][index].State
	vm.AllowedActions = domain.AllowedVMActions(vm.State)
	d.vms[uuid] = vm
	d.publish("snapshot.reverted", "warning", "vm_snapshot", uuid+"/"+name, vm.Name, "虚拟机已回滚到快照 "+name)
	return nil
}

func (d *DemoBackend) DeleteSnapshot(_ context.Context, uuid, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.vms[uuid]; !ok {
		return domain.ErrNotFound
	}
	for i := range d.snapshots[uuid] {
		if d.snapshots[uuid][i].Name == name {
			d.snapshots[uuid] = append(d.snapshots[uuid][:i], d.snapshots[uuid][i+1:]...)
			d.publish("snapshot.deleted", "warning", "vm_snapshot", uuid+"/"+name, name, "快照 "+name+" 已删除")
			return nil
		}
	}
	return domain.ErrNotFound
}

func (d *DemoBackend) ListStoragePools(context.Context) ([]domain.StoragePool, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]domain.StoragePool, 0, len(d.pools))
	for _, pool := range d.pools {
		pool.VolumeCount = demoPoolVolumeCount(pool, d.volumes[pool.Name])
		result = append(result, pool)
	}
	return result, nil
}

func (d *DemoBackend) GetStoragePool(_ context.Context, name string) (domain.StoragePool, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	pool, ok := d.pools[name]
	if !ok {
		return domain.StoragePool{}, domain.ErrNotFound
	}
	pool.VolumeCount = demoPoolVolumeCount(pool, d.volumes[pool.Name])
	return pool, nil
}

func (d *DemoBackend) CreateStoragePool(_ context.Context, spec domain.StoragePoolCreateSpec) (domain.StoragePool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.pools[spec.Name]; ok {
		return domain.StoragePool{}, domain.ErrConflict
	}
	uuid, err := newDemoUUID()
	if err != nil {
		return domain.StoragePool{}, err
	}
	pool := domain.StoragePool{
		Name: spec.Name, UUID: uuid, Type: "dir", State: domain.StoragePoolStateInactive,
		Autostart: spec.Autostart, Persistent: true, TargetPath: spec.TargetPath,
	}
	if spec.Start {
		pool.State = domain.StoragePoolStateActive
		pool.CapacityBytes = 100 * 1024 * 1024 * 1024
		pool.AvailableBytes = pool.CapacityBytes
	}
	d.pools[spec.Name] = pool
	d.volumes[spec.Name] = []domain.StorageVolume{}
	d.publish("storage.pool.created", "info", "storage_pool", spec.Name, spec.Name, "存储池 "+spec.Name+" 已创建")
	return pool, nil
}

func (d *DemoBackend) PerformStoragePoolAction(_ context.Context, name string, action domain.StoragePoolAction) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	pool, ok := d.pools[name]
	if !ok {
		return domain.ErrNotFound
	}
	switch action {
	case domain.StoragePoolActionStart:
		if pool.State != domain.StoragePoolStateInactive {
			return domain.ErrConflict
		}
		pool.State = domain.StoragePoolStateActive
		pool.CapacityBytes = 100 * 1024 * 1024 * 1024
		pool.AllocationBytes = 0
		pool.AvailableBytes = pool.CapacityBytes
		d.publish("storage.pool.started", "info", "storage_pool", name, name, "存储池 "+name+" 已启动")
	case domain.StoragePoolActionStop:
		if pool.State != domain.StoragePoolStateActive {
			return domain.ErrConflict
		}
		pool.State = domain.StoragePoolStateInactive
		d.publish("storage.pool.stopped", "info", "storage_pool", name, name, "存储池 "+name+" 已停止")
	case domain.StoragePoolActionRefresh:
		if pool.State != domain.StoragePoolStateActive {
			return domain.ErrConflict
		}
		pool.VolumeCount = len(d.volumes[name])
		d.publish("storage.pool.refreshed", "info", "storage_pool", name, name, "存储池 "+name+" 已刷新")
	default:
		return domain.ErrInvalidInput
	}
	pool.VolumeCount = demoPoolVolumeCount(pool, d.volumes[name])
	d.pools[name] = pool
	return nil
}

func (d *DemoBackend) DeleteStoragePool(_ context.Context, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	pool, ok := d.pools[name]
	if !ok {
		return domain.ErrNotFound
	}
	if pool.State != domain.StoragePoolStateInactive {
		return domain.ErrConflict
	}
	delete(d.pools, name)
	delete(d.volumes, name)
	d.publish("storage.pool.deleted", "warning", "storage_pool", name, name, "存储池 "+name+" 已删除")
	return nil
}

func (d *DemoBackend) ListStorageVolumes(_ context.Context, poolName string) ([]domain.StorageVolume, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	pool, ok := d.pools[poolName]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if pool.State != domain.StoragePoolStateActive {
		return nil, domain.ErrUnavailable
	}
	return append([]domain.StorageVolume(nil), d.volumes[poolName]...), nil
}

func (d *DemoBackend) CreateStorageVolume(_ context.Context, poolName string, spec domain.StorageVolumeCreateSpec) (domain.StorageVolume, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	pool, ok := d.pools[poolName]
	if !ok {
		return domain.StorageVolume{}, domain.ErrNotFound
	}
	if pool.State != domain.StoragePoolStateActive {
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	targetPath := pool.TargetPath
	for _, volume := range d.volumes[poolName] {
		if volume.Name == spec.Name {
			return domain.StorageVolume{}, domain.ErrConflict
		}
	}
	volume := domain.StorageVolume{
		Name: spec.Name, Path: targetPath + "/" + spec.Name, Type: "file", Format: spec.Format,
		CapacityBytes: spec.CapacityBytes,
	}
	d.volumes[poolName] = append(d.volumes[poolName], volume)
	d.publish("storage.volume.created", "info", "storage_volume", poolName+"/"+volume.Name, volume.Name, "存储卷 "+volume.Name+" 已创建")
	return volume, nil
}

func (d *DemoBackend) UploadStorageVolume(_ context.Context, poolName string, spec domain.StorageVolumeCreateSpec, reader io.Reader) (domain.StorageVolume, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	pool, ok := d.pools[poolName]
	if !ok {
		return domain.StorageVolume{}, domain.ErrNotFound
	}
	if pool.State != domain.StoragePoolStateActive {
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	targetPath := pool.TargetPath
	if reader == nil || spec.CapacityBytes == 0 {
		return domain.StorageVolume{}, domain.ErrInvalidInput
	}
	for _, volume := range d.volumes[poolName] {
		if volume.Name == spec.Name {
			return domain.StorageVolume{}, domain.ErrConflict
		}
	}
	written, err := io.CopyN(io.Discard, reader, int64(spec.CapacityBytes))
	if err != nil || uint64(written) != spec.CapacityBytes {
		return domain.StorageVolume{}, fmt.Errorf("%w: upload ended before declared size", domain.ErrInvalidInput)
	}
	volume := domain.StorageVolume{
		Name: spec.Name, Path: filepath.Join(targetPath, spec.Name), Type: "file", Format: spec.Format,
		CapacityBytes: spec.CapacityBytes, AllocationBytes: spec.CapacityBytes,
	}
	d.volumes[poolName] = append(d.volumes[poolName], volume)
	d.publish("storage.volume.uploaded", "info", "storage_volume", poolName+"/"+volume.Name, volume.Name, "ISO "+volume.Name+" 已上传")
	return volume, nil
}

func (d *DemoBackend) DeleteStorageVolume(_ context.Context, poolName, volumeName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	pool, ok := d.pools[poolName]
	if !ok {
		return domain.ErrNotFound
	}
	if pool.State != domain.StoragePoolStateActive {
		return domain.ErrUnavailable
	}
	index := -1
	var target domain.StorageVolume
	for i, volume := range d.volumes[poolName] {
		if volume.Name == volumeName {
			index = i
			target = volume
			break
		}
	}
	if index < 0 {
		return domain.ErrNotFound
	}
	for _, vm := range d.vms {
		for _, disk := range vm.Disks {
			if diskUsesVolume(disk.Source, target.Path, target.Name) {
				return fmt.Errorf("%w: attached to %s", domain.ErrVolumeInUse, vm.Name)
			}
		}
	}
	d.volumes[poolName] = append(d.volumes[poolName][:index], d.volumes[poolName][index+1:]...)
	d.publish("storage.volume.deleted", "warning", "storage_volume", poolName+"/"+target.Name, target.Name, "存储卷 "+target.Name+" 已删除")
	return nil
}

func (d *DemoBackend) demoPoolForPathLocked(path string) (string, string, bool) {
	cleanPath := filepath.Clean(path)
	bestName, bestTarget := "", ""
	for name, pool := range d.pools {
		if pool.State != domain.StoragePoolStateActive || pool.TargetPath == "" {
			continue
		}
		target := filepath.Clean(pool.TargetPath)
		if !strings.HasPrefix(cleanPath, target+string(filepath.Separator)) {
			continue
		}
		if len(target) > len(bestTarget) {
			bestName, bestTarget = name, target
		}
	}
	return bestName, bestTarget, bestName != ""
}

func demoStoragePools() map[string]domain.StoragePool {
	return map[string]domain.StoragePool{
		"default": {
			Name: "default", UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Type: "dir",
			State: domain.StoragePoolStateActive, Autostart: true, Persistent: true,
			TargetPath: "/var/lib/libvirt/images", CapacityBytes: 500 * 1024 * 1024 * 1024,
			AllocationBytes: 92 * 1024 * 1024 * 1024, AvailableBytes: 408 * 1024 * 1024 * 1024,
		},
		"iso-store": {
			Name: "iso-store", UUID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Type: "dir",
			State: domain.StoragePoolStateActive, Autostart: true, Persistent: true,
			TargetPath: "/var/lib/libvirt/iso", CapacityBytes: 120 * 1024 * 1024 * 1024,
			AllocationBytes: 9 * 1024 * 1024 * 1024, AvailableBytes: 111 * 1024 * 1024 * 1024,
		},
		"archive": {
			Name: "archive", UUID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Type: "dir",
			State: domain.StoragePoolStateInactive, Autostart: false, Persistent: true,
			TargetPath: "/srv/virtmgr/archive",
		},
	}
}

func demoPoolVolumeCount(pool domain.StoragePool, volumes []domain.StorageVolume) int {
	if pool.State != domain.StoragePoolStateActive {
		return 0
	}
	return len(volumes)
}

func demoStorageVolumes() map[string][]domain.StorageVolume {
	return map[string][]domain.StorageVolume{
		"default": {
			{Name: "demo-web-01.qcow2", Path: "/var/lib/libvirt/images/demo-web-01.qcow2", Type: "file", Format: "qcow2", CapacityBytes: 20 * 1024 * 1024 * 1024, AllocationBytes: 8 * 1024 * 1024 * 1024},
			{Name: "demo-db-01.qcow2", Path: "/var/lib/libvirt/images/demo-db-01.qcow2", Type: "file", Format: "qcow2", CapacityBytes: 80 * 1024 * 1024 * 1024, AllocationBytes: 42 * 1024 * 1024 * 1024},
			{Name: "demo-build-01.qcow2", Path: "/var/lib/libvirt/images/demo-build-01.qcow2", Type: "file", Format: "qcow2", CapacityBytes: 30 * 1024 * 1024 * 1024, AllocationBytes: 12 * 1024 * 1024 * 1024},
			{Name: "demo-legacy-01.qcow2", Path: "/var/lib/libvirt/images/demo-legacy-01.qcow2", Type: "file", Format: "qcow2", CapacityBytes: 20 * 1024 * 1024 * 1024, AllocationBytes: 30 * 1024 * 1024 * 1024},
		},
		"iso-store": {
			{Name: "debian-13-netinst.iso", Path: "/var/lib/libvirt/iso/debian-13-netinst.iso", Type: "file", Format: "raw", CapacityBytes: 700 * 1024 * 1024, AllocationBytes: 700 * 1024 * 1024},
			{Name: "virtio-win.iso", Path: "/var/lib/libvirt/iso/virtio-win.iso", Type: "file", Format: "raw", CapacityBytes: 600 * 1024 * 1024, AllocationBytes: 600 * 1024 * 1024},
		},
		"archive": {},
	}
}

func (d *DemoBackend) ListNetworks(context.Context) ([]domain.VirtualNetwork, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]domain.VirtualNetwork, 0, len(d.networks))
	for _, network := range d.networks {
		result = append(result, cloneNetwork(network))
	}
	return result, nil
}

func (d *DemoBackend) ListDHCPLeases(_ context.Context, networkName string) ([]domain.DHCPLease, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, ok := d.networks[networkName]; !ok {
		return nil, domain.ErrNotFound
	}
	return append([]domain.DHCPLease{}, d.leases[networkName]...), nil
}

func (d *DemoBackend) AddNetworkDHCPHost(_ context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	network, ok := d.networks[name]
	if !ok {
		return domain.VirtualNetwork{}, domain.ErrNotFound
	}
	if !updateDemoNetworkDHCPHost(&network, host, false) {
		return domain.VirtualNetwork{}, domain.ErrNetworkUnsupported
	}
	d.networks[name] = network
	d.publish("network.dhcp.host.added", "info", "network", name, name, "虚拟网络 "+name+" 已添加静态主机 "+host.MAC)
	return cloneNetwork(network), nil
}

func (d *DemoBackend) DeleteNetworkDHCPHost(_ context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	network, ok := d.networks[name]
	if !ok {
		return domain.VirtualNetwork{}, domain.ErrNotFound
	}
	if !updateDemoNetworkDHCPHost(&network, host, true) {
		return domain.VirtualNetwork{}, domain.ErrNetworkUnsupported
	}
	d.networks[name] = network
	d.publish("network.dhcp.host.deleted", "warning", "network", name, name, "虚拟网络 "+name+" 已删除静态主机 "+host.MAC)
	return cloneNetwork(network), nil
}

func (d *DemoBackend) CreateNetwork(_ context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.networks[definition.Name]; exists {
		return domain.VirtualNetwork{}, domain.ErrConflict
	}
	uuid, err := newDemoUUID()
	if err != nil {
		return domain.VirtualNetwork{}, err
	}
	network := domain.VirtualNetwork{
		Name: definition.Name, UUID: uuid, Bridge: d.nextDemoBridgeLocked(), Active: definition.Start,
		Persistent: true, Autostart: definition.Autostart, ForwardMode: definition.Mode,
		Addresses: []domain.NetworkAddress{{
			Family: "ipv4", Address: definition.IPv4Address, Prefix: definition.Prefix, DHCPEnabled: definition.DHCPStart != "",
			DHCPStart: definition.DHCPStart, DHCPEnd: definition.DHCPEnd,
		}},
	}
	d.networks[network.Name] = network
	d.leases[network.Name] = []domain.DHCPLease{}
	d.publish("network.created", "info", "network", network.Name, network.Name, "虚拟网络 "+network.Name+" 已创建")
	return cloneNetwork(network), nil
}

func (d *DemoBackend) UpdateNetwork(_ context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	network, ok := d.networks[definition.Name]
	if !ok {
		return domain.VirtualNetwork{}, domain.ErrNotFound
	}
	if network.Active {
		return domain.VirtualNetwork{}, domain.ErrNetworkActive
	}
	if !network.Persistent || (network.ForwardMode != "" && network.ForwardMode != "nat" && network.ForwardMode != "isolated") {
		return domain.VirtualNetwork{}, domain.ErrNetworkUnsupported
	}
	network.Autostart = definition.Autostart
	network.ForwardMode = definition.Mode
	addresses := append([]domain.NetworkAddress(nil), network.Addresses...)
	updatedAddress := domain.NetworkAddress{
		Family: "ipv4", Address: definition.IPv4Address, Prefix: definition.Prefix, DHCPEnabled: definition.DHCPStart != "",
		DHCPStart: definition.DHCPStart, DHCPEnd: definition.DHCPEnd,
	}
	replaced := false
	for index := range addresses {
		if addresses[index].Family == "" || addresses[index].Family == "ipv4" {
			updatedAddress.Hosts = append([]domain.NetworkDHCPHost(nil), addresses[index].Hosts...)
			addresses[index] = updatedAddress
			replaced = true
			break
		}
	}
	if !replaced {
		addresses = append(addresses, updatedAddress)
	}
	network.Addresses = addresses
	d.networks[definition.Name] = network
	d.publish("network.updated", "info", "network", network.Name, network.Name, "虚拟网络 "+network.Name+" 配置已更新")
	return cloneNetwork(network), nil
}

func (d *DemoBackend) PerformNetworkAction(_ context.Context, name string, action domain.VirtualNetworkAction) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	network, ok := d.networks[name]
	if !ok {
		return domain.ErrNotFound
	}
	switch action {
	case domain.VirtualNetworkActionStart:
		if network.Active {
			return domain.ErrConflict
		}
		network.Active = true
	case domain.VirtualNetworkActionStop:
		if !network.Active {
			return domain.ErrConflict
		}
		network.Active = false
	default:
		return domain.ErrInvalidInput
	}
	d.networks[name] = network
	kind, message := "network.started", "虚拟网络 "+name+" 已启动"
	if action == domain.VirtualNetworkActionStop {
		kind, message = "network.stopped", "虚拟网络 "+name+" 已停止"
	}
	d.publish(kind, "info", "network", name, name, message)
	return nil
}

func (d *DemoBackend) DeleteNetwork(_ context.Context, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	network, ok := d.networks[name]
	if !ok {
		return domain.ErrNotFound
	}
	if network.Active {
		return domain.ErrConflict
	}
	delete(d.networks, name)
	delete(d.leases, name)
	d.publish("network.deleted", "warning", "network", name, name, "虚拟网络 "+name+" 已删除")
	return nil
}

func (d *DemoBackend) nextDemoBridgeLocked() string {
	used := make(map[string]bool, len(d.networks))
	for _, network := range d.networks {
		used[network.Bridge] = true
	}
	for index := 0; ; index++ {
		candidate := fmt.Sprintf("virbr%d", index)
		if !used[candidate] {
			return candidate
		}
	}
}

func cloneNetwork(network domain.VirtualNetwork) domain.VirtualNetwork {
	addresses := make([]domain.NetworkAddress, len(network.Addresses))
	copy(addresses, network.Addresses)
	network.Addresses = addresses
	for index := range network.Addresses {
		network.Addresses[index].Hosts = append([]domain.NetworkDHCPHost(nil), network.Addresses[index].Hosts...)
	}
	return network
}

func updateDemoNetworkDHCPHost(network *domain.VirtualNetwork, host domain.NetworkDHCPHost, remove bool) bool {
	for addressIndex := range network.Addresses {
		address := &network.Addresses[addressIndex]
		if address.Family != "" && address.Family != "ipv4" {
			continue
		}
		if remove {
			for hostIndex := range address.Hosts {
				if strings.EqualFold(address.Hosts[hostIndex].MAC, host.MAC) {
					address.Hosts = append(address.Hosts[:hostIndex], address.Hosts[hostIndex+1:]...)
					return true
				}
			}
			return false
		}
		address.Hosts = append(address.Hosts, host)
		return true
	}
	return false
}

func demoNetworks() []domain.VirtualNetwork {
	return []domain.VirtualNetwork{
		{
			Name: "default", UUID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", Bridge: "virbr0",
			Active: true, Persistent: true, Autostart: true, ForwardMode: "nat",
			Addresses: []domain.NetworkAddress{{
				Family: "ipv4", Address: "192.168.122.1", Prefix: 24, DHCPEnabled: true,
				DHCPStart: "192.168.122.2", DHCPEnd: "192.168.122.254",
				Hosts: []domain.NetworkDHCPHost{{MAC: "52:54:00:12:34:56", IP: "192.168.122.101", Name: "demo-web-01"}},
			}},
		},
		{
			Name: "br-prod", UUID: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", Bridge: "br0",
			Active: true, Persistent: true, Autostart: true, ForwardMode: "bridge",
			Addresses: []domain.NetworkAddress{},
		},
		{
			Name: "lab-isolated", UUID: "ffffffff-ffff-4fff-8fff-ffffffffffff", Bridge: "virbr1",
			Active: false, Persistent: true, Autostart: false, ForwardMode: "isolated",
			Addresses: []domain.NetworkAddress{{Family: "ipv4", Address: "10.10.0.1", Prefix: 24}},
		},
	}
}

func demoNetworkLeases() map[string][]domain.DHCPLease {
	now := time.Now().UTC()
	webExpiry := now.Add(4 * time.Hour)
	dbExpiry := now.Add(7 * time.Hour)
	return map[string][]domain.DHCPLease{
		"default": {
			{
				Interface: "vnet0", ExpiresAt: &webExpiry, Family: "ipv4",
				MAC: "52:54:00:12:34:56", IPAddress: "192.168.122.101", Prefix: 24,
				Hostname: "demo-web-01", ClientID: "01:52:54:00:12:34:56", IAID: "web-01",
			},
			{
				Interface: "vnet1", ExpiresAt: &dbExpiry, Family: "ipv4",
				MAC: "52:54:00:65:43:21", IPAddress: "192.168.122.102", Prefix: 24,
				Hostname: "demo-db-01", ClientID: "01:52:54:00:65:43:21", IAID: "db-01",
			},
		},
		"br-prod":      {},
		"lab-isolated": {},
	}
}

func (d *DemoBackend) PerformAction(_ context.Context, uuid string, action domain.VMAction) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[uuid]
	if !ok {
		return domain.ErrNotFound
	}
	if !domain.ContainsVMAction(domain.AllowedVMActions(vm.State), action) {
		return domain.ErrActionNotAllowed
	}
	switch action {
	case domain.VMActionStart, domain.VMActionResume, domain.VMActionReboot, domain.VMActionReset:
		vm.State = domain.VMStateRunning
	case domain.VMActionShutdown, domain.VMActionDestroy:
		vm.State = domain.VMStateShutoff
	case domain.VMActionPause:
		vm.State = domain.VMStatePaused
	default:
		return domain.ErrInvalidInput
	}
	vm.StateReason = 0
	vm.AllowedActions = domain.AllowedVMActions(vm.State)
	d.vms[uuid] = vm
	kind, severity, message := "domain.changed", "info", "虚拟机状态发生变化"
	switch action {
	case domain.VMActionStart:
		kind, message = "domain.started", "虚拟机已启动"
	case domain.VMActionShutdown:
		kind, message = "domain.shutdown", "虚拟机已关机"
	case domain.VMActionReboot, domain.VMActionReset:
		kind, message = "domain.started", "虚拟机已重启"
	case domain.VMActionPause:
		kind, severity, message = "domain.suspended", "warning", "虚拟机已暂停"
	case domain.VMActionResume:
		kind, message = "domain.resumed", "虚拟机已恢复运行"
	case domain.VMActionDestroy:
		kind, severity, message = "domain.stopped", "warning", "虚拟机已强制下电"
	}
	d.publish(kind, severity, "vm", uuid, vm.Name, message)
	return nil
}

func (d *DemoBackend) publish(kind, severity, targetKind, targetID, targetName, message string) {
	if d.events == nil {
		return
	}
	d.events.Publish(domain.SystemEvent{
		Source: "demo", Kind: kind, Severity: severity,
		TargetKind: targetKind, TargetID: targetID, TargetName: targetName, Message: message,
	})
}

func (d *DemoBackend) seedDemoEvents() {
	if d.events == nil {
		return
	}
	now := time.Now().UTC()
	events := []domain.SystemEvent{
		{OccurredAt: now.Add(-12 * time.Minute), Source: "demo", Kind: "domain.started", Severity: "info", TargetKind: "vm", TargetID: "11111111-1111-4111-8111-111111111111", TargetName: "demo-web-01", Message: "虚拟机已启动"},
		{OccurredAt: now.Add(-5 * time.Minute), Source: "demo", Kind: "domain.started", Severity: "info", TargetKind: "vm", TargetID: "22222222-2222-4222-8222-222222222222", TargetName: "demo-db-01", Message: "虚拟机已启动"},
		{OccurredAt: now.Add(-2 * time.Minute), Source: "demo", Kind: "domain.suspended", Severity: "warning", TargetKind: "vm", TargetID: "33333333-3333-4333-8333-333333333333", TargetName: "demo-build-01", Message: "虚拟机已暂停"},
	}
	for _, event := range events {
		d.events.Publish(event)
	}
}

func cloneVM(vm domain.VM) domain.VM {
	vm.Disks = append([]domain.Disk(nil), vm.Disks...)
	vm.Interfaces = append([]domain.NetworkInterface(nil), vm.Interfaces...)
	vm.Graphics = append([]domain.Graphics(nil), vm.Graphics...)
	vm.Passthrough = append([]domain.PassthroughDevice{}, vm.Passthrough...)
	vm.IPAddresses = append([]string{}, vm.IPAddresses...)
	vm.AllowedActions = append([]domain.VMAction(nil), vm.AllowedActions...)
	return vm
}

func demoVMDomainXML(vm domain.VM) string {
	var builder strings.Builder
	builder.WriteString(xml.Header)
	fmt.Fprintf(&builder, "<domain type='kvm'>\n  <name>%s</name>\n  <uuid>%s</uuid>\n", escapeXML(vm.Name), escapeXML(vm.UUID))
	fmt.Fprintf(&builder, "  <memory unit='KiB'>%d</memory>\n  <vcpu>%d</vcpu>\n", vm.MemoryBytes/1024, vm.VCPUs)
	if vm.Firmware == domain.VMFirmwareUEFI {
		secure := "no"
		if vm.SecureBoot {
			secure = "yes"
		}
		fmt.Fprintf(&builder, "  <os firmware='efi'><type arch='%s' machine='%s'>%s</type><loader secure='%s'/></os>\n", escapeXML(vm.Arch), escapeXML(vm.Machine), escapeXML(vm.OSType), secure)
	} else {
		fmt.Fprintf(&builder, "  <os><type arch='%s' machine='%s'>%s</type></os>\n", escapeXML(vm.Arch), escapeXML(vm.Machine), escapeXML(vm.OSType))
	}
	builder.WriteString("  <devices>\n")
	for _, disk := range vm.Disks {
		fmt.Fprintf(&builder, "    <disk type='file' device='%s'><driver name='qemu' type='%s'/><source file='%s'/><target dev='%s' bus='%s'/>", escapeXML(disk.Device), escapeXML(disk.Format), escapeXML(disk.Source), escapeXML(disk.Target), escapeXML(disk.Bus))
		if disk.ReadOnly {
			builder.WriteString("<readonly/>")
		}
		if strings.HasSuffix(disk.Source, "-cidata.iso") {
			builder.WriteString("<serial>cloud-init</serial>")
		}
		builder.WriteString("</disk>\n")
	}
	for _, network := range vm.Interfaces {
		fmt.Fprintf(&builder, "    <interface type='%s'><source network='%s'/><mac address='%s'/><model type='%s'/></interface>\n", escapeXML(network.Type), escapeXML(network.Source), escapeXML(network.MAC), escapeXML(network.Model))
	}
	for _, graphics := range vm.Graphics {
		fmt.Fprintf(&builder, "    <graphics type='%s' listen='%s' port='%d'/>\n", escapeXML(graphics.Type), escapeXML(graphics.Listen), graphics.Port)
	}
	for _, device := range vm.Passthrough {
		hostdevXML, err := passthroughDeviceXML(device)
		if err == nil {
			fmt.Fprintf(&builder, "    %s\n", hostdevXML)
		}
	}
	builder.WriteString("  </devices>\n</domain>\n")
	return builder.String()
}

func newDemoUUID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate demo UUID: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func demoMAC(index int) string {
	return fmt.Sprintf("52:54:00:01:00:%02x", index)
}

func demoVMs() []domain.VM {
	return []domain.VM{
		{
			UUID: "11111111-1111-4111-8111-111111111111", Name: "demo-web-01", State: domain.VMStateRunning,
			Persistent: true, Autostart: true, VCPUs: 4, MaxVCPUs: 8, MemoryBytes: 8 * 1024 * 1024 * 1024, MaxMemoryBytes: 12 * 1024 * 1024 * 1024,
			OSType: "hvm", Arch: "x86_64", Machine: "pc-q35-10.2", Firmware: domain.VMFirmwareBIOS,
			Disks:       []domain.Disk{{Device: "disk", Target: "vda", Bus: "virtio", Format: "qcow2", Source: "/var/lib/libvirt/images/demo-web-01.qcow2", CapacityBytes: 20 * 1024 * 1024 * 1024, AllocationBytes: 8 * 1024 * 1024 * 1024}},
			Interfaces:  []domain.NetworkInterface{{Type: "network", MAC: "52:54:00:01:00:01", Source: "default", Model: "virtio"}},
			Graphics:    []domain.Graphics{{Type: "vnc", Listen: "127.0.0.1", Port: 5901}},
			IPAddresses: []string{"192.168.122.101"}, Hostname: "demo-web-01", AgentConnected: true,
			AllowedActions: domain.AllowedVMActions(domain.VMStateRunning),
		},
		{
			UUID: "22222222-2222-4222-8222-222222222222", Name: "demo-db-01", State: domain.VMStateRunning,
			Persistent: true, Autostart: true, VCPUs: 8, MaxVCPUs: 16, MemoryBytes: 16 * 1024 * 1024 * 1024, MaxMemoryBytes: 24 * 1024 * 1024 * 1024,
			OSType: "hvm", Arch: "x86_64", Machine: "pc-q35-10.2", Firmware: domain.VMFirmwareUEFI, SecureBoot: true,
			Disks:       []domain.Disk{{Device: "disk", Target: "vda", Bus: "virtio", Format: "qcow2", Source: "/var/lib/libvirt/images/demo-db-01.qcow2", CapacityBytes: 80 * 1024 * 1024 * 1024, AllocationBytes: 42 * 1024 * 1024 * 1024}},
			Interfaces:  []domain.NetworkInterface{{Type: "network", MAC: "52:54:00:01:00:02", Source: "default", Model: "virtio"}},
			Graphics:    []domain.Graphics{{Type: "vnc", Listen: "127.0.0.1", Port: 5902}},
			IPAddresses: []string{"192.168.122.102"}, Hostname: "demo-db-01", AgentConnected: true,
			AllowedActions: domain.AllowedVMActions(domain.VMStateRunning),
		},
		{
			UUID: "33333333-3333-4333-8333-333333333333", Name: "demo-build-01", State: domain.VMStatePaused,
			Persistent: true, Autostart: false, VCPUs: 2, MaxVCPUs: 4, MemoryBytes: 4 * 1024 * 1024 * 1024, MaxMemoryBytes: 8 * 1024 * 1024 * 1024,
			OSType: "hvm", Arch: "x86_64", Machine: "pc-q35-10.2", Firmware: domain.VMFirmwareBIOS,
			Disks:          []domain.Disk{{Device: "disk", Target: "vda", Bus: "virtio", Format: "qcow2", Source: "/var/lib/libvirt/images/demo-build-01.qcow2", CapacityBytes: 30 * 1024 * 1024 * 1024, AllocationBytes: 12 * 1024 * 1024 * 1024}},
			Interfaces:     []domain.NetworkInterface{{Type: "network", MAC: "52:54:00:01:00:03", Source: "default", Model: "virtio"}},
			Graphics:       []domain.Graphics{{Type: "vnc", Listen: "127.0.0.1", Port: 5903}},
			IPAddresses:    []string{},
			AllowedActions: domain.AllowedVMActions(domain.VMStatePaused),
		},
		{
			UUID: "44444444-4444-4444-8444-444444444444", Name: "demo-legacy-01", State: domain.VMStateShutoff,
			Persistent: true, Autostart: false, VCPUs: 2, MaxVCPUs: 2, MemoryBytes: 2 * 1024 * 1024 * 1024, MaxMemoryBytes: 2 * 1024 * 1024 * 1024,
			OSType: "hvm", Arch: "x86_64", Machine: "pc-i440fx-10.2", Firmware: domain.VMFirmwareBIOS,
			Disks:          []domain.Disk{{Device: "disk", Target: "vda", Bus: "virtio", Format: "qcow2", Source: "/var/lib/libvirt/images/demo-legacy-01.qcow2", CapacityBytes: 20 * 1024 * 1024 * 1024, AllocationBytes: 30 * 1024 * 1024 * 1024}},
			Interfaces:     []domain.NetworkInterface{{Type: "network", MAC: "52:54:00:01:00:04", Source: "default", Model: "virtio"}},
			Graphics:       []domain.Graphics{{Type: "vnc", Listen: "127.0.0.1", Port: 5904}},
			IPAddresses:    []string{},
			AllowedActions: domain.AllowedVMActions(domain.VMStateShutoff),
		},
	}
}
