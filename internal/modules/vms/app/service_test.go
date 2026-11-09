package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type fakeVMRepository struct {
	vms           []domain.VM
	err           error
	actionErr     error
	autostartErr  error
	resourcesErr  error
	diskResizeErr error
	createErr     error
	cloneErr      error
	backupErr     error
	deleteErr     error
	performed     []domain.VMAction
	actionUpdates map[string]domain.VM
}

func (f fakeVMRepository) ListVMs(context.Context) ([]domain.VM, error) {
	return f.vms, f.err
}

func (f fakeVMRepository) GetVM(_ context.Context, uuid string) (domain.VM, error) {
	if f.err != nil {
		return domain.VM{}, f.err
	}
	for _, vm := range f.vms {
		if vm.UUID == uuid {
			return vm, nil
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func (f fakeVMRepository) GetVMXML(_ context.Context, uuid string, format domain.VMXMLFormat) (domain.VMXML, error) {
	if f.err != nil {
		return domain.VMXML{}, f.err
	}
	return domain.VMXML{UUID: uuid, Format: format}, nil
}

func (f fakeVMRepository) DefineVMXML(_ context.Context, uuid, xmlText string) (domain.VMXML, error) {
	if f.err != nil {
		return domain.VMXML{}, f.err
	}
	return domain.VMXML{UUID: uuid, Name: "api-01", Format: domain.VMXMLFormatConfig, XML: xmlText}, nil
}

func (f fakeVMRepository) SetVMAutostart(context.Context, string, bool) error {
	return f.autostartErr
}

func (f fakeVMRepository) UpdateVMResources(_ context.Context, uuid string, spec domain.VMResourcesSpec) (domain.VM, error) {
	if f.resourcesErr != nil {
		return domain.VM{}, f.resourcesErr
	}
	for _, vm := range f.vms {
		if vm.UUID == uuid {
			vm.VCPUs = spec.VCPUs
			vm.MaxVCPUs = max(vm.MaxVCPUs, spec.VCPUs)
			vm.MemoryBytes = spec.MemoryMiB * 1024 * 1024
			vm.MaxMemoryBytes = max(vm.MaxMemoryBytes, vm.MemoryBytes)
			return vm, nil
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func (f fakeVMRepository) ResizeVMDisk(_ context.Context, uuid, target string, spec domain.VMDiskResizeSpec) (domain.VM, error) {
	if f.diskResizeErr != nil {
		return domain.VM{}, f.diskResizeErr
	}
	for _, vm := range f.vms {
		if vm.UUID != uuid {
			continue
		}
		for index := range vm.Disks {
			if vm.Disks[index].Target == target {
				vm.Disks[index].CapacityBytes = spec.CapacityBytes
				return vm, nil
			}
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func (f fakeVMRepository) AttachHostDevice(context.Context, string, string) (domain.VM, error) {
	if f.err != nil {
		return domain.VM{}, f.err
	}
	return domain.VM{}, nil
}

func (f fakeVMRepository) DetachHostDevice(context.Context, string, string) (domain.VM, error) {
	if f.err != nil {
		return domain.VM{}, f.err
	}
	return domain.VM{}, nil
}

func (f fakeVMRepository) PerformAction(_ context.Context, uuid string, action domain.VMAction) error {
	f.performed = append(f.performed, action)
	return f.actionErr
}

func (f fakeVMRepository) CreateVM(_ context.Context, spec domain.VMCreateSpec) (domain.VM, error) {
	if f.createErr != nil {
		return domain.VM{}, f.createErr
	}
	state := domain.VMStateShutoff
	if spec.Start {
		state = domain.VMStateRunning
	}
	return domain.VM{UUID: "uuid-created", Name: spec.Name, State: state}, nil
}

func (f fakeVMRepository) CloneVM(_ context.Context, uuid string, spec domain.VMCloneSpec, progress func(int, string)) (domain.VM, error) {
	if f.cloneErr != nil {
		return domain.VM{}, f.cloneErr
	}
	if progress != nil {
		progress(100, "克隆完成")
	}
	state := domain.VMStateShutoff
	if spec.Start {
		state = domain.VMStateRunning
	}
	return domain.VM{UUID: "uuid-cloned", Name: spec.Name, State: state}, nil
}

func (f fakeVMRepository) BackupVM(_ context.Context, uuid string, spec domain.VMBackupSpec, progress func(int, string)) (domain.VMBackup, error) {
	if f.backupErr != nil {
		return domain.VMBackup{}, f.backupErr
	}
	if progress != nil {
		progress(60, "正在复制磁盘")
		progress(100, "备份完成")
	}
	return domain.VMBackup{
		FormatVersion: domain.VMBackupFormatVersion, VMUUID: uuid, VMName: "api-01", Name: spec.Name,
		TargetPool: spec.TargetPool, ManifestVolume: spec.Name + "-manifest.json",
		ConfigVolume: spec.Name + "-domain.xml", ConfigBytes: 100, TotalBytes: 2048,
		Disks: []domain.VMBackupDisk{{SourceVolume: "api-01.qcow2", BackupVolume: spec.Name + "-disk-01.qcow2", Format: "qcow2", CapacityBytes: 1024}},
	}, nil
}

func (f fakeVMRepository) DeleteVM(_ context.Context, _ string) error {
	return f.deleteErr
}

func (f fakeVMRepository) ListSnapshots(context.Context, string) ([]domain.VMSnapshot, error) {
	return nil, f.err
}

func (f fakeVMRepository) CreateSnapshot(context.Context, string, domain.VMSnapshotCreateSpec) (domain.VMSnapshot, error) {
	return domain.VMSnapshot{}, f.err
}

func (f fakeVMRepository) RevertSnapshot(context.Context, string, string) error {
	return f.err
}

func (f fakeVMRepository) DeleteSnapshot(context.Context, string, string) error {
	return f.err
}

func (f fakeVMRepository) OpenConsole(context.Context, string, uint) (io.ReadWriteCloser, error) {
	return nil, f.err
}

func (f fakeVMRepository) OpenSerialConsole(context.Context, string, string) (io.ReadWriteCloser, error) {
	return nil, f.err
}

func (f fakeVMRepository) SendConsoleKey(context.Context, string, []uint) error {
	return f.err
}

type fakeAuditRecorder struct {
	events []domain.AuditEvent
	err    error
}

func (f *fakeAuditRecorder) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	f.events = append(f.events, event)
	return f.err
}

func operatorSession() domain.Session {
	return domain.Session{User: domain.User{ID: 1, Username: "operator", Role: domain.RoleOperator}, ExpiresAt: time.Now().Add(time.Hour)}
}

type dynamicReadOnly struct{ value bool }

func (m *dynamicReadOnly) ReadOnly() bool { return m.value }

type fakeVMMetrics struct {
	result domain.VMMetrics
	err    error
	uuid   string
}

func (f *fakeVMMetrics) VM(_ context.Context, uuid string) (domain.VMMetrics, error) {
	f.uuid = uuid
	return f.result, f.err
}

func TestServiceMetricsDelegatesAndValidatesUUID(t *testing.T) {
	provider := &fakeVMMetrics{result: domain.VMMetrics{UUID: "uuid-a", ResourceMetrics: domain.ResourceMetrics{CPUPercent: 42}}}
	service := New(fakeVMRepository{}, nil, false, WithMetrics(provider))
	result, err := service.Metrics(context.Background(), " uuid-a ")
	if err != nil {
		t.Fatalf("Metrics() error = %v", err)
	}
	if provider.uuid != "uuid-a" || result.CPUPercent != 42 {
		t.Fatalf("result = %+v, uuid = %q", result, provider.uuid)
	}
	if _, err := service.Metrics(context.Background(), " "); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("empty uuid error = %v", err)
	}
	if _, err := New(fakeVMRepository{}, nil, false).Metrics(context.Background(), "uuid-a"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("missing provider error = %v", err)
	}
	provider.err = domain.ErrNotFound
	if _, err := service.Metrics(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("provider error = %v", err)
	}
}

func TestServiceUsesDynamicReadOnlyMode(t *testing.T) {
	mode := &dynamicReadOnly{value: true}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}}}
	service := New(repository, nil, false, WithReadOnlyMode(mode))
	if _, err := service.PerformAction(context.Background(), ActionCommand{UUID: "uuid-a", Action: domain.VMActionShutdown, Session: operatorSession()}); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only action error = %v", err)
	}
	mode.value = false
	if _, err := service.PerformAction(context.Background(), ActionCommand{UUID: "uuid-a", Action: domain.VMActionShutdown, Session: operatorSession()}); err != nil {
		t.Fatalf("writable action error = %v", err)
	}
}

func TestListFiltersAndSorts(t *testing.T) {
	service := New(fakeVMRepository{vms: []domain.VM{
		{UUID: "uuid-b", Name: "web-02", State: domain.VMStateRunning},
		{UUID: "uuid-a", Name: "api-01", State: domain.VMStateShutoff},
		{UUID: "uuid-c", Name: "web-01", State: domain.VMStateShutoff},
	}}, nil, false)
	result, err := service.List(context.Background(), domain.ListQuery{Query: "WEB", State: "shutoff"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.Total != 1 || result.Items[0].Name != "web-01" {
		t.Fatalf("result = %+v", result)
	}
	all, err := service.List(context.Background(), domain.ListQuery{})
	if err != nil || all.Total != 3 {
		t.Fatalf("all = %+v, %v", all, err)
	}
	if all.Items[0].Name != "api-01" || all.Items[2].Name != "web-02" {
		t.Fatalf("items not sorted: %+v", all.Items)
	}
}

func TestGetVM(t *testing.T) {
	service := New(fakeVMRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}}, nil, false)
	vm, err := service.Get(context.Background(), "uuid-a")
	if err != nil || vm.Name != "api-01" {
		t.Fatalf("Get() = %+v, %v", vm, err)
	}
	if _, err := service.Get(context.Background(), " "); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("empty UUID error = %v", err)
	}
}

func TestGetVMXML(t *testing.T) {
	repository := &xmlRepository{
		fakeVMRepository: fakeVMRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}},
		result:           domain.VMXML{UUID: "uuid-a", Name: "api-01", XML: "<domain/>"},
	}
	service := New(repository, nil, false)
	result, err := service.GetXML(context.Background(), " uuid-a ", "CONFIG")
	if err != nil {
		t.Fatalf("GetXML() error = %v", err)
	}
	if result.Name != "api-01" || repository.format != domain.VMXMLFormatConfig {
		t.Fatalf("result = %+v format = %q", result, repository.format)
	}

	_, err = service.GetXML(context.Background(), "uuid-a", "")
	if err != nil {
		t.Fatalf("GetXML(default) error = %v", err)
	}
	if repository.format != domain.VMXMLFormatLive {
		t.Fatalf("default format = %q", repository.format)
	}
}

func TestGetVMXMLValidationAndError(t *testing.T) {
	repository := &xmlRepository{fakeVMRepository: fakeVMRepository{err: domain.ErrUnavailable}}
	service := New(repository, nil, false)
	if _, err := service.GetXML(context.Background(), " ", "live"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("empty UUID error = %v", err)
	}
	if _, err := service.GetXML(context.Background(), "uuid-a", "runtime"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid format error = %v", err)
	}
	if _, err := service.GetXML(context.Background(), "uuid-a", "live"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("repository error = %v", err)
	}
}

func TestExportActiveVMIncludesLiveXMLSnapshotsAndAudit(t *testing.T) {
	repository := &exportRepository{
		fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}},
		configXML:        domain.VMXML{UUID: "uuid-a", Name: "api-01", XML: "<domain type='kvm'><name>api-01</name></domain>"},
		liveXML:          domain.VMXML{UUID: "uuid-a", Name: "api-01", XML: "<domain type='kvm'><name>api-01</name><devices/></domain>"},
		snapshots:        []domain.VMSnapshot{{Name: "before-upgrade", CreatedAt: 100}},
	}
	audit := &fakeAuditRecorder{}
	session := operatorSession()
	session.User.Role = domain.RoleViewer
	result, err := New(repository, audit, true).Export(context.Background(), ExportCommand{
		UUID: " uuid-a ", Request: MutationContext{Session: session, RequestID: "req-export", SourceIP: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if result.FormatVersion != domain.VMExportFormatVersion || result.VM.Name != "api-01" || result.ExportedAt.IsZero() {
		t.Fatalf("export = %+v", result)
	}
	if result.ConfigXML.Format != domain.VMXMLFormatConfig || result.ConfigXML.XML == "" {
		t.Fatalf("config xml = %+v", result.ConfigXML)
	}
	if result.LiveXML == nil || result.LiveXML.Format != domain.VMXMLFormatLive || result.LiveXML.XML == "" {
		t.Fatalf("live xml = %+v", result.LiveXML)
	}
	if result.Snapshots.Total != 1 || result.Snapshots.Items[0].Name != "before-upgrade" {
		t.Fatalf("snapshots = %+v", result.Snapshots)
	}
	if result.Disclaimer == "" {
		t.Fatal("export has no disclaimer")
	}
	if len(repository.formats) != 2 || repository.formats[0] != domain.VMXMLFormatConfig || repository.formats[1] != domain.VMXMLFormatLive {
		t.Fatalf("xml formats = %v", repository.formats)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	event := audit.events[0]
	if event.Action != "vm.export" || event.TargetKind != "vm" || event.TargetID != "uuid-a" || event.Result != "success" || event.Actor != "operator" || event.Role != domain.RoleViewer {
		t.Fatalf("audit event = %+v", event)
	}
	if event.Detail["name"] != "api-01" || event.Detail["includeLiveXml"] != true || event.Detail["snapshotCount"] != 1 {
		t.Fatalf("audit detail = %+v", event.Detail)
	}
}

func TestExportShutoffVMOnlyIncludesPersistentXML(t *testing.T) {
	repository := &exportRepository{
		fakeVMRepository: fakeVMRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01", State: domain.VMStateShutoff}}},
		configXML:        domain.VMXML{UUID: "uuid-a", Name: "api-01", XML: "<domain/>"},
	}
	audit := &fakeAuditRecorder{}
	result, err := New(repository, audit, false).Export(context.Background(), ExportCommand{
		UUID: "uuid-a", Request: MutationContext{Session: operatorSession()},
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if result.LiveXML != nil {
		t.Fatalf("live xml = %+v", result.LiveXML)
	}
	if len(repository.formats) != 1 || repository.formats[0] != domain.VMXMLFormatConfig {
		t.Fatalf("xml formats = %v", repository.formats)
	}
	if audit.events[0].Detail["includeLiveXml"] != false {
		t.Fatalf("audit detail = %+v", audit.events[0].Detail)
	}
}

func TestExportValidatesUUIDAndAuditsBackendFailures(t *testing.T) {
	if _, err := New(fakeVMRepository{}, nil, false).Export(context.Background(), ExportCommand{UUID: " "}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("empty UUID error = %v", err)
	}

	tests := []struct {
		name       string
		repository *exportRepository
		want       error
	}{
		{name: "missing VM", repository: &exportRepository{fakeVMRepository: fakeVMRepository{err: domain.ErrNotFound}}, want: domain.ErrNotFound},
		{name: "config XML", repository: &exportRepository{
			fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}},
			xmlErr:           map[domain.VMXMLFormat]error{domain.VMXMLFormatConfig: domain.ErrUnavailable},
		}, want: domain.ErrUnavailable},
		{name: "live XML", repository: &exportRepository{
			fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}},
			xmlErr:           map[domain.VMXMLFormat]error{domain.VMXMLFormatLive: domain.ErrUnavailable},
		}, want: domain.ErrUnavailable},
		{name: "snapshots", repository: &exportRepository{
			fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}},
			snapshotErr:      domain.ErrUnavailable,
		}, want: domain.ErrUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			audit := &fakeAuditRecorder{}
			_, err := New(test.repository, audit, false).Export(context.Background(), ExportCommand{
				UUID: "uuid-a", Request: MutationContext{Session: operatorSession()},
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if len(audit.events) != 1 || audit.events[0].Action != "vm.export" || audit.events[0].Result != "failure" || audit.events[0].TargetID != "uuid-a" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestExportFailsClosedWhenAuditFails(t *testing.T) {
	repository := &exportRepository{
		fakeVMRepository: fakeVMRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01", State: domain.VMStateShutoff}}},
		configXML:        domain.VMXML{UUID: "uuid-a", Name: "api-01", XML: "<domain/>"},
	}
	_, err := New(repository, &fakeAuditRecorder{err: errors.New("audit unavailable")}, false).Export(context.Background(), ExportCommand{
		UUID: "uuid-a", Request: MutationContext{Session: operatorSession()},
	})
	if err == nil || !strings.Contains(err.Error(), "record VM export audit") {
		t.Fatalf("error = %v", err)
	}
}

func TestPerformActionRejectsViewer(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}}}
	service := New(repository, audit, false)
	session := operatorSession()
	session.User.Role = domain.RoleViewer

	_, err := service.PerformAction(context.Background(), ActionCommand{UUID: "uuid-a", Action: domain.VMActionShutdown, Session: session})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("PerformAction() error = %v", err)
	}
	if len(repository.performed) != 0 {
		t.Fatalf("viewer action reached backend: %v", repository.performed)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestPerformActionRejectsReadOnly(t *testing.T) {
	audit := &fakeAuditRecorder{}
	service := New(fakeVMRepository{vms: []domain.VM{runningVM()}}, audit, true)
	_, err := service.PerformAction(context.Background(), ActionCommand{UUID: "uuid-a", Action: domain.VMActionShutdown, Session: operatorSession()})
	if !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("PerformAction() error = %v", err)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.shutdown" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestPerformActionRejectsInvalidState(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}}}
	service := New(repository, audit, false)
	_, err := service.PerformAction(context.Background(), ActionCommand{UUID: "uuid-a", Action: domain.VMActionStart, Session: operatorSession()})
	if !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("PerformAction() error = %v", err)
	}
	if len(repository.performed) != 0 {
		t.Fatalf("invalid action reached backend: %v", repository.performed)
	}
	if audit.events[0].Detail["previousState"] != "running" {
		t.Fatalf("audit detail = %+v", audit.events[0].Detail)
	}
}

func TestPerformActionRecordsSuccess(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}}}
	service := New(repository, audit, false)
	vm, err := service.PerformAction(context.Background(), ActionCommand{
		UUID: "uuid-a", Action: domain.VMActionShutdown, Session: operatorSession(),
		RequestID: "req-1", SourceIP: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("PerformAction() error = %v", err)
	}
	if vm.State != domain.VMStateShutoff || len(repository.performed) != 1 {
		t.Fatalf("vm = %+v performed=%v", vm, repository.performed)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	event := audit.events[0]
	if event.Result != "success" || event.Actor != "operator" || event.RequestID != "req-1" || event.TargetID != "uuid-a" {
		t.Fatalf("audit event = %+v", event)
	}
	if event.Detail["previousState"] != "running" || event.Detail["newState"] != "shutoff" {
		t.Fatalf("audit state detail = %+v", event.Detail)
	}
}

func TestPerformActionPropagatesBackendFailure(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{
		vms: []domain.VM{runningVM()}, actionErr: errors.New("qemu failed"),
	}}
	service := New(repository, audit, false)
	_, err := service.PerformAction(context.Background(), ActionCommand{UUID: "uuid-a", Action: domain.VMActionShutdown, Session: operatorSession()})
	if err == nil {
		t.Fatal("PerformAction() error = nil")
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestBatchActionContinuesAfterPerVMFailure(t *testing.T) {
	audit := &fakeAuditRecorder{}
	second := runningVM()
	second.UUID = "uuid-b"
	second.Name = "api-02"
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM(), second}}}
	service := New(repository, audit, false)

	result, err := service.BatchAction(context.Background(), BatchActionCommand{
		Action:    domain.VMActionShutdown,
		UUIDs:     []string{"uuid-a", "missing", "uuid-b", "uuid-a"},
		Session:   operatorSession(),
		RequestID: "req-batch",
		SourceIP:  "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("BatchAction() error = %v", err)
	}
	if result.Succeeded != 2 || result.Failed != 1 || len(result.Items) != 3 {
		t.Fatalf("result = %+v", result)
	}
	if result.Items[0].Name != "api-01" || !result.Items[0].Success || result.Items[0].VM == nil {
		t.Fatalf("first item = %+v", result.Items[0])
	}
	if result.Items[1].Success || result.Items[1].Error != "虚拟机不存在" {
		t.Fatalf("missing item = %+v", result.Items[1])
	}
	if result.Items[2].Name != "api-02" || !result.Items[2].Success {
		t.Fatalf("last item = %+v", result.Items[2])
	}
	if len(repository.performed) != 2 {
		t.Fatalf("performed actions = %v", repository.performed)
	}
	if len(audit.events) != 3 {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestBatchActionRejectsViewerAndReadOnly(t *testing.T) {
	viewer := operatorSession()
	viewer.User.Role = domain.RoleViewer
	viewerAudit := &fakeAuditRecorder{}
	viewerService := New(&recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}}}, viewerAudit, false)

	if _, err := viewerService.BatchAction(context.Background(), BatchActionCommand{
		Action: domain.VMActionShutdown, UUIDs: []string{"uuid-a"}, Session: viewer,
	}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("viewer error = %v", err)
	}
	if len(viewerAudit.events) != 1 || viewerAudit.events[0].Action != "vm.batch" || viewerAudit.events[0].Result != "failure" {
		t.Fatalf("viewer audit = %+v", viewerAudit.events)
	}

	readOnlyAudit := &fakeAuditRecorder{}
	readOnlyService := New(&recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}}}, readOnlyAudit, true)
	if _, err := readOnlyService.BatchAction(context.Background(), BatchActionCommand{
		Action: domain.VMActionShutdown, UUIDs: []string{"uuid-a"}, Session: operatorSession(),
	}); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only error = %v", err)
	}
	if len(readOnlyAudit.events) != 1 || readOnlyAudit.events[0].Action != "vm.batch" || readOnlyAudit.events[0].Detail["count"] != 1 {
		t.Fatalf("read-only audit = %+v", readOnlyAudit.events)
	}
}

func TestBatchActionValidatesInput(t *testing.T) {
	service := New(&recordingRepository{}, nil, false)
	session := operatorSession()
	invalid := [][]string{
		nil,
		{" "},
		make([]string, maxBatchVMCount+1),
	}
	for _, uuids := range invalid {
		if _, err := service.BatchAction(context.Background(), BatchActionCommand{
			Action: domain.VMActionStart, UUIDs: uuids, Session: session,
		}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("uuids=%v error = %v", uuids, err)
		}
	}
	if _, err := service.BatchAction(context.Background(), BatchActionCommand{
		Action: "explode", UUIDs: []string{"uuid-a"}, Session: session,
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid action error = %v", err)
	}
}

func TestCreateVM(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{}}
	service := New(repository, audit, false)
	vm, err := service.Create(context.Background(), CreateCommand{
		Spec: domain.VMCreateSpec{
			Name: " api-02 ", VCPUs: 2, MemoryMiB: 2048,
			DiskPath: "/var/lib/libvirt/images/api-02.qcow2",
		},
		Request: MutationContext{Session: operatorSession(), RequestID: "req-create", SourceIP: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if vm.Name != "api-02" || vm.State != domain.VMStateShutoff {
		t.Fatalf("vm = %+v", vm)
	}
	if len(repository.createdSpecs) != 1 {
		t.Fatalf("created specs = %+v", repository.createdSpecs)
	}
	spec := repository.createdSpecs[0]
	if spec.Name != "api-02" || spec.DiskFormat != "qcow2" || spec.Network != "default" || spec.Firmware != domain.VMFirmwareBIOS || spec.SecureBoot {
		t.Fatalf("normalized spec = %+v", spec)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.create" || audit.events[0].TargetID != "uuid-created" {
		t.Fatalf("audit events = %+v", audit.events)
	}
	if audit.events[0].Detail["firmware"] != domain.VMFirmwareBIOS || audit.events[0].Detail["secureBoot"] != false {
		t.Fatalf("audit detail = %+v", audit.events[0].Detail)
	}
}

func TestCreateVMUEFISecureBoot(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{}}
	service := New(repository, audit, false)
	spec := withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) {
		spec.Firmware = domain.VMFirmwareUEFI
		spec.SecureBoot = true
	})
	if _, err := service.Create(context.Background(), CreateCommand{
		Spec: spec, Request: MutationContext{Session: operatorSession()},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(repository.createdSpecs) != 1 || repository.createdSpecs[0].Firmware != domain.VMFirmwareUEFI || !repository.createdSpecs[0].SecureBoot {
		t.Fatalf("created specs = %+v", repository.createdSpecs)
	}
	if len(audit.events) != 1 || audit.events[0].Detail["firmware"] != domain.VMFirmwareUEFI || audit.events[0].Detail["secureBoot"] != true {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestCreateVMCloudInit(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{}}
	service := New(repository, audit, false)
	const userData = "#cloud-config\nruncmd:\n  - echo do-not-audit\n"
	spec := withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) {
		spec.CloudInit = &domain.CloudInitSpec{UserData: "  " + userData + "  ", Pool: " default "}
	})
	if _, err := service.Create(context.Background(), CreateCommand{
		Spec: spec, Request: MutationContext{Session: operatorSession()},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(repository.createdSpecs) != 1 || repository.createdSpecs[0].CloudInit == nil {
		t.Fatalf("created specs = %+v", repository.createdSpecs)
	}
	created := repository.createdSpecs[0].CloudInit
	if created.Pool != "default" || created.UserData != strings.TrimSpace(userData) {
		t.Fatalf("cloud-init spec = %+v", created)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	detail := audit.events[0].Detail
	if detail["cloudInit"] != true || detail["cloudInitPool"] != "default" || detail["cloudInitUserDataBytes"] != len(strings.TrimSpace(userData)) {
		t.Fatalf("audit detail = %+v", detail)
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "do-not-audit") {
		t.Fatalf("audit detail leaked cloud-init user data: %s", encoded)
	}
}

func TestCreateVMRejectsViewerReadOnlyAndInvalidSpec(t *testing.T) {
	tests := []struct {
		name     string
		session  domain.Session
		readOnly bool
		spec     domain.VMCreateSpec
		want     error
	}{
		{
			name: "viewer", session: domain.Session{User: domain.User{Role: domain.RoleViewer}},
			spec: validCreateSpec(), want: domain.ErrForbidden,
		},
		{
			name: "read only", session: operatorSession(), readOnly: true,
			spec: validCreateSpec(), want: domain.ErrReadOnly,
		},
		{name: "bad name", session: operatorSession(), spec: withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) { spec.Name = "-bad" }), want: domain.ErrInvalidInput},
		{name: "no vcpu", session: operatorSession(), spec: withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) { spec.VCPUs = 0 }), want: domain.ErrInvalidInput},
		{name: "small memory", session: operatorSession(), spec: withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) { spec.MemoryMiB = 64 }), want: domain.ErrInvalidInput},
		{name: "relative disk", session: operatorSession(), spec: withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) { spec.DiskPath = "disk.qcow2" }), want: domain.ErrInvalidInput},
		{name: "bad format", session: operatorSession(), spec: withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) { spec.DiskFormat = "vmdk" }), want: domain.ErrInvalidInput},
		{name: "bad network", session: operatorSession(), spec: withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) { spec.Network = "bad network" }), want: domain.ErrInvalidInput},
		{name: "relative iso", session: operatorSession(), spec: withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) { spec.ISOPath = "install.iso" }), want: domain.ErrInvalidInput},
		{name: "bad firmware", session: operatorSession(), spec: withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) { spec.Firmware = "coreboot" }), want: domain.ErrInvalidInput},
		{name: "secure boot without uefi", session: operatorSession(), spec: withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) { spec.SecureBoot = true }), want: domain.ErrInvalidInput},
		{name: "empty cloud-init", session: operatorSession(), spec: withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) {
			spec.CloudInit = &domain.CloudInitSpec{UserData: " ", Pool: "default"}
		}), want: domain.ErrInvalidInput},
		{name: "bad cloud-init pool", session: operatorSession(), spec: withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) {
			spec.CloudInit = &domain.CloudInitSpec{UserData: "#cloud-config\n", Pool: "-bad"}
		}), want: domain.ErrInvalidInput},
		{name: "large cloud-init", session: operatorSession(), spec: withSpec(validCreateSpec(), func(spec *domain.VMCreateSpec) {
			spec.CloudInit = &domain.CloudInitSpec{UserData: strings.Repeat("a", maxCloudInitUserDataBytes+1), Pool: "default"}
		}), want: domain.ErrInvalidInput},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			audit := &fakeAuditRecorder{}
			repository := &recordingRepository{fakeVMRepository: fakeVMRepository{}}
			service := New(repository, audit, test.readOnly)
			_, err := service.Create(context.Background(), CreateCommand{Spec: test.spec, Request: MutationContext{Session: test.session}})
			if !errors.Is(err, test.want) {
				t.Fatalf("Create() error = %v, want %v", err, test.want)
			}
			if len(repository.createdSpecs) != 0 {
				t.Fatalf("repository was called: %+v", repository.createdSpecs)
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestCreateVMPropagatesConflict(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{createErr: domain.ErrConflict}}
	service := New(repository, audit, false)
	_, err := service.Create(context.Background(), CreateCommand{Spec: validCreateSpec(), Request: MutationContext{Session: operatorSession()}})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Create() error = %v", err)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

type fakeTaskRecorder struct {
	mu           sync.Mutex
	startedKind  string
	startedID    string
	startedStep  string
	progress     []int
	succeeded    bool
	failed       error
	finishedStep string
	done         chan struct{}
}

func newFakeTaskRecorder() *fakeTaskRecorder {
	return &fakeTaskRecorder{startedID: "task-clone", done: make(chan struct{})}
}

func (f *fakeTaskRecorder) Start(_ context.Context, kind, _, _, _, step string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startedKind = kind
	f.startedStep = step
	return f.startedID, nil
}

func (f *fakeTaskRecorder) Progress(_ context.Context, _ string, progress int, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.progress = append(f.progress, progress)
	return nil
}

func (f *fakeTaskRecorder) Succeed(_ context.Context, _ string, step string) error {
	f.mu.Lock()
	f.succeeded = true
	f.finishedStep = step
	f.mu.Unlock()
	close(f.done)
	return nil
}

func (f *fakeTaskRecorder) Fail(_ context.Context, _ string, progress int, _ string, cause error) error {
	f.mu.Lock()
	f.failed = cause
	f.progress = append(f.progress, progress)
	f.mu.Unlock()
	close(f.done)
	return nil
}

func (f *fakeTaskRecorder) wait(t *testing.T) {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(time.Second):
		t.Fatal("clone task did not finish")
	}
}

func TestCloneVMStartsTaskAndAudits(t *testing.T) {
	source := persistentVM()
	source.State = domain.VMStateShutoff
	source.AllowedActions = domain.AllowedVMActions(source.State)
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{source}}}
	audit := &fakeAuditRecorder{}
	taskRecorder := newFakeTaskRecorder()
	service := New(repository, audit, false, WithTaskRecorder(taskRecorder))

	task, err := service.Clone(context.Background(), CloneCommand{
		UUID: "uuid-a",
		Spec: domain.VMCloneSpec{Name: "api-clone", Autostart: true, Start: true},
		Request: MutationContext{
			Session: operatorSession(), RequestID: "req-clone", SourceIP: "127.0.0.1",
		},
	})
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	if task.ID != "task-clone" || task.Kind != "vm.clone" || task.Status != domain.TaskStatusRunning {
		t.Fatalf("task = %+v", task)
	}
	taskRecorder.wait(t)

	taskRecorder.mu.Lock()
	defer taskRecorder.mu.Unlock()
	if taskRecorder.startedKind != "vm.clone" || taskRecorder.startedStep != "准备克隆 api-clone" {
		t.Fatalf("started kind=%q step=%q", taskRecorder.startedKind, taskRecorder.startedStep)
	}
	if !taskRecorder.succeeded || taskRecorder.failed != nil || len(taskRecorder.progress) != 2 || taskRecorder.progress[1] != 100 {
		t.Fatalf("task recorder = %+v", taskRecorder)
	}
	if repository.clonedUUID != "uuid-a" || repository.clonedSpec.Name != "api-clone" || !repository.clonedSpec.Autostart || !repository.clonedSpec.Start {
		t.Fatalf("backend clone uuid=%q spec=%+v", repository.clonedUUID, repository.clonedSpec)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.clone" || audit.events[0].Result != "success" || audit.events[0].TargetID != "uuid-cloned" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestCloneVMRejectsInvalidSourceAndMarksTaskFailure(t *testing.T) {
	t.Run("running source", func(t *testing.T) {
		repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{persistentVM()}}}
		audit := &fakeAuditRecorder{}
		taskRecorder := newFakeTaskRecorder()
		service := New(repository, audit, false, WithTaskRecorder(taskRecorder))
		_, err := service.Clone(context.Background(), CloneCommand{
			UUID: "uuid-a", Spec: domain.VMCloneSpec{Name: "api-clone"}, Request: MutationContext{Session: operatorSession()},
		})
		if !errors.Is(err, domain.ErrActionNotAllowed) {
			t.Fatalf("Clone() error = %v", err)
		}
		if taskRecorder.startedKind != "" || repository.clonedUUID != "" || len(audit.events) != 1 || audit.events[0].Result != "failure" {
			t.Fatalf("unexpected side effects: task=%+v repo=%+v audit=%+v", taskRecorder, repository, audit.events)
		}
	})

	t.Run("duplicate name", func(t *testing.T) {
		source := persistentVM()
		source.State = domain.VMStateShutoff
		source.Name = "api-01"
		repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{source}}}
		service := New(repository, nil, false, WithTaskRecorder(newFakeTaskRecorder()))
		_, err := service.Clone(context.Background(), CloneCommand{
			UUID: "uuid-a", Spec: domain.VMCloneSpec{Name: "api-01"}, Request: MutationContext{Session: operatorSession()},
		})
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("Clone() error = %v", err)
		}
	})

	t.Run("backend failure", func(t *testing.T) {
		source := persistentVM()
		source.State = domain.VMStateShutoff
		repository := &recordingRepository{fakeVMRepository: fakeVMRepository{
			vms: []domain.VM{source}, cloneErr: errors.New("disk copy failed"),
		}}
		taskRecorder := newFakeTaskRecorder()
		service := New(repository, nil, false, WithTaskRecorder(taskRecorder))
		if _, err := service.Clone(context.Background(), CloneCommand{
			UUID: "uuid-a", Spec: domain.VMCloneSpec{Name: "api-clone"}, Request: MutationContext{Session: operatorSession()},
		}); err != nil {
			t.Fatalf("Clone() error = %v", err)
		}
		taskRecorder.wait(t)
		taskRecorder.mu.Lock()
		defer taskRecorder.mu.Unlock()
		if taskRecorder.failed == nil || taskRecorder.succeeded {
			t.Fatalf("task recorder = %+v", taskRecorder)
		}
	})
}

func TestBackupVMStartsTaskAndAuditsWithoutXML(t *testing.T) {
	source := persistentVM()
	source.State = domain.VMStateShutoff
	source.AllowedActions = domain.AllowedVMActions(source.State)
	source.Disks = []domain.Disk{
		{Device: "disk", Source: "/var/lib/libvirt/images/api-01.qcow2", Format: "qcow2"},
		{Device: "cdrom", Source: "/var/lib/libvirt/iso/install.iso", Format: "raw", ReadOnly: true},
	}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{source}}}
	audit := &fakeAuditRecorder{}
	taskRecorder := newFakeTaskRecorder()
	service := New(repository, audit, false, WithTaskRecorder(taskRecorder))

	task, err := service.Backup(context.Background(), BackupCommand{
		UUID: "uuid-a", Spec: domain.VMBackupSpec{Name: "api-backup", TargetPool: "archive"},
		Request: MutationContext{Session: operatorSession(), RequestID: "req-backup", SourceIP: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	if task.ID != "task-clone" || task.Kind != "vm.backup" || task.Status != domain.TaskStatusRunning {
		t.Fatalf("task = %+v", task)
	}
	taskRecorder.wait(t)

	taskRecorder.mu.Lock()
	defer taskRecorder.mu.Unlock()
	if taskRecorder.startedKind != "vm.backup" || taskRecorder.startedStep != "准备备份 api-backup" {
		t.Fatalf("started kind=%q step=%q", taskRecorder.startedKind, taskRecorder.startedStep)
	}
	if !taskRecorder.succeeded || taskRecorder.failed != nil || len(taskRecorder.progress) != 2 || taskRecorder.progress[1] != 100 {
		t.Fatalf("task recorder = %+v", taskRecorder)
	}
	if repository.backupUUID != "uuid-a" || repository.backupSpec.Name != "api-backup" || repository.backupSpec.TargetPool != "archive" {
		t.Fatalf("backend backup uuid=%q spec=%+v", repository.backupUUID, repository.backupSpec)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.backup" || audit.events[0].Result != "success" || audit.events[0].TargetID != "uuid-a" {
		t.Fatalf("audit events = %+v", audit.events)
	}
	detail := audit.events[0].Detail
	if detail["diskCount"] != 1 || detail["targetPool"] != "archive" || detail["backupName"] != "api-backup" || detail["totalBytes"] != uint64(2048) {
		t.Fatalf("audit detail = %+v", detail)
	}
	if _, exists := detail["xml"]; exists {
		t.Fatalf("audit detail must not contain XML: %+v", detail)
	}
}

func TestBackupVMRejectsInvalidRequests(t *testing.T) {
	shutoff := persistentVM()
	shutoff.State = domain.VMStateShutoff
	shutoff.AllowedActions = domain.AllowedVMActions(shutoff.State)
	for _, test := range []struct {
		name     string
		readOnly bool
		source   domain.VM
		spec     domain.VMBackupSpec
		role     domain.Role
		want     error
	}{
		{name: "viewer", source: shutoff, spec: domain.VMBackupSpec{Name: "backup", TargetPool: "archive"}, role: domain.RoleViewer, want: domain.ErrForbidden},
		{name: "read only", readOnly: true, source: shutoff, spec: domain.VMBackupSpec{Name: "backup", TargetPool: "archive"}, role: domain.RoleOperator, want: domain.ErrReadOnly},
		{name: "running source", source: persistentVM(), spec: domain.VMBackupSpec{Name: "backup", TargetPool: "archive"}, role: domain.RoleOperator, want: domain.ErrActionNotAllowed},
		{name: "invalid name", source: shutoff, spec: domain.VMBackupSpec{Name: "../backup", TargetPool: "archive"}, role: domain.RoleOperator, want: domain.ErrInvalidInput},
		{name: "invalid pool", source: shutoff, spec: domain.VMBackupSpec{Name: "backup", TargetPool: "bad/pool"}, role: domain.RoleOperator, want: domain.ErrInvalidInput},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{test.source}}}
			audit := &fakeAuditRecorder{}
			taskRecorder := newFakeTaskRecorder()
			session := operatorSession()
			session.User.Role = test.role
			_, err := New(repository, audit, test.readOnly, WithTaskRecorder(taskRecorder)).Backup(context.Background(), BackupCommand{
				UUID: "uuid-a", Spec: test.spec, Request: MutationContext{Session: session},
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if taskRecorder.startedKind != "" || repository.backupUUID != "" {
				t.Fatalf("rejected request reached backend: task=%+v repo=%+v", taskRecorder, repository)
			}
			if len(audit.events) != 1 || audit.events[0].Action != "vm.backup" || audit.events[0].Result != "failure" {
				t.Fatalf("audit = %+v", audit.events)
			}
		})
	}
}

func TestBackupVMFailureAuditsAndFailsTask(t *testing.T) {
	source := persistentVM()
	source.State = domain.VMStateCrashed
	source.AllowedActions = domain.AllowedVMActions(source.State)
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{source}, backupErr: errors.New("disk copy failed")}}
	audit := &fakeAuditRecorder{}
	taskRecorder := newFakeTaskRecorder()
	service := New(repository, audit, false, WithTaskRecorder(taskRecorder))
	if _, err := service.Backup(context.Background(), BackupCommand{
		UUID: "uuid-a", Spec: domain.VMBackupSpec{Name: "api-backup", TargetPool: "archive"}, Request: MutationContext{Session: operatorSession()},
	}); err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	taskRecorder.wait(t)
	taskRecorder.mu.Lock()
	defer taskRecorder.mu.Unlock()
	if taskRecorder.failed == nil || taskRecorder.succeeded {
		t.Fatalf("task recorder = %+v", taskRecorder)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.backup" || audit.events[0].Result != "failure" {
		t.Fatalf("audit = %+v", audit.events)
	}
}

func TestDeleteVM(t *testing.T) {
	audit := &fakeAuditRecorder{}
	shutoff := runningVM()
	shutoff.State = domain.VMStateShutoff
	shutoff.AllowedActions = domain.AllowedVMActions(shutoff.State)
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{shutoff}}}
	service := New(repository, audit, false)
	err := service.Delete(context.Background(), DeleteCommand{UUID: "uuid-a", Request: MutationContext{Session: operatorSession()}})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(repository.deleted) != 1 || len(repository.vms) != 0 {
		t.Fatalf("deleted=%v vms=%+v", repository.deleted, repository.vms)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.delete" || audit.events[0].Detail["state"] != "shutoff" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestDeleteVMRejectsRunning(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}}}
	service := New(repository, audit, false)
	err := service.Delete(context.Background(), DeleteCommand{UUID: "uuid-a", Request: MutationContext{Session: operatorSession()}})
	if !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(repository.deleted) != 0 {
		t.Fatalf("delete reached backend: %v", repository.deleted)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestDeleteVMRejectsViewerAndReadOnly(t *testing.T) {
	for _, test := range []struct {
		name     string
		session  domain.Session
		readOnly bool
		want     error
	}{
		{name: "viewer", session: domain.Session{User: domain.User{Role: domain.RoleViewer}}, want: domain.ErrForbidden},
		{name: "read only", session: operatorSession(), readOnly: true, want: domain.ErrReadOnly},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}}}
			service := New(repository, nil, test.readOnly)
			err := service.Delete(context.Background(), DeleteCommand{UUID: "uuid-a", Request: MutationContext{Session: test.session}})
			if !errors.Is(err, test.want) {
				t.Fatalf("Delete() error = %v, want %v", err, test.want)
			}
			if len(repository.deleted) != 0 {
				t.Fatalf("delete reached backend: %v", repository.deleted)
			}
		})
	}
}

func TestSetAutostartRecordsSuccess(t *testing.T) {
	audit := &fakeAuditRecorder{}
	vm := persistentVM()
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{vm}}}
	service := New(repository, audit, false)

	updated, err := service.SetAutostart(context.Background(), SetAutostartCommand{
		UUID: "uuid-a", Enabled: true,
		Request: MutationContext{Session: operatorSession(), RequestID: "req-autostart", SourceIP: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("SetAutostart() error = %v", err)
	}
	if !updated.Autostart || repository.autostartUUID != "uuid-a" || !repository.autostartEnabled {
		t.Fatalf("updated=%+v uuid=%q enabled=%v", updated, repository.autostartUUID, repository.autostartEnabled)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	event := audit.events[0]
	if event.Action != "vm.autostart" || event.Result != "success" || event.TargetID != "uuid-a" || event.RequestID != "req-autostart" {
		t.Fatalf("audit event = %+v", event)
	}
	if event.Detail["previousAutostart"] != false || event.Detail["newAutostart"] != true {
		t.Fatalf("audit detail = %+v", event.Detail)
	}
}

func TestSetAutostartRejectsForbiddenReadOnlyAndTransientVM(t *testing.T) {
	viewer := operatorSession()
	viewer.User.Role = domain.RoleViewer
	tests := []struct {
		name     string
		vm       domain.VM
		session  domain.Session
		readOnly bool
		want     error
	}{
		{name: "viewer", vm: persistentVM(), session: viewer, want: domain.ErrForbidden},
		{name: "read only", vm: persistentVM(), session: operatorSession(), readOnly: true, want: domain.ErrReadOnly},
		{name: "transient", vm: runningVM(), session: operatorSession(), want: domain.ErrActionNotAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			audit := &fakeAuditRecorder{}
			repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{test.vm}}}
			service := New(repository, audit, test.readOnly)
			_, err := service.SetAutostart(context.Background(), SetAutostartCommand{
				UUID: "uuid-a", Enabled: true, Request: MutationContext{Session: test.session},
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("SetAutostart() error = %v, want %v", err, test.want)
			}
			if repository.autostartCallsCount != 0 {
				t.Fatalf("autostart reached backend %d times", repository.autostartCallsCount)
			}
			if len(audit.events) != 1 || audit.events[0].Action != "vm.autostart" || audit.events[0].Result != "failure" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestSetAutostartAuditsBackendFailure(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{
		vms: []domain.VM{persistentVM()}, autostartErr: errors.New("libvirt failed"),
	}}
	service := New(repository, audit, false)
	_, err := service.SetAutostart(context.Background(), SetAutostartCommand{
		UUID: "uuid-a", Enabled: true, Request: MutationContext{Session: operatorSession()},
	})
	if err == nil {
		t.Fatal("SetAutostart() error = nil")
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" || audit.events[0].Detail["error"] == nil {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestSetAutostartFailsClosedWhenAuditFails(t *testing.T) {
	audit := &fakeAuditRecorder{err: errors.New("audit unavailable")}
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{persistentVM()}}}
	service := New(repository, audit, false)
	_, err := service.SetAutostart(context.Background(), SetAutostartCommand{
		UUID: "uuid-a", Enabled: true, Request: MutationContext{Session: operatorSession()},
	})
	if err == nil || !strings.Contains(err.Error(), "record VM autostart audit") {
		t.Fatalf("SetAutostart() error = %v", err)
	}
}

func TestSnapshotListCreateRevertAndDelete(t *testing.T) {
	repository := &snapshotRepository{
		fakeVMRepository: fakeVMRepository{vms: []domain.VM{runningVM()}},
		snapshots: []domain.VMSnapshot{
			{Name: "older", CreatedAt: 100},
			{Name: "newer", CreatedAt: 200},
		},
	}
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false)

	list, err := service.ListSnapshots(context.Background(), " uuid-a ")
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if list.Total != 2 || list.Items[0].Name != "newer" || repository.listUUID != "uuid-a" {
		t.Fatalf("list = %+v uuid = %q", list, repository.listUUID)
	}

	created, err := service.CreateSnapshot(context.Background(), SnapshotCreateCommand{
		UUID: " uuid-a ", Spec: domain.VMSnapshotCreateSpec{Name: " before-upgrade ", Description: " stable state "},
		Request: MutationContext{Session: operatorSession(), RequestID: "req-snapshot", SourceIP: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	if created.Name != "before-upgrade" || repository.createdUUID != "uuid-a" || repository.createdSpec.Description != "stable state" {
		t.Fatalf("created = %+v repo = %+v uuid=%q", created, repository.createdSpec, repository.createdUUID)
	}

	if err := service.RevertSnapshot(context.Background(), SnapshotCommand{
		UUID: "uuid-a", Name: " before-upgrade ", Request: MutationContext{Session: operatorSession()},
	}); err != nil {
		t.Fatalf("RevertSnapshot() error = %v", err)
	}
	if repository.revertedUUID != "uuid-a" || repository.revertedName != "before-upgrade" {
		t.Fatalf("reverted uuid=%q name=%q", repository.revertedUUID, repository.revertedName)
	}

	if err := service.DeleteSnapshot(context.Background(), SnapshotCommand{
		UUID: "uuid-a", Name: "before-upgrade", Request: MutationContext{Session: operatorSession()},
	}); err != nil {
		t.Fatalf("DeleteSnapshot() error = %v", err)
	}
	if repository.deletedSnapshotUUID != "uuid-a" || repository.deletedSnapshotName != "before-upgrade" {
		t.Fatalf("deleted uuid=%q name=%q", repository.deletedSnapshotUUID, repository.deletedSnapshotName)
	}

	if len(audit.events) != 3 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	for _, event := range audit.events {
		if event.TargetKind != "vm_snapshot" || event.TargetID != "uuid-a/before-upgrade" || event.Result != "success" || event.Actor != "operator" {
			t.Fatalf("audit event = %+v", event)
		}
	}
	if audit.events[0].Action != "vm.snapshot.create" || audit.events[0].Detail["stateAtSnapshot"] != "running" {
		t.Fatalf("create audit = %+v", audit.events[0])
	}
}

func TestSnapshotMutationsRejectInvalidRequests(t *testing.T) {
	longDescription := strings.Repeat("x", 1025)
	for _, test := range []struct {
		name     string
		readOnly bool
		run      func(*Service) error
		want     error
	}{
		{name: "create viewer", run: func(service *Service) error {
			session := operatorSession()
			session.User.Role = domain.RoleViewer
			_, err := service.CreateSnapshot(context.Background(), SnapshotCreateCommand{UUID: "uuid-a", Spec: domain.VMSnapshotCreateSpec{Name: "snap"}, Request: MutationContext{Session: session}})
			return err
		}, want: domain.ErrForbidden},
		{name: "create read only", readOnly: true, run: func(service *Service) error {
			_, err := service.CreateSnapshot(context.Background(), SnapshotCreateCommand{UUID: "uuid-a", Spec: domain.VMSnapshotCreateSpec{Name: "snap"}, Request: MutationContext{Session: operatorSession()}})
			return err
		}, want: domain.ErrReadOnly},
		{name: "invalid name", run: func(service *Service) error {
			_, err := service.CreateSnapshot(context.Background(), SnapshotCreateCommand{UUID: "uuid-a", Spec: domain.VMSnapshotCreateSpec{Name: "../snap"}, Request: MutationContext{Session: operatorSession()}})
			return err
		}, want: domain.ErrInvalidInput},
		{name: "description too long", run: func(service *Service) error {
			_, err := service.CreateSnapshot(context.Background(), SnapshotCreateCommand{UUID: "uuid-a", Spec: domain.VMSnapshotCreateSpec{Name: "snap", Description: longDescription}, Request: MutationContext{Session: operatorSession()}})
			return err
		}, want: domain.ErrInvalidInput},
		{name: "revert viewer", run: func(service *Service) error {
			session := operatorSession()
			session.User.Role = domain.RoleViewer
			return service.RevertSnapshot(context.Background(), SnapshotCommand{UUID: "uuid-a", Name: "snap", Request: MutationContext{Session: session}})
		}, want: domain.ErrForbidden},
		{name: "revert invalid name", run: func(service *Service) error {
			return service.RevertSnapshot(context.Background(), SnapshotCommand{UUID: "uuid-a", Name: "bad/name", Request: MutationContext{Session: operatorSession()}})
		}, want: domain.ErrInvalidInput},
		{name: "delete read only", readOnly: true, run: func(service *Service) error {
			return service.DeleteSnapshot(context.Background(), SnapshotCommand{UUID: "uuid-a", Name: "snap", Request: MutationContext{Session: operatorSession()}})
		}, want: domain.ErrReadOnly},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &snapshotRepository{}
			audit := &fakeAuditRecorder{}
			err := test.run(New(repository, audit, test.readOnly))
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if repository.createdSpec.Name != "" || repository.revertedName != "" || repository.deletedSnapshotName != "" {
				t.Fatal("rejected request reached repository")
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" || audit.events[0].TargetKind != "vm_snapshot" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestSnapshotRepositoryErrorsAreAudited(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*Service) error
	}{
		{name: "create", run: func(service *Service) error {
			_, err := service.CreateSnapshot(context.Background(), SnapshotCreateCommand{UUID: "uuid-a", Spec: domain.VMSnapshotCreateSpec{Name: "snap"}, Request: MutationContext{Session: operatorSession()}})
			return err
		}},
		{name: "revert", run: func(service *Service) error {
			return service.RevertSnapshot(context.Background(), SnapshotCommand{UUID: "uuid-a", Name: "snap", Request: MutationContext{Session: operatorSession()}})
		}},
		{name: "delete", run: func(service *Service) error {
			return service.DeleteSnapshot(context.Background(), SnapshotCommand{UUID: "uuid-a", Name: "snap", Request: MutationContext{Session: operatorSession()}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &snapshotRepository{snapshotErr: domain.ErrSnapshotRisky}
			audit := &fakeAuditRecorder{}
			err := test.run(New(repository, audit, false))
			if !errors.Is(err, domain.ErrSnapshotRisky) {
				t.Fatalf("error = %v", err)
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestHostDeviceAttachDetachAudits(t *testing.T) {
	repository := &hostDeviceRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{
		{UUID: "uuid-a", Name: "api-01", State: domain.VMStateShutoff},
		{UUID: "uuid-b", Name: "api-02", State: domain.VMStateShutoff},
	}}}
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false)
	request := MutationContext{Session: operatorSession(), RequestID: "req-device", SourceIP: "127.0.0.1"}

	vm, err := service.AttachHostDevice(context.Background(), HostDeviceCommand{UUID: " uuid-a ", Device: " pci_0000_3b_00_0 ", Request: request})
	if err != nil {
		t.Fatalf("AttachHostDevice() error = %v", err)
	}
	if len(vm.Passthrough) != 1 || repository.attachedUUID != "uuid-a" || repository.attachedName != "pci_0000_3b_00_0" {
		t.Fatalf("attached vm=%+v uuid=%q device=%q", vm, repository.attachedUUID, repository.attachedName)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.device.attach" || audit.events[0].Result != "success" || audit.events[0].Detail["deviceType"] != "pci" || audit.events[0].Detail["address"] != "0000:3b:00.0" {
		t.Fatalf("attach audit = %+v", audit.events)
	}

	vm, err = service.DetachHostDevice(context.Background(), HostDeviceCommand{UUID: "uuid-a", Device: "pci_0000_3b_00_0", Request: request})
	if err != nil {
		t.Fatalf("DetachHostDevice() error = %v", err)
	}
	if len(vm.Passthrough) != 0 || repository.detachedUUID != "uuid-a" || repository.detachedName != "pci_0000_3b_00_0" {
		t.Fatalf("detached vm=%+v uuid=%q device=%q", vm, repository.detachedUUID, repository.detachedName)
	}
	if len(audit.events) != 2 || audit.events[1].Action != "vm.device.detach" || audit.events[1].Result != "success" || audit.events[1].TargetID != "uuid-a" {
		t.Fatalf("detach audit = %+v", audit.events)
	}
}

func TestHostDeviceMutationsRejectInvalidRequests(t *testing.T) {
	tests := []struct {
		name     string
		readOnly bool
		vms      []domain.VM
		run      func(*Service) error
		want     error
	}{
		{name: "attach viewer", vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}, run: func(service *Service) error {
			session := operatorSession()
			session.User.Role = domain.RoleViewer
			_, err := service.AttachHostDevice(context.Background(), HostDeviceCommand{UUID: "uuid-a", Device: "pci_0000_3b_00_0", Request: MutationContext{Session: session}})
			return err
		}, want: domain.ErrForbidden},
		{name: "attach read only", readOnly: true, vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}, run: func(service *Service) error {
			_, err := service.AttachHostDevice(context.Background(), HostDeviceCommand{UUID: "uuid-a", Device: "pci_0000_3b_00_0", Request: MutationContext{Session: operatorSession()}})
			return err
		}, want: domain.ErrReadOnly},
		{name: "attach duplicate", vms: []domain.VM{{UUID: "uuid-a", Name: "api-01", Passthrough: []domain.PassthroughDevice{{Name: "pci_0000_3b_00_0", Type: "pci"}}}}, run: func(service *Service) error {
			_, err := service.AttachHostDevice(context.Background(), HostDeviceCommand{UUID: "uuid-a", Device: "pci_0000_3b_00_0", Request: MutationContext{Session: operatorSession()}})
			return err
		}, want: domain.ErrConflict},
		{name: "attach used by other VM", vms: []domain.VM{
			{UUID: "uuid-a", Name: "api-01"},
			{UUID: "uuid-b", Name: "api-02", Passthrough: []domain.PassthroughDevice{{Name: "pci_0000_3b_00_0", Type: "pci"}}},
		}, run: func(service *Service) error {
			_, err := service.AttachHostDevice(context.Background(), HostDeviceCommand{UUID: "uuid-a", Device: "pci_0000_3b_00_0", Request: MutationContext{Session: operatorSession()}})
			return err
		}, want: domain.ErrConflict},
		{name: "detach missing", vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}, run: func(service *Service) error {
			_, err := service.DetachHostDevice(context.Background(), HostDeviceCommand{UUID: "uuid-a", Device: "pci_0000_3b_00_0", Request: MutationContext{Session: operatorSession()}})
			return err
		}, want: domain.ErrNotFound},
		{name: "invalid device name", vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}, run: func(service *Service) error {
			_, err := service.AttachHostDevice(context.Background(), HostDeviceCommand{UUID: "uuid-a", Device: "../pci", Request: MutationContext{Session: operatorSession()}})
			return err
		}, want: domain.ErrInvalidInput},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &hostDeviceRepository{fakeVMRepository: fakeVMRepository{vms: append([]domain.VM(nil), test.vms...)}}
			audit := &fakeAuditRecorder{}
			err := test.run(New(repository, audit, test.readOnly))
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if test.want != domain.ErrInvalidInput && (len(audit.events) != 1 || audit.events[0].Result != "failure") {
				t.Fatalf("audit events = %+v", audit.events)
			}
			if repository.attachCalls+repository.detachCalls != 0 {
				t.Fatalf("backend called for rejected request: attach=%d detach=%d", repository.attachCalls, repository.detachCalls)
			}
		})
	}
}

func TestHostDeviceBackendFailureAudits(t *testing.T) {
	repository := &hostDeviceRepository{
		fakeVMRepository: fakeVMRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}},
		attachErr:        domain.ErrUnavailable,
	}
	audit := &fakeAuditRecorder{}
	_, err := New(repository, audit, false).AttachHostDevice(context.Background(), HostDeviceCommand{
		UUID: "uuid-a", Device: "pci_0000_3b_00_0", Request: MutationContext{Session: operatorSession()},
	})
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" || audit.events[0].Action != "vm.device.attach" {
		t.Fatalf("audit = %+v", audit.events)
	}
}

type recordingRepository struct {
	fakeVMRepository
	actionUpdates       map[string]domain.VM
	clonedUUID          string
	clonedSpec          domain.VMCloneSpec
	backupUUID          string
	backupSpec          domain.VMBackupSpec
	createdSpecs        []domain.VMCreateSpec
	deleted             []string
	autostartUUID       string
	autostartEnabled    bool
	autostartCallsCount int
}

func (f *recordingRepository) CloneVM(_ context.Context, uuid string, spec domain.VMCloneSpec, progress func(int, string)) (domain.VM, error) {
	f.clonedUUID = uuid
	f.clonedSpec = spec
	if f.cloneErr != nil {
		return domain.VM{}, f.cloneErr
	}
	if progress != nil {
		progress(60, "正在复制磁盘")
		progress(100, "克隆完成")
	}
	state := domain.VMStateShutoff
	if spec.Start {
		state = domain.VMStateRunning
	}
	return domain.VM{UUID: "uuid-cloned", Name: spec.Name, State: state}, nil
}

func (f *recordingRepository) BackupVM(_ context.Context, uuid string, spec domain.VMBackupSpec, progress func(int, string)) (domain.VMBackup, error) {
	f.backupUUID = uuid
	f.backupSpec = spec
	if f.backupErr != nil {
		return domain.VMBackup{}, f.backupErr
	}
	if progress != nil {
		progress(60, "正在复制磁盘")
		progress(100, "备份完成")
	}
	return domain.VMBackup{
		FormatVersion: domain.VMBackupFormatVersion, VMUUID: uuid, VMName: "api-01", Name: spec.Name,
		TargetPool: spec.TargetPool, ManifestVolume: spec.Name + "-manifest.json",
		ConfigVolume: spec.Name + "-domain.xml", ConfigBytes: 1024, TotalBytes: 2048,
		Disks: []domain.VMBackupDisk{{SourceVolume: "api-01.qcow2", BackupVolume: spec.Name + "-disk-01.qcow2", Format: "qcow2", CapacityBytes: 1024}},
	}, nil
}

type snapshotRepository struct {
	fakeVMRepository
	snapshots           []domain.VMSnapshot
	snapshotErr         error
	listUUID            string
	createdUUID         string
	createdSpec         domain.VMSnapshotCreateSpec
	revertedUUID        string
	revertedName        string
	deletedSnapshotUUID string
	deletedSnapshotName string
}

type hostDeviceRepository struct {
	fakeVMRepository
	attachErr    error
	detachErr    error
	attachedUUID string
	attachedName string
	detachedUUID string
	detachedName string
	attachCalls  int
	detachCalls  int
}

func (f *hostDeviceRepository) AttachHostDevice(_ context.Context, uuid, deviceName string) (domain.VM, error) {
	f.attachedUUID = uuid
	f.attachedName = deviceName
	f.attachCalls++
	if f.attachErr != nil {
		return domain.VM{}, f.attachErr
	}
	for index := range f.vms {
		if f.vms[index].UUID != uuid {
			continue
		}
		f.vms[index].Passthrough = append(f.vms[index].Passthrough, domain.PassthroughDevice{
			Name: deviceName, Type: "pci", VendorID: "0x10de", ProductID: "0x2204", Address: "0000:3b:00.0",
		})
		return f.vms[index], nil
	}
	return domain.VM{}, domain.ErrNotFound
}

func (f *hostDeviceRepository) DetachHostDevice(_ context.Context, uuid, deviceName string) (domain.VM, error) {
	f.detachedUUID = uuid
	f.detachedName = deviceName
	f.detachCalls++
	if f.detachErr != nil {
		return domain.VM{}, f.detachErr
	}
	for index := range f.vms {
		if f.vms[index].UUID != uuid {
			continue
		}
		for deviceIndex, device := range f.vms[index].Passthrough {
			if device.Name == deviceName {
				f.vms[index].Passthrough = append(f.vms[index].Passthrough[:deviceIndex], f.vms[index].Passthrough[deviceIndex+1:]...)
				return f.vms[index], nil
			}
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func (f *snapshotRepository) ListSnapshots(_ context.Context, uuid string) ([]domain.VMSnapshot, error) {
	f.listUUID = uuid
	return append([]domain.VMSnapshot(nil), f.snapshots...), f.snapshotErr
}

func (f *snapshotRepository) CreateSnapshot(_ context.Context, uuid string, spec domain.VMSnapshotCreateSpec) (domain.VMSnapshot, error) {
	f.createdUUID = uuid
	f.createdSpec = spec
	if f.snapshotErr != nil {
		return domain.VMSnapshot{}, f.snapshotErr
	}
	return domain.VMSnapshot{Name: spec.Name, Description: spec.Description, CreatedAt: 300, State: domain.VMStateRunning, Current: true}, nil
}

func (f *snapshotRepository) RevertSnapshot(_ context.Context, uuid, name string) error {
	f.revertedUUID = uuid
	f.revertedName = name
	return f.snapshotErr
}

func (f *snapshotRepository) DeleteSnapshot(_ context.Context, uuid, name string) error {
	f.deletedSnapshotUUID = uuid
	f.deletedSnapshotName = name
	return f.snapshotErr
}

type xmlRepository struct {
	fakeVMRepository
	format domain.VMXMLFormat
	result domain.VMXML
}

func (f *xmlRepository) GetVMXML(_ context.Context, uuid string, format domain.VMXMLFormat) (domain.VMXML, error) {
	f.format = format
	if f.err != nil {
		return domain.VMXML{}, f.err
	}
	result := f.result
	if result.UUID == "" {
		result.UUID = uuid
	}
	result.Format = format
	return result, nil
}

type exportRepository struct {
	fakeVMRepository
	configXML   domain.VMXML
	liveXML     domain.VMXML
	xmlErr      map[domain.VMXMLFormat]error
	snapshots   []domain.VMSnapshot
	snapshotErr error
	formats     []domain.VMXMLFormat
}

func (f *exportRepository) GetVMXML(_ context.Context, uuid string, format domain.VMXMLFormat) (domain.VMXML, error) {
	f.formats = append(f.formats, format)
	if err := f.xmlErr[format]; err != nil {
		return domain.VMXML{}, err
	}
	result := f.configXML
	if format == domain.VMXMLFormatLive {
		result = f.liveXML
	}
	if result.UUID == "" {
		result.UUID = uuid
	}
	result.Format = format
	return result, nil
}

func (f *exportRepository) ListSnapshots(context.Context, string) ([]domain.VMSnapshot, error) {
	return append([]domain.VMSnapshot(nil), f.snapshots...), f.snapshotErr
}

func (f *recordingRepository) CreateVM(ctx context.Context, spec domain.VMCreateSpec) (domain.VM, error) {
	f.createdSpecs = append(f.createdSpecs, spec)
	if f.createErr != nil {
		return domain.VM{}, f.createErr
	}
	state := domain.VMStateShutoff
	if spec.Start {
		state = domain.VMStateRunning
	}
	vm := domain.VM{UUID: "uuid-created", Name: spec.Name, State: state, AllowedActions: domain.AllowedVMActions(state)}
	f.vms = append(f.vms, vm)
	return vm, nil
}

func (f *recordingRepository) DeleteVM(_ context.Context, uuid string) error {
	f.deleted = append(f.deleted, uuid)
	if f.deleteErr != nil {
		return f.deleteErr
	}
	for i := range f.vms {
		if f.vms[i].UUID == uuid {
			f.vms = append(f.vms[:i], f.vms[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *recordingRepository) SetVMAutostart(_ context.Context, uuid string, enabled bool) error {
	f.autostartUUID = uuid
	f.autostartEnabled = enabled
	f.autostartCallsCount++
	if f.autostartErr != nil {
		return f.autostartErr
	}
	for i := range f.vms {
		if f.vms[i].UUID == uuid {
			f.vms[i].Autostart = enabled
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *recordingRepository) PerformAction(ctx context.Context, uuid string, action domain.VMAction) error {
	f.performed = append(f.performed, action)
	if f.actionErr != nil {
		return f.actionErr
	}
	for i := range f.vms {
		if f.vms[i].UUID != uuid {
			continue
		}
		vm := f.vms[i]
		if action == domain.VMActionShutdown || action == domain.VMActionDestroy {
			vm.State = domain.VMStateShutoff
		}
		vm.AllowedActions = domain.AllowedVMActions(vm.State)
		f.vms[i] = vm
		return nil
	}
	return domain.ErrNotFound
}

func runningVM() domain.VM {
	return domain.VM{
		UUID: "uuid-a", Name: "api-01", State: domain.VMStateRunning,
		AllowedActions: domain.AllowedVMActions(domain.VMStateRunning),
	}
}

func persistentVM() domain.VM {
	vm := runningVM()
	vm.Persistent = true
	vm.Autostart = false
	return vm
}

func validCreateSpec() domain.VMCreateSpec {
	return domain.VMCreateSpec{
		Name: "api-02", VCPUs: 2, MemoryMiB: 2048,
		DiskPath: "/var/lib/libvirt/images/api-02.qcow2", DiskFormat: "qcow2", Network: "default",
	}
}

func withSpec(spec domain.VMCreateSpec, mutate func(*domain.VMCreateSpec)) domain.VMCreateSpec {
	mutate(&spec)
	return spec
}
