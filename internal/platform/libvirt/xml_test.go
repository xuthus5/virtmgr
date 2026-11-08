package libvirtbackend

import (
	"testing"
)

const sampleDomainXML = `<domain type='kvm'>
  <name>demo-vm</name>
  <uuid>7c7c1e64-6f0f-4d66-9b2a-9fb52c4c9270</uuid>
  <memory unit='KiB'>4194304</memory>
  <vcpu>2</vcpu>
  <os><type arch='x86_64' machine='pc-q35-10.2'>hvm</type></os>
  <devices>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source file='/var/lib/libvirt/images/demo.qcow2'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <disk type='file' device='cdrom'>
      <source file='/var/lib/libvirt/images/demo.iso'/>
      <target dev='sda' bus='sata'/>
      <readonly/>
    </disk>
    <interface type='network'>
      <mac address='52:54:00:01:12:99'/>
      <source network='default'/>
      <model type='virtio'/>
    </interface>
    <graphics type='vnc' port='5901' listen='127.0.0.1'/>
  </devices>
</domain>`

const sampleUEFIDomainXML = `<domain type='kvm'>
  <name>secure-vm</name>
  <os firmware='efi'>
    <type arch='x86_64' machine='pc-q35-10.2'>hvm</type>
    <firmware>
      <feature enabled='yes' name='enrolled-keys'/>
      <feature enabled='yes' name='secure-boot'/>
    </firmware>
    <loader readonly='yes' secure='yes' type='pflash' format='qcow2'>/usr/share/edk2/ovmf/OVMF_CODE_4M.secboot.qcow2</loader>
    <nvram template='/usr/share/edk2/ovmf/OVMF_VARS_4M.secboot.qcow2' templateFormat='qcow2' format='qcow2'>/var/lib/libvirt/qemu/nvram/secure-vm_VARS.qcow2</nvram>
  </os>
</domain>`

func TestParseDomainXML(t *testing.T) {
	details, err := parseDomainXML(sampleDomainXML)
	if err != nil {
		t.Fatalf("parseDomainXML() error = %v", err)
	}
	if details.OSType != "hvm" || details.Arch != "x86_64" || details.Machine != "pc-q35-10.2" {
		t.Fatalf("os details = %+v", details)
	}
	if len(details.Disks) != 2 || details.Disks[0].Target != "vda" || details.Disks[0].Format != "qcow2" {
		t.Fatalf("disks = %+v", details.Disks)
	}
	if !details.Disks[1].ReadOnly {
		t.Fatal("cdrom must be read-only")
	}
	if len(details.Interfaces) != 1 || details.Interfaces[0].Source != "default" || details.Interfaces[0].MAC == "" {
		t.Fatalf("interfaces = %+v", details.Interfaces)
	}
	if len(details.Graphics) != 1 || details.Graphics[0].Port != 5901 {
		t.Fatalf("graphics = %+v", details.Graphics)
	}
	if details.Firmware != "bios" || details.SecureBoot {
		t.Fatalf("firmware = %q secureBoot=%v", details.Firmware, details.SecureBoot)
	}
}

func TestParseDomainXMLUEFI(t *testing.T) {
	details, err := parseDomainXML(sampleUEFIDomainXML)
	if err != nil {
		t.Fatalf("parseDomainXML() error = %v", err)
	}
	if details.Firmware != "uefi" || !details.SecureBoot {
		t.Fatalf("firmware = %q secureBoot=%v", details.Firmware, details.SecureBoot)
	}
}

func TestParseDomainXMLDetectsUEFIWithoutFirmwareAttribute(t *testing.T) {
	details, err := parseDomainXML(`<domain><os><type arch='x86_64'>hvm</type><loader type='pflash'>/usr/share/OVMF/OVMF_CODE.fd</loader><nvram>/var/lib/libvirt/qemu/nvram/vm_VARS.fd</nvram></os></domain>`)
	if err != nil {
		t.Fatalf("parseDomainXML() error = %v", err)
	}
	if details.Firmware != "uefi" || details.SecureBoot {
		t.Fatalf("firmware = %q secureBoot=%v", details.Firmware, details.SecureBoot)
	}
}

func TestParseDomainXMLRejectsInvalidXML(t *testing.T) {
	if _, err := parseDomainXML("<domain>"); err == nil {
		t.Fatal("parseDomainXML() error = nil")
	}
}

func TestParseMemoryBytes(t *testing.T) {
	value, err := parseMemoryBytes("4", "GiB")
	if err != nil || value != 4*1024*1024*1024 {
		t.Fatalf("parseMemoryBytes() = %d, %v", value, err)
	}
	if _, err := parseMemoryBytes("4", "parsecs"); err == nil {
		t.Fatal("parseMemoryBytes() accepted unknown unit")
	}
}
