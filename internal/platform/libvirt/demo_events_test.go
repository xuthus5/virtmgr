package libvirtbackend

import (
	"context"
	"testing"

	"virtmgr/internal/domain"
	"virtmgr/internal/platform/eventbus"
)

func TestDemoBackendPublishesLifecycleEvents(t *testing.T) {
	bus := eventbus.New(50)
	backend := NewDemo(bus)
	if recent := bus.Recent(10); len(recent) != 3 {
		t.Fatalf("seed events = %+v", recent)
	}
	const uuid = "44444444-4444-4444-8444-444444444444"
	if err := backend.PerformAction(context.Background(), uuid, domain.VMActionStart); err != nil {
		t.Fatalf("PerformAction() error = %v", err)
	}
	recent := bus.Recent(1)
	if len(recent) != 1 || recent[0].Kind != "domain.started" || recent[0].TargetName != "demo-legacy-01" {
		t.Fatalf("recent = %+v", recent)
	}
}

func TestDemoBackendPublishesAutostartEvent(t *testing.T) {
	bus := eventbus.New(50)
	backend := NewDemo(bus)
	const uuid = "33333333-3333-4333-8333-333333333333"
	if err := backend.SetVMAutostart(context.Background(), uuid, true); err != nil {
		t.Fatalf("SetVMAutostart() error = %v", err)
	}
	recent := bus.Recent(1)
	if len(recent) != 1 || recent[0].Kind != "domain.autostart" || recent[0].TargetName != "demo-build-01" {
		t.Fatalf("recent = %+v", recent)
	}
}
