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

	actionBody, _ := json.Marshal(domain.GuestAgentActionSpec{Action: domain.GuestAgentActionSyncTime})
	actionRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/vms/11111111-1111-4111-8111-111111111111/guest-agent/actions", bytes.NewReader(actionBody))
	actionRequest.Header.Set("Content-Type", "application/json")
	actionRequest.Header.Set("X-CSRF-Token", csrf)
	actionResponse, err := client.Do(actionRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer actionResponse.Body.Close()
	if actionResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("action status = %d body=%s", actionResponse.StatusCode, readBody(actionResponse))
	}
}
