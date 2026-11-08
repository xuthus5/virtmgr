package libvirtbackend

import (
	"encoding/xml"
	"testing"

	"virtmgr/internal/domain"
)

func TestBuildSnapshotXML(t *testing.T) {
	text := buildSnapshotXML(domain.VMSnapshotCreateSpec{Name: "before&upgrade", Description: "stable <state>"})
	var parsed snapshotXML
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Name != "before&upgrade" || parsed.Description != "stable <state>" {
		t.Fatalf("parsed snapshot = %+v XML=%s", parsed, text)
	}
}

func TestParseSnapshotXML(t *testing.T) {
	const text = `<domainsnapshot><name>baseline</name><description>clean install</description><state>running</state><creationTime>1700000000</creationTime><parent><name>root</name></parent></domainsnapshot>`
	var parsed snapshotXML
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Name != "baseline" || parsed.Description != "clean install" || parsed.State != "running" || parsed.CreationTime != 1700000000 || parsed.Parent.Name != "root" {
		t.Fatalf("parsed snapshot = %+v", parsed)
	}
	if got := mapSnapshotState(parsed.State); got != domain.VMStateRunning {
		t.Fatalf("mapSnapshotState(%q) = %q", parsed.State, got)
	}
}

func TestMapSnapshotState(t *testing.T) {
	tests := map[string]domain.VMState{
		"running":     domain.VMStateRunning,
		"blocked":     domain.VMStateBlocked,
		"paused":      domain.VMStatePaused,
		"shutdown":    domain.VMStateShutdown,
		"shutoff":     domain.VMStateShutoff,
		"crashed":     domain.VMStateCrashed,
		"pmsuspended": domain.VMStatePMSuspended,
		"unexpected":  domain.VMStateUnknown,
	}
	for input, want := range tests {
		if got := mapSnapshotState(input); got != want {
			t.Fatalf("mapSnapshotState(%q) = %q, want %q", input, got, want)
		}
	}
}
