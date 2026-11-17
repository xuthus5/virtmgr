package libvirtbackend

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
)

func TestDemoLinkedCloneProtectsBackingVolume(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	if _, err := backend.CreateStorageVolume(ctx, "default", domain.StorageVolumeCreateSpec{Name: "demo-linked-base.qcow2", Format: "qcow2", CapacityBytes: 1 << 30}); err != nil {
		t.Fatal(err)
	}
	volume, err := backend.CloneStorageVolumeLinked(ctx, "default", "demo-linked-base.qcow2", "default", "demo-linked.qcow2")
	if err != nil {
		t.Fatal(err)
	}
	if volume.Path != "/var/lib/libvirt/images/demo-linked.qcow2" || volume.AllocationBytes != 0 {
		t.Fatalf("linked clone volume = %+v", volume)
	}
	if err := backend.DeleteStorageVolume(ctx, "default", "demo-linked-base.qcow2"); !errors.Is(err, domain.ErrVolumeInUse) {
		t.Fatalf("delete parent error = %v", err)
	}
	if err := backend.DeleteStorageVolume(ctx, "default", "demo-linked.qcow2"); err != nil {
		t.Fatalf("delete child error = %v", err)
	}
	if err := backend.DeleteStorageVolume(ctx, "default", "demo-linked-base.qcow2"); err != nil {
		t.Fatalf("delete parent after child error = %v", err)
	}
}

func TestDemoConvertStorageVolumeCopiesAndChangesFormat(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	if _, err := backend.CreateStorageVolume(ctx, "default", domain.StorageVolumeCreateSpec{Name: "convert-source.qcow2", Format: "qcow2", CapacityBytes: 30 << 30}); err != nil {
		t.Fatal(err)
	}
	converted, err := backend.ConvertStorageVolume(ctx, domain.StorageVolumeConvertSpec{
		SourcePool: "default", SourceVolume: "convert-source.qcow2",
		TargetPool: "default", TargetName: "convert-source.raw", TargetFormat: "raw",
	})
	if err != nil {
		t.Fatalf("ConvertStorageVolume() error = %v", err)
	}
	if converted.Format != "raw" || converted.Path != "/var/lib/libvirt/images/convert-source.raw" || converted.CapacityBytes != 30<<30 {
		t.Fatalf("converted volume = %+v", converted)
	}
	volumes, err := backend.ListStorageVolumes(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, volume := range volumes {
		if volume.Name == "convert-source.raw" && volume.Format == "raw" {
			found = true
		}
	}
	if !found {
		t.Fatalf("converted volume not listed: %+v", volumes)
	}
	if _, err := backend.ConvertStorageVolume(ctx, domain.StorageVolumeConvertSpec{
		SourcePool: "default", SourceVolume: "demo-build-01.qcow2",
		TargetPool: "default", TargetName: "demo-build-01-copy.qcow2", TargetFormat: "qcow2",
	}); !errors.Is(err, domain.ErrVolumeInUse) {
		t.Fatalf("in-use conversion error = %v", err)
	}
	if _, err := backend.ConvertStorageVolume(ctx, domain.StorageVolumeConvertSpec{
		SourcePool: "default", SourceVolume: "convert-source.qcow2",
		TargetPool: "default", TargetName: "demo-build-01.qcow2", TargetFormat: "qcow2",
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("existing target error = %v", err)
	}
}
