package libvirtbackend

import (
	"testing"

	libvirt "libvirt.org/go/libvirt"
)

func TestMapDomainLifecycleEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    *libvirt.DomainEventLifecycle
		kind     string
		severity string
	}{
		{name: "started", event: &libvirt.DomainEventLifecycle{Event: libvirt.DOMAIN_EVENT_STARTED}, kind: "domain.started", severity: "info"},
		{name: "updated", event: &libvirt.DomainEventLifecycle{Event: libvirt.DOMAIN_EVENT_DEFINED, Detail: int(libvirt.DOMAIN_EVENT_DEFINED_UPDATED)}, kind: "domain.updated", severity: "info"},
		{name: "stopped", event: &libvirt.DomainEventLifecycle{Event: libvirt.DOMAIN_EVENT_STOPPED}, kind: "domain.stopped", severity: "warning"},
		{name: "crashed", event: &libvirt.DomainEventLifecycle{Event: libvirt.DOMAIN_EVENT_STOPPED, Detail: int(libvirt.DOMAIN_EVENT_STOPPED_CRASHED)}, kind: "domain.crashed", severity: "critical"},
		{name: "undefined", event: &libvirt.DomainEventLifecycle{Event: libvirt.DOMAIN_EVENT_UNDEFINED}, kind: "domain.undefined", severity: "warning"},
		{name: "unknown", event: &libvirt.DomainEventLifecycle{Event: libvirt.DomainEventType(999)}, kind: "domain.changed", severity: "info"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind, severity, message := mapDomainLifecycleEvent(test.event)
			if kind != test.kind || severity != test.severity || message == "" {
				t.Fatalf("mapped = %q %q %q", kind, severity, message)
			}
		})
	}
}
