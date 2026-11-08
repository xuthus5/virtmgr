package libvirtbackend

import (
	"context"
	"reflect"
	"testing"

	"virtmgr/internal/domain"
)

func TestParseGuestInfo(t *testing.T) {
	hostname, err := parseGuestHostname(`{"return":{"host-name":" db-01 "}}`)
	if err != nil || hostname != "db-01" {
		t.Fatalf("parseGuestHostname() = %q, %v", hostname, err)
	}
	if _, err := parseGuestHostname(`not-json`); err == nil {
		t.Fatal("parseGuestHostname() accepted invalid JSON")
	}

	addresses, err := parseGuestIPAddresses(`{"return":[
  {"name":"lo","ip-addresses":[{"ip-address":"127.0.0.1","ip-address-type":"ipv4","prefix":8}]},
  {"name":"eth0","ip-addresses":[
    {"ip-address":"2001:db8::10","ip-address-type":"ipv6","prefix":64},
    {"ip-address":"192.168.122.10","ip-address-type":"ipv4","prefix":24},
    {"ip-address":"192.168.122.10","ip-address-type":"ipv4","prefix":24},
    {"ip-address":"fe80::1%eth0","ip-address-type":"ipv6","prefix":64},
    {"ip-address":"2001:db8::20%eth0","ip-address-type":"ipv6","prefix":64}
  ]}
]}`)
	if err != nil {
		t.Fatalf("parseGuestIPAddresses() error = %v", err)
	}
	want := []string{"192.168.122.10", "2001:db8::10", "2001:db8::20"}
	if !reflect.DeepEqual(addresses, want) {
		t.Fatalf("addresses = %+v, want %+v", addresses, want)
	}
	if _, err := parseGuestIPAddresses(`[]`); err == nil {
		t.Fatal("parseGuestIPAddresses() accepted malformed response")
	}
}

func TestDemoBackendGuestInfo(t *testing.T) {
	backend := NewDemo()
	vm, err := backend.GetVM(context.Background(), "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if !vm.AgentConnected || vm.Hostname != "demo-web-01" || len(vm.IPAddresses) != 1 || vm.IPAddresses[0] != "192.168.122.101" {
		t.Fatalf("guest info = %+v", vm)
	}
	shutoff, err := backend.GetVM(context.Background(), "44444444-4444-4444-8444-444444444444")
	if err != nil {
		t.Fatal(err)
	}
	if shutoff.AgentConnected || shutoff.Hostname != "" || shutoff.IPAddresses == nil {
		t.Fatalf("shutoff guest info = %+v", shutoff)
	}
}

func TestGuestInfoCandidateIndexes(t *testing.T) {
	vms := []domain.VM{
		{State: domain.VMStateRunning},
		{State: domain.VMStateShutoff},
		{State: domain.VMStateBlocked},
		{State: domain.VMStatePaused},
		{State: domain.VMStateCrashed},
	}
	got := guestInfoCandidateIndexes(vms)
	want := []int{0, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("guestInfoCandidateIndexes() = %v, want %v", got, want)
	}
}
