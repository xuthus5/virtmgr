package libvirtbackend

import (
	"testing"

	"virtmgr/internal/domain"
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
    <serial type='pty'>
      <target type='isa-serial' port='0'/>
      <alias name='serial0'/>
    </serial>
    <console type='pty'>
      <target type='serial' port='0'/>
      <alias name='serial0'/>
    </console>
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
	if len(details.Serials) != 1 || details.Serials[0].Device != "serial0" || details.Serials[0].Label != "Serial 1" {
		t.Fatalf("serials = %+v", details.Serials)
	}
	if details.Firmware != "bios" || details.SecureBoot {
		t.Fatalf("firmware = %q secureBoot=%v", details.Firmware, details.SecureBoot)
	}
}

func TestParseDomainXMLSerialConsoleFallbacks(t *testing.T) {
	tests := []struct {
		name string
		xml  string
		want []domain.SerialConsole
	}{
		{
			name: "serial without console",
			xml:  `<domain><devices><serial type='pty'><target type='isa-serial' port='1'/><alias name='serial1'/></serial></devices></domain>`,
			want: []domain.SerialConsole{{Device: "serial1", Label: "Serial 2", TargetPort: 1, TargetType: "isa-serial"}},
		},
		{
			name: "console inherits serial alias",
			xml:  `<domain><devices><serial type='pty'><target type='isa-serial' port='0'/><alias name='serial0'/></serial><console type='pty'><target type='serial' port='0'/></console></devices></domain>`,
			want: []domain.SerialConsole{{Device: "serial0", Label: "Serial 1", TargetPort: 0, TargetType: "serial"}},
		},
		{
			name: "duplicate console and serial are collapsed",
			xml:  `<domain><devices><console type='pty'><target type='serial' port='0'/><alias name='serial0'/></console><serial type='pty'><target type='isa-serial' port='0'/><alias name='serial0'/></serial></devices></domain>`,
			want: []domain.SerialConsole{{Device: "serial0", Label: "Serial 1", TargetPort: 0, TargetType: "serial"}},
		},
		{
			name: "non pty console is ignored",
			xml:  `<domain><devices><console type='vc'><target type='serial' port='0'/></console></devices></domain>`,
			want: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			details, err := parseDomainXML(test.xml)
			if err != nil {
				t.Fatalf("parseDomainXML() error = %v", err)
			}
			if len(details.Serials) != len(test.want) {
				t.Fatalf("serials = %+v", details.Serials)
			}
			for index := range test.want {
				if details.Serials[index] != test.want[index] {
					t.Fatalf("serials[%d] = %+v, want %+v", index, details.Serials[index], test.want[index])
				}
			}
		})
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
