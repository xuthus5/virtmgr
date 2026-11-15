package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseVMXMLIdentity(t *testing.T) {
	identity, err := ParseVMXMLIdentity(`<?xml version="1.0"?>
<domain type="kvm">
  <name> api-01 </name>
  <uuid>uuid-a</uuid>
  <devices/>
</domain>`)
	if err != nil {
		t.Fatalf("ParseVMXMLIdentity() error = %v", err)
	}
	if identity.Name != "api-01" || identity.UUID != "uuid-a" {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestParseVMXMLIdentityRejectsInvalidOrUnsafeXML(t *testing.T) {
	tests := map[string]string{
		"empty":            " ",
		"wrong root":       `<network><name>api-01</name><uuid>uuid-a</uuid></network>`,
		"missing uuid":     `<domain><name>api-01</name></domain>`,
		"broken xml":       `<domain><name>api-01</name>`,
		"multiple roots":   `<domain><name>api-01</name><uuid>uuid-a</uuid></domain><domain/>`,
		"qemu commandline": `<domain xmlns:qemu="http://libvirt.org/schemas/domain/qemu/1.0"><name>api-01</name><uuid>uuid-a</uuid><qemu:commandline><qemu:arg value="-s"/></qemu:commandline></domain>`,
	}
	for name, text := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseVMXMLIdentity(text); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestParseVMXMLIdentityAllowsMetadataNamespaces(t *testing.T) {
	identity, err := ParseVMXMLIdentity(`<domain xmlns:virtmgr="https://example.test/virtmgr"><name>api-01</name><uuid>uuid-a</uuid><metadata><virtmgr:owner>ops</virtmgr:owner></metadata></domain>`)
	if err != nil {
		t.Fatalf("ParseVMXMLIdentity() error = %v", err)
	}
	if identity.Name != "api-01" || strings.TrimSpace(identity.UUID) != "uuid-a" {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestEjectVMISOXMLRemovesInstallerAndKeepsCloudInit(t *testing.T) {
	text := `<?xml version="1.0"?>
<domain type="kvm" xmlns:qemu="http://libvirt.org/schemas/domain/qemu/1.0">
  <name>demo-01</name>
  <uuid>uuid-a</uuid>
  <os><type>hvm</type><boot dev="cdrom"/></os>
  <devices>
    <disk type="file" device="disk"><source file="/images/demo.qcow2"/><target dev="vda" bus="virtio"/><boot order="2"/></disk>
    <disk type="file" device="cdrom"><source file="/images/install.iso"/><target dev="sda" bus="sata"/><readonly/><boot order="1"/></disk>
    <disk type="file" device="cdrom"><source file="/images/demo-cidata.iso"/><target dev="sdb" bus="sata"/><readonly/><serial>cloud-init</serial></disk>
  </devices>
  <qemu:commandline><qemu:arg value="-no-reboot"/></qemu:commandline>
</domain>`
	result, err := EjectVMISOXML(text)
	if err != nil {
		t.Fatalf("EjectVMISOXML() error = %v", err)
	}
	if !result.Ejected || result.Target != "sda" || result.Source != "/images/install.iso" {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(result.XML, "install.iso") || !strings.Contains(result.XML, "demo-cidata.iso") || !strings.Contains(result.XML, "<serial>cloud-init</serial>") {
		t.Fatalf("ejected XML = %s", result.XML)
	}
	if !strings.Contains(result.XML, `<boot order="1"></boot>`) || strings.Contains(result.XML, `dev="cdrom"`) {
		t.Fatalf("boot order not updated: %s", result.XML)
	}
	if strings.Count(result.XML, "<?xml") != 1 {
		t.Fatalf("XML declaration count = %d: %s", strings.Count(result.XML, "<?xml"), result.XML)
	}
	if _, err := ParseVMXMLIdentity(result.XML); err != nil {
		t.Fatalf("ejected XML identity error = %v", err)
	}
}

func TestEjectVMISOXMLReportsWhenNoInstallerISOExists(t *testing.T) {
	result, err := EjectVMISOXML(`<domain><name>demo</name><uuid>uuid-a</uuid><devices><disk type="file" device="disk"><source file="/images/demo.qcow2"/><target dev="vda"/></disk></devices></domain>`)
	if err != nil {
		t.Fatalf("EjectVMISOXML() error = %v", err)
	}
	if result.Ejected {
		t.Fatalf("result = %+v", result)
	}
}
