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
