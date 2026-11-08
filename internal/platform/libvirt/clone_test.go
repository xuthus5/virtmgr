package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

const cloneSourceXML = `<?xml version="1.0" encoding="UTF-8"?>
<domain type="kvm">
  <name>source-vm</name>
  <uuid>11111111-1111-4111-8111-111111111111</uuid>
  <genid>aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa</genid>
  <os><type arch="x86_64" machine="pc-q35-10.2">hvm</type><nvram>/var/lib/libvirt/qemu/nvram/source_VARS.fd</nvram></os>
  <devices>
    <disk type="file" device="disk"><driver name="qemu" type="qcow2"/><source file="/var/lib/libvirt/images/source.qcow2"/><target dev="vda" bus="virtio"/></disk>
    <disk type="file" device="cdrom"><driver name="qemu" type="raw"/><source file="/var/lib/libvirt/images/install.iso"/><target dev="sda" bus="sata"/><readonly/></disk>
    <interface type="network"><source network="default"/><mac address="52:54:00:12:34:56"/><model type="virtio"/></interface>
    <graphics type="vnc" port="5901" autoport="no" listen="127.0.0.1"/>
  </devices>
</domain>`

func TestRewriteCloneDomainXML(t *testing.T) {
	rewritten, err := rewriteCloneDomainXML(cloneSourceXML, "source-vm-clone", map[string]string{
		"/var/lib/libvirt/images/source.qcow2": "/var/lib/libvirt/images/source-vm-clone-1.qcow2",
	})
	if err != nil {
		t.Fatalf("rewriteCloneDomainXML() error = %v", err)
	}
	if !strings.Contains(rewritten, "<name>source-vm-clone</name>") {
		t.Fatalf("name was not replaced: %s", rewritten)
	}
	for _, forbidden := range []string{"<uuid>", "<genid>", "<nvram>", "52:54:00:12:34:56"} {
		if strings.Contains(rewritten, forbidden) {
			t.Fatalf("rewritten XML still contains %q: %s", forbidden, rewritten)
		}
	}
	if !strings.Contains(rewritten, "/var/lib/libvirt/images/source-vm-clone-1.qcow2") || !strings.Contains(rewritten, "/var/lib/libvirt/images/install.iso") {
		t.Fatalf("disk sources were not rewritten correctly: %s", rewritten)
	}
	details, err := parseDomainXML(rewritten)
	if err != nil {
		t.Fatalf("parse rewritten XML: %v", err)
	}
	if len(details.Graphics) != 1 || details.Graphics[0].Port != 0 {
		t.Fatalf("graphics port was not reset: %+v", details.Graphics)
	}
	var decoded struct {
		Devices struct {
			Graphics []struct {
				Autoport string `xml:"autoport,attr"`
			} `xml:"graphics"`
		} `xml:"devices"`
	}
	if err := xml.Unmarshal([]byte(rewritten), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Devices.Graphics) != 1 || decoded.Devices.Graphics[0].Autoport != "yes" {
		t.Fatalf("autoport was not enabled: %+v", decoded.Devices.Graphics)
	}
}

func TestCloneDisksFromDomainXML(t *testing.T) {
	disks, err := cloneDisksFromDomainXML(cloneSourceXML)
	if err != nil {
		t.Fatal(err)
	}
	if len(disks) != 1 || disks[0].source != "/var/lib/libvirt/images/source.qcow2" || disks[0].format != "qcow2" {
		t.Fatalf("disks = %+v", disks)
	}
	if format, err := cloneDiskFormat("", "disk.RAW"); err != nil || format != "raw" {
		t.Fatalf("cloneDiskFormat() = %q, %v", format, err)
	}
	if _, err := cloneDiskFormat("vmdk", "disk.vmdk"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("unsupported format error = %v", err)
	}
	if name := cloneVolumeName("clone", 1, "source.qcow2", "qcow2"); name != "clone-2.qcow2" {
		t.Fatalf("cloneVolumeName() = %q", name)
	}
	longName := strings.Repeat("a", 127)
	if name := cloneVolumeName(longName, 0, "source.qcow2", "qcow2"); len(name) > 127 || !strings.HasSuffix(name, ".qcow2") {
		t.Fatalf("long clone volume name = %q (%d)", name, len(name))
	}
}

func TestDemoBackendCloneVM(t *testing.T) {
	backend := NewDemo()
	ctx := context.Background()
	const sourceUUID = "44444444-4444-4444-8444-444444444444"
	source, err := backend.GetVM(ctx, sourceUUID)
	if err != nil {
		t.Fatal(err)
	}
	progress := make([]int, 0, 4)
	clone, err := backend.CloneVM(ctx, sourceUUID, domain.VMCloneSpec{Name: "demo-clone-01", Start: true}, func(percent int, _ string) {
		progress = append(progress, percent)
	})
	if err != nil {
		t.Fatalf("CloneVM() error = %v", err)
	}
	if clone.UUID == source.UUID || clone.Name != "demo-clone-01" || clone.State != domain.VMStateRunning {
		t.Fatalf("clone = %+v", clone)
	}
	if clone.Disks[0].Source == source.Disks[0].Source || clone.Interfaces[0].MAC == source.Interfaces[0].MAC {
		t.Fatalf("clone did not isolate disk or MAC: %+v", clone)
	}
	if len(progress) == 0 || progress[len(progress)-1] != 100 {
		t.Fatalf("progress = %+v", progress)
	}
	volumes, err := backend.ListStorageVolumes(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	foundVolume := false
	for _, volume := range volumes {
		if volume.Path == clone.Disks[0].Source {
			foundVolume = true
			break
		}
	}
	if !foundVolume {
		t.Fatalf("clone volume not registered: %+v", volumes)
	}
	if _, err := backend.CloneVM(ctx, sourceUUID, domain.VMCloneSpec{Name: "demo-clone-01"}, nil); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate clone error = %v", err)
	}
	if _, err := backend.CloneVM(ctx, "22222222-2222-4222-8222-222222222222", domain.VMCloneSpec{Name: "running-clone"}, nil); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("running clone error = %v", err)
	}
}
