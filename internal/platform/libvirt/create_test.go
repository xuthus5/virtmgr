package libvirtbackend

import (
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

func TestBuildDomainXML(t *testing.T) {
	spec := domain.VMCreateSpec{
		Name: "web<01>", VCPUs: 4, MemoryMiB: 4096,
		DiskPath: "/var/lib/libvirt/images/web&01.qcow2", DiskFormat: "qcow2",
		Network: "default", ISOPath: "/var/lib/libvirt/images/install.iso",
	}
	text := buildDomainXML(spec)
	if !strings.Contains(text, "web&lt;01&gt;") || !strings.Contains(text, "web&amp;01.qcow2") {
		t.Fatalf("XML is not escaped: %s", text)
	}
	details, err := parseDomainXML(text)
	if err != nil {
		t.Fatalf("parseDomainXML() error = %v", err)
	}
	if details.Arch != "x86_64" || details.Machine != "" {
		t.Fatalf("details = %+v", details)
	}
	if details.Firmware != domain.VMFirmwareBIOS || details.SecureBoot {
		t.Fatalf("firmware details = %+v", details)
	}
	if len(details.Disks) != 2 || details.Disks[0].Source != spec.DiskPath || !details.Disks[1].ReadOnly {
		t.Fatalf("disks = %+v", details.Disks)
	}
	if len(details.Interfaces) != 1 || details.Interfaces[0].Source != "default" {
		t.Fatalf("interfaces = %+v", details.Interfaces)
	}
	if len(details.Graphics) != 1 || details.Graphics[0].Type != "vnc" {
		t.Fatalf("graphics = %+v", details.Graphics)
	}
	if strings.Contains(text, "<boot dev=") || !strings.Contains(text, "<boot order='1'/>") || !strings.Contains(text, "<boot order='2'/>") {
		t.Fatalf("device boot order is invalid: %s", text)
	}
}

func TestBuildDomainXMLUEFI(t *testing.T) {
	for _, test := range []struct {
		name       string
		secureBoot bool
		secureAttr string
	}{
		{name: "uefi", secureBoot: false, secureAttr: "no"},
		{name: "uefi secure boot", secureBoot: true, secureAttr: "yes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			text := buildDomainXML(domain.VMCreateSpec{
				Name: "web-01", VCPUs: 2, MemoryMiB: 2048,
				DiskPath: "/var/lib/libvirt/images/web-01.qcow2", DiskFormat: "qcow2",
				Network: "default", Firmware: domain.VMFirmwareUEFI, SecureBoot: test.secureBoot,
			})
			for _, fragment := range []string{"<os firmware='efi'>", "machine='q35'", "<loader secure='" + test.secureAttr + "'/>"} {
				if !strings.Contains(text, fragment) {
					t.Fatalf("XML missing %q: %s", fragment, text)
				}
			}
			details, err := parseDomainXML(text)
			if err != nil {
				t.Fatalf("parseDomainXML() error = %v", err)
			}
			if details.Firmware != domain.VMFirmwareUEFI || details.SecureBoot != test.secureBoot || details.Machine != "q35" {
				t.Fatalf("firmware details = %+v", details)
			}
		})
	}
}

func TestBuildDomainXMLWithoutISO(t *testing.T) {
	text := buildDomainXML(domain.VMCreateSpec{
		Name: "web-01", VCPUs: 2, MemoryMiB: 2048,
		DiskPath: "/var/lib/libvirt/images/web-01.qcow2", DiskFormat: "raw", Network: "br0",
	})
	if strings.Contains(text, "<boot dev=") || !strings.Contains(text, "<boot order='1'/>") || strings.Contains(text, "<readonly/>") || !strings.Contains(text, "type='raw'") {
		t.Fatalf("unexpected XML: %s", text)
	}
}

func TestBuildDomainXMLCustomDevices(t *testing.T) {
	text := buildDomainXML(domain.VMCreateSpec{
		Name: "windows-01", VCPUs: 4, MemoryMiB: 8192,
		DiskPath: "/var/lib/libvirt/images/windows-01.qcow2", DiskFormat: "qcow2",
		DiskBus: domain.VMDiskBusSCSI, Network: "default", NICModel: domain.VMNICModelE1000,
		ISOPath: "/var/lib/libvirt/images/windows.iso", BootOrder: domain.VMBootOrderDiskFirst,
	})
	for _, fragment := range []string{
		"<controller type='scsi' index='0' model='virtio-scsi'/>",
		"<target dev='sda' bus='scsi'/><boot order='1'/>",
		"<target dev='sdb' bus='sata'/><readonly/><boot order='2'/>",
		"<model type='e1000'/>",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("XML missing %q: %s", fragment, text)
		}
	}
	details, err := parseDomainXML(text)
	if err != nil {
		t.Fatalf("parseDomainXML() error = %v", err)
	}
	if len(details.Disks) != 2 || details.Disks[0].Bus != "scsi" || details.Interfaces[0].Model != "e1000" {
		t.Fatalf("details = %+v", details)
	}
}
