package libvirtbackend

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
)

func TestFindWritableDisk(t *testing.T) {
	disks := []domain.Disk{
		{Device: "cdrom", Target: "sda", Source: "/tmp/install.iso", ReadOnly: true},
		{Device: "disk", Target: "vda", Source: "/var/lib/libvirt/images/data.qcow2"},
	}
	disk, ok := findWritableDisk(disks, " vda ")
	if !ok || disk.Target != "vda" {
		t.Fatalf("findWritableDisk(vda) = %+v, %v", disk, ok)
	}
	if _, ok := findWritableDisk(disks, "sda"); ok {
		t.Fatal("findWritableDisk accepted a read-only cdrom")
	}
	if _, ok := findWritableDisk(disks, "vdb"); ok {
		t.Fatal("findWritableDisk found a missing target")
	}
}

func TestDemoBackendResizeVMDisk(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	const uuid = "22222222-2222-4222-8222-222222222222"
	const newCapacity = uint64(100 * 1024 * 1024 * 1024)

	updated, err := backend.ResizeVMDisk(ctx, uuid, "vda", domain.VMDiskResizeSpec{CapacityBytes: newCapacity})
	if err != nil {
		t.Fatalf("ResizeVMDisk() error = %v", err)
	}
	if len(updated.Disks) != 1 || updated.Disks[0].CapacityBytes != newCapacity {
		t.Fatalf("updated disks = %+v", updated.Disks)
	}
	volumes, err := backend.ListStorageVolumes(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, volume := range volumes {
		if volume.Path == updated.Disks[0].Source {
			found = volume.CapacityBytes == newCapacity
			break
		}
	}
	if !found {
		t.Fatalf("storage volume was not resized: %+v", volumes)
	}
	if _, err := backend.ResizeVMDisk(ctx, uuid, "vda", domain.VMDiskResizeSpec{CapacityBytes: newCapacity}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("shrink/no-op error = %v", err)
	}
	if _, err := backend.ResizeVMDisk(ctx, uuid, "vdb", domain.VMDiskResizeSpec{CapacityBytes: newCapacity}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing disk error = %v", err)
	}
}
