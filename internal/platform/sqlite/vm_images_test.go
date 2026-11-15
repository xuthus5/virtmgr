package sqlite

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
)

func TestVMImageStoreAndTemplateReference(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	image, err := store.CreateVMImage(ctx, domain.VMImage{
		Name: "ubuntu-24.04", Description: "cloud image", Pool: "default", Volume: "ubuntu.qcow2",
		Format: "qcow2", OSFamily: "linux", OSName: "Ubuntu 24.04", Architecture: "x86_64",
		CapacityBytes: 4096, AllocationBytes: 2048,
	})
	if err != nil {
		t.Fatalf("CreateVMImage() error = %v", err)
	}
	if image.ID < 1 || image.Name != "ubuntu-24.04" || image.CreatedAt.IsZero() {
		t.Fatalf("image = %+v", image)
	}
	if _, err := store.CreateVMImage(ctx, domain.VMImage{Name: "duplicate", Pool: "default", Volume: "ubuntu.qcow2", Format: "qcow2"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate volume error = %v", err)
	}
	template, err := store.CreateVMTemplate(ctx, domain.VMTemplateSpec{
		Name: "ubuntu-base", VCPUs: 2, MemoryMiB: 2048, DiskFormat: "qcow2", Network: "default",
		Firmware: domain.VMFirmwareBIOS, ImageID: image.ID, GuestAgent: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("CreateVMTemplate() error = %v", err)
	}
	if template.ImageID != image.ID {
		t.Fatalf("template image = %+v", template)
	}
	if err := store.DeleteVMImage(ctx, image.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("referenced delete error = %v", err)
	}
	if err := store.DeleteVMTemplate(ctx, template.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteVMImage(ctx, image.ID); err != nil {
		t.Fatalf("unreferenced delete error = %v", err)
	}
	if _, err := store.GetVMImage(ctx, image.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted image error = %v", err)
	}
}
