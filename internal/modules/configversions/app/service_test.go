package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"virtmgr/internal/domain"
	"virtmgr/internal/platform/sqlite"
)

type fakeBackend struct {
	mu   sync.Mutex
	xmls map[string]string
	vms  []domain.VM
}

func (b *fakeBackend) ListVMs(context.Context) ([]domain.VM, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]domain.VM(nil), b.vms...), nil
}

func (b *fakeBackend) GetVM(_ context.Context, uuid string) (domain.VM, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, vm := range b.vms {
		if vm.UUID == uuid {
			return vm, nil
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func (b *fakeBackend) GetVMXML(_ context.Context, uuid string, format domain.VMXMLFormat) (domain.VMXML, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	text, ok := b.xmls[uuid]
	if !ok {
		return domain.VMXML{}, domain.ErrNotFound
	}
	return domain.VMXML{UUID: uuid, Name: "test-vm", Format: format, XML: text}, nil
}

func (b *fakeBackend) DefineVMXML(_ context.Context, uuid, xmlText string) (domain.VMXML, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.xmls[uuid]; !ok {
		return domain.VMXML{}, domain.ErrNotFound
	}
	b.xmls[uuid] = xmlText
	return domain.VMXML{UUID: uuid, Name: "test-vm", Format: domain.VMXMLFormatConfig, XML: xmlText}, nil
}

func newTestService(t *testing.T, xmlText string) (*Service, *fakeBackend, *sqlite.Store) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewStore(db)
	uuid := "11111111-1111-4111-8111-111111111111"
	backend := &fakeBackend{
		xmls: map[string]string{uuid: xmlText},
		vms:  []domain.VM{{UUID: uuid, Name: "test-vm", Persistent: true, State: domain.VMStateShutoff}},
	}
	return New(store, backend, store, false), backend, store
}

func captureForTest(t *testing.T, service *Service, source domain.VMConfigVersionSource) domain.VMConfigVersion {
	t.Helper()
	item, err := service.CaptureVM(context.Background(), "11111111-1111-4111-8111-111111111111", domain.VMConfigCapture{
		Source: source,
		Actor:  "tester",
		Role:   domain.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestCaptureDeduplicatesConsecutiveXML(t *testing.T) {
	service, backend, _ := newTestService(t, "<domain><name>test-vm</name></domain>")
	first := captureForTest(t, service, domain.VMConfigSourceBaseline)
	second := captureForTest(t, service, domain.VMConfigSourceExternal)
	if first.VersionNo != 1 || second.VersionNo != 1 {
		t.Fatalf("versions = %d, %d; want both 1", first.VersionNo, second.VersionNo)
	}

	backend.mu.Lock()
	backend.xmls["11111111-1111-4111-8111-111111111111"] = "<domain><name>test-vm</name><vcpu>4</vcpu></domain>"
	backend.mu.Unlock()
	third := captureForTest(t, service, domain.VMConfigSourceResources)
	if third.VersionNo != 2 {
		t.Fatalf("version = %d; want 2", third.VersionNo)
	}
}

func TestListAndGetRoundTripCompressedXML(t *testing.T) {
	ctx := context.Background()
	xmlText := "<domain>\n  <name>test-vm</name>\n</domain>\n"
	service, _, _ := newTestService(t, xmlText)
	captureForTest(t, service, domain.VMConfigSourceCreate)

	list, err := service.List(ctx, "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || !list.CurrentCaptured || !list.Items[0].IsCurrent {
		t.Fatalf("unexpected list: %+v", list)
	}
	detail, err := service.Get(ctx, "11111111-1111-4111-8111-111111111111", 1)
	if err != nil {
		t.Fatal(err)
	}
	if detail.XML != strings.TrimSpace(xmlText) {
		t.Fatalf("XML mismatch: %q", detail.XML)
	}
}

func TestRollbackRejectsStaleHashAndRecordsFailure(t *testing.T) {
	ctx := context.Background()
	service, _, store := newTestService(t, "<domain><name>test-vm</name><vcpu>2</vcpu></domain>")
	captureForTest(t, service, domain.VMConfigSourceBaseline)
	_, err := service.Rollback(ctx, RollbackCommand{
		UUID:           "11111111-1111-4111-8111-111111111111",
		VersionNo:      1,
		ExpectedSHA256: "stale",
		Request:        adminContext(),
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v; want conflict", err)
	}
	events, err := store.ListAuditEvents(ctx, domain.AuditEventQuery{Action: "vm.config.rollback"})
	if err != nil {
		t.Fatal(err)
	}
	if events.Total != 1 || events.Items[0].Result != "failure" {
		t.Fatalf("audit = %+v", events)
	}
}

func TestRollbackCapturesCurrentAndCreatesRollbackVersion(t *testing.T) {
	ctx := context.Background()
	oldXML := "<domain><name>test-vm</name><vcpu>2</vcpu></domain>"
	newXML := "<domain><name>test-vm</name><vcpu>4</vcpu></domain>"
	service, backend, store := newTestService(t, oldXML)
	captureForTest(t, service, domain.VMConfigSourceBaseline)
	backend.mu.Lock()
	backend.xmls["11111111-1111-4111-8111-111111111111"] = newXML
	backend.mu.Unlock()

	list, err := service.List(ctx, "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if list.CurrentCaptured {
		t.Fatal("new external XML should not be marked as captured")
	}
	result, err := service.Rollback(ctx, RollbackCommand{
		UUID:           "11111111-1111-4111-8111-111111111111",
		VersionNo:      1,
		ExpectedSHA256: list.CurrentSHA256,
		Note:           "restore known-good config",
		Request:        adminContext(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != domain.VMConfigSourceRollback || result.SourceVersion != 1 || result.VersionNo != 3 {
		t.Fatalf("rollback result = %+v", result)
	}
	if result.XML != oldXML {
		t.Fatalf("rolled back XML = %q", result.XML)
	}
	items, err := store.ListVMConfigVersions(ctx, "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[1].Source != domain.VMConfigSourcePreRollback || items[0].Source != domain.VMConfigSourceRollback {
		t.Fatalf("versions = %+v", items)
	}
	events, err := store.ListAuditEvents(ctx, domain.AuditEventQuery{Action: "vm.config.rollback"})
	if err != nil {
		t.Fatal(err)
	}
	if events.Total != 1 || events.Items[0].Result != "success" {
		t.Fatalf("audit = %+v", events)
	}
}

func TestEnsureBaselineCapturesPersistentVMs(t *testing.T) {
	service, _, _ := newTestService(t, "<domain><name>test-vm</name></domain>")
	if err := service.EnsureBaseline(context.Background()); err != nil {
		t.Fatal(err)
	}
	list, err := service.List(context.Background(), "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || list.Items[0].Source != domain.VMConfigSourceBaseline {
		t.Fatalf("baseline = %+v", list)
	}
}

func TestRollbackRequiresAdministrator(t *testing.T) {
	service, _, _ := newTestService(t, "<domain><name>test-vm</name></domain>")
	captureForTest(t, service, domain.VMConfigSourceBaseline)
	_, err := service.Rollback(context.Background(), RollbackCommand{
		UUID:           "11111111-1111-4111-8111-111111111111",
		VersionNo:      1,
		ExpectedSHA256: hashXML("<domain><name>test-vm</name></domain>"),
		Request: MutationContext{Session: domain.Session{User: domain.User{
			Username: "operator", Role: domain.RoleOperator,
		}}},
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v; want forbidden", err)
	}
}

func adminContext() MutationContext {
	return MutationContext{
		Session: domain.Session{User: domain.User{Username: "admin", Role: domain.RoleAdmin}},
	}
}
