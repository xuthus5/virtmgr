package bootstrap

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"virtmgr/internal/domain"
)

func TestGuestAgentAPIAndChannelLifecycle(t *testing.T) {
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)

	getInfo := func(uuid string, refresh bool) domain.GuestAgentInfo {
		t.Helper()
		url := server.URL + "/api/v1/vms/" + uuid + "/guest-agent"
		if refresh {
			url += "?refresh=true"
		}
		response, err := client.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("guest agent status = %d body=%s", response.StatusCode, readBody(response))
		}
		var info domain.GuestAgentInfo
		if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
			t.Fatal(err)
		}
		return info
	}

	info := getInfo("11111111-1111-4111-8111-111111111111", false)
	if info.State != domain.GuestAgentStateConnected || !info.ChannelConfigured || !info.Capabilities.Quiesce {
		t.Fatalf("connected guest agent = %+v", info)
	}
	legacy := getInfo("44444444-4444-4444-8444-444444444444", false)
	if legacy.State != domain.GuestAgentStateNotConfigured || legacy.ChannelConfigured {
		t.Fatalf("legacy guest agent = %+v", legacy)
	}

	channelRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/44444444-4444-4444-8444-444444444444/guest-agent/channel", nil)
	channelRequest.Header.Set("X-CSRF-Token", csrf)
	channelResponse, err := client.Do(channelRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer channelResponse.Body.Close()
	if channelResponse.StatusCode != http.StatusOK {
		t.Fatalf("channel status = %d body=%s", channelResponse.StatusCode, readBody(channelResponse))
	}
	legacy = getInfo("44444444-4444-4444-8444-444444444444", true)
	if !legacy.ChannelConfigured || legacy.State != domain.GuestAgentStateDisconnected {
		t.Fatalf("legacy after channel = %+v", legacy)
	}

	quiescedBody, _ := json.Marshal(domain.VMSnapshotCreateSpec{Name: "quiesced-stopped", Quiesce: true})
	quiescedRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/44444444-4444-4444-8444-444444444444/snapshots", bytes.NewReader(quiescedBody))
	quiescedRequest.Header.Set("Content-Type", "application/json")
	quiescedRequest.Header.Set("X-CSRF-Token", csrf)
	quiescedResponse, err := client.Do(quiescedRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer quiescedResponse.Body.Close()
	if quiescedResponse.StatusCode != http.StatusConflict {
		t.Fatalf("stopped quiesced snapshot status = %d body=%s", quiescedResponse.StatusCode, readBody(quiescedResponse))
	}

	quiescedBody, _ = json.Marshal(domain.VMSnapshotCreateSpec{Name: "quiesced-running", Quiesce: true})
	quiescedRequest, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/11111111-1111-4111-8111-111111111111/snapshots", bytes.NewReader(quiescedBody))
	quiescedRequest.Header.Set("Content-Type", "application/json")
	quiescedRequest.Header.Set("X-CSRF-Token", csrf)
	quiescedResponse, err = client.Do(quiescedRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer quiescedResponse.Body.Close()
	if quiescedResponse.StatusCode != http.StatusCreated {
		t.Fatalf("running quiesced snapshot status = %d body=%s", quiescedResponse.StatusCode, readBody(quiescedResponse))
	}
	snapshotListResponse, err := client.Get(server.URL + "/api/v1/vms/11111111-1111-4111-8111-111111111111/snapshots")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshotListResponse.Body.Close()
	var snapshotList domain.VMSnapshotList
	if err := json.NewDecoder(snapshotListResponse.Body).Decode(&snapshotList); err != nil {
		t.Fatal(err)
	}
	foundQuiesced := false
	for _, snapshot := range snapshotList.Items {
		if snapshot.Name == "quiesced-running" && snapshot.Quiesced {
			foundQuiesced = true
		}
	}
	if !foundQuiesced {
		t.Fatalf("quiesced metadata missing from snapshots: %+v", snapshotList)
	}

	actionBody, _ := json.Marshal(domain.GuestAgentActionSpec{Action: domain.GuestAgentActionSyncTime})
	actionRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/11111111-1111-4111-8111-111111111111/guest-agent/actions", bytes.NewReader(actionBody))
	actionRequest.Header.Set("Content-Type", "application/json")
	actionRequest.Header.Set("X-CSRF-Token", csrf)
	actionResponse, err := client.Do(actionRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer actionResponse.Body.Close()
	if actionResponse.StatusCode != http.StatusOK {
		t.Fatalf("action status = %d body=%s", actionResponse.StatusCode, readBody(actionResponse))
	}
	var actionTask domain.Task
	if err := json.NewDecoder(actionResponse.Body).Decode(&actionTask); err != nil {
		t.Fatal(err)
	}
	if actionTask.ID == "" || actionTask.Status != domain.TaskStatusSucceeded {
		t.Fatalf("action task = %+v", actionTask)
	}
}

func TestGuestAgentHighRiskControlsAPI(t *testing.T) {
	app := newTestAppWithReadOnly(t, false)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)
	uuid := "11111111-1111-4111-8111-111111111111"
	requestJSON := func(method, path string, body any) *http.Response {
		t.Helper()
		var reader *bytes.Reader
		if body == nil {
			reader = bytes.NewReader(nil)
		} else {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = bytes.NewReader(encoded)
		}
		request, _ := http.NewRequest(method, server.URL+path, reader)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", csrf)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	getJSON := func(path string, target any) {
		t.Helper()
		response, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d body=%s", path, response.StatusCode, readBody(response))
		}
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			t.Fatal(err)
		}
	}
	expectOK := func(label string, response *http.Response) {
		t.Helper()
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", label, response.StatusCode, readBody(response))
		}
	}

	var content domain.GuestAgentFileContent
	getJSON("/api/v1/vms/"+uuid+"/guest-agent/files?path=/etc/os-release&maxBytes=4096", &content)
	if content.Path != "/etc/os-release" || content.Size == 0 || content.SHA256 == "" {
		t.Fatalf("file content = %+v", content)
	}
	writeResponse := requestJSON(http.MethodPost, "/api/v1/vms/"+uuid+"/guest-agent/files", domain.GuestAgentFileWriteSpec{
		Path: "/tmp/virtmgr-api-test", DataBase64: "aGVsbG8=", Overwrite: true,
	})
	expectOK("file write", writeResponse)
	expectOK("password reset", requestJSON(http.MethodPost, "/api/v1/vms/"+uuid+"/guest-agent/password", domain.GuestAgentPasswordSpec{Username: "root", Password: "temporary-secret"}))
	var keys domain.GuestAgentAuthorizedKeys
	getJSON("/api/v1/vms/"+uuid+"/guest-agent/ssh-keys?username=root", &keys)
	expectOK("ssh add", requestJSON(http.MethodPost, "/api/v1/vms/"+uuid+"/guest-agent/ssh-keys/add", domain.GuestAgentSSHKeySpec{
		Username: "root", Keys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyOnly test@example"},
	}))
	var processes struct {
		Items []domain.GuestAgentProcess `json:"items"`
		Total int                        `json:"total"`
	}
	getJSON("/api/v1/vms/"+uuid+"/guest-agent/processes", &processes)
	if processes.Total == 0 {
		t.Fatal("process list is empty")
	}
	expectOK("process signal", requestJSON(http.MethodPost, "/api/v1/vms/"+uuid+"/guest-agent/processes/signal", domain.GuestAgentProcessSignalSpec{PID: 918, Signal: "term"}))
	var users struct {
		Items []domain.GuestAgentUser `json:"items"`
	}
	getJSON("/api/v1/vms/"+uuid+"/guest-agent/users", &users)
	if len(users.Items) == 0 {
		t.Fatal("login users list is empty")
	}
	expectOK("policy update", requestJSON(http.MethodPut, "/api/v1/vms/"+uuid+"/guest-agent/policy", domain.GuestAgentPolicySpec{Expected: true}))
	var policy domain.GuestAgentPolicy
	getJSON("/api/v1/vms/"+uuid+"/guest-agent/policy", &policy)
	if !policy.Expected {
		t.Fatalf("policy = %+v", policy)
	}
	var health domain.GuestAgentHealthHistory
	getJSON("/api/v1/vms/"+uuid+"/guest-agent/health?hours=24", &health)
	var cloudInit domain.GuestAgentCloudInit
	getJSON("/api/v1/vms/"+uuid+"/guest-agent/cloud-init", &cloudInit)
	if !cloudInit.Installed || cloudInit.State != "done" || cloudInit.Log == "" || cloudInit.LogPath == "" {
		t.Fatalf("cloud-init diagnostics = %+v", cloudInit)
	}
}

func TestGuestAgentControlMutationsRequireCSRFAndWritableMode(t *testing.T) {
	app := newTestAppWithReadOnly(t, true)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	client, csrf := loginClient(t, server.URL)
	uuid := "11111111-1111-4111-8111-111111111111"
	body, _ := json.Marshal(domain.GuestAgentPasswordSpec{Username: "root", Password: "temporary-secret"})

	withoutCSRF, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+uuid+"/guest-agent/password", bytes.NewReader(body))
	withoutCSRF.Header.Set("Content-Type", "application/json")
	response, err := client.Do(withoutCSRF)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("missing CSRF status = %d body=%s", response.StatusCode, readBody(response))
	}

	withCSRF, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/"+uuid+"/guest-agent/password", bytes.NewReader(body))
	withCSRF.Header.Set("Content-Type", "application/json")
	withCSRF.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(withCSRF)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("read-only status = %d body=%s", response.StatusCode, readBody(response))
	}
}
