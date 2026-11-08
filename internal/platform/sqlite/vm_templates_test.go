package sqlite

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
)

func TestVMTemplateStoreCRUD(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	spec := domain.VMTemplateSpec{
		Name: "linux-base", Description: "base image", VCPUs: 2, MemoryMiB: 2048,
		DiskFormat: "qcow2", Network: "default", Firmware: domain.VMFirmwareBIOS,
		CloudInit: true, CloudInitUserData: "#cloud-config\n", Autostart: true,
	}
	created, err := store.CreateVMTemplate(ctx, spec)
	if err != nil {
		t.Fatalf("CreateVMTemplate() error = %v", err)
	}
	if created.ID < 1 || created.Name != "linux-base" || !created.CloudInit || !created.Autostart {
		t.Fatalf("created template = %+v", created)
	}
	byName, err := store.VMTemplateByName(ctx, "LINUX-BASE")
	if err != nil || byName.ID != created.ID {
		t.Fatalf("VMTemplateByName() = %+v, %v", byName, err)
	}
	if _, err := store.CreateVMTemplate(ctx, spec); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate create error = %v", err)
	}

	spec.Name = "secure-web"
	spec.Firmware = domain.VMFirmwareUEFI
	spec.SecureBoot = true
	spec.MemoryMiB = 4096
	updated, err := store.UpdateVMTemplate(ctx, created.ID, spec)
	if err != nil {
		t.Fatalf("UpdateVMTemplate() error = %v", err)
	}
	if updated.Name != "secure-web" || updated.Firmware != domain.VMFirmwareUEFI || !updated.SecureBoot || updated.MemoryMiB != 4096 {
		t.Fatalf("updated template = %+v", updated)
	}
	list, err := store.ListVMTemplates(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "secure-web" {
		t.Fatalf("ListVMTemplates() = %+v, %v", list, err)
	}
	if _, err := store.UpdateVMTemplate(ctx, 999, spec); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing update error = %v", err)
	}
	if err := store.DeleteVMTemplate(ctx, created.ID); err != nil {
		t.Fatalf("DeleteVMTemplate() error = %v", err)
	}
	if _, err := store.GetVMTemplate(ctx, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted template error = %v", err)
	}
	if err := store.DeleteVMTemplate(ctx, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete error = %v", err)
	}
}
