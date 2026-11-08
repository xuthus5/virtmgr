package domain

import "testing"

func TestAllowedVMActions(t *testing.T) {
	if got := AllowedVMActions(VMStateRunning); !ContainsVMAction(got, VMActionShutdown) || !ContainsVMAction(got, VMActionDestroy) {
		t.Fatalf("running actions = %v", got)
	}
	if got := AllowedVMActions(VMStateShutoff); len(got) != 1 || got[0] != VMActionStart {
		t.Fatalf("shutoff actions = %v", got)
	}
	if got := AllowedVMActions(VMStatePaused); len(got) != 2 || got[0] != VMActionResume {
		t.Fatalf("paused actions = %v", got)
	}
}

func TestValidVMAction(t *testing.T) {
	if action, ok := ValidVMAction("start"); !ok || action != VMActionStart {
		t.Fatalf("ValidVMAction(start) = %q, %v", action, ok)
	}
	if _, ok := ValidVMAction("delete-disk"); ok {
		t.Fatal("ValidVMAction accepted unknown action")
	}
}

func TestCanOperateVMs(t *testing.T) {
	if CanOperateVMs(RoleViewer) {
		t.Fatal("viewer must not operate VMs")
	}
	if !CanOperateVMs(RoleOperator) || !CanOperateVMs(RoleAdmin) {
		t.Fatal("operator and admin must be able to operate VMs")
	}
	if !CanEditVMXML(RoleAdmin) || CanEditVMXML(RoleOperator) || CanEditVMXML(RoleViewer) {
		t.Fatal("only admin must be able to edit VM XML")
	}
}

func TestValidVMXMLFormat(t *testing.T) {
	if format, ok := ValidVMXMLFormat("live"); !ok || format != VMXMLFormatLive {
		t.Fatalf("ValidVMXMLFormat(live) = %q, %v", format, ok)
	}
	if format, ok := ValidVMXMLFormat("config"); !ok || format != VMXMLFormatConfig {
		t.Fatalf("ValidVMXMLFormat(config) = %q, %v", format, ok)
	}
	if _, ok := ValidVMXMLFormat("runtime"); ok {
		t.Fatal("ValidVMXMLFormat accepted unknown format")
	}
}

func TestNormalizeVMFirmware(t *testing.T) {
	tests := []struct {
		input string
		want  VMFirmware
		ok    bool
	}{
		{input: "", want: VMFirmwareBIOS, ok: true},
		{input: "bios", want: VMFirmwareBIOS, ok: true},
		{input: " UEFI ", want: VMFirmwareUEFI, ok: true},
		{input: "efi", want: VMFirmwareUEFI, ok: true},
		{input: "coreboot", ok: false},
	}
	for _, test := range tests {
		got, ok := NormalizeVMFirmware(test.input)
		if got != test.want || ok != test.ok {
			t.Fatalf("NormalizeVMFirmware(%q) = %q, %v; want %q, %v", test.input, got, ok, test.want, test.ok)
		}
	}
}
