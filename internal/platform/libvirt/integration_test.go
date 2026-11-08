package libvirtbackend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestRealLibvirtReadOnly(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	host, err := client.Host(ctx)
	if err != nil || host.CPUs == 0 || host.MemoryBytes == 0 {
		t.Fatalf("Host() = %+v, %v", host, err)
	}
	hostMetrics, err := client.HostMetrics(ctx)
	if err != nil || hostMetrics.CPUCapacity == 0 || hostMetrics.MemoryTotalBytes == 0 {
		t.Fatalf("HostMetrics() = %+v, %v", hostMetrics, err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		devices, err := client.ListHostDevices(ctx)
		if err != nil {
			t.Fatalf("ListHostDevices() attempt %d error = %v", attempt+1, err)
		}
		for _, device := range devices {
			if device.Name == "" || device.Type == "" {
				t.Fatalf("invalid host device: %+v", device)
			}
		}
	}
	vms, err := client.ListVMs(ctx)
	if err != nil {
		t.Fatalf("ListVMs() error = %v", err)
	}
	for _, vm := range vms {
		if vm.UUID == "" || vm.Name == "" {
			t.Fatalf("invalid vm: %+v", vm)
		}
	}
	if len(vms) > 0 {
		vm, err := client.GetVM(ctx, vms[0].UUID)
		if err != nil {
			t.Fatalf("GetVM() error = %v", err)
		}
		if vm.UUID != vms[0].UUID || vm.Name != vms[0].Name {
			t.Fatalf("GetVM() = %+v, want UUID %s", vm, vms[0].UUID)
		}
		vmMetrics, err := client.VMMetrics(ctx, vm.UUID)
		if err != nil || vmMetrics.MemoryTotalBytes == 0 {
			t.Fatalf("VMMetrics() = %+v, %v", vmMetrics, err)
		}
	}
	pools, err := client.ListStoragePools(ctx)
	if err != nil {
		t.Fatalf("ListStoragePools() error = %v", err)
	}
	for _, pool := range pools {
		if pool.Name == "" {
			t.Fatalf("invalid storage pool: %+v", pool)
		}
		if pool.State == "active" {
			volumes, err := client.ListStorageVolumes(ctx, pool.Name)
			if err != nil {
				t.Fatalf("ListStorageVolumes(%s) error = %v", pool.Name, err)
			}
			for _, volume := range volumes {
				if volume.Name == "" {
					t.Fatalf("invalid volume in pool %s: %+v", pool.Name, volume)
				}
			}
			break
		}
	}
	networks, err := client.ListNetworks(ctx)
	if err != nil {
		t.Fatalf("ListNetworks() error = %v", err)
	}
	for _, network := range networks {
		if network.Name == "" {
			t.Fatalf("invalid network: %+v", network)
		}
		if network.Active {
			leases, err := client.ListDHCPLeases(ctx, network.Name)
			if err != nil {
				t.Fatalf("ListDHCPLeases(%s) error = %v", network.Name, err)
			}
			for _, lease := range leases {
				if lease.IPAddress == "" || lease.Family == "" {
					t.Fatalf("invalid DHCP lease in network %s: %+v", network.Name, lease)
				}
			}
		}
	}
}

func TestRealLibvirtCloudInitCreate(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolName := "virtmgr-it-" + suffix
	poolDir := t.TempDir()
	pool, err := client.CreateStoragePool(ctx, domain.StoragePoolCreateSpec{
		Name: poolName, Type: "dir", TargetPath: poolDir, Start: true,
	})
	if err != nil {
		t.Fatalf("CreateStoragePool() error = %v", err)
	}
	defer func() {
		_ = client.PerformStoragePoolAction(ctx, pool.Name, domain.StoragePoolActionStop)
		_ = client.DeleteStoragePool(ctx, pool.Name)
	}()
	vm, err := client.CreateVM(ctx, domain.VMCreateSpec{
		Name: "virtmgr-cloudinit-it-" + suffix, VCPUs: 1, MemoryMiB: 128,
		DiskPath: filepath.Join(poolDir, "disk.qcow2"), DiskFormat: "qcow2", Network: "default",
		Firmware: domain.VMFirmwareBIOS,
		CloudInit: &domain.CloudInitSpec{
			UserData: "#cloud-config\nruncmd:\n  - echo integration-test\n", Pool: pool.Name,
		},
	})
	if err != nil {
		t.Fatalf("CreateVM() error = %v", err)
	}
	defer func() { _ = client.DeleteVM(ctx, vm.UUID) }()
	if !vm.CloudInit {
		t.Fatalf("created VM cloudInit = false: %+v", vm)
	}
	xmlResult, err := client.GetVMXML(ctx, vm.UUID, domain.VMXMLFormatConfig)
	if err != nil {
		t.Fatalf("GetVMXML() error = %v", err)
	}
	if !strings.Contains(xmlResult.XML, "<serial>cloud-init</serial>") || !strings.Contains(xmlResult.XML, "-cidata.iso") {
		t.Fatalf("domain XML missing cloud-init seed: %s", xmlResult.XML)
	}
	volumes, err := client.ListStorageVolumes(ctx, pool.Name)
	if err != nil {
		t.Fatalf("ListStorageVolumes() error = %v", err)
	}
	foundSeed := false
	for _, volume := range volumes {
		if volume.Name == cloudInitSeedVolumeName(vm.Name) {
			foundSeed = true
			break
		}
	}
	if !foundSeed {
		t.Fatalf("cloud-init seed volume not found: %+v", volumes)
	}
}

func TestRealLibvirtUpdateNetwork(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	name := "virtmgr-net-update-it-" + suffix
	created, err := client.CreateNetwork(ctx, domain.VirtualNetworkDefinition{
		Name: name, Mode: "isolated", IPv4Address: "10.211.0.1", Prefix: 24,
		DHCPStart: "10.211.0.2", DHCPEnd: "10.211.0.254",
	})
	if err != nil {
		t.Fatalf("CreateNetwork() error = %v", err)
	}
	defer func() {
		_ = client.PerformNetworkAction(ctx, created.Name, domain.VirtualNetworkActionStop)
		_ = client.DeleteNetwork(ctx, created.Name)
	}()
	updated, err := client.UpdateNetwork(ctx, domain.VirtualNetworkDefinition{
		Name: name, Mode: "nat", IPv4Address: "10.212.0.1", Prefix: 24,
		DHCPStart: "10.212.0.20", DHCPEnd: "10.212.0.120", Autostart: true,
	})
	if err != nil {
		t.Fatalf("UpdateNetwork() error = %v", err)
	}
	if updated.Name != name || updated.Active || !updated.Autostart || updated.ForwardMode != "nat" || len(updated.Addresses) == 0 {
		t.Fatalf("updated network = %+v", updated)
	}
	address := updated.Addresses[0]
	if address.Address != "10.212.0.1" || address.Prefix != 24 || address.DHCPStart != "10.212.0.20" || address.DHCPEnd != "10.212.0.120" {
		t.Fatalf("updated address = %+v", address)
	}
}

func TestRealLibvirtActiveNetworkDHCPHost(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	name := "virtmgr-dhcp-it-" + suffix
	created, err := client.CreateNetwork(ctx, domain.VirtualNetworkDefinition{
		Name: name, Mode: "isolated", IPv4Address: "10.213.0.1", Prefix: 24,
		DHCPStart: "10.213.0.2", DHCPEnd: "10.213.0.254", Start: true,
	})
	if err != nil {
		t.Fatalf("CreateNetwork() error = %v", err)
	}
	defer func() {
		_ = client.PerformNetworkAction(ctx, created.Name, domain.VirtualNetworkActionStop)
		_ = client.DeleteNetwork(ctx, created.Name)
	}()
	host := domain.NetworkDHCPHost{MAC: "52:54:00:de:ad:01", IP: "10.213.0.50", Name: "probe-host"}
	updated, err := client.AddNetworkDHCPHost(ctx, created.Name, host)
	if err != nil {
		t.Fatalf("AddNetworkDHCPHost() error = %v", err)
	}
	if !updated.Active || len(updated.Addresses) == 0 || !containsDHCPHost(updated.Addresses[0].Hosts, host) {
		t.Fatalf("updated network = %+v", updated)
	}
	updated, err = client.DeleteNetworkDHCPHost(ctx, created.Name, host)
	if err != nil {
		t.Fatalf("DeleteNetworkDHCPHost() error = %v", err)
	}
	if !updated.Active || len(updated.Addresses) == 0 || containsDHCPHost(updated.Addresses[0].Hosts, host) {
		t.Fatalf("network after DHCP host delete = %+v", updated)
	}
}

func containsDHCPHost(hosts []domain.NetworkDHCPHost, want domain.NetworkDHCPHost) bool {
	for _, host := range hosts {
		if strings.EqualFold(host.MAC, want.MAC) && host.IP == want.IP && host.Name == want.Name {
			return true
		}
	}
	return false
}

func TestRealLibvirtUpdateVMResources(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolName := "virtmgr-resources-it-" + suffix
	poolDir := t.TempDir()
	pool, err := client.CreateStoragePool(ctx, domain.StoragePoolCreateSpec{
		Name: poolName, Type: "dir", TargetPath: poolDir, Start: true,
	})
	if err != nil {
		t.Fatalf("CreateStoragePool() error = %v", err)
	}
	defer func() {
		_ = client.PerformStoragePoolAction(ctx, pool.Name, domain.StoragePoolActionStop)
		_ = client.DeleteStoragePool(ctx, pool.Name)
	}()
	vm, err := client.CreateVM(ctx, domain.VMCreateSpec{
		Name: "virtmgr-resources-it-" + suffix, VCPUs: 1, MemoryMiB: 128,
		DiskPath: filepath.Join(poolDir, "disk.qcow2"), DiskFormat: "qcow2", Network: "default",
		Firmware: domain.VMFirmwareBIOS,
	})
	if err != nil {
		t.Fatalf("CreateVM() error = %v", err)
	}
	defer func() { _ = client.DeleteVM(ctx, vm.UUID) }()
	if vm.State != domain.VMStateShutoff {
		t.Fatalf("created VM state = %s", vm.State)
	}
	updated, err := client.UpdateVMResources(ctx, vm.UUID, domain.VMResourcesSpec{VCPUs: 2, MemoryMiB: 192})
	if err != nil {
		t.Fatalf("UpdateVMResources() error = %v", err)
	}
	if updated.VCPUs != 2 || updated.MaxVCPUs < 2 || updated.MemoryBytes != 192<<20 || updated.MaxMemoryBytes < 192<<20 {
		t.Fatalf("updated VM = %+v", updated)
	}
	xmlResult, err := client.GetVMXML(ctx, vm.UUID, domain.VMXMLFormatConfig)
	if err != nil {
		t.Fatalf("GetVMXML() error = %v", err)
	}
	if !strings.Contains(xmlResult.XML, "<vcpu") || !strings.Contains(xmlResult.XML, ">2</vcpu>") || !strings.Contains(xmlResult.XML, "196608") {
		t.Fatalf("updated config XML = %s", xmlResult.XML)
	}
}

func TestRealLibvirtResizeVMDisk(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolName := "virtmgr-disk-resize-it-" + suffix
	poolDir := t.TempDir()
	pool, err := client.CreateStoragePool(ctx, domain.StoragePoolCreateSpec{
		Name: poolName, Type: "dir", TargetPath: poolDir, Start: true,
	})
	if err != nil {
		t.Fatalf("CreateStoragePool() error = %v", err)
	}
	defer func() {
		_ = client.PerformStoragePoolAction(ctx, pool.Name, domain.StoragePoolActionStop)
		_ = client.DeleteStoragePool(ctx, pool.Name)
	}()
	volume, err := client.CreateStorageVolume(ctx, pool.Name, domain.StorageVolumeCreateSpec{
		Name: "disk.qcow2", Format: "qcow2", CapacityBytes: 64 << 20,
	})
	if err != nil {
		t.Fatalf("CreateStorageVolume() error = %v", err)
	}
	vm, err := client.CreateVM(ctx, domain.VMCreateSpec{
		Name: "virtmgr-disk-resize-it-" + suffix, VCPUs: 1, MemoryMiB: 128,
		DiskPath: volume.Path, DiskFormat: "qcow2", Network: "default", Firmware: domain.VMFirmwareBIOS,
	})
	if err != nil {
		t.Fatalf("CreateVM() error = %v", err)
	}
	defer func() { _ = client.DeleteVM(ctx, vm.UUID) }()
	loaded, err := client.GetVM(ctx, vm.UUID)
	if err != nil {
		t.Fatalf("GetVM() error = %v", err)
	}
	if len(loaded.Disks) == 0 || loaded.Disks[0].CapacityBytes != 64<<20 {
		t.Fatalf("loaded VM disks = %+v", loaded.Disks)
	}

	updated, err := client.ResizeVMDisk(ctx, vm.UUID, "vda", domain.VMDiskResizeSpec{CapacityBytes: 96 << 20})
	if err != nil {
		t.Fatalf("ResizeVMDisk() error = %v", err)
	}
	if len(updated.Disks) == 0 || updated.Disks[0].CapacityBytes != 96<<20 {
		t.Fatalf("updated VM disks = %+v", updated.Disks)
	}
	volumes, err := client.ListStorageVolumes(ctx, pool.Name)
	if err != nil {
		t.Fatalf("ListStorageVolumes() error = %v", err)
	}
	if len(volumes) != 1 || volumes[0].CapacityBytes != 96<<20 {
		t.Fatalf("resized volumes = %+v", volumes)
	}
	if _, err := client.ResizeVMDisk(ctx, vm.UUID, "vda", domain.VMDiskResizeSpec{CapacityBytes: 80 << 20}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("shrink error = %v", err)
	}
}

func TestRealLibvirtResizeUnmanagedFileDisk(t *testing.T) {
	if os.Getenv("VIRTMGR_TEST_REAL_LIBVIRT") != "1" {
		t.Skip("set VIRTMGR_TEST_REAL_LIBVIRT=1 to run against qemu:///system")
	}
	client, err := New("qemu:///system", "integration-test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolName := "virtmgr-unmanaged-resize-it-" + suffix
	poolDir := t.TempDir()
	pool, err := client.CreateStoragePool(ctx, domain.StoragePoolCreateSpec{
		Name: poolName, Type: "dir", TargetPath: poolDir, Start: true,
	})
	if err != nil {
		t.Fatalf("CreateStoragePool() error = %v", err)
	}
	defer func() {
		_ = client.PerformStoragePoolAction(ctx, pool.Name, domain.StoragePoolActionStop)
		_ = client.DeleteStoragePool(ctx, pool.Name)
	}()
	nestedDir := filepath.Join(poolDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o750); err != nil {
		t.Fatal(err)
	}
	diskPath := filepath.Join(nestedDir, "disk.qcow2")
	if output, err := exec.Command("qemu-img", "create", "-f", "qcow2", diskPath, "64M").CombinedOutput(); err != nil {
		t.Fatalf("qemu-img create: %v: %s", err, output)
	}
	vm, err := client.CreateVM(ctx, domain.VMCreateSpec{
		Name: "virtmgr-unmanaged-resize-it-" + suffix, VCPUs: 1, MemoryMiB: 128,
		DiskPath: diskPath, DiskFormat: "qcow2", Network: "default", Firmware: domain.VMFirmwareBIOS,
	})
	if err != nil {
		t.Fatalf("CreateVM() error = %v", err)
	}
	defer func() { _ = client.DeleteVM(ctx, vm.UUID) }()
	loaded, err := client.GetVM(ctx, vm.UUID)
	if err != nil {
		t.Fatalf("GetVM() error = %v", err)
	}
	if len(loaded.Disks) == 0 || loaded.Disks[0].CapacityBytes != 64<<20 {
		t.Fatalf("loaded VM disks = %+v", loaded.Disks)
	}

	updated, err := client.ResizeVMDisk(ctx, vm.UUID, "vda", domain.VMDiskResizeSpec{CapacityBytes: 96 << 20})
	if err != nil {
		t.Fatalf("ResizeVMDisk(unmanaged) error = %v", err)
	}
	if len(updated.Disks) == 0 || updated.Disks[0].CapacityBytes != 96<<20 {
		t.Fatalf("updated VM disks = %+v", updated.Disks)
	}
	info, err := exec.Command("qemu-img", "info", "--output=json", diskPath).Output()
	if err != nil {
		t.Fatalf("qemu-img info: %v", err)
	}
	if !strings.Contains(string(info), `"virtual-size": 100663296`) {
		t.Fatalf("qemu-img info = %s", info)
	}
}
