package libvirtbackend

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

func TestDemoBackendLifecycle(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	const uuid = "44444444-4444-4444-8444-444444444444"

	vm, err := backend.GetVM(ctx, uuid)
	if err != nil || vm.State != domain.VMStateShutoff {
		t.Fatalf("GetVM() = %+v, %v", vm, err)
	}
	if err := backend.PerformAction(ctx, uuid, domain.VMActionStart); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	vm, _ = backend.GetVM(ctx, uuid)
	if vm.State != domain.VMStateRunning || !domain.ContainsVMAction(vm.AllowedActions, domain.VMActionShutdown) {
		t.Fatalf("started vm = %+v", vm)
	}
	if err := backend.PerformAction(ctx, uuid, domain.VMActionPause); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	vm, _ = backend.GetVM(ctx, uuid)
	if vm.State != domain.VMStatePaused {
		t.Fatalf("paused vm = %+v", vm)
	}
	if err := backend.PerformAction(ctx, uuid, domain.VMActionResume); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if err := backend.PerformAction(ctx, uuid, domain.VMActionStart); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("Start() while running error = %v", err)
	}
	if err := backend.PerformAction(ctx, uuid, domain.VMActionShutdown); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	vm, _ = backend.GetVM(ctx, uuid)
	if vm.State != domain.VMStateShutoff {
		t.Fatalf("shutdown vm = %+v", vm)
	}
}

func TestDemoBackendNotFoundAndIsolation(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	if _, err := backend.GetVM(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetVM(missing) error = %v", err)
	}
	if err := backend.PerformAction(ctx, "missing", domain.VMActionStart); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("PerformAction(missing) error = %v", err)
	}

	vm, err := backend.GetVM(ctx, "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	vm.Name = "mutated"
	loaded, _ := backend.GetVM(ctx, vm.UUID)
	if loaded.Name == "mutated" {
		t.Fatal("GetVM returned the backend's internal value")
	}
}

func TestDemoBackendSetAutostart(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	const uuid = "33333333-3333-4333-8333-333333333333"

	vm, err := backend.GetVM(ctx, uuid)
	if err != nil || vm.Autostart {
		t.Fatalf("initial VM = %+v, %v", vm, err)
	}
	if err := backend.SetVMAutostart(ctx, uuid, true); err != nil {
		t.Fatalf("SetVMAutostart(true) error = %v", err)
	}
	vm, err = backend.GetVM(ctx, uuid)
	if err != nil || !vm.Autostart {
		t.Fatalf("enabled VM = %+v, %v", vm, err)
	}
	if err := backend.SetVMAutostart(ctx, uuid, false); err != nil {
		t.Fatalf("SetVMAutostart(false) error = %v", err)
	}
	vm, _ = backend.GetVM(ctx, uuid)
	if vm.Autostart {
		t.Fatalf("disabled VM = %+v", vm)
	}
	if err := backend.SetVMAutostart(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetVMAutostart(missing) error = %v", err)
	}
}

func TestDemoBackendUpdateVMResources(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	const runningUUID = "22222222-2222-4222-8222-222222222222"
	const shutoffUUID = "44444444-4444-4444-8444-444444444444"

	vm, err := backend.UpdateVMResources(ctx, runningUUID, domain.VMResourcesSpec{VCPUs: 10, MemoryMiB: 20 * 1024})
	if err != nil {
		t.Fatalf("UpdateVMResources(running) error = %v", err)
	}
	if vm.VCPUs != 10 || vm.MemoryBytes != 20<<30 || vm.MaxVCPUs != 16 || vm.MaxMemoryBytes != 24<<30 {
		t.Fatalf("running VM = %+v", vm)
	}
	if _, err := backend.UpdateVMResources(ctx, runningUUID, domain.VMResourcesSpec{VCPUs: 17, MemoryMiB: 16 * 1024}); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("running vCPU overflow error = %v", err)
	}
	if _, err := backend.UpdateVMResources(ctx, runningUUID, domain.VMResourcesSpec{VCPUs: 8, MemoryMiB: 25 * 1024}); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("running memory overflow error = %v", err)
	}

	vm, err = backend.UpdateVMResources(ctx, shutoffUUID, domain.VMResourcesSpec{VCPUs: 4, MemoryMiB: 4096})
	if err != nil {
		t.Fatalf("UpdateVMResources(shutoff) error = %v", err)
	}
	if vm.VCPUs != 4 || vm.MaxVCPUs != 4 || vm.MemoryBytes != 4<<30 || vm.MaxMemoryBytes != 4<<30 {
		t.Fatalf("shutoff VM = %+v", vm)
	}
	if _, err := backend.UpdateVMResources(ctx, "missing", domain.VMResourcesSpec{VCPUs: 2, MemoryMiB: 2048}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing VM error = %v", err)
	}
}

func TestDemoBackendVMXML(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	const uuid = "22222222-2222-4222-8222-222222222222"
	result, err := backend.GetVMXML(ctx, uuid, domain.VMXMLFormatConfig)
	if err != nil {
		t.Fatalf("GetVMXML() error = %v", err)
	}
	if result.UUID != uuid || result.Name != "demo-db-01" || result.Format != domain.VMXMLFormatConfig {
		t.Fatalf("GetVMXML() = %+v", result)
	}
	for _, fragment := range []string{"<domain type='kvm'>", "<name>demo-db-01</name>", "<uuid>" + uuid + "</uuid>", "/var/lib/libvirt/images/demo-db-01.qcow2", "<interface type='network'>"} {
		if !strings.Contains(result.XML, fragment) {
			t.Fatalf("XML missing %q: %s", fragment, result.XML)
		}
	}
	if _, err := backend.GetVMXML(ctx, "missing", domain.VMXMLFormatLive); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetVMXML(missing) error = %v", err)
	}
	if !strings.Contains(result.XML, "<os firmware='efi'>") || !strings.Contains(result.XML, "<loader secure='yes'/>") {
		t.Fatalf("UEFI Secure Boot XML missing: %s", result.XML)
	}

	updatedText := strings.Replace(result.XML, "<vcpu>8</vcpu>", "<vcpu>10</vcpu>", 1)
	updated, err := backend.DefineVMXML(ctx, uuid, updatedText)
	if err != nil {
		t.Fatalf("DefineVMXML() error = %v", err)
	}
	if updated.Format != domain.VMXMLFormatConfig || !strings.Contains(updated.XML, "<vcpu>10</vcpu>") {
		t.Fatalf("DefineVMXML() = %+v", updated)
	}
	config, err := backend.GetVMXML(ctx, uuid, domain.VMXMLFormatConfig)
	if err != nil || !strings.Contains(config.XML, "<vcpu>10</vcpu>") {
		t.Fatalf("updated config XML = %s, %v", config.XML, err)
	}
	live, err := backend.GetVMXML(ctx, uuid, domain.VMXMLFormatLive)
	if err != nil || strings.Contains(live.XML, "<vcpu>10</vcpu>") {
		t.Fatalf("live XML should remain active state: %s, %v", live.XML, err)
	}
	if _, err := backend.DefineVMXML(ctx, uuid, strings.Replace(updatedText, "<name>demo-db-01</name>", "<name>other</name>", 1)); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("mismatched DefineVMXML() error = %v", err)
	}
	if _, err := backend.DefineVMXML(ctx, "missing", updatedText); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing DefineVMXML() error = %v", err)
	}
}

func TestDemoBackendHostDevicePassthrough(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	const uuid = "44444444-4444-4444-8444-444444444444"
	const otherUUID = "11111111-1111-4111-8111-111111111111"
	const pci = "pci_0000_3b_00_0"
	const usb = "usb_device_046d_c52b_1_2"

	vm, err := backend.AttachHostDevice(ctx, uuid, pci)
	if err != nil {
		t.Fatalf("AttachHostDevice(PCI) error = %v", err)
	}
	if len(vm.Passthrough) != 1 || vm.Passthrough[0].Name != pci || vm.Passthrough[0].Type != "pci" || vm.Passthrough[0].Product != "GA102 [GeForce RTX 3090]" {
		t.Fatalf("PCI passthrough = %+v", vm.Passthrough)
	}
	xmlResult, err := backend.GetVMXML(ctx, uuid, domain.VMXMLFormatConfig)
	if err != nil || !strings.Contains(xmlResult.XML, "type='pci'") || !strings.Contains(xmlResult.XML, "bus='0x3b'") {
		t.Fatalf("passthrough XML = %s, %v", xmlResult.XML, err)
	}
	if _, err := backend.AttachHostDevice(ctx, uuid, pci); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate attach error = %v", err)
	}
	if _, err := backend.AttachHostDevice(ctx, otherUUID, pci); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("cross-VM attach error = %v", err)
	}
	vm, err = backend.AttachHostDevice(ctx, uuid, usb)
	if err != nil || len(vm.Passthrough) != 2 || vm.Passthrough[1].Type != "usb" {
		t.Fatalf("USB passthrough = %+v, %v", vm.Passthrough, err)
	}
	vm, err = backend.DetachHostDevice(ctx, uuid, pci)
	if err != nil || len(vm.Passthrough) != 1 || vm.Passthrough[0].Name != usb {
		t.Fatalf("detach result = %+v, %v", vm.Passthrough, err)
	}
	if _, err := backend.DetachHostDevice(ctx, uuid, pci); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing detach error = %v", err)
	}
	if _, err := backend.AttachHostDevice(ctx, uuid, "net_enp2s0"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("unsupported device error = %v", err)
	}
}

func TestDemoBackendCreateAndDelete(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	spec := domain.VMCreateSpec{
		Name: "demo-created", VCPUs: 2, MemoryMiB: 2048,
		DiskPath: "/var/lib/libvirt/images/demo-created.qcow2", DiskFormat: "qcow2",
		Network: "default", ISOPath: "/var/lib/libvirt/images/install.iso",
		Firmware: domain.VMFirmwareUEFI, SecureBoot: true, Start: true,
		CloudInit: &domain.CloudInitSpec{UserData: "#cloud-config\n", Pool: "default"},
	}
	vm, err := backend.CreateVM(ctx, spec)
	if err != nil {
		t.Fatalf("CreateVM() error = %v", err)
	}
	if vm.Name != spec.Name || vm.State != domain.VMStateRunning || len(vm.Disks) != 3 || vm.Disks[1].Device != "cdrom" || !vm.CloudInit || vm.Firmware != domain.VMFirmwareUEFI || !vm.SecureBoot {
		t.Fatalf("created vm = %+v", vm)
	}
	if _, err := backend.CreateVM(ctx, domain.VMCreateSpec{
		Name: "demo-invalid-firmware", VCPUs: 2, MemoryMiB: 2048,
		DiskPath: "/var/lib/libvirt/images/invalid.qcow2", DiskFormat: "qcow2", Network: "default",
		SecureBoot: true,
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid firmware CreateVM() error = %v", err)
	}
	if _, err := backend.CreateVM(ctx, spec); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate CreateVM() error = %v", err)
	}
	if err := backend.DeleteVM(ctx, vm.UUID); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("DeleteVM(running) error = %v", err)
	}
	if err := backend.PerformAction(ctx, vm.UUID, domain.VMActionDestroy); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
	if err := backend.DeleteVM(ctx, vm.UUID); err != nil {
		t.Fatalf("DeleteVM() error = %v", err)
	}
	if _, err := backend.GetVM(ctx, vm.UUID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted GetVM() error = %v", err)
	}
}

func TestDemoBackendStorage(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	pools, err := backend.ListStoragePools(ctx)
	if err != nil || len(pools) != 3 {
		t.Fatalf("ListStoragePools() = %+v, %v", pools, err)
	}
	defaultPool := storagePoolByName(t, pools, "default")
	if defaultPool.VolumeCount != 4 {
		t.Fatalf("default pool = %+v", defaultPool)
	}
	volumes, err := backend.ListStorageVolumes(ctx, "default")
	if err != nil || len(volumes) != 4 || volumes[0].Format != "qcow2" {
		t.Fatalf("ListStorageVolumes(default) = %+v, %v", volumes, err)
	}
	if _, err := backend.ListStorageVolumes(ctx, "archive"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("inactive pool error = %v", err)
	}
	if _, err := backend.ListStorageVolumes(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing pool error = %v", err)
	}

	created, err := backend.CreateStorageVolume(ctx, "default", domain.StorageVolumeCreateSpec{
		Name: "demo-e2e-01.qcow2", Format: "qcow2", CapacityBytes: 10 << 30,
	})
	if err != nil {
		t.Fatalf("CreateStorageVolume() error = %v", err)
	}
	if created.Path != "/var/lib/libvirt/images/demo-e2e-01.qcow2" || created.CapacityBytes != 10<<30 {
		t.Fatalf("created volume = %+v", created)
	}
	volumes, err = backend.ListStorageVolumes(ctx, "default")
	if err != nil || len(volumes) != 5 {
		t.Fatalf("volumes after create = %+v, %v", volumes, err)
	}
	pools, _ = backend.ListStoragePools(ctx)
	defaultPool = storagePoolByName(t, pools, "default")
	if defaultPool.VolumeCount != 5 {
		t.Fatalf("pool volume count = %d", defaultPool.VolumeCount)
	}
	if _, err := backend.CreateStorageVolume(ctx, "default", domain.StorageVolumeCreateSpec{
		Name: "demo-e2e-01.qcow2", Format: "qcow2", CapacityBytes: 10 << 30,
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate create error = %v", err)
	}
	if _, err := backend.CreateStorageVolume(ctx, "archive", domain.StorageVolumeCreateSpec{
		Name: "archived.qcow2", Format: "qcow2", CapacityBytes: 10 << 30,
	}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("inactive create error = %v", err)
	}
	if _, err := backend.CreateStorageVolume(ctx, "missing", domain.StorageVolumeCreateSpec{
		Name: "missing.qcow2", Format: "qcow2", CapacityBytes: 10 << 30,
	}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing pool create error = %v", err)
	}
	if err := backend.DeleteStorageVolume(ctx, "default", "demo-web-01.qcow2"); !errors.Is(err, domain.ErrVolumeInUse) {
		t.Fatalf("attached volume delete error = %v", err)
	}
	if err := backend.DeleteStorageVolume(ctx, "default", "demo-e2e-01.qcow2"); err != nil {
		t.Fatalf("DeleteStorageVolume() error = %v", err)
	}
	volumes, err = backend.ListStorageVolumes(ctx, "default")
	if err != nil || len(volumes) != 4 {
		t.Fatalf("volumes after delete = %+v, %v", volumes, err)
	}
	if err := backend.DeleteStorageVolume(ctx, "default", "demo-e2e-01.qcow2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("duplicate delete error = %v", err)
	}
	if err := backend.DeleteStorageVolume(ctx, "archive", "archived.qcow2"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("inactive pool delete error = %v", err)
	}
}

func storagePoolByName(t *testing.T, pools []domain.StoragePool, name string) domain.StoragePool {
	t.Helper()
	for _, pool := range pools {
		if pool.Name == name {
			return pool
		}
	}
	t.Fatalf("storage pool %s not found in %+v", name, pools)
	return domain.StoragePool{}
}

func TestDemoBackendUploadStorageVolume(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	size := uint64(1 << 20)
	spec := domain.StorageVolumeCreateSpec{Name: "uploaded.iso", Format: "raw", CapacityBytes: size}
	volume, err := backend.UploadStorageVolume(ctx, "iso-store", spec, bytes.NewReader(make([]byte, size)))
	if err != nil {
		t.Fatalf("UploadStorageVolume() error = %v", err)
	}
	if volume.Name != spec.Name || volume.Path != "/var/lib/libvirt/iso/uploaded.iso" || volume.CapacityBytes != size || volume.AllocationBytes != size {
		t.Fatalf("uploaded volume = %+v", volume)
	}
	volumes, err := backend.ListStorageVolumes(ctx, "iso-store")
	if err != nil || len(volumes) != 3 {
		t.Fatalf("volumes after upload = %+v, %v", volumes, err)
	}
	if _, err := backend.UploadStorageVolume(ctx, "iso-store", spec, bytes.NewReader(make([]byte, size))); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate upload error = %v", err)
	}
	truncated := domain.StorageVolumeCreateSpec{Name: "truncated.iso", Format: "raw", CapacityBytes: size}
	if _, err := backend.UploadStorageVolume(ctx, "iso-store", truncated, bytes.NewReader(make([]byte, 32))); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("truncated upload error = %v", err)
	}
	if _, err := backend.UploadStorageVolume(ctx, "archive", spec, bytes.NewReader(make([]byte, size))); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("inactive pool upload error = %v", err)
	}
	if _, err := backend.UploadStorageVolume(ctx, "missing", spec, bytes.NewReader(make([]byte, size))); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing pool upload error = %v", err)
	}
}

func TestDemoBackendHotplugLifecycle(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	const uuid = "11111111-1111-4111-8111-111111111111"
	before, err := backend.GetVM(ctx, uuid)
	if err != nil {
		t.Fatalf("GetVM() error = %v", err)
	}
	if _, err := backend.CreateStorageVolume(ctx, "default", domain.StorageVolumeCreateSpec{
		Name: "hotplug-test.qcow2", Format: "qcow2", CapacityBytes: 5 << 30,
	}); err != nil {
		t.Fatalf("CreateStorageVolume() error = %v", err)
	}
	attached, err := backend.AttachVMDisk(ctx, uuid, domain.VMDiskAttachSpec{Pool: "default", Volume: "hotplug-test.qcow2"})
	if err != nil {
		t.Fatalf("AttachVMDisk() error = %v", err)
	}
	if len(attached.Disks) != len(before.Disks)+1 {
		t.Fatalf("attached disks = %+v", attached.Disks)
	}
	target := attached.Disks[len(attached.Disks)-1].Target
	if _, err := backend.DetachVMDisk(ctx, uuid, before.Disks[0].Target); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("detach primary disk error = %v", err)
	}
	detached, err := backend.DetachVMDisk(ctx, uuid, target)
	if err != nil || len(detached.Disks) != len(before.Disks) {
		t.Fatalf("DetachVMDisk() = %+v, %v", detached.Disks, err)
	}

	withNetwork, err := backend.AttachVMNetwork(ctx, uuid, domain.VMNetworkAttachSpec{Network: "default", Model: domain.VMNICModelVirtio})
	if err != nil || len(withNetwork.Interfaces) != len(before.Interfaces)+1 {
		t.Fatalf("AttachVMNetwork() = %+v, %v", withNetwork.Interfaces, err)
	}
	mac := withNetwork.Interfaces[len(withNetwork.Interfaces)-1].MAC
	withoutNetwork, err := backend.DetachVMNetwork(ctx, uuid, mac)
	if err != nil || len(withoutNetwork.Interfaces) != len(before.Interfaces) {
		t.Fatalf("DetachVMNetwork() = %+v, %v", withoutNetwork.Interfaces, err)
	}
	if _, err := backend.DetachVMNetwork(ctx, uuid, before.Interfaces[0].MAC); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("detach last network error = %v", err)
	}
}

func TestDemoBackendSnapshotLifecycle(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	const uuid = "44444444-4444-4444-8444-444444444444"

	snapshots, err := backend.ListSnapshots(ctx, uuid)
	if err != nil || len(snapshots) != 0 {
		t.Fatalf("initial snapshots = %+v, %v", snapshots, err)
	}
	baseline, err := backend.CreateSnapshot(ctx, uuid, domain.VMSnapshotCreateSpec{Name: "baseline", Description: "clean state"})
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	if baseline.Name != "baseline" || baseline.State != domain.VMStateShutoff || !baseline.Current || baseline.CreatedAt == 0 {
		t.Fatalf("baseline = %+v", baseline)
	}
	if _, err := backend.CreateSnapshot(ctx, uuid, domain.VMSnapshotCreateSpec{Name: "baseline"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate snapshot error = %v", err)
	}
	if err := backend.PerformAction(ctx, uuid, domain.VMActionStart); err != nil {
		t.Fatalf("start VM error = %v", err)
	}
	runningSnapshot, err := backend.CreateSnapshot(ctx, uuid, domain.VMSnapshotCreateSpec{Name: "running"})
	if err != nil || runningSnapshot.State != domain.VMStateRunning || !runningSnapshot.Current {
		t.Fatalf("running snapshot = %+v, %v", runningSnapshot, err)
	}
	if err := backend.PerformAction(ctx, uuid, domain.VMActionDestroy); err != nil {
		t.Fatalf("destroy VM error = %v", err)
	}
	if err := backend.RevertSnapshot(ctx, uuid, "running"); err != nil {
		t.Fatalf("RevertSnapshot() error = %v", err)
	}
	vm, err := backend.GetVM(ctx, uuid)
	if err != nil || vm.State != domain.VMStateRunning {
		t.Fatalf("reverted VM = %+v, %v", vm, err)
	}
	snapshots, err = backend.ListSnapshots(ctx, uuid)
	if err != nil || len(snapshots) != 2 {
		t.Fatalf("snapshots after revert = %+v, %v", snapshots, err)
	}
	for _, snapshot := range snapshots {
		if snapshot.Name == "running" && !snapshot.Current {
			t.Fatalf("running snapshot is not current: %+v", snapshots)
		}
		if snapshot.Name == "baseline" && snapshot.Current {
			t.Fatalf("baseline snapshot is current: %+v", snapshots)
		}
	}
	if err := backend.DeleteSnapshot(ctx, uuid, "baseline"); err != nil {
		t.Fatalf("DeleteSnapshot() error = %v", err)
	}
	if err := backend.RevertSnapshot(ctx, uuid, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing revert error = %v", err)
	}
	if err := backend.DeleteSnapshot(ctx, uuid, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing delete error = %v", err)
	}
}

func TestDemoBackendNetworks(t *testing.T) {
	networks, err := NewDemo().ListNetworks(context.Background())
	if err != nil || len(networks) != 3 {
		t.Fatalf("ListNetworks() = %+v, %v", networks, err)
	}
	var defaultNetwork, isolatedNetwork *domain.VirtualNetwork
	for i := range networks {
		switch networks[i].Name {
		case "default":
			defaultNetwork = &networks[i]
		case "lab-isolated":
			isolatedNetwork = &networks[i]
		}
	}
	if defaultNetwork == nil || defaultNetwork.Bridge != "virbr0" || !defaultNetwork.Active {
		t.Fatalf("default network = %+v", defaultNetwork)
	}
	if len(defaultNetwork.Addresses) != 1 || defaultNetwork.Addresses[0].DHCPStart != "192.168.122.2" {
		t.Fatalf("default addresses = %+v", defaultNetwork.Addresses)
	}
	if isolatedNetwork == nil || isolatedNetwork.Active {
		t.Fatalf("inactive network = %+v", isolatedNetwork)
	}
	for _, network := range networks {
		if network.Addresses == nil {
			t.Fatalf("network %s returned nil addresses", network.Name)
		}
	}
}

func TestDemoBackendStoragePoolLifecycle(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	pool, err := backend.CreateStoragePool(ctx, domain.StoragePoolCreateSpec{
		Name: "demo-pool-01", Type: "dir", TargetPath: "/srv/virtmgr/demo-pool-01", Autostart: true,
	})
	if err != nil {
		t.Fatalf("CreateStoragePool() error = %v", err)
	}
	if pool.Name != "demo-pool-01" || pool.State != domain.StoragePoolStateInactive || !pool.Autostart || pool.TargetPath != "/srv/virtmgr/demo-pool-01" {
		t.Fatalf("created pool = %+v", pool)
	}
	if _, err := backend.CreateStoragePool(ctx, domain.StoragePoolCreateSpec{Name: pool.Name, TargetPath: pool.TargetPath}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate create error = %v", err)
	}
	if err := backend.PerformStoragePoolAction(ctx, pool.Name, domain.StoragePoolActionStart); err != nil {
		t.Fatalf("start pool error = %v", err)
	}
	started, err := backend.GetStoragePool(ctx, pool.Name)
	if err != nil || started.State != domain.StoragePoolStateActive || started.CapacityBytes == 0 {
		t.Fatalf("started pool = %+v, %v", started, err)
	}
	if err := backend.PerformStoragePoolAction(ctx, pool.Name, domain.StoragePoolActionStart); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate start error = %v", err)
	}
	if err := backend.PerformStoragePoolAction(ctx, pool.Name, domain.StoragePoolActionRefresh); err != nil {
		t.Fatalf("refresh pool error = %v", err)
	}
	volume, err := backend.CreateStorageVolume(ctx, pool.Name, domain.StorageVolumeCreateSpec{Name: "data.qcow2", Format: "qcow2", CapacityBytes: 1 << 30})
	if err != nil || volume.Path != "/srv/virtmgr/demo-pool-01/data.qcow2" {
		t.Fatalf("volume in created pool = %+v, %v", volume, err)
	}
	if err := backend.DeleteStoragePool(ctx, pool.Name); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("delete active pool error = %v", err)
	}
	if err := backend.PerformStoragePoolAction(ctx, pool.Name, domain.StoragePoolActionStop); err != nil {
		t.Fatalf("stop pool error = %v", err)
	}
	if _, err := backend.ListStorageVolumes(ctx, pool.Name); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("inactive pool volumes error = %v", err)
	}
	if err := backend.DeleteStoragePool(ctx, pool.Name); err != nil {
		t.Fatalf("delete pool error = %v", err)
	}
	if _, err := backend.GetStoragePool(ctx, pool.Name); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted pool lookup error = %v", err)
	}
}

func TestDemoBackendNetworkLifecycle(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	definition := domain.VirtualNetworkDefinition{
		Name: "demo-net-01", Mode: "isolated", IPv4Address: "10.77.0.1", Prefix: 24,
		DHCPStart: "10.77.0.2", DHCPEnd: "10.77.0.254", Autostart: true,
	}
	network, err := backend.CreateNetwork(ctx, definition)
	if err != nil {
		t.Fatalf("CreateNetwork() error = %v", err)
	}
	if network.Name != "demo-net-01" || network.Active || network.ForwardMode != "isolated" || len(network.Addresses) != 1 {
		t.Fatalf("created network = %+v", network)
	}
	if _, err := backend.CreateNetwork(ctx, definition); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate create error = %v", err)
	}
	updated, err := backend.UpdateNetwork(ctx, domain.VirtualNetworkDefinition{
		Name: network.Name, Mode: "nat", IPv4Address: "10.78.0.1", Prefix: 24,
		DHCPStart: "10.78.0.10", DHCPEnd: "10.78.0.100", Autostart: true,
	})
	if err != nil {
		t.Fatalf("UpdateNetwork() error = %v", err)
	}
	if updated.ForwardMode != "nat" || !updated.Autostart || len(updated.Addresses) != 1 || updated.Addresses[0].Address != "10.78.0.1" {
		t.Fatalf("updated network = %+v", updated)
	}
	host := domain.NetworkDHCPHost{MAC: "52:54:00:aa:bb:cc", IP: "10.78.0.50", Name: "demo-host"}
	if _, err := backend.AddNetworkDHCPHost(ctx, network.Name, host); err != nil {
		t.Fatalf("AddNetworkDHCPHost() error = %v", err)
	}
	updated, err = backend.UpdateNetwork(ctx, domain.VirtualNetworkDefinition{
		Name: network.Name, Mode: "nat", IPv4Address: "10.78.0.1", Prefix: 24,
		DHCPStart: "10.78.0.10", DHCPEnd: "10.78.0.100", Autostart: true,
	})
	if err != nil || len(updated.Addresses) != 1 || len(updated.Addresses[0].Hosts) != 1 || updated.Addresses[0].Hosts[0].MAC != host.MAC {
		t.Fatalf("updated network DHCP hosts = %+v, %v", updated, err)
	}
	if err := backend.PerformNetworkAction(ctx, network.Name, domain.VirtualNetworkActionStart); err != nil {
		t.Fatalf("start network error = %v", err)
	}
	activeHost := domain.NetworkDHCPHost{MAC: "52:54:00:aa:bb:cd", IP: "10.78.0.51", Name: "active-host"}
	activeUpdated, err := backend.AddNetworkDHCPHost(ctx, network.Name, activeHost)
	if err != nil || len(activeUpdated.Addresses[0].Hosts) != 2 {
		t.Fatalf("active AddNetworkDHCPHost() = %+v, %v", activeUpdated, err)
	}
	activeUpdated, err = backend.DeleteNetworkDHCPHost(ctx, network.Name, activeHost)
	if err != nil || len(activeUpdated.Addresses[0].Hosts) != 1 {
		t.Fatalf("active DeleteNetworkDHCPHost() = %+v, %v", activeUpdated, err)
	}
	if _, err := backend.UpdateNetwork(ctx, definition); !errors.Is(err, domain.ErrNetworkActive) {
		t.Fatalf("active update error = %v", err)
	}
	networks, err := backend.ListNetworks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var started *domain.VirtualNetwork
	for i := range networks {
		if networks[i].Name == network.Name {
			started = &networks[i]
		}
	}
	if started == nil || !started.Active {
		t.Fatalf("started network = %+v", started)
	}
	if err := backend.PerformNetworkAction(ctx, network.Name, domain.VirtualNetworkActionStart); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate start error = %v", err)
	}
	if err := backend.DeleteNetwork(ctx, network.Name); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("delete active network error = %v", err)
	}
	if err := backend.PerformNetworkAction(ctx, network.Name, domain.VirtualNetworkActionStop); err != nil {
		t.Fatalf("stop network error = %v", err)
	}
	if err := backend.DeleteNetwork(ctx, network.Name); err != nil {
		t.Fatalf("delete network error = %v", err)
	}
	networks, err = backend.ListNetworks(ctx)
	if err != nil || len(networks) != 3 {
		t.Fatalf("networks after delete = %+v, %v", networks, err)
	}
	if err := backend.DeleteNetwork(ctx, network.Name); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("duplicate delete error = %v", err)
	}
}

func TestDemoBackendDHCPLeases(t *testing.T) {
	backend := NewDemo()
	leases, err := backend.ListDHCPLeases(context.Background(), "default")
	if err != nil || len(leases) != 2 {
		t.Fatalf("ListDHCPLeases(default) = %+v, %v", leases, err)
	}
	if leases[0].Hostname != "demo-web-01" || leases[0].IPAddress != "192.168.122.101" || leases[0].ExpiresAt == nil {
		t.Fatalf("first lease = %+v", leases[0])
	}
	inactive, err := backend.ListDHCPLeases(context.Background(), "lab-isolated")
	if err != nil || len(inactive) != 0 {
		t.Fatalf("inactive leases = %+v, %v", inactive, err)
	}
	if _, err := backend.ListDHCPLeases(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing network error = %v", err)
	}
}
