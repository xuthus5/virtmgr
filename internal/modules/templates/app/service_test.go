package app

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

type memoryRepository struct {
	templates map[int64]domain.VMTemplate
	next      int64
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{templates: map[int64]domain.VMTemplate{}}
}

func (m *memoryRepository) ListVMTemplates(context.Context) ([]domain.VMTemplate, error) {
	result := make([]domain.VMTemplate, 0, len(m.templates))
	for _, template := range m.templates {
		result = append(result, template)
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name) })
	return result, nil
}

func (m *memoryRepository) GetVMTemplate(_ context.Context, id int64) (domain.VMTemplate, error) {
	template, ok := m.templates[id]
	if !ok {
		return domain.VMTemplate{}, domain.ErrNotFound
	}
	return template, nil
}

func (m *memoryRepository) VMTemplateByName(_ context.Context, name string) (domain.VMTemplate, error) {
	for _, template := range m.templates {
		if strings.EqualFold(template.Name, name) {
			return template, nil
		}
	}
	return domain.VMTemplate{}, domain.ErrNotFound
}

func (m *memoryRepository) CreateVMTemplate(_ context.Context, spec domain.VMTemplateSpec) (domain.VMTemplate, error) {
	m.next++
	template := fromSpec(m.next, spec)
	m.templates[template.ID] = template
	return template, nil
}

func (m *memoryRepository) UpdateVMTemplate(_ context.Context, id int64, spec domain.VMTemplateSpec) (domain.VMTemplate, error) {
	if _, ok := m.templates[id]; !ok {
		return domain.VMTemplate{}, domain.ErrNotFound
	}
	template := fromSpec(id, spec)
	m.templates[id] = template
	return template, nil
}

func (m *memoryRepository) DeleteVMTemplate(_ context.Context, id int64) error {
	if _, ok := m.templates[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.templates, id)
	return nil
}

func fromSpec(id int64, spec domain.VMTemplateSpec) domain.VMTemplate {
	return domain.VMTemplate{
		ID: id, Name: spec.Name, Description: spec.Description, VCPUs: spec.VCPUs,
		MemoryMiB: spec.MemoryMiB, DiskFormat: spec.DiskFormat, Network: spec.Network,
		ISOPath: spec.ISOPath, Firmware: spec.Firmware, SecureBoot: spec.SecureBoot,
		CloudInit: spec.CloudInit, CloudInitUserData: spec.CloudInitUserData,
		Autostart: spec.Autostart, Start: spec.Start,
	}
}

type auditRecorder struct{ events []domain.AuditEvent }

func (a *auditRecorder) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	a.events = append(a.events, event)
	return nil
}

type dynamicReadOnly struct{ value bool }

func (d *dynamicReadOnly) ReadOnly() bool { return d.value }

func requestFor(role domain.Role) MutationContext {
	return MutationContext{
		Session:   domain.Session{User: domain.User{ID: 1, Username: "operator", Role: role}},
		RequestID: "req-template",
		SourceIP:  "127.0.0.1",
	}
}

func validSpec() domain.VMTemplateSpec {
	return domain.VMTemplateSpec{
		Name: "linux-base", Description: "通用 Linux 模板", VCPUs: 2, MemoryMiB: 2048,
		DiskFormat: "qcow2", Network: "default", Firmware: "uefi", SecureBoot: true,
		CloudInit: true, CloudInitUserData: "#cloud-config\nruncmd:\n  - echo template\n",
		Autostart: true,
	}
}

func TestVMTemplateServiceCRUDAudit(t *testing.T) {
	repository := newMemoryRepository()
	audit := &auditRecorder{}
	service := New(repository, audit, false)
	created, err := service.Create(context.Background(), CreateCommand{Spec: validSpec(), Request: requestFor(domain.RoleOperator)})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != 1 || created.CloudInitUserData == "" {
		t.Fatalf("created = %+v", created)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm_template.create" || audit.events[0].TargetKind != "vm_template" {
		t.Fatalf("create audit = %+v", audit.events)
	}
	if _, leaked := audit.events[0].Detail["cloudInitUserData"]; leaked {
		t.Fatalf("audit leaked cloud-init user data: %+v", audit.events[0].Detail)
	}
	if audit.events[0].Detail["cloudInitUserDataBytes"] == nil {
		t.Fatalf("audit missing cloud-init byte count: %+v", audit.events[0].Detail)
	}
	spec := created
	updateSpec := domain.VMTemplateSpec{
		Name: "secure-web", Description: spec.Description, VCPUs: 4, MemoryMiB: 4096,
		DiskFormat: "raw", Network: "default", Firmware: "uefi", SecureBoot: true,
		CloudInit: false, Autostart: false, Start: true,
	}
	updated, err := service.Update(context.Background(), UpdateCommand{ID: created.ID, Spec: updateSpec, Request: requestFor(domain.RoleAdmin)})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "secure-web" || updated.DiskFormat != "raw" || updated.CloudInitUserData != "" {
		t.Fatalf("updated = %+v", updated)
	}
	if len(audit.events) != 2 || audit.events[1].Action != "vm_template.update" || audit.events[1].Detail["previousName"] != "linux-base" {
		t.Fatalf("update audit = %+v", audit.events)
	}
	if err := service.Delete(context.Background(), DeleteCommand{ID: created.ID, Request: requestFor(domain.RoleOperator)}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(audit.events) != 3 || audit.events[2].Action != "vm_template.delete" {
		t.Fatalf("delete audit = %+v", audit.events)
	}
	if _, err := service.Get(context.Background(), created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted Get() error = %v", err)
	}
}

func TestVMTemplateServiceValidationAndPermissions(t *testing.T) {
	repository := newMemoryRepository()
	service := New(repository, nil, false)
	for name, mutate := range map[string]func(*domain.VMTemplateSpec){
		"bad name":         func(spec *domain.VMTemplateSpec) { spec.Name = "bad name" },
		"zero vcpus":       func(spec *domain.VMTemplateSpec) { spec.VCPUs = 0 },
		"small memory":     func(spec *domain.VMTemplateSpec) { spec.MemoryMiB = 64 },
		"bad format":       func(spec *domain.VMTemplateSpec) { spec.DiskFormat = "vmdk" },
		"relative iso":     func(spec *domain.VMTemplateSpec) { spec.ISOPath = "install.iso" },
		"secure boot bios": func(spec *domain.VMTemplateSpec) { spec.Firmware = "bios" },
		"empty cloud init": func(spec *domain.VMTemplateSpec) { spec.CloudInitUserData = "" },
	} {
		t.Run(name, func(t *testing.T) {
			spec := validSpec()
			mutate(&spec)
			if _, err := service.Create(context.Background(), CreateCommand{Spec: spec, Request: requestFor(domain.RoleOperator)}); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
	if _, err := service.Create(context.Background(), CreateCommand{Spec: validSpec(), Request: requestFor(domain.RoleViewer)}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("viewer create error = %v", err)
	}
	mode := &dynamicReadOnly{value: true}
	readOnlyService := New(repository, nil, false, WithReadOnlyMode(mode))
	if _, err := readOnlyService.Create(context.Background(), CreateCommand{Spec: validSpec(), Request: requestFor(domain.RoleAdmin)}); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only create error = %v", err)
	}
}

func TestVMTemplateServiceRejectsDuplicateNames(t *testing.T) {
	service := New(newMemoryRepository(), nil, false)
	first := validSpec()
	first.Name = "Linux-Base"
	if _, err := service.Create(context.Background(), CreateCommand{Spec: first, Request: requestFor(domain.RoleAdmin)}); err != nil {
		t.Fatal(err)
	}
	duplicate := validSpec()
	duplicate.Name = "linux-base"
	if _, err := service.Create(context.Background(), CreateCommand{Spec: duplicate, Request: requestFor(domain.RoleAdmin)}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate error = %v", err)
	}
}
