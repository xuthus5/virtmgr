package vmshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"virtmgr/internal/domain"
	vmsapp "virtmgr/internal/modules/vms/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/consoletoken"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

type handlerRepository struct {
	vms              []domain.VM
	actionErr        error
	actionSeen       domain.VMAction
	autostartErr     error
	autostartSeen    bool
	autostartUUID    string
	resourcesErr     error
	resourcesSeen    domain.VMResourcesSpec
	resourcesUUID    string
	diskResizeErr    error
	diskResizeSeen   domain.VMDiskResizeSpec
	diskResizeUUID   string
	diskResizeTarget string
	attachErr        error
	attachedDevice   string
	detachErr        error
	detachedDevice   string
	xmlFormat        domain.VMXMLFormat
	xmlErr           error
	defineXMLErr     error
	definedXML       string
	definedXMLUUID   string
	createErr        error
	created          domain.VMCreateSpec
	cloneMu          sync.Mutex
	cloneErr         error
	cloneUUID        string
	cloneSpec        domain.VMCloneSpec
	backupErr        error
	backupUUID       string
	backupSpec       domain.VMBackupSpec
	deleteErr        error
	deleted          string
	snapshots        []domain.VMSnapshot
	snapshotErr      error
	snapshotSpec     domain.VMSnapshotCreateSpec
	revertedSnapshot string
	deletedSnapshot  string
	openedConsole    uint
	openedSerial     string
	sentKeys         []uint
}

type testConsoleConn struct{ *strings.Reader }

type handlerTaskRecorder struct{}

type handlerMetrics struct{}

func (handlerMetrics) VM(_ context.Context, uuid string) (domain.VMMetrics, error) {
	return domain.VMMetrics{UUID: uuid, ResourceMetrics: domain.ResourceMetrics{
		CPUPercent: 25, MemoryUsedBytes: 4 << 30, MemoryTotalBytes: 16 << 30,
		NetworkRxBytesPerSecond: 1000, NetworkTxBytesPerSecond: 2000,
	}}, nil
}

func (handlerTaskRecorder) Start(context.Context, string, string, string, string, string) (string, error) {
	return "task-clone", nil
}

func (handlerTaskRecorder) Progress(context.Context, string, int, string) error { return nil }
func (handlerTaskRecorder) Succeed(context.Context, string, string) error       { return nil }
func (handlerTaskRecorder) Fail(context.Context, string, int, string, error) error {
	return nil
}

func (testConsoleConn) Write(buffer []byte) (int, error) { return len(buffer), nil }
func (testConsoleConn) Close() error                     { return nil }

func (f *handlerRepository) ListVMs(context.Context) ([]domain.VM, error) {
	return f.vms, nil
}

func (f *handlerRepository) GetVM(_ context.Context, uuid string) (domain.VM, error) {
	for _, vm := range f.vms {
		if vm.UUID == uuid {
			return vm, nil
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func (f *handlerRepository) GetVMXML(_ context.Context, uuid string, format domain.VMXMLFormat) (domain.VMXML, error) {
	f.xmlFormat = format
	if f.xmlErr != nil {
		return domain.VMXML{}, f.xmlErr
	}
	return domain.VMXML{UUID: uuid, Name: "api-01", Format: format, XML: "<domain/>"}, nil
}

func (f *handlerRepository) DefineVMXML(_ context.Context, uuid, xmlText string) (domain.VMXML, error) {
	f.definedXML = xmlText
	f.definedXMLUUID = uuid
	if f.defineXMLErr != nil {
		return domain.VMXML{}, f.defineXMLErr
	}
	return domain.VMXML{UUID: uuid, Name: "api-01", Format: domain.VMXMLFormatConfig, XML: xmlText}, nil
}

func (f *handlerRepository) SetVMAutostart(_ context.Context, uuid string, enabled bool) error {
	f.autostartSeen = enabled
	f.autostartUUID = uuid
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

func (f *handlerRepository) UpdateVMResources(_ context.Context, uuid string, spec domain.VMResourcesSpec) (domain.VM, error) {
	f.resourcesUUID = uuid
	f.resourcesSeen = spec
	if f.resourcesErr != nil {
		return domain.VM{}, f.resourcesErr
	}
	for i := range f.vms {
		if f.vms[i].UUID == uuid {
			f.vms[i].VCPUs = spec.VCPUs
			f.vms[i].MaxVCPUs = max(f.vms[i].MaxVCPUs, spec.VCPUs)
			f.vms[i].MemoryBytes = spec.MemoryMiB * 1024 * 1024
			f.vms[i].MaxMemoryBytes = max(f.vms[i].MaxMemoryBytes, f.vms[i].MemoryBytes)
			return f.vms[i], nil
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func (f *handlerRepository) ResizeVMDisk(_ context.Context, uuid, target string, spec domain.VMDiskResizeSpec) (domain.VM, error) {
	f.diskResizeUUID = uuid
	f.diskResizeTarget = target
	f.diskResizeSeen = spec
	if f.diskResizeErr != nil {
		return domain.VM{}, f.diskResizeErr
	}
	for i := range f.vms {
		if f.vms[i].UUID != uuid {
			continue
		}
		for index := range f.vms[i].Disks {
			if f.vms[i].Disks[index].Target == target {
				f.vms[i].Disks[index].CapacityBytes = spec.CapacityBytes
				return f.vms[i], nil
			}
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func (f *handlerRepository) AttachHostDevice(_ context.Context, uuid, deviceName string) (domain.VM, error) {
	f.attachedDevice = deviceName
	if f.attachErr != nil {
		return domain.VM{}, f.attachErr
	}
	for i := range f.vms {
		if f.vms[i].UUID != uuid {
			continue
		}
		f.vms[i].Passthrough = append(f.vms[i].Passthrough, domain.PassthroughDevice{Name: deviceName, Type: "pci"})
		return f.vms[i], nil
	}
	return domain.VM{}, domain.ErrNotFound
}

func (f *handlerRepository) DetachHostDevice(_ context.Context, uuid, deviceName string) (domain.VM, error) {
	f.detachedDevice = deviceName
	if f.detachErr != nil {
		return domain.VM{}, f.detachErr
	}
	for i := range f.vms {
		if f.vms[i].UUID != uuid {
			continue
		}
		for index, device := range f.vms[i].Passthrough {
			if device.Name == deviceName {
				f.vms[i].Passthrough = append(f.vms[i].Passthrough[:index], f.vms[i].Passthrough[index+1:]...)
				return f.vms[i], nil
			}
		}
	}
	return domain.VM{}, domain.ErrNotFound
}

func (f *handlerRepository) PerformAction(_ context.Context, uuid string, action domain.VMAction) error {
	f.actionSeen = action
	if f.actionErr != nil {
		return f.actionErr
	}
	for i := range f.vms {
		if f.vms[i].UUID == uuid {
			f.vms[i].State = domain.VMStateRunning
			f.vms[i].AllowedActions = domain.AllowedVMActions(domain.VMStateRunning)
		}
	}
	return nil
}

func (f *handlerRepository) CreateVM(_ context.Context, spec domain.VMCreateSpec) (domain.VM, error) {
	f.created = spec
	if f.createErr != nil {
		return domain.VM{}, f.createErr
	}
	state := domain.VMStateShutoff
	if spec.Start {
		state = domain.VMStateRunning
	}
	return domain.VM{UUID: "uuid-created", Name: spec.Name, State: state}, nil
}

func (f *handlerRepository) CloneVM(_ context.Context, uuid string, spec domain.VMCloneSpec, progress func(int, string)) (domain.VM, error) {
	f.cloneMu.Lock()
	defer f.cloneMu.Unlock()
	f.cloneUUID = uuid
	f.cloneSpec = spec
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

func (f *handlerRepository) BackupVM(_ context.Context, uuid string, spec domain.VMBackupSpec, progress func(int, string)) (domain.VMBackup, error) {
	f.cloneMu.Lock()
	defer f.cloneMu.Unlock()
	f.backupUUID = uuid
	f.backupSpec = spec
	if f.backupErr != nil {
		return domain.VMBackup{}, f.backupErr
	}
	if progress != nil {
		progress(100, "备份完成")
	}
	return domain.VMBackup{
		FormatVersion: domain.VMBackupFormatVersion, VMUUID: uuid, VMName: "api-01", Name: spec.Name,
		TargetPool: spec.TargetPool, ManifestVolume: spec.Name + "-manifest.json",
		ConfigVolume: spec.Name + "-domain.xml", TotalBytes: 2048,
	}, nil
}

func (f *handlerRepository) DeleteVM(_ context.Context, uuid string) error {
	f.deleted = uuid
	return f.deleteErr
}

func (f *handlerRepository) ListSnapshots(context.Context, string) ([]domain.VMSnapshot, error) {
	return f.snapshots, f.snapshotErr
}

func (f *handlerRepository) CreateSnapshot(_ context.Context, _ string, spec domain.VMSnapshotCreateSpec) (domain.VMSnapshot, error) {
	f.snapshotSpec = spec
	if f.snapshotErr != nil {
		return domain.VMSnapshot{}, f.snapshotErr
	}
	return domain.VMSnapshot{Name: spec.Name, Description: spec.Description, CreatedAt: 123, State: domain.VMStateShutoff, Current: true}, nil
}

func (f *handlerRepository) RevertSnapshot(_ context.Context, _ string, name string) error {
	f.revertedSnapshot = name
	return f.snapshotErr
}

func (f *handlerRepository) DeleteSnapshot(_ context.Context, _ string, name string) error {
	f.deletedSnapshot = name
	return f.snapshotErr
}

func (f *handlerRepository) OpenConsole(_ context.Context, _ string, graphicsIndex uint) (io.ReadWriteCloser, error) {
	f.openedConsole = graphicsIndex
	if f.snapshotErr != nil {
		return nil, f.snapshotErr
	}
	return testConsoleConn{Reader: strings.NewReader("RFB 003.003\n")}, nil
}

func (f *handlerRepository) OpenSerialConsole(_ context.Context, _ string, device string) (io.ReadWriteCloser, error) {
	f.openedSerial = device
	if f.snapshotErr != nil {
		return nil, f.snapshotErr
	}
	return testConsoleConn{Reader: strings.NewReader("demo login: ")}, nil
}

func (f *handlerRepository) SendConsoleKey(_ context.Context, _ string, keycodes []uint) error {
	f.sentKeys = append([]uint(nil), keycodes...)
	return f.actionErr
}

func newHandlerRouter(repository *handlerRepository, session *domain.Session, csrfOK bool) *chi.Mux {
	router := chi.NewRouter()
	router.Use(func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			if session != nil {
				r = r.WithContext(authctx.WithSession(r.Context(), *session))
			}
			next.ServeHTTP(w, r)
		})
	})
	handler := New(vmsapp.New(repository, nil, false,
		vmsapp.WithConsoleTokens(consoletoken.New(time.Minute)),
		vmsapp.WithTaskRecorder(handlerTaskRecorder{}),
		vmsapp.WithMetrics(handlerMetrics{}),
	))
	handler.Mount(router, func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			if !csrfOK {
				w.WriteHeader(stdhttp.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	return router
}

func TestGetVM(t *testing.T) {
	repository := &handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01", State: domain.VMStateShutoff}}}
	router := newHandlerRouter(repository, nil, true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/vms/uuid-a", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestConsoleTokenAndWebSocket(t *testing.T) {
	repository := &handlerRepository{vms: []domain.VM{{
		UUID: "uuid-console", Name: "console-01", State: domain.VMStateRunning,
		Graphics: []domain.Graphics{{Type: "vnc", Port: 5901}},
	}}}
	session := domain.Session{User: domain.User{ID: 9, Username: "operator", Role: domain.RoleOperator}}
	router := newHandlerRouter(repository, &session, true)
	server := httptest.NewServer(router)
	defer server.Close()

	response, err := stdhttp.Post(server.URL+"/vms/uuid-console/console-token", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != stdhttp.StatusCreated {
		t.Fatalf("token status = %d body=%s", response.StatusCode, response.Body)
	}
	var ticket domain.ConsoleTicket
	if err := json.NewDecoder(response.Body).Decode(&ticket); err != nil {
		t.Fatal(err)
	}
	if ticket.Path == "" || ticket.Token == "" {
		t.Fatalf("ticket = %+v", ticket)
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + strings.TrimPrefix(ticket.Path, "/api/v1")
	connection, handshake, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		body := ""
		if handshake != nil {
			defer handshake.Body.Close()
			payload := make([]byte, 512)
			count, _ := handshake.Body.Read(payload)
			body = string(payload[:count])
		}
		t.Fatalf("websocket dial: %v status=%v body=%s", err, handshake, body)
	}
	defer connection.Close()
	_, message, err := connection.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket message: %v", err)
	}
	if string(message) != "RFB 003.003\n" {
		t.Fatalf("message = %q", message)
	}

	invalid, err := stdhttp.Get(server.URL + "/vms/uuid-console/console?token=invalid")
	if err != nil {
		t.Fatal(err)
	}
	defer invalid.Body.Close()
	if invalid.StatusCode != stdhttp.StatusUnauthorized {
		t.Fatalf("invalid token status = %d", invalid.StatusCode)
	}
}

func TestGetVMXML(t *testing.T) {
	repository := &handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}}
	router := newHandlerRouter(repository, nil, true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/vms/uuid-a/xml?format=config", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result domain.VMXML
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.UUID != "uuid-a" || result.Format != domain.VMXMLFormatConfig || result.XML != "<domain/>" {
		t.Fatalf("result = %+v", result)
	}
	if repository.xmlFormat != domain.VMXMLFormatConfig {
		t.Fatalf("repository format = %q", repository.xmlFormat)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/vms/uuid-a/xml?format=runtime", nil))
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("invalid format status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestUpdateVMXML(t *testing.T) {
	const xmlText = `<domain type="kvm"><name>api-01</name><uuid>uuid-a</uuid><vcpu>4</vcpu></domain>`
	session := operatorSession()
	session.User.Username = "admin"
	session.User.Role = domain.RoleAdmin
	repository := &handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01", Persistent: true, State: domain.VMStateShutoff}}}
	router := newHandlerRouter(repository, &session, true)
	requestBody, _ := json.Marshal(updateXMLInput{XML: xmlText})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPut, "/vms/uuid-a/xml", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result domain.VMXML
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Format != domain.VMXMLFormatConfig || result.XML != xmlText || repository.definedXMLUUID != "uuid-a" || repository.definedXML != xmlText {
		t.Fatalf("result = %+v repository uuid=%q xml=%q", result, repository.definedXMLUUID, repository.definedXML)
	}
}

func TestUpdateVMXMLRequiresAdminCSRFAndValidBody(t *testing.T) {
	const xmlText = `<domain><name>api-01</name><uuid>uuid-a</uuid></domain>`
	viewer := operatorSession()
	viewer.User.Role = domain.RoleViewer
	repository := &handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01", Persistent: true}}}
	router := newHandlerRouter(repository, &viewer, true)
	requestBody, _ := json.Marshal(updateXMLInput{XML: xmlText})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPut, "/vms/uuid-a/xml", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden {
		t.Fatalf("viewer status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	admin := operatorSession()
	admin.User.Role = domain.RoleAdmin
	router = newHandlerRouter(repository, &admin, false)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPut, "/vms/uuid-a/xml", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden || repository.definedXML != "" {
		t.Fatalf("csrf status = %d xml=%q", recorder.Code, repository.definedXML)
	}

	router = newHandlerRouter(repository, &admin, true)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPut, "/vms/uuid-a/xml", bytes.NewBufferString(`{"xml":"x","unexpected":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("invalid body status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestExportVM(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{
		vms:       []domain.VM{{UUID: "uuid-a", Name: "api-01", State: domain.VMStateRunning}},
		snapshots: []domain.VMSnapshot{{Name: "baseline", CreatedAt: 100, Current: true}},
	}
	router := newHandlerRouter(repository, &session, true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/vms/uuid-a/export", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("content type = %q", contentType)
	}
	if disposition := recorder.Header().Get("Content-Disposition"); disposition != `attachment; filename="api-01-config-export.json"` {
		t.Fatalf("content disposition = %q", disposition)
	}
	var result domain.VMExport
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.FormatVersion != domain.VMExportFormatVersion || result.VM.Name != "api-01" || result.ConfigXML.Format != domain.VMXMLFormatConfig {
		t.Fatalf("export = %+v", result)
	}
	if result.LiveXML == nil || result.LiveXML.Format != domain.VMXMLFormatLive || result.Snapshots.Total != 1 {
		t.Fatalf("export live=%+v snapshots=%+v", result.LiveXML, result.Snapshots)
	}
}

func TestExportVMRequiresSessionAndMapsErrors(t *testing.T) {
	router := newHandlerRouter(&handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}}, nil, true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/vms/uuid-a/export", nil))
	if recorder.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("anonymous status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	session := operatorSession()
	router = newHandlerRouter(&handlerRepository{}, &session, true)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/vms/missing/export", nil))
	if recorder.Code != stdhttp.StatusNotFound {
		t.Fatalf("missing status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestExportFilenameSanitizesVMName(t *testing.T) {
	tests := map[string]string{
		"api-01":         "api-01-config-export.json",
		" ../../web vm ": "web_vm-config-export.json",
		"...":            "vm-config-export.json",
	}
	for name, want := range tests {
		if got := exportFilename(name); got != want {
			t.Fatalf("exportFilename(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestGetVMMetrics(t *testing.T) {
	repository := &handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}}
	router := newHandlerRouter(repository, nil, true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/vms/uuid-a/metrics", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result domain.VMMetrics
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.UUID != "uuid-a" || result.CPUPercent != 25 || result.MemoryTotalBytes != 16<<30 || result.NetworkRxBytesPerSecond != 1000 {
		t.Fatalf("result = %+v", result)
	}
}

func TestCreateVMRequiresCSRFAndCreates(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{}
	body := `{"name":"api-02","vcpus":2,"memoryMiB":2048,"diskPath":"/var/lib/libvirt/images/api-02.qcow2","diskFormat":"qcow2","network":"default","autostart":false,"start":false}`

	router := newHandlerRouter(repository, &session, false)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/vms", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden || repository.created.Name != "" {
		t.Fatalf("missing csrf status = %d created=%+v", recorder.Code, repository.created)
	}

	router = newHandlerRouter(repository, &session, true)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPost, "/vms", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusCreated || repository.created.Name != "api-02" {
		t.Fatalf("status = %d created=%+v body=%s", recorder.Code, repository.created, recorder.Body.String())
	}
}

func TestCreateVMRejectsInvalidJSON(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{}
	router := newHandlerRouter(repository, &session, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/vms", bytes.NewBufferString(`{"name":`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusBadRequest || repository.created.Name != "" {
		t.Fatalf("status = %d created=%+v", recorder.Code, repository.created)
	}
}

func TestCloneVMRequiresCSRFAndReturnsTask(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{vms: []domain.VM{{
		UUID: "uuid-a", Name: "api-01", State: domain.VMStateShutoff, Persistent: true,
		AllowedActions: domain.AllowedVMActions(domain.VMStateShutoff),
	}}}
	body := `{"name":"api-clone","autostart":true,"start":false}`

	router := newHandlerRouter(repository, &session, false)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/clone", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden {
		t.Fatalf("missing csrf status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	router = newHandlerRouter(repository, &session, true)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/clone", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var task domain.Task
	if err := json.NewDecoder(recorder.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.ID != "task-clone" || task.Kind != "vm.clone" || task.TargetID != "uuid-a" || task.Status != domain.TaskStatusRunning {
		t.Fatalf("task = %+v", task)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		repository.cloneMu.Lock()
		cloned := repository.cloneUUID != ""
		repository.cloneMu.Unlock()
		if cloned {
			break
		}
		time.Sleep(time.Millisecond)
	}
	repository.cloneMu.Lock()
	defer repository.cloneMu.Unlock()
	if repository.cloneUUID != "uuid-a" || repository.cloneSpec.Name != "api-clone" || !repository.cloneSpec.Autostart {
		t.Fatalf("clone uuid=%q spec=%+v", repository.cloneUUID, repository.cloneSpec)
	}
}

func TestBackupVMRequiresCSRFAndReturnsTask(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{vms: []domain.VM{{
		UUID: "uuid-a", Name: "api-01", State: domain.VMStateShutoff, Persistent: true,
		AllowedActions: domain.AllowedVMActions(domain.VMStateShutoff),
	}}}
	body := `{"name":"api-backup","targetPool":"archive"}`

	router := newHandlerRouter(repository, &session, false)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/backups", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden {
		t.Fatalf("missing csrf status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	router = newHandlerRouter(repository, &session, true)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/backups", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var task domain.Task
	if err := json.NewDecoder(recorder.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.ID != "task-clone" || task.Kind != "vm.backup" || task.TargetID != "uuid-a" || task.Status != domain.TaskStatusRunning {
		t.Fatalf("task = %+v", task)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		repository.cloneMu.Lock()
		backedUp := repository.backupUUID != ""
		repository.cloneMu.Unlock()
		if backedUp {
			break
		}
		time.Sleep(time.Millisecond)
	}
	repository.cloneMu.Lock()
	defer repository.cloneMu.Unlock()
	if repository.backupUUID != "uuid-a" || repository.backupSpec.Name != "api-backup" || repository.backupSpec.TargetPool != "archive" {
		t.Fatalf("backup uuid=%q spec=%+v", repository.backupUUID, repository.backupSpec)
	}
}

func TestBackupVMRejectsInvalidJSON(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{}
	router := newHandlerRouter(repository, &session, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/backups", bytes.NewBufferString(`{"name":`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusBadRequest || repository.backupUUID != "" {
		t.Fatalf("status = %d backup=%+v", recorder.Code, repository.backupSpec)
	}
}

func TestDeleteVM(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{vms: []domain.VM{{
		UUID: "uuid-a", Name: "api-01", State: domain.VMStateShutoff,
		AllowedActions: domain.AllowedVMActions(domain.VMStateShutoff),
	}}}
	router := newHandlerRouter(repository, &session, true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodDelete, "/vms/uuid-a", nil))
	if recorder.Code != stdhttp.StatusNoContent || repository.deleted != "uuid-a" {
		t.Fatalf("status = %d deleted=%q body=%s", recorder.Code, repository.deleted, recorder.Body.String())
	}
}

func TestDeleteVMRejectsRunning(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{vms: []domain.VM{runningHandlerVM()}}
	router := newHandlerRouter(repository, &session, true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodDelete, "/vms/uuid-a", nil))
	if recorder.Code != stdhttp.StatusConflict || repository.deleted != "" {
		t.Fatalf("status = %d deleted=%q", recorder.Code, repository.deleted)
	}
}

func TestSetAutostartUpdatesVMAndRequiresCSRF(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{vms: []domain.VM{{
		UUID: "uuid-a", Name: "api-01", State: domain.VMStateRunning, Persistent: true,
	}}}
	router := newHandlerRouter(repository, &session, true)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/autostart", bytes.NewBufferString(`{"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK || !repository.autostartSeen || repository.autostartUUID != "uuid-a" {
		t.Fatalf("status = %d uuid=%q enabled=%v body=%s", recorder.Code, repository.autostartUUID, repository.autostartSeen, recorder.Body.String())
	}
	var vm domain.VM
	if err := json.NewDecoder(recorder.Body).Decode(&vm); err != nil {
		t.Fatal(err)
	}
	if !vm.Autostart {
		t.Fatalf("updated VM = %+v", vm)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/autostart", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("missing enabled status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	csrfRepository := &handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01", Persistent: true}}}
	router = newHandlerRouter(csrfRepository, &session, false)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/autostart", bytes.NewBufferString(`{"enabled":false}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden || csrfRepository.autostartUUID != "" {
		t.Fatalf("csrf status = %d uuid=%q", recorder.Code, csrfRepository.autostartUUID)
	}
}

func TestSetAutostartRejectsAnonymousAndViewer(t *testing.T) {
	repository := &handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01", Persistent: true}}}
	router := newHandlerRouter(repository, nil, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/autostart", bytes.NewBufferString(`{"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", recorder.Code)
	}

	viewer := operatorSession()
	viewer.User.Role = domain.RoleViewer
	router = newHandlerRouter(repository, &viewer, true)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/autostart", bytes.NewBufferString(`{"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden || repository.autostartUUID != "" {
		t.Fatalf("viewer status = %d uuid=%q", recorder.Code, repository.autostartUUID)
	}
}

func TestUpdateResourcesUpdatesVMAndRequiresCSRF(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{vms: []domain.VM{{
		UUID: "uuid-a", Name: "api-01", State: domain.VMStateRunning, Persistent: true,
		VCPUs: 2, MaxVCPUs: 4, MemoryBytes: 2 << 30, MaxMemoryBytes: 4 << 30,
	}}}
	router := newHandlerRouter(repository, &session, true)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/resources", bytes.NewBufferString(`{"vcpus":4,"memoryMiB":3072}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK || repository.resourcesUUID != "uuid-a" || repository.resourcesSeen.VCPUs != 4 || repository.resourcesSeen.MemoryMiB != 3072 {
		t.Fatalf("status = %d uuid=%q spec=%+v body=%s", recorder.Code, repository.resourcesUUID, repository.resourcesSeen, recorder.Body.String())
	}
	var vm domain.VM
	if err := json.NewDecoder(recorder.Body).Decode(&vm); err != nil {
		t.Fatal(err)
	}
	if vm.VCPUs != 4 || vm.MemoryBytes != 3<<30 {
		t.Fatalf("updated VM = %+v", vm)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/resources", bytes.NewBufferString(`{"vcpus":`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("invalid json status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	csrfRepository := &handlerRepository{vms: []domain.VM{{
		UUID: "uuid-a", Name: "api-01", State: domain.VMStateRunning, Persistent: true,
		VCPUs: 2, MaxVCPUs: 4, MemoryBytes: 2 << 30, MaxMemoryBytes: 4 << 30,
	}}}
	router = newHandlerRouter(csrfRepository, &session, false)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/resources", bytes.NewBufferString(`{"vcpus":3,"memoryMiB":3072}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden || csrfRepository.resourcesUUID != "" {
		t.Fatalf("csrf status = %d uuid=%q", recorder.Code, csrfRepository.resourcesUUID)
	}
}

func TestUpdateResourcesRejectsAnonymousViewerAndActiveOverflow(t *testing.T) {
	repository := &handlerRepository{vms: []domain.VM{{
		UUID: "uuid-a", Name: "api-01", State: domain.VMStateRunning, Persistent: true,
		VCPUs: 2, MaxVCPUs: 4, MemoryBytes: 2 << 30, MaxMemoryBytes: 4 << 30,
	}}}
	router := newHandlerRouter(repository, nil, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/resources", bytes.NewBufferString(`{"vcpus":3,"memoryMiB":3072}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", recorder.Code)
	}

	viewer := operatorSession()
	viewer.User.Role = domain.RoleViewer
	router = newHandlerRouter(repository, &viewer, true)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/resources", bytes.NewBufferString(`{"vcpus":3,"memoryMiB":3072}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden || repository.resourcesUUID != "" {
		t.Fatalf("viewer status = %d uuid=%q", recorder.Code, repository.resourcesUUID)
	}

	operator := operatorSession()
	router = newHandlerRouter(repository, &operator, true)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/resources", bytes.NewBufferString(`{"vcpus":5,"memoryMiB":3072}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusConflict || repository.resourcesUUID != "" {
		t.Fatalf("overflow status = %d uuid=%q body=%s", recorder.Code, repository.resourcesUUID, recorder.Body.String())
	}
}

func TestResizeDiskRoute(t *testing.T) {
	session := operatorSession()
	const currentCapacity = uint64(20 << 30)
	repository := &handlerRepository{vms: []domain.VM{{
		UUID: "uuid-a", Name: "api-01", State: domain.VMStateRunning, Persistent: true,
		Disks: []domain.Disk{{
			Device: "disk", Target: "vda", Bus: "virtio", Format: "qcow2",
			Source: "/var/lib/libvirt/images/api-01.qcow2", CapacityBytes: currentCapacity,
		}},
	}}}
	router := newHandlerRouter(repository, &session, true)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/disks/vda", bytes.NewBufferString(`{"capacityBytes":42949672960}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK || repository.diskResizeUUID != "uuid-a" || repository.diskResizeTarget != "vda" || repository.diskResizeSeen.CapacityBytes != 40<<30 {
		t.Fatalf("status = %d uuid=%q target=%q spec=%+v body=%s", recorder.Code, repository.diskResizeUUID, repository.diskResizeTarget, repository.diskResizeSeen, recorder.Body.String())
	}
	var vm domain.VM
	if err := json.NewDecoder(recorder.Body).Decode(&vm); err != nil {
		t.Fatal(err)
	}
	if len(vm.Disks) != 1 || vm.Disks[0].CapacityBytes != 40<<30 {
		t.Fatalf("updated VM = %+v", vm)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/disks/vda", bytes.NewBufferString(`{"capacityBytes":`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("invalid json status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	csrfRepository := &handlerRepository{vms: repository.vms}
	router = newHandlerRouter(csrfRepository, &session, false)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/disks/vda", bytes.NewBufferString(`{"capacityBytes":42949672960}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden || csrfRepository.diskResizeTarget != "" {
		t.Fatalf("csrf status = %d target=%q", recorder.Code, csrfRepository.diskResizeTarget)
	}
}

func TestResizeDiskRouteRejectsUnauthorizedViewerAndShrink(t *testing.T) {
	const currentCapacity = uint64(20 << 30)
	newVM := func() []domain.VM {
		return []domain.VM{{
			UUID: "uuid-a", Name: "api-01", State: domain.VMStateRunning, Persistent: true,
			Disks: []domain.Disk{{
				Device: "disk", Target: "vda", Bus: "virtio", Format: "qcow2",
				Source: "/var/lib/libvirt/images/api-01.qcow2", CapacityBytes: currentCapacity,
			}},
		}}
	}
	repository := &handlerRepository{vms: newVM()}
	router := newHandlerRouter(repository, nil, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/disks/vda", bytes.NewBufferString(`{"capacityBytes":42949672960}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", recorder.Code)
	}

	viewer := operatorSession()
	viewer.User.Role = domain.RoleViewer
	router = newHandlerRouter(repository, &viewer, true)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/disks/vda", bytes.NewBufferString(`{"capacityBytes":42949672960}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden || repository.diskResizeTarget != "" {
		t.Fatalf("viewer status = %d target=%q", recorder.Code, repository.diskResizeTarget)
	}

	operator := operatorSession()
	router = newHandlerRouter(repository, &operator, true)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPatch, "/vms/uuid-a/disks/vda", bytes.NewBufferString(`{"capacityBytes":10737418240}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusBadRequest || repository.diskResizeTarget != "" {
		t.Fatalf("shrink status = %d target=%q body=%s", recorder.Code, repository.diskResizeTarget, recorder.Body.String())
	}
}

func TestHostDeviceRoutes(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01", State: domain.VMStateShutoff}}}
	router := newHandlerRouter(repository, &session, true)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/devices", bytes.NewBufferString(`{"device":"pci_0000_3b_00_0"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK || repository.attachedDevice != "pci_0000_3b_00_0" {
		t.Fatalf("attach status = %d device=%q body=%s", recorder.Code, repository.attachedDevice, recorder.Body.String())
	}
	var vm domain.VM
	if err := json.NewDecoder(recorder.Body).Decode(&vm); err != nil {
		t.Fatal(err)
	}
	if len(vm.Passthrough) != 1 || vm.Passthrough[0].Name != "pci_0000_3b_00_0" {
		t.Fatalf("attached VM = %+v", vm)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodDelete, "/vms/uuid-a/devices/pci_0000_3b_00_0", nil))
	if recorder.Code != stdhttp.StatusOK || repository.detachedDevice != "pci_0000_3b_00_0" {
		t.Fatalf("detach status = %d device=%q body=%s", recorder.Code, repository.detachedDevice, recorder.Body.String())
	}
}

func TestHostDeviceRoutesRequireSessionCSRFAndValidateInput(t *testing.T) {
	repository := &handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}}
	router := newHandlerRouter(repository, nil, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/devices", bytes.NewBufferString(`{"device":"pci_0000_3b_00_0"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", recorder.Code)
	}

	session := operatorSession()
	router = newHandlerRouter(repository, &session, false)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/devices", bytes.NewBufferString(`{"device":"pci_0000_3b_00_0"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden || repository.attachedDevice != "" {
		t.Fatalf("csrf status = %d device=%q", recorder.Code, repository.attachedDevice)
	}

	router = newHandlerRouter(repository, &session, true)
	for _, body := range []string{`{}`, `{"device":"../pci"}`, `{"device":"pci_0000_3b_00_0","extra":true}`} {
		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/devices", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != stdhttp.StatusBadRequest {
			t.Fatalf("body=%s status = %d response=%s", body, recorder.Code, recorder.Body.String())
		}
	}
}

func TestActionRequiresSession(t *testing.T) {
	repository := &handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}}
	router := newHandlerRouter(repository, nil, true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/actions/start", nil))
	if recorder.Code != stdhttp.StatusUnauthorized || repository.actionSeen != "" {
		t.Fatalf("status = %d action=%q", recorder.Code, repository.actionSeen)
	}
}

func TestActionRequiresCSRF(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}}
	router := newHandlerRouter(repository, &session, false)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/actions/start", nil))
	if recorder.Code != stdhttp.StatusForbidden || repository.actionSeen != "" {
		t.Fatalf("status = %d action=%q", recorder.Code, repository.actionSeen)
	}
}

func TestActionRejectsUnknownAction(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{vms: []domain.VM{{UUID: "uuid-a", Name: "api-01"}}}
	router := newHandlerRouter(repository, &session, true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/actions/nuke", nil))
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestActionSuccess(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{vms: []domain.VM{{
		UUID: "uuid-a", Name: "api-01", State: domain.VMStateShutoff,
		AllowedActions: domain.AllowedVMActions(domain.VMStateShutoff),
	}}}
	router := newHandlerRouter(repository, &session, true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/actions/start", nil))
	if recorder.Code != stdhttp.StatusOK || repository.actionSeen != domain.VMActionStart {
		t.Fatalf("status = %d action=%q body=%s", recorder.Code, repository.actionSeen, recorder.Body.String())
	}
}

func TestActionMapsBackendFailure(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{
		vms: []domain.VM{{
			UUID: "uuid-a", Name: "api-01", State: domain.VMStateShutoff,
			AllowedActions: domain.AllowedVMActions(domain.VMStateShutoff),
		}},
		actionErr: errors.New("qemu failed"),
	}
	router := newHandlerRouter(repository, &session, true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/actions/start", nil))
	if recorder.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestBatchActionRequiresCSRFAndReturnsPerVMResults(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{vms: []domain.VM{
		{UUID: "uuid-a", Name: "api-01", State: domain.VMStateRunning, AllowedActions: domain.AllowedVMActions(domain.VMStateRunning)},
		{UUID: "uuid-b", Name: "api-02", State: domain.VMStateRunning, AllowedActions: domain.AllowedVMActions(domain.VMStateRunning)},
	}}
	body := `{"action":"shutdown","uuids":["uuid-a","uuid-b"]}`

	router := newHandlerRouter(repository, &session, false)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/vms/batch-actions", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden {
		t.Fatalf("missing csrf status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	router = newHandlerRouter(repository, &session, true)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(stdhttp.MethodPost, "/vms/batch-actions", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result domain.VMBatchActionResult
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Action != domain.VMActionShutdown || result.Succeeded != 2 || result.Failed != 0 || len(result.Items) != 2 {
		t.Fatalf("result = %+v", result)
	}
	if result.Items[0].Name != "api-01" || result.Items[1].Name != "api-02" {
		t.Fatalf("items = %+v", result.Items)
	}
}

func TestBatchActionRejectsInvalidRequest(t *testing.T) {
	session := operatorSession()
	router := newHandlerRouter(&handlerRepository{}, &session, true)
	for _, body := range []string{
		`{"action":"explode","uuids":["uuid-a"]}`,
		`{"action":"start","uuids":[]}`,
		`{"action":"start","uuids":[" "]} `,
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(stdhttp.MethodPost, "/vms/batch-actions", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != stdhttp.StatusBadRequest {
			t.Fatalf("body=%s status = %d response=%s", body, recorder.Code, recorder.Body.String())
		}
	}
}

func TestSnapshotRoutes(t *testing.T) {
	session := operatorSession()
	repository := &handlerRepository{snapshots: []domain.VMSnapshot{{Name: "baseline", CreatedAt: 100, State: domain.VMStateShutoff, Current: true}}}
	router := newHandlerRouter(repository, &session, true)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/vms/uuid-a/snapshots", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("list status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var list domain.VMSnapshotList
	if err := json.NewDecoder(recorder.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || list.Items[0].Name != "baseline" {
		t.Fatalf("list = %+v", list)
	}

	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/snapshots", bytes.NewBufferString(`{"name":"before-upgrade","description":"stable"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusCreated || repository.snapshotSpec.Name != "before-upgrade" || repository.snapshotSpec.Description != "stable" {
		t.Fatalf("create status = %d spec=%+v body=%s", recorder.Code, repository.snapshotSpec, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/snapshots/before-upgrade/revert", nil))
	if recorder.Code != stdhttp.StatusNoContent || repository.revertedSnapshot != "before-upgrade" {
		t.Fatalf("revert status = %d snapshot=%q", recorder.Code, repository.revertedSnapshot)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodDelete, "/vms/uuid-a/snapshots/before-upgrade", nil))
	if recorder.Code != stdhttp.StatusNoContent || repository.deletedSnapshot != "before-upgrade" {
		t.Fatalf("delete status = %d snapshot=%q", recorder.Code, repository.deletedSnapshot)
	}
}

func TestSnapshotRoutesRequireSessionCSRFAndMapRiskyRevert(t *testing.T) {
	repository := &handlerRepository{snapshotErr: domain.ErrSnapshotRisky}
	router := newHandlerRouter(repository, nil, true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/snapshots", bytes.NewBufferString(`{"name":"snap"}`)))
	if recorder.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", recorder.Code)
	}

	session := operatorSession()
	router = newHandlerRouter(repository, &session, false)
	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/snapshots", bytes.NewBufferString(`{"name":"snap"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden || repository.snapshotSpec.Name != "" {
		t.Fatalf("csrf status = %d spec=%+v", recorder.Code, repository.snapshotSpec)
	}

	router = newHandlerRouter(repository, &session, true)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/vms/uuid-a/snapshots/snap/revert", nil))
	if recorder.Code != stdhttp.StatusConflict {
		t.Fatalf("risky revert status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func operatorSession() domain.Session {
	return domain.Session{User: domain.User{ID: 1, Username: "operator", Role: domain.RoleOperator}}
}

func runningHandlerVM() domain.VM {
	return domain.VM{
		UUID: "uuid-a", Name: "api-01", State: domain.VMStateRunning,
		AllowedActions: domain.AllowedVMActions(domain.VMStateRunning),
	}
}
