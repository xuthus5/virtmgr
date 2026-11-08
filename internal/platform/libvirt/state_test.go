package libvirtbackend

import (
	"testing"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func TestFormatVersion(t *testing.T) {
	if got := formatVersion(12_000_000); got != "12.0.0" {
		t.Fatalf("formatVersion() = %q", got)
	}
	if got := formatVersion(10_002_003); got != "10.2.3" {
		t.Fatalf("formatVersion() = %q", got)
	}
}

func TestMapDomainState(t *testing.T) {
	tests := map[libvirt.DomainState]domain.VMState{
		libvirt.DOMAIN_RUNNING:     domain.VMStateRunning,
		libvirt.DOMAIN_PAUSED:      domain.VMStatePaused,
		libvirt.DOMAIN_CRASHED:     domain.VMStateCrashed,
		libvirt.DOMAIN_SHUTOFF:     domain.VMStateShutoff,
		libvirt.DOMAIN_PMSUSPENDED: domain.VMStatePMSuspended,
	}
	for state, want := range tests {
		if got := mapDomainState(state); got != want {
			t.Fatalf("mapDomainState(%d) = %q, want %q", state, got, want)
		}
	}
}

func TestAllowedActions(t *testing.T) {
	running := allowedActions(libvirt.DOMAIN_RUNNING)
	if len(running) == 0 || running[0] != domain.VMActionShutdown {
		t.Fatalf("running actions = %v", running)
	}
	shutoff := allowedActions(libvirt.DOMAIN_SHUTOFF)
	if len(shutoff) != 1 || shutoff[0] != domain.VMActionStart {
		t.Fatalf("shutoff actions = %v", shutoff)
	}
}
