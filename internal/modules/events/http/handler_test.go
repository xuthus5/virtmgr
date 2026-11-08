package eventshttp

import (
	"bufio"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"virtmgr/internal/domain"
	eventsapp "virtmgr/internal/modules/events/app"
	"virtmgr/internal/platform/eventbus"

	"github.com/go-chi/chi/v5"
)

func TestListEvents(t *testing.T) {
	bus := eventbus.New(10)
	bus.Publish(domain.SystemEvent{Kind: "domain.started", Message: "started"})
	router := chi.NewRouter()
	New(eventsapp.New(bus)).Mount(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/events?limit=1", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var list domain.SystemEventList
	if err := json.NewDecoder(recorder.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || list.Items[0].Kind != "domain.started" {
		t.Fatalf("list = %+v", list)
	}
}

func TestStreamEventsSendsSnapshotAndLiveEvent(t *testing.T) {
	bus := eventbus.New(10)
	bus.Publish(domain.SystemEvent{Kind: "domain.started", Message: "started"})
	router := chi.NewRouter()
	New(eventsapp.New(bus)).Mount(router)
	server := httptest.NewServer(router)
	defer server.Close()

	client := &stdhttp.Client{Timeout: 5 * time.Second}
	response, err := client.Get(server.URL + "/events/stream?limit=1")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != stdhttp.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	reader := bufio.NewReader(response.Body)
	if event := readSSEEvent(t, reader); event.Kind != "domain.started" {
		t.Fatalf("snapshot event = %+v", event)
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		bus.Publish(domain.SystemEvent{Kind: "domain.stopped", Message: "stopped"})
	}()
	if event := readSSEEvent(t, reader); event.Kind != "domain.stopped" {
		t.Fatalf("live event = %+v", event)
	}
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) domain.SystemEvent {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				t.Fatal("stream ended before event")
			}
			t.Fatal(err)
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event domain.SystemEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		return event
	}
}
