package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"virtmgr/internal/domain"
	"virtmgr/internal/platform/config"
	"virtmgr/internal/platform/sqlite"
)

func newTestApp(t *testing.T) *App {
	return newTestAppWithReadOnly(t, true)
}

func newTestAppWithReadOnly(t *testing.T, readOnly bool) *App {
	t.Helper()
	cfg := config.Config{
		Listen:        "127.0.0.1:0",
		LibvirtURI:    "test:///default",
		DatabasePath:  filepath.Join(t.TempDir(), "virtmgr.db"),
		HostName:      "test-host",
		SessionTTL:    time.Hour,
		AdminUsername: "admin",
		AdminPassword: "secret",
		ReadOnly:      readOnly,
		DemoMode:      true,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app, err := New(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = app.backend.Close()
		_ = app.db.Close()
	})
	return app
}

type loginResponse struct {
	CSRFToken string `json:"csrfToken"`
}

func loginClient(t *testing.T, baseURL string) (*http.Client, string) {
	return loginAs(t, baseURL, "admin", "secret")
}

func loginAs(t *testing.T, baseURL, username, password string) (*http.Client, string) {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	loginBody, _ := json.Marshal(map[string]string{"username": username, "password": password})
	response, err := client.Post(baseURL+"/api/v1/session", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("login request error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("login status = %d body=%s", response.StatusCode, body)
	}
	var session loginResponse
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if session.CSRFToken == "" {
		t.Fatal("login response has no CSRF token")
	}
	return client, session.CSRFToken
}

func TestProtectedAPIAndDemoData(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewServer(app.Handler())
	defer server.Close()

	client, _ := loginClient(t, server.URL)

	overviewResponse, err := client.Get(server.URL + "/api/v1/overview")
	if err != nil {
		t.Fatal(err)
	}
	defer overviewResponse.Body.Close()
	if overviewResponse.StatusCode != http.StatusOK {
		t.Fatalf("overview status = %d", overviewResponse.StatusCode)
	}
	var overview domain.Overview
	if err := json.NewDecoder(overviewResponse.Body).Decode(&overview); err != nil {
		t.Fatal(err)
	}
	if overview.Host.Name != "virtmgr-demo" || overview.Counts.Total != 4 || overview.Counts.Running != 2 {
		t.Fatalf("overview = %+v", overview)
	}

	vmsResponse, err := client.Get(server.URL + "/api/v1/vms?q=db&state=running")
	if err != nil {
		t.Fatal(err)
	}
	defer vmsResponse.Body.Close()
	var vms domain.VMSummaryList
	if err := json.NewDecoder(vmsResponse.Body).Decode(&vms); err != nil {
		t.Fatal(err)
	}
	if vms.Total != 1 || vms.Items[0].Name != "demo-db-01" {
		t.Fatalf("vms = %+v", vms)
	}

	detailResponse, err := client.Get(server.URL + "/api/v1/vms/22222222-2222-4222-8222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	defer detailResponse.Body.Close()
	var vm domain.VM
	if err := json.NewDecoder(detailResponse.Body).Decode(&vm); err != nil {
		t.Fatal(err)
	}
	if detailResponse.StatusCode != http.StatusOK || vm.Name != "demo-db-01" || len(vm.Disks) != 1 {
		t.Fatalf("detail status=%d vm=%+v", detailResponse.StatusCode, vm)
	}

	hostResponse, err := client.Get(server.URL + "/api/v1/host")
	if err != nil {
		t.Fatal(err)
	}
	defer hostResponse.Body.Close()
	if hostResponse.StatusCode != http.StatusOK {
		t.Fatalf("host status = %d body=%s", hostResponse.StatusCode, readBody(hostResponse))
	}
	var host domain.HostDetails
	if err := json.NewDecoder(hostResponse.Body).Decode(&host); err != nil {
		t.Fatal(err)
	}
	if host.Host.Name != "virtmgr-demo" || host.Host.CPUs != 16 || host.Counts.Total != 4 || len(host.StoragePools) != 3 || len(host.Networks) != 3 {
		t.Fatalf("host = %+v", host)
	}

	devicesResponse, err := client.Get(server.URL + "/api/v1/host/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer devicesResponse.Body.Close()
	if devicesResponse.StatusCode != http.StatusOK {
		t.Fatalf("host devices status = %d body=%s", devicesResponse.StatusCode, readBody(devicesResponse))
	}
	var devices domain.HostDeviceList
	if err := json.NewDecoder(devicesResponse.Body).Decode(&devices); err != nil {
		t.Fatal(err)
	}
	if devices.Total != 6 || len(devices.Items) != 6 || devices.Items[0].Name != "net_enp2s0" {
		t.Fatalf("host devices = %+v", devices)
	}

	tasksResponse, err := client.Get(server.URL + "/api/v1/tasks")
	if err != nil {
		t.Fatal(err)
	}
	defer tasksResponse.Body.Close()
	if tasksResponse.StatusCode != http.StatusOK {
		t.Fatalf("tasks status = %d body=%s", tasksResponse.StatusCode, readBody(tasksResponse))
	}
	var tasks domain.TaskList
	if err := json.NewDecoder(tasksResponse.Body).Decode(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks.Total != 4 || len(tasks.Items) != 4 || tasks.Items[0].ID != "task-demo-clone" || tasks.Items[0].Status != domain.TaskStatusRunning {
		t.Fatalf("tasks = %+v", tasks)
	}

	eventsResponse, err := client.Get(server.URL + "/api/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer eventsResponse.Body.Close()
	if eventsResponse.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d body=%s", eventsResponse.StatusCode, readBody(eventsResponse))
	}
	var events domain.SystemEventList
	if err := json.NewDecoder(eventsResponse.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	if events.Total != 3 || events.Items[0].Kind != "domain.suspended" {
		t.Fatalf("events = %+v", events)
	}
}

func TestResourceMetricsAPI(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, _ := loginClient(t, server.URL)

	hostMetrics := func() domain.HostMetrics {
		response, err := client.Get(server.URL + "/api/v1/host/metrics")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("host metrics status = %d body=%s", response.StatusCode, readBody(response))
		}
		var result domain.HostMetrics
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	firstHost := hostMetrics()
	if firstHost.HostName != "virtmgr-demo" || firstHost.MemoryTotalBytes != 64<<30 || firstHost.CPUPercent != 0 {
		t.Fatalf("first host metrics = %+v", firstHost)
	}
	time.Sleep(2 * time.Millisecond)
	secondHost := hostMetrics()
	if secondHost.CPUPercent <= 0 || secondHost.NetworkRxBytesPerSecond == 0 || secondHost.MemoryUsedBytes <= firstHost.MemoryUsedBytes {
		t.Fatalf("second host metrics = %+v", secondHost)
	}

	const uuid = "22222222-2222-4222-8222-222222222222"
	vmMetrics := func() domain.VMMetrics {
		response, err := client.Get(server.URL + "/api/v1/vms/" + uuid + "/metrics")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("vm metrics status = %d body=%s", response.StatusCode, readBody(response))
		}
		var result domain.VMMetrics
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	firstVM := vmMetrics()
	if firstVM.UUID != uuid || firstVM.MemoryTotalBytes != 16<<30 || firstVM.CPUPercent != 0 {
		t.Fatalf("first vm metrics = %+v", firstVM)
	}
	time.Sleep(2 * time.Millisecond)
	secondVM := vmMetrics()
	if secondVM.CPUPercent <= 0 || secondVM.NetworkTxBytesPerSecond == 0 {
		t.Fatalf("second vm metrics = %+v", secondVM)
	}
}

func TestVMXMLAPI(t *testing.T) {
	const uuid = "22222222-2222-4222-8222-222222222222"
	app := newTestApp(t)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, _ := loginClient(t, server.URL)

	response, err := client.Get(server.URL + "/api/v1/vms/" + uuid + "/xml?format=config")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.StatusCode, readBody(response))
	}
	var result domain.VMXML
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.UUID != uuid || result.Name != "demo-db-01" || result.Format != domain.VMXMLFormatConfig || !strings.Contains(result.XML, "<domain") {
		t.Fatalf("xml result = %+v", result)
	}

	response, err = client.Get(server.URL + "/api/v1/vms/" + uuid + "/xml?format=runtime")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid format status = %d", response.StatusCode)
	}
}

func TestVMXMLUpdateAPI(t *testing.T) {
	const vmUUID = "44444444-4444-4444-8444-444444444444"
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)

	response, err := client.Get(server.URL + "/api/v1/vms/" + vmUUID + "/xml?format=config")
	if err != nil {
		t.Fatal(err)
	}
	var current domain.VMXML
	if err := json.NewDecoder(response.Body).Decode(&current); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(current.XML, "<vcpu>2</vcpu>") {
		t.Fatalf("current status=%d xml=%s", response.StatusCode, current.XML)
	}
	updatedText := strings.Replace(current.XML, "<vcpu>2</vcpu>", "<vcpu>3</vcpu>", 1)
	body, _ := json.Marshal(map[string]string{"xml": updatedText})
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/api/v1/vms/"+vmUUID+"/xml", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var updated domain.VMXML
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || updated.Format != domain.VMXMLFormatConfig || !strings.Contains(updated.XML, "<vcpu>3</vcpu>") {
		t.Fatalf("update status=%d xml=%+v", response.StatusCode, updated)
	}

	response, err = client.Get(server.URL + "/api/v1/vms/" + vmUUID + "/xml?format=config")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(response.Body).Decode(&current); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if !strings.Contains(current.XML, "<vcpu>3</vcpu>") {
		t.Fatalf("readback xml = %s", current.XML)
	}

	response, err = client.Get(server.URL + "/api/v1/audit-events?action=vm.xml.update&pageSize=10")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var events domain.AuditEventList
	if err := json.NewDecoder(response.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	if events.Total != 1 || events.Items[0].TargetKind != "vm" || events.Items[0].TargetID != vmUUID || events.Items[0].Result != "success" {
		t.Fatalf("update audit = %+v", events)
	}
	if events.Items[0].Detail["xmlSha256"] == "" || events.Items[0].Detail["xmlBytes"] == nil {
		t.Fatalf("update audit detail = %+v", events.Items[0].Detail)
	}
}

func TestVMBackupAPI(t *testing.T) {
	const vmUUID = "44444444-4444-4444-8444-444444444444"
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)

	body := `{"name":"api-backup-01","targetPool":"default"}`
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+vmUUID+"/backups", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var task domain.Task
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted || task.Kind != "vm.backup" || task.TargetID != vmUUID || task.Status != domain.TaskStatusRunning {
		t.Fatalf("backup status=%d task=%+v", response.StatusCode, task)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		response, err = client.Get(server.URL + "/api/v1/tasks/" + task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if task.Status == domain.TaskStatusSucceeded || task.Status == domain.TaskStatusFailed || task.Status == domain.TaskStatusInterrupted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("backup task did not finish: %+v", task)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if task.Status != domain.TaskStatusSucceeded || task.Progress != 100 || !strings.Contains(task.Step, "备份完成") {
		t.Fatalf("backup task = %+v", task)
	}

	response, err = client.Get(server.URL + "/api/v1/backups?vmUuid=" + vmUUID)
	if err != nil {
		t.Fatal(err)
	}
	var backups domain.BackupList
	if err := json.NewDecoder(response.Body).Decode(&backups); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if backups.Total != 1 || len(backups.Items) != 1 || backups.Items[0].Status != domain.BackupStatusCompleted || backups.Items[0].DiskCount != 1 {
		t.Fatalf("backup catalog = %+v", backups)
	}
	backup := backups.Items[0]

	response, err = client.Get(server.URL + "/api/v1/audit-events?action=backup.create&pageSize=10")
	if err != nil {
		t.Fatal(err)
	}
	var events domain.AuditEventList
	if err := json.NewDecoder(response.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if events.Total != 1 || events.Items[0].TargetKind != "backup" || events.Items[0].TargetID != backup.ID || events.Items[0].Result != "success" {
		t.Fatalf("backup audit = %+v", events)
	}
	detail := events.Items[0].Detail
	if detail["targetPool"] != "default" || detail["backupName"] != "api-backup-01" || detail["diskCount"] != float64(1) || detail["totalBytes"] == nil {
		t.Fatalf("backup audit detail = %+v", detail)
	}
	if _, exists := detail["xml"]; exists {
		t.Fatalf("backup audit must not contain XML: %+v", detail)
	}
}

func TestVMExportAPI(t *testing.T) {
	const uuid = "22222222-2222-4222-8222-222222222222"
	app := newTestApp(t)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, _ := loginClient(t, server.URL)

	response, err := client.Get(server.URL + "/api/v1/vms/" + uuid + "/export")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.StatusCode, readBody(response))
	}
	if disposition := response.Header.Get("Content-Disposition"); disposition != `attachment; filename="demo-db-01-config-export.json"` {
		t.Fatalf("content disposition = %q", disposition)
	}
	var exported domain.VMExport
	if err := json.NewDecoder(response.Body).Decode(&exported); err != nil {
		t.Fatal(err)
	}
	if exported.FormatVersion != domain.VMExportFormatVersion || exported.ExportedAt.IsZero() || exported.VM.Name != "demo-db-01" {
		t.Fatalf("export = %+v", exported)
	}
	if exported.ConfigXML.Format != domain.VMXMLFormatConfig || !strings.Contains(exported.ConfigXML.XML, "<domain") {
		t.Fatalf("config xml = %+v", exported.ConfigXML)
	}
	if exported.LiveXML == nil || exported.LiveXML.Format != domain.VMXMLFormatLive || exported.Snapshots.Total != 0 {
		t.Fatalf("live xml=%+v snapshots=%+v", exported.LiveXML, exported.Snapshots)
	}

	response, err = client.Get(server.URL + "/api/v1/audit-events?action=vm.export&pageSize=10")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var events domain.AuditEventList
	if err := json.NewDecoder(response.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	if events.Total != 1 || events.Items[0].TargetKind != "vm" || events.Items[0].TargetID != uuid || events.Items[0].Result != "success" {
		t.Fatalf("export audit = %+v", events)
	}
}

func TestSnapshotAPILifecycle(t *testing.T) {
	const vmUUID = "44444444-4444-4444-8444-444444444444"
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)

	response, err := client.Get(server.URL + "/api/v1/vms/" + vmUUID + "/snapshots")
	if err != nil {
		t.Fatal(err)
	}
	var snapshots domain.VMSnapshotList
	if err := json.NewDecoder(response.Body).Decode(&snapshots); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || snapshots.Total != 0 {
		t.Fatalf("initial snapshots status=%d snapshots=%+v", response.StatusCode, snapshots)
	}

	body := `{"name":"before-upgrade","description":"stable state"}`
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+vmUUID+"/snapshots", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+vmUUID+"/snapshots", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var created domain.VMSnapshot
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated || created.Name != "before-upgrade" || !created.Current || created.State != domain.VMStateShutoff {
		t.Fatalf("create status=%d snapshot=%+v", response.StatusCode, created)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+vmUUID+"/snapshots/before-upgrade/revert", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("revert status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+vmUUID+"/snapshots", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/vms/"+vmUUID+"/snapshots/before-upgrade", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", response.StatusCode)
	}

	response, err = client.Get(server.URL + "/api/v1/audit-events?targetKind=vm_snapshot&pageSize=20")
	if err != nil {
		t.Fatal(err)
	}
	var events domain.AuditEventList
	if err := json.NewDecoder(response.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if events.Total != 4 || events.Items[0].TargetID != vmUUID+"/before-upgrade" {
		t.Fatalf("snapshot audit = %+v", events)
	}
	actions := map[string]bool{}
	for _, event := range events.Items {
		actions[event.Action] = true
	}
	for _, action := range []string{"vm.snapshot.create", "vm.snapshot.revert", "vm.snapshot.delete"} {
		if !actions[action] {
			t.Fatalf("missing audit action %s in %+v", action, events.Items)
		}
	}
}

func TestSnapshotAPIRejectsReadOnly(t *testing.T) {
	const vmUUID = "44444444-4444-4444-8444-444444444444"
	app := newTestApp(t)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+vmUUID+"/snapshots", bytes.NewBufferString(`{"name":"blocked"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", response.StatusCode, readBody(response))
	}
	if code := errorCode(t, response); code != "READ_ONLY" {
		t.Fatalf("error code = %q", code)
	}
}

func TestSnapshotPolicyAPI(t *testing.T) {
	const vmUUID = "44444444-4444-4444-8444-444444444444"
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)

	response, err := client.Get(server.URL + "/api/v1/vms/" + vmUUID + "/snapshot-policies")
	if err != nil {
		t.Fatal(err)
	}
	var policies domain.SnapshotPolicyList
	if err := json.NewDecoder(response.Body).Decode(&policies); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || policies.Total != 0 {
		t.Fatalf("initial policies status=%d policies=%+v", response.StatusCode, policies)
	}

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+vmUUID+"/snapshot-policies", bytes.NewBufferString(`{"name":"api-nightly","intervalMinutes":60,"retention":2,"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var created domain.SnapshotPolicy
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated || created.ID < 1 || created.Name != "api-nightly" || created.NextRunAt.IsZero() {
		t.Fatalf("create policy status=%d policy=%+v", response.StatusCode, created)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/snapshot-policies/"+strconv.FormatInt(created.ID, 10)+"/run", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var run domain.SnapshotPolicyRunResult
	if err := json.NewDecoder(response.Body).Decode(&run); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || run.Snapshot == nil || run.Policy.LastSnapshot != run.Snapshot.Name || !strings.Contains(run.Snapshot.Description, "[policy:") {
		t.Fatalf("run policy status=%d result=%+v", response.StatusCode, run)
	}

	response, err = client.Get(server.URL + "/api/v1/vms/" + vmUUID + "/snapshots")
	if err != nil {
		t.Fatal(err)
	}
	var snapshots domain.VMSnapshotList
	if err := json.NewDecoder(response.Body).Decode(&snapshots); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || snapshots.Total != 1 || snapshots.Items[0].Name != run.Snapshot.Name {
		t.Fatalf("snapshots status=%d snapshots=%+v", response.StatusCode, snapshots)
	}

	request, _ = http.NewRequest(http.MethodPatch, server.URL+"/api/v1/snapshot-policies/"+strconv.FormatInt(created.ID, 10), bytes.NewBufferString(`{"name":"api-weekly","intervalMinutes":120,"retention":1,"enabled":false}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var updated domain.SnapshotPolicy
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || updated.Name != "api-weekly" || updated.IntervalMinutes != 120 || updated.Retention != 1 || updated.Enabled {
		t.Fatalf("update policy status=%d policy=%+v", response.StatusCode, updated)
	}

	response, err = client.Get(server.URL + "/api/v1/vms/" + vmUUID + "/snapshot-policies")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(response.Body).Decode(&policies); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || policies.Total != 1 || policies.Items[0].LastSnapshot != run.Snapshot.Name || policies.Items[0].Name != "api-weekly" {
		t.Fatalf("updated policies status=%d policies=%+v", response.StatusCode, policies)
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/snapshot-policies/"+strconv.FormatInt(created.ID, 10), nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete policy status = %d", response.StatusCode)
	}

	response, err = client.Get(server.URL + "/api/v1/audit-events?targetKind=vm&pageSize=50")
	if err != nil {
		t.Fatal(err)
	}
	var events domain.AuditEventList
	if err := json.NewDecoder(response.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	counts := map[string]int{}
	for _, event := range events.Items {
		if event.Result == "success" {
			counts[event.Action]++
		}
	}
	for _, action := range []string{"vm.snapshot_policy.create", "vm.snapshot_policy.update", "vm.snapshot_policy.delete", "vm.snapshot.schedule"} {
		if counts[action] != 1 {
			t.Fatalf("audit action %s count=%d events=%+v", action, counts[action], events.Items)
		}
	}
	if events.Items[0].RequestID == "" || events.Items[0].SourceIP == "" {
		t.Fatalf("audit metadata missing: %+v", events.Items[0])
	}

	response, err = client.Get(server.URL + "/api/v1/vms/" + vmUUID + "/snapshot-policies")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(response.Body).Decode(&policies); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || policies.Total != 0 {
		t.Fatalf("final policies status=%d policies=%+v", response.StatusCode, policies)
	}
}

func TestVMActionConfirmationReadOnlyAndSuccess(t *testing.T) {
	const legacyUUID = "44444444-4444-4444-8444-444444444444"

	t.Run("read only rejects after CSRF", func(t *testing.T) {
		app := newTestAppWithReadOnly(t, true)
		server := httptest.NewServer(app.Handler())
		defer server.Close()
		client, csrf := loginClient(t, server.URL)

		request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+legacyUUID+"/actions/start", nil)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("missing CSRF status = %d body=%s", response.StatusCode, body)
		}

		request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+legacyUUID+"/actions/start", nil)
		request.Header.Set("X-CSRF-Token", csrf)
		response, err = client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("read-only status = %d body=%s", response.StatusCode, body)
		}
		var body struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil || body.Error.Code != "READ_ONLY" {
			t.Fatalf("read-only error = %+v, %v", body, err)
		}
	})

	t.Run("writable action updates demo VM", func(t *testing.T) {
		app := newTestAppWithReadOnly(t, false)
		server := httptest.NewServer(app.Handler())
		defer server.Close()
		client, csrf := loginClient(t, server.URL)

		request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+legacyUUID+"/actions/start", nil)
		request.Header.Set("X-CSRF-Token", csrf)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("action status = %d body=%s", response.StatusCode, body)
		}
		var vm domain.VM
		if err := json.NewDecoder(response.Body).Decode(&vm); err != nil {
			t.Fatal(err)
		}
		if vm.State != domain.VMStateRunning {
			t.Fatalf("vm state = %q", vm.State)
		}

		auditDB, err := sqlite.Open(app.config.DatabasePath)
		if err != nil {
			t.Fatalf("open audit database: %v", err)
		}
		defer auditDB.Close()
		var auditCount int
		if err := auditDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_events WHERE action = 'vm.start' AND result = 'success'`).Scan(&auditCount); err != nil {
			t.Fatalf("query audit events: %v", err)
		}
		if auditCount != 1 {
			t.Fatalf("audit event count = %d", auditCount)
		}
	})
}

func TestRuntimeReadOnlySwitchControlsWritesAndSession(t *testing.T) {
	const legacyUUID = "44444444-4444-4444-8444-444444444444"
	app := newTestAppWithReadOnly(t, true)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)

	getSession := func() bool {
		response, err := client.Get(server.URL + "/api/v1/session")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var session struct {
			ReadOnly bool `json:"readOnly"`
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("session status = %d body=%s", response.StatusCode, readBody(response))
		}
		if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
			t.Fatal(err)
		}
		return session.ReadOnly
	}

	if !getSession() {
		t.Fatal("initial session is not read-only")
	}
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/api/v1/settings", bytes.NewBufferString(`{"readOnly":false}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("disable read-only status = %d", response.StatusCode)
	}
	if getSession() {
		t.Fatal("session still reports read-only after update")
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+legacyUUID+"/actions/start", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("action after disabling read-only status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodPut, server.URL+"/api/v1/settings", bytes.NewBufferString(`{"readOnly":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("enable read-only status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+legacyUUID+"/actions/destroy", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden || errorCode(t, response) != "READ_ONLY" {
		t.Fatalf("write after re-enabling read-only status = %d", response.StatusCode)
	}
	response.Body.Close()

	auditResponse, err := client.Get(server.URL + "/api/v1/audit-events?action=settings.readonly.update")
	if err != nil {
		t.Fatal(err)
	}
	defer auditResponse.Body.Close()
	var audit domain.AuditEventList
	if err := json.NewDecoder(auditResponse.Body).Decode(&audit); err != nil {
		t.Fatal(err)
	}
	if audit.Total != 2 {
		t.Fatalf("settings audit events = %+v", audit)
	}
}

func TestVMCreateAndDelete(t *testing.T) {
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)

	createBody := bytes.NewBufferString(`{
		"name":"api-created-01",
		"vcpus":2,
		"memoryMiB":2048,
		"diskPath":"/var/lib/libvirt/images/api-created-01.qcow2",
		"diskFormat":"qcow2",
		"network":"default",
		"firmware":"uefi",
		"secureBoot":true,
		"cloudInit":{"userData":"#cloud-config\nruncmd:\n  - echo bootstrap-secret","pool":"default"},
		"autostart":false,
		"start":false
	}`)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms", createBody)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("create status = %d body=%s", response.StatusCode, body)
	}
	var vm domain.VM
	if err := json.NewDecoder(response.Body).Decode(&vm); err != nil {
		t.Fatal(err)
	}
	if vm.Name != "api-created-01" || vm.State != domain.VMStateShutoff || vm.UUID == "" || vm.Firmware != domain.VMFirmwareUEFI || !vm.SecureBoot || !vm.CloudInit {
		t.Fatalf("created vm = %+v", vm)
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/vms/"+vm.UUID, nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("delete status = %d body=%s", response.StatusCode, body)
	}

	auditDB, err := sqlite.Open(app.config.DatabasePath)
	if err != nil {
		t.Fatalf("open audit database: %v", err)
	}
	defer auditDB.Close()
	var auditCount int
	if err := auditDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_events WHERE action IN ('vm.create', 'vm.delete') AND result = 'success'`).Scan(&auditCount); err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	if auditCount != 2 {
		t.Fatalf("audit event count = %d", auditCount)
	}
	var auditDetail string
	if err := auditDB.QueryRowContext(context.Background(), `SELECT detail_json FROM audit_events WHERE action = 'vm.create' AND result = 'success'`).Scan(&auditDetail); err != nil {
		t.Fatalf("query audit detail: %v", err)
	}
	if !strings.Contains(auditDetail, `"firmware":"uefi"`) || !strings.Contains(auditDetail, `"secureBoot":true`) || !strings.Contains(auditDetail, `"cloudInit":true`) {
		t.Fatalf("audit detail = %s", auditDetail)
	}
	if strings.Contains(auditDetail, "bootstrap-secret") {
		t.Fatalf("audit detail leaked cloud-init user data: %s", auditDetail)
	}
}

func TestVMHostDevicePassthroughAPI(t *testing.T) {
	const legacyUUID = "44444444-4444-4444-8444-444444444444"
	const pciDevice = "pci_0000_3b_00_0"
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+legacyUUID+"/devices", bytes.NewBufferString(`{"device":"`+pciDevice+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("attach status = %d body=%s", response.StatusCode, readBody(response))
	}
	var vm domain.VM
	if err := json.NewDecoder(response.Body).Decode(&vm); err != nil {
		t.Fatal(err)
	}
	if len(vm.Passthrough) != 1 || vm.Passthrough[0].Name != pciDevice || vm.Passthrough[0].Type != "pci" {
		t.Fatalf("attached vm = %+v", vm)
	}

	response, err = client.Get(server.URL + "/api/v1/vms/" + legacyUUID + "/xml?format=config")
	if err != nil {
		t.Fatal(err)
	}
	var vmXML domain.VMXML
	if err := json.NewDecoder(response.Body).Decode(&vmXML); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(vmXML.XML, "<hostdev mode='subsystem' type='pci'") {
		t.Fatalf("passthrough XML status=%d xml=%s", response.StatusCode, vmXML.XML)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+legacyUUID+"/devices", bytes.NewBufferString(`{"device":"`+pciDevice+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate attach status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/vms/"+legacyUUID+"/devices/"+pciDevice, nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("detach status = %d body=%s", response.StatusCode, readBody(response))
	}
	if err := json.NewDecoder(response.Body).Decode(&vm); err != nil {
		t.Fatal(err)
	}
	if len(vm.Passthrough) != 0 {
		t.Fatalf("detached vm = %+v", vm)
	}

	auditDB, err := sqlite.Open(app.config.DatabasePath)
	if err != nil {
		t.Fatalf("open audit database: %v", err)
	}
	defer auditDB.Close()
	for _, action := range []string{"vm.device.attach", "vm.device.detach"} {
		var count int
		if err := auditDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_events WHERE action = ? AND result = 'success'`, action).Scan(&count); err != nil {
			t.Fatalf("query %s audit: %v", action, err)
		}
		if count != 1 {
			t.Fatalf("%s audit count = %d", action, count)
		}
	}
}

func TestVMTemplateAPI(t *testing.T) {
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)

	response, err := client.Get(server.URL + "/api/v1/templates")
	if err != nil {
		t.Fatal(err)
	}
	var templates domain.VMTemplateList
	if err := json.NewDecoder(response.Body).Decode(&templates); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || templates.Total != 2 || templates.Items[0].Name != "linux-base" {
		t.Fatalf("demo templates status=%d templates=%+v", response.StatusCode, templates)
	}

	createBody := bytes.NewBufferString(`{"name":"api-template","description":"api test","vcpus":2,"memoryMiB":2048,"diskFormat":"qcow2","network":"default","firmware":"uefi","secureBoot":true,"cloudInit":true,"cloudInitUserData":"#cloud-config\nruncmd:\n  - echo template-secret\n","autostart":false,"start":false}`)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/templates", createBody)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create template status = %d body=%s", response.StatusCode, readBody(response))
	}
	var created domain.VMTemplate
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID < 1 || created.Name != "api-template" || !created.CloudInit || !created.SecureBoot {
		t.Fatalf("created template = %+v", created)
	}

	auditDB, err := sqlite.Open(app.config.DatabasePath)
	if err != nil {
		t.Fatalf("open audit database: %v", err)
	}
	defer auditDB.Close()
	var auditDetail string
	if err := auditDB.QueryRowContext(context.Background(), `SELECT detail_json FROM audit_events WHERE action = 'vm_template.create' AND result = 'success'`).Scan(&auditDetail); err != nil {
		t.Fatalf("query template audit detail: %v", err)
	}
	if !strings.Contains(auditDetail, `"cloudInit":true`) || !strings.Contains(auditDetail, `"cloudInitUserDataBytes"`) {
		t.Fatalf("template audit detail = %s", auditDetail)
	}
	if strings.Contains(auditDetail, "template-secret") {
		t.Fatalf("template audit leaked cloud-init user data: %s", auditDetail)
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/templates/"+strconv.FormatInt(created.ID, 10), nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete template status = %d", response.StatusCode)
	}
}

func TestAuditHistoryAPI(t *testing.T) {
	const legacyUUID = "44444444-4444-4444-8444-444444444444"
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+legacyUUID+"/actions/start", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("start status = %d body=%s", response.StatusCode, readBody(response))
	}

	response, err = client.Get(server.URL + "/api/v1/audit-events?action=vm.start&result=success&targetKind=vm&page=1&pageSize=10")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("audit status = %d body=%s", response.StatusCode, readBody(response))
	}
	var events domain.AuditEventList
	if err := json.NewDecoder(response.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	if events.Total != 1 || len(events.Items) != 1 || events.Page != 1 || events.PageSize != 10 {
		t.Fatalf("audit events = %+v", events)
	}
	event := events.Items[0]
	if event.Actor != "admin" || event.Role != domain.RoleAdmin || event.Action != "vm.start" || event.TargetID != legacyUUID {
		t.Fatalf("audit event = %+v", event)
	}
	if event.RequestID == "" || event.SourceIP == "" || event.Detail["previousState"] != "shutoff" {
		t.Fatalf("audit event metadata = %+v", event)
	}
}

func TestUserManagementAPI(t *testing.T) {
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	adminClient, csrf := loginClient(t, server.URL)

	response, err := adminClient.Get(server.URL + "/api/v1/users")
	if err != nil {
		t.Fatal(err)
	}
	var users domain.UserList
	if err := json.NewDecoder(response.Body).Decode(&users); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || users.Total != 1 || users.Items[0].Username != "admin" {
		t.Fatalf("initial users status=%d users=%+v", response.StatusCode, users)
	}

	createBody := bytes.NewBufferString(`{"username":"api-viewer","password":"api-viewer-secret-123","role":"viewer"}`)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/users", createBody)
	request.Header.Set("Content-Type", "application/json")
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d body=%s", response.StatusCode, readBody(response))
	}
	response.Body.Close()

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/users", bytes.NewBufferString(`{"username":"api-viewer","password":"api-viewer-secret-123","role":"viewer"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", response.StatusCode, readBody(response))
	}
	var created domain.User
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Username != "api-viewer" || created.Role != domain.RoleViewer || created.Disabled {
		t.Fatalf("created user = %+v", created)
	}

	viewerClient, _ := loginAs(t, server.URL, "api-viewer", "api-viewer-secret-123")
	response, err = viewerClient.Get(server.URL + "/api/v1/users")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer list users status = %d", response.StatusCode)
	}

	updateBody := bytes.NewBufferString(`{"role":"operator","disabled":true}`)
	request, _ = http.NewRequest(http.MethodPatch, server.URL+"/api/v1/users/"+strconv.FormatInt(created.ID, 10), updateBody)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d body=%s", response.StatusCode, readBody(response))
	}
	var updated domain.User
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Role != domain.RoleOperator || !updated.Disabled {
		t.Fatalf("updated user = %+v", updated)
	}

	response, err = viewerClient.Get(server.URL + "/api/v1/session")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("disabled session status = %d", response.StatusCode)
	}

	response, err = adminClient.Get(server.URL + "/api/v1/audit-events?action=user.create&result=success")
	if err != nil {
		t.Fatal(err)
	}
	var auditEvents domain.AuditEventList
	if err := json.NewDecoder(response.Body).Decode(&auditEvents); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if auditEvents.Total != 1 || auditEvents.Items[0].TargetID != strconv.FormatInt(created.ID, 10) {
		t.Fatalf("create audit = %+v", auditEvents)
	}

	request, _ = http.NewRequest(http.MethodPatch, server.URL+"/api/v1/users/1", bytes.NewBufferString(`{"role":"viewer","disabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("self modification status = %d", response.StatusCode)
	}
}

func TestStorageAPI(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, _ := loginClient(t, server.URL)

	response, err := client.Get(server.URL + "/api/v1/storage/pools")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var pools domain.StoragePoolList
	if err := json.NewDecoder(response.Body).Decode(&pools); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || pools.Total != 3 || pools.Items[0].Name != "archive" {
		t.Fatalf("pools status=%d pools=%+v", response.StatusCode, pools)
	}

	response, err = client.Get(server.URL + "/api/v1/storage/pools/default/volumes")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var volumes domain.StorageVolumeList
	if err := json.NewDecoder(response.Body).Decode(&volumes); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || volumes.Total != 4 || volumes.Items[0].Name != "demo-build-01.qcow2" {
		t.Fatalf("volumes status=%d volumes=%+v", response.StatusCode, volumes)
	}
}

func TestNetworksAPI(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, _ := loginClient(t, server.URL)

	response, err := client.Get(server.URL + "/api/v1/networks")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var networks domain.VirtualNetworkList
	if err := json.NewDecoder(response.Body).Decode(&networks); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || networks.Total != 3 {
		t.Fatalf("networks status=%d networks=%+v", response.StatusCode, networks)
	}
	if networks.Items[0].Name != "br-prod" || networks.Items[1].Name != "default" || networks.Items[1].Bridge != "virbr0" {
		t.Fatalf("networks = %+v", networks.Items)
	}

	response, err = client.Get(server.URL + "/api/v1/networks/default/leases")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var leases domain.DHCPLeaseList
	if err := json.NewDecoder(response.Body).Decode(&leases); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || leases.Network != "default" || leases.Total != 2 {
		t.Fatalf("leases status=%d leases=%+v", response.StatusCode, leases)
	}
	if leases.Items[0].Hostname != "demo-web-01" || leases.Items[1].Hostname != "demo-db-01" {
		t.Fatalf("lease order = %+v", leases.Items)
	}
}

func TestNetworkManagementAPI(t *testing.T) {
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	adminClient, csrf := loginClient(t, server.URL)

	body := `{"name":"api-net-01","mode":"isolated","subnet":"10.99.0.0/24","autostart":false,"start":false}`
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/networks", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/networks", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", response.StatusCode, readBody(response))
	}
	var network domain.VirtualNetwork
	if err := json.NewDecoder(response.Body).Decode(&network); err != nil {
		t.Fatal(err)
	}
	if network.Name != "api-net-01" || network.Active || network.ForwardMode != "isolated" || len(network.Addresses) != 1 || network.Addresses[0].DHCPStart != "10.99.0.2" {
		t.Fatalf("created network = %+v", network)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/networks", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate status = %d body=%s", response.StatusCode, readBody(response))
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/networks/api-net-01/actions/start", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("start status = %d body=%s", response.StatusCode, readBody(response))
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/networks/api-net-01/hosts", bytes.NewBufferString(`{"mac":"52:54:00:12:34:56","ip":"10.99.0.50","name":"api-host-01"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("add DHCP host status = %d body=%s", response.StatusCode, readBody(response))
	}
	response.Body.Close()

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/networks/api-net-01/hosts/52:54:00:12:34:56", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("delete DHCP host status = %d body=%s", response.StatusCode, readBody(response))
	}
	response.Body.Close()

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/networks/api-net-01/hosts/import", bytes.NewBufferString(`{"hosts":[{"mac":"52:54:00:12:34:57","ip":"10.99.0.51","name":"api-host-02"},{"mac":"52:54:00:12:34:58","ip":"10.99.0.52"}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var imported domain.NetworkDHCPHostImportResult
	if err := json.NewDecoder(response.Body).Decode(&imported); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || imported.Imported != 2 || imported.Skipped != 0 || len(imported.Network.Addresses) != 1 || len(imported.Network.Addresses[0].Hosts) != 2 {
		t.Fatalf("import DHCP hosts status=%d result=%+v", response.StatusCode, imported)
	}

	response, err = adminClient.Get(server.URL + "/api/v1/networks/api-net-01/hosts/export")
	if err != nil {
		t.Fatal(err)
	}
	exportedHosts := readBody(response)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(exportedHosts, "52:54:00:12:34:57,10.99.0.51,api-host-02") || !strings.Contains(exportedHosts, "52:54:00:12:34:58,10.99.0.52") {
		t.Fatalf("export DHCP hosts status=%d body=%s", response.StatusCode, exportedHosts)
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/networks/api-net-01", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("active delete status = %d body=%s", response.StatusCode, readBody(response))
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/networks/api-net-01/actions/stop", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("stop status = %d body=%s", response.StatusCode, readBody(response))
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/networks/api-net-01", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", response.StatusCode, readBody(response))
	}

	response, err = adminClient.Get(server.URL + "/api/v1/networks")
	if err != nil {
		t.Fatal(err)
	}
	var networks domain.VirtualNetworkList
	if err := json.NewDecoder(response.Body).Decode(&networks); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || networks.Total != 3 {
		t.Fatalf("networks after delete status=%d networks=%+v", response.StatusCode, networks)
	}

	response, err = adminClient.Get(server.URL + "/api/v1/audit-events?targetKind=network&pageSize=20")
	if err != nil {
		t.Fatal(err)
	}
	var events domain.AuditEventList
	if err := json.NewDecoder(response.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if events.Total != 7 {
		t.Fatalf("network audit events = %+v", events)
	}
	actions := make(map[string]bool, len(events.Items))
	for _, event := range events.Items {
		actions[event.Action] = true
		if event.TargetID != "api-net-01" || event.TargetKind != "network" {
			t.Fatalf("network audit event = %+v", event)
		}
	}
	for _, action := range []string{"network.create", "network.start", "network.stop", "network.delete", "network.dhcp.host.add", "network.dhcp.host.delete", "network.dhcp.host.import"} {
		if !actions[action] {
			t.Fatalf("missing audit action %s in %+v", action, events.Items)
		}
	}

	viewerRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/users", bytes.NewBufferString(`{"username":"network-viewer","password":"network-viewer-secret","role":"viewer"}`))
	viewerRequest.Header.Set("Content-Type", "application/json")
	viewerRequest.Header.Set("X-CSRF-Token", csrf)
	response, err = adminClient.Do(viewerRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create viewer status = %d body=%s", response.StatusCode, readBody(response))
	}
	viewerClient, viewerCSRF := loginAs(t, server.URL, "network-viewer", "network-viewer-secret")
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/networks/lab-isolated/actions/start", nil)
	request.Header.Set("X-CSRF-Token", viewerCSRF)
	response, err = viewerClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer start status = %d body=%s", response.StatusCode, readBody(response))
	}
}

func TestNetworkManagementRejectsReadOnly(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/networks", bytes.NewBufferString(`{"name":"blocked-net","mode":"nat","subnet":"10.88.0.0/24"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("create status = %d body=%s", response.StatusCode, readBody(response))
	}
	if code := errorCode(t, response); code != "READ_ONLY" {
		t.Fatalf("create error code = %q", code)
	}
}

func TestCreateStorageVolumeAPI(t *testing.T) {
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)

	body := `{"name":"e2e-volume-01.qcow2","format":"qcow2","capacityBytes":21474836480}`
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/storage/pools/default/volumes", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("missing CSRF status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/storage/pools/default/volumes", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var volume domain.StorageVolume
	if err := json.NewDecoder(response.Body).Decode(&volume); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated || volume.Name != "e2e-volume-01.qcow2" || volume.CapacityBytes != 20<<30 {
		t.Fatalf("create status=%d volume=%+v", response.StatusCode, volume)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/storage/pools/default/volumes", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate status = %d body=%s", response.StatusCode, readBody(response))
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/storage/pools/default/volumes/e2e-volume-01.qcow2", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", response.StatusCode, readBody(response))
	}

	response, err = client.Get(server.URL + "/api/v1/storage/pools/default/volumes")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var volumes domain.StorageVolumeList
	if err := json.NewDecoder(response.Body).Decode(&volumes); err != nil {
		t.Fatal(err)
	}
	if volumes.Total != 4 {
		t.Fatalf("volumes after delete = %+v", volumes)
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/storage/pools/default/volumes/demo-web-01.qcow2", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("attached delete status = %d body=%s", response.StatusCode, readBody(response))
	}
}

func TestUploadISOAPI(t *testing.T) {
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	adminClient, adminCSRF := loginClient(t, server.URL)
	body := bytes.Repeat([]byte{0x49}, 1<<20)
	endpoint := server.URL + "/api/v1/storage/pools/iso-store/volumes/upload?name=e2e-upload.iso"

	request, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/octet-stream")
	response, err := adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-CSRF-Token", adminCSRF)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", response.StatusCode, readBody(response))
	}
	var volume domain.StorageVolume
	if err := json.NewDecoder(response.Body).Decode(&volume); err != nil {
		t.Fatal(err)
	}
	if volume.Name != "e2e-upload.iso" || volume.Format != "raw" || volume.CapacityBytes != 1<<20 || volume.Path != "/var/lib/libvirt/iso/e2e-upload.iso" {
		t.Fatalf("uploaded volume = %+v", volume)
	}

	request, _ = http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-CSRF-Token", adminCSRF)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate upload status = %d", response.StatusCode)
	}

	createViewer, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/users", bytes.NewBufferString(`{"username":"upload-viewer","password":"upload-viewer-secret-123","role":"viewer"}`))
	createViewer.Header.Set("Content-Type", "application/json")
	createViewer.Header.Set("X-CSRF-Token", adminCSRF)
	response, err = adminClient.Do(createViewer)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create viewer status = %d body=%s", response.StatusCode, readBody(response))
	}
	viewerClient, viewerCSRF := loginAs(t, server.URL, "upload-viewer", "upload-viewer-secret-123")
	request, _ = http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-CSRF-Token", viewerCSRF)
	response, err = viewerClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer upload status = %d", response.StatusCode)
	}

	response, err = adminClient.Get(server.URL + "/api/v1/audit-events?action=storage.volume.upload&result=success")
	if err != nil {
		t.Fatal(err)
	}
	var events domain.AuditEventList
	if err := json.NewDecoder(response.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if events.Total != 1 || events.Items[0].TargetID != "iso-store/e2e-upload.iso" {
		t.Fatalf("upload audit = %+v", events)
	}

	response, err = adminClient.Get(server.URL + "/api/v1/storage/pools/iso-store/volumes")
	if err != nil {
		t.Fatal(err)
	}
	var volumes domain.StorageVolumeList
	if err := json.NewDecoder(response.Body).Decode(&volumes); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if volumes.Total != 3 {
		t.Fatalf("iso volumes = %+v", volumes)
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/storage/pools/iso-store/volumes/e2e-upload.iso", nil)
	request.Header.Set("X-CSRF-Token", adminCSRF)
	response, err = adminClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete upload status = %d", response.StatusCode)
	}
}

func TestCreateStorageVolumeRejectsReadOnly(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/storage/pools/default/volumes", bytes.NewBufferString(`{"name":"blocked.qcow2","format":"qcow2","capacityBytes":1073741824}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", response.StatusCode, readBody(response))
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/storage/pools/default/volumes/demo-web-01.qcow2", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("delete status = %d body=%s", response.StatusCode, readBody(response))
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/storage/pools/iso-store/volumes/upload?name=blocked.iso", bytes.NewReader(make([]byte, 1<<20)))
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("upload status = %d body=%s", response.StatusCode, readBody(response))
	}
	if code := errorCode(t, response); code != "READ_ONLY" {
		t.Fatalf("upload error code = %q", code)
	}
}

func readBody(response *http.Response) string {
	body, _ := io.ReadAll(response.Body)
	return string(body)
}

func errorCode(t *testing.T, response *http.Response) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return body.Error.Code
}

func TestProtectedAPIRejectsAnonymous(t *testing.T) {
	app := newTestApp(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/vms", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestHealthAndReady(t *testing.T) {
	app := newTestApp(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, recorder.Code)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	app := newTestApp(t)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)

	for header, want := range map[string]string{
		"Content-Security-Policy":      "default-src 'self'",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Permissions-Policy":           "camera=()",
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
	} {
		if got := recorder.Header().Get(header); !strings.Contains(got, want) {
			t.Fatalf("%s = %q, want to contain %q", header, got, want)
		}
	}
	csp := recorder.Header().Get("Content-Security-Policy")
	for _, want := range []string{"https://static.cloudflareinsights.com", "connect-src 'self' https://cloudflareinsights.com"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("Content-Security-Policy = %q, want to contain %q", csp, want)
		}
	}
}

func TestSecurityHeadersAddsHSTSOnlyForHTTPSDeployments(t *testing.T) {
	for _, test := range []struct {
		name    string
		enabled bool
		want    string
	}{
		{name: "enabled", enabled: true, want: "max-age=31536000"},
		{name: "disabled", enabled: false, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			securityHeaders(test.enabled)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			if got := recorder.Header().Get("Strict-Transport-Security"); got != test.want {
				t.Fatalf("Strict-Transport-Security = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMissingStaticAssetReturnsNotFound(t *testing.T) {
	app := newTestApp(t)
	request := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
