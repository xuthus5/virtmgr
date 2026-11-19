package domain

import (
	"errors"
	"strings"
	"testing"
)

const configurationDomainXML = `<domain type='kvm'>
  <name>config-vm</name>
  <uuid>7c7c1e64-6f0f-4d66-9b2a-9fb52c4c9270</uuid>
  <os><type arch='x86_64' machine='pc-q35-10.2'>hvm</type><boot dev='hd'/></os>
  <cpu mode='host-model'><topology sockets='1' cores='2' threads='1'/></cpu>
  <devices>
    <disk type='file' device='disk'><source file='/var/lib/libvirt/images/config.qcow2'/><target dev='vda' bus='virtio'/><boot order='2'/></disk>
    <disk type='file' device='cdrom'><source file='/var/lib/libvirt/images/install.iso'/><target dev='sda' bus='sata'/><readonly/></disk>
    <interface type='network'><mac address='52:54:00:01:12:99'/><source network='default'/><model type='virtio'/><boot order='3'/></interface>
    <hostdev mode='subsystem' type='pci'><source><address domain='0x0000' bus='0x01' slot='0x00' function='0x0'/></source><boot order='4'/></hostdev>
  </devices>
</domain>`

func TestSetVMXMLBootDevicesReplacesPersistentBootOrder(t *testing.T) {
	updated, err := SetVMXMLBootDevices(configurationDomainXML, []VMBootDevice{VMBootDeviceCDROM, VMBootDeviceDisk, VMBootDeviceNetwork})
	if err != nil {
		t.Fatalf("SetVMXMLBootDevices() error = %v", err)
	}
	for _, want := range []string{"<boot dev=\"cdrom\"></boot>", "<boot dev=\"hd\"></boot>", "<boot dev=\"network\"></boot>"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated XML missing %q:\n%s", want, updated)
		}
	}
	if strings.Count(updated, "<boot ") != 3 {
		t.Fatalf("boot device count mismatch:\n%s", updated)
	}
	if strings.Contains(updated, `order="2"`) || strings.Contains(updated, `order="3"`) || strings.Contains(updated, `order="4"`) {
		t.Fatalf("legacy device boot order was not removed:\n%s", updated)
	}
	identity, err := ParseVMXMLIdentity(updated)
	if err != nil || identity.Name != "config-vm" {
		t.Fatalf("identity = %+v, error = %v", identity, err)
	}
}

func TestSetVMXMLCPUConfigClearsModeSpecificAttributes(t *testing.T) {
	custom, err := SetVMXMLCPUConfig(configurationDomainXML, VMCPUConfigSpec{Model: "Cascadelake-Server", Sockets: 1, Cores: 2, Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(custom, `<cpu mode="custom" match="exact">`) {
		t.Fatalf("custom CPU XML unexpected:\n%s", custom)
	}
	hostModel, err := SetVMXMLCPUConfig(custom, VMCPUConfigSpec{Model: "host-model", Sockets: 1, Cores: 2, Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hostModel, `<cpu mode="host-model" check="partial">`) || strings.Contains(hostModel, `match="exact"`) || strings.Contains(hostModel, "Cascadelake-Server") {
		t.Fatalf("host-model CPU XML unexpected:\n%s", hostModel)
	}
}

func TestSetVMXMLCPUConfigSupportsModelAndTopology(t *testing.T) {
	updated, err := SetVMXMLCPUConfig(configurationDomainXML, VMCPUConfigSpec{Model: "Cascadelake-Server", Sockets: 2, Cores: 2, Threads: 1})
	if err != nil {
		t.Fatalf("SetVMXMLCPUConfig() error = %v", err)
	}
	for _, want := range []string{"mode=\"custom\"", "match=\"exact\"", "Cascadelake-Server", "sockets=\"2\"", "cores=\"2\"", "threads=\"1\""} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated XML missing %q:\n%s", want, updated)
		}
	}

	updated, err = SetVMXMLCPUConfig(updated, VMCPUConfigSpec{Model: "host-passthrough", Sockets: 1, Cores: 4, Threads: 1})
	if err != nil {
		t.Fatalf("SetVMXMLCPUConfig(host-passthrough) error = %v", err)
	}
	if !strings.Contains(updated, "<cpu mode=\"host-passthrough\" check=\"none\">") || strings.Contains(updated, "Cascadelake-Server") {
		t.Fatalf("host-passthrough XML unexpected:\n%s", updated)
	}
}

func TestSetVMNetworkQoSXMLUpdatesAndClearsBandwidth(t *testing.T) {
	updated, err := SetVMNetworkQoSXML(configurationDomainXML, "52:54:00:01:12:99", VMNetworkQoSSpec{
		InboundAverageBytes: 10 << 20, InboundPeakBytes: 20 << 20, InboundBurstBytes: 1 << 20,
		OutboundAverageBytes: 5 << 20,
	})
	if err != nil {
		t.Fatalf("SetVMNetworkQoSXML() error = %v", err)
	}
	for _, want := range []string{"<bandwidth>", "<inbound average=\"10485760\" peak=\"20971520\" burst=\"1048576\">", "<outbound average=\"5242880\">"} {
		if !strings.Contains(updated.XML, want) {
			t.Fatalf("updated XML missing %q:\n%s", want, updated.XML)
		}
	}
	if !strings.Contains(updated.DeviceXML, "<bandwidth>") || strings.Contains(updated.DeviceXML, "<domain") {
		t.Fatalf("device XML is not a standalone interface subtree:\n%s", updated.DeviceXML)
	}

	cleared, err := SetVMNetworkQoSXML(updated.XML, "52:54:00:01:12:99", VMNetworkQoSSpec{})
	if err != nil {
		t.Fatalf("SetVMNetworkQoSXML(clear) error = %v", err)
	}
	if strings.Contains(cleared.XML, "<bandwidth>") {
		t.Fatalf("bandwidth was not removed:\n%s", cleared.XML)
	}
}

func TestSetVMNetworkModelXMLReplacesModel(t *testing.T) {
	updated, err := SetVMNetworkModelXML(configurationDomainXML, "52:54:00:01:12:99", VMNICModelE1000)
	if err != nil {
		t.Fatalf("SetVMNetworkModelXML() error = %v", err)
	}
	if !strings.Contains(updated, `<model type="e1000"></model>`) || strings.Contains(updated, `<model type="virtio">`) {
		t.Fatalf("network model was not replaced:\n%s", updated)
	}
	if _, err := SetVMNetworkModelXML(configurationDomainXML, "52:54:00:01:12:00", VMNICModelE1000); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing interface error = %v", err)
	}
	if _, err := SetVMNetworkModelXML(configurationDomainXML, "52:54:00:01:12:99", VMNICModel("rtl8139")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid model error = %v", err)
	}
}
