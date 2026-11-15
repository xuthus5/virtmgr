package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type memoryImageRepository struct {
	images []domain.VMImage
	next   int64
}

func (m *memoryImageRepository) ListVMImages(context.Context) ([]domain.VMImage, error) {
	return append([]domain.VMImage(nil), m.images...), nil
}

func (m *memoryImageRepository) GetVMImage(_ context.Context, id int64) (domain.VMImage, error) {
	for _, image := range m.images {
		if image.ID == id {
			return image, nil
		}
	}
	return domain.VMImage{}, domain.ErrNotFound
}

func (m *memoryImageRepository) CreateVMImage(_ context.Context, image domain.VMImage) (domain.VMImage, error) {
	m.next++
	image.ID = m.next
	image.CreatedAt = time.Unix(1_700_000_000, 0).UTC()
	image.UpdatedAt = image.CreatedAt
	m.images = append(m.images, image)
	return image, nil
}

func (m *memoryImageRepository) UpdateVMImageMetadata(_ context.Context, image domain.VMImage) (domain.VMImage, error) {
	for index := range m.images {
		if m.images[index].ID == image.ID {
			m.images[index] = image
			return image, nil
		}
	}
	return domain.VMImage{}, domain.ErrNotFound
}

func (m *memoryImageRepository) DeleteVMImage(_ context.Context, id int64) error {
	for index := range m.images {
		if m.images[index].ID == id {
			m.images = append(m.images[:index], m.images[index+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakeVolumes struct {
	items    []domain.StorageVolume
	checksum string
	err      error
}

func (f *fakeVolumes) ListStorageVolumes(context.Context, string) ([]domain.StorageVolume, error) {
	return f.items, f.err
}

func (f *fakeVolumes) StorageVolumeChecksum(context.Context, string, string) (string, error) {
	return f.checksum, f.err
}

type imageAudit struct{ events []domain.AuditEvent }

func (a *imageAudit) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	a.events = append(a.events, event)
	return nil
}

func imageRequest(role domain.Role) MutationContext {
	return MutationContext{Session: domain.Session{User: domain.User{ID: 1, Username: "operator", Role: role}}, RequestID: "req-image", SourceIP: "127.0.0.1"}
}

func TestImageRegisterRefreshDelete(t *testing.T) {
	repository := &memoryImageRepository{}
	volumes := &fakeVolumes{items: []domain.StorageVolume{{Name: "ubuntu.qcow2", Format: "qcow2", CapacityBytes: 4096, AllocationBytes: 2048}}, checksum: "sha256-test"}
	audit := &imageAudit{}
	service := New(repository, volumes, audit, false)
	created, err := service.Register(context.Background(), domain.VMImageCreateSpec{
		Name: " ubuntu-24.04 ", Pool: "default", Volume: "ubuntu.qcow2", OSFamily: "linux",
	}, imageRequest(domain.RoleOperator))
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if created.ID != 1 || created.Name != "ubuntu-24.04" || created.Format != "qcow2" || created.CapacityBytes != 4096 || created.SHA256 != "" {
		t.Fatalf("created = %+v", created)
	}
	refreshed, err := service.Refresh(context.Background(), created.ID, imageRequest(domain.RoleOperator))
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if refreshed.SHA256 != "sha256-test" || refreshed.AllocationBytes != 2048 {
		t.Fatalf("refreshed = %+v", refreshed)
	}
	if err := service.Delete(context.Background(), created.ID, imageRequest(domain.RoleOperator)); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := service.Get(context.Background(), created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted image error = %v", err)
	}
	if len(audit.events) != 3 || audit.events[0].Action != "image.register" || audit.events[1].Action != "image.refresh" || audit.events[2].Action != "image.unregister" {
		t.Fatalf("audit = %+v", audit.events)
	}
}

func TestImageValidationPermissionsAndReadOnly(t *testing.T) {
	volumes := &fakeVolumes{items: []domain.StorageVolume{
		{Name: "install.iso", Format: "iso"},
		{Name: "disk.img", Format: ""},
	}}
	service := New(&memoryImageRepository{}, volumes, nil, false)
	for name, test := range map[string]struct {
		spec    domain.VMImageCreateSpec
		request MutationContext
		want    error
	}{
		"viewer":         {domain.VMImageCreateSpec{Name: "image", Pool: "default", Volume: "disk.img"}, imageRequest(domain.RoleViewer), domain.ErrForbidden},
		"bad name":       {domain.VMImageCreateSpec{Name: "-bad", Pool: "default", Volume: "disk.img"}, imageRequest(domain.RoleOperator), domain.ErrInvalidInput},
		"missing volume": {domain.VMImageCreateSpec{Name: "image", Pool: "default", Volume: "missing.qcow2"}, imageRequest(domain.RoleOperator), domain.ErrNotFound},
		"iso":            {domain.VMImageCreateSpec{Name: "image", Pool: "default", Volume: "install.iso"}, imageRequest(domain.RoleOperator), domain.ErrInvalidInput},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.Register(context.Background(), test.spec, test.request); !errors.Is(err, test.want) {
				t.Fatalf("Register() error = %v, want %v", err, test.want)
			}
		})
	}
	created, err := service.Register(context.Background(), domain.VMImageCreateSpec{Name: "image", Pool: "default", Volume: "disk.img"}, imageRequest(domain.RoleOperator))
	if err != nil || created.Format != "raw" {
		t.Fatalf("inferred format = %+v error = %v", created, err)
	}
	readOnlyService := New(&memoryImageRepository{}, volumes, nil, true)
	if _, err := readOnlyService.Register(context.Background(), domain.VMImageCreateSpec{Name: "image", Pool: "default", Volume: "disk.img"}, imageRequest(domain.RoleOperator)); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read only error = %v", err)
	}
}
