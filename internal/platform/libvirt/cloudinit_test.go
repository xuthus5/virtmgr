package libvirtbackend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"virtmgr/internal/domain"

	diskfs "github.com/diskfs/go-diskfs"
)

func TestBuildCloudInitISO(t *testing.T) {
	const userData = "#cloud-config\nruncmd:\n  - echo virtmgr-cloud-init-ok\n"
	data, err := buildCloudInitISO("vm-01", domain.CloudInitSpec{
		UserData:      userData,
		NetworkConfig: "version: 2\nethernets:\n  eth0:\n    addresses: [192.0.2.10/24]\n",
		VendorData:    "vendor: true\n",
	})
	if err != nil {
		t.Fatalf("buildCloudInitISO() error = %v", err)
	}
	if len(data) != cloudInitSeedSize {
		t.Fatalf("ISO size = %d, want %d", len(data), cloudInitSeedSize)
	}
	path := filepath.Join(t.TempDir(), "seed.iso")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	image, err := diskfs.Open(path)
	if err != nil {
		t.Fatalf("open generated ISO: %v", err)
	}
	defer func() { _ = image.Close() }()
	fs, err := image.GetFilesystem(0)
	if err != nil {
		t.Fatalf("read generated ISO filesystem: %v", err)
	}
	if label := strings.Trim(fs.Label(), " \x00"); label != "cidata" {
		t.Fatalf("ISO label = %q", label)
	}
	metaData, err := fs.ReadFile("meta-data")
	if err != nil {
		t.Fatalf("read meta-data: %v", err)
	}
	if !strings.Contains(string(metaData), "instance-id: vm-01") || !strings.Contains(string(metaData), "local-hostname: vm-01") {
		t.Fatalf("meta-data = %q", metaData)
	}
	readUserData, err := fs.ReadFile("user-data")
	if err != nil {
		t.Fatalf("read user-data: %v", err)
	}
	if !strings.Contains(string(readUserData), "virtmgr-cloud-init-ok") {
		t.Fatalf("user-data = %q", readUserData)
	}
	networkConfig, err := fs.ReadFile("network-config")
	if err != nil || !strings.Contains(string(networkConfig), "192.0.2.10/24") {
		t.Fatalf("network-config = %q error = %v", networkConfig, err)
	}
	vendorData, err := fs.ReadFile("vendor-data")
	if err != nil || !strings.Contains(string(vendorData), "vendor: true") {
		t.Fatalf("vendor-data = %q error = %v", vendorData, err)
	}
}

func TestBuildCloudInitISORejectsEmptyInput(t *testing.T) {
	if _, err := buildCloudInitISO("", domain.CloudInitSpec{UserData: "#cloud-config\n"}); err == nil {
		t.Fatal("buildCloudInitISO() accepted empty VM name")
	}
	if _, err := buildCloudInitISO("vm-01", domain.CloudInitSpec{UserData: "  "}); err == nil {
		t.Fatal("buildCloudInitISO() accepted empty user data")
	}
}

func TestBuildDomainXMLWithCloudInit(t *testing.T) {
	text := buildDomainXMLWithCloudInit(domain.VMCreateSpec{
		Name: "web-01", VCPUs: 2, MemoryMiB: 2048,
		DiskPath: "/var/lib/libvirt/images/web-01.qcow2", DiskFormat: "qcow2", Network: "default",
	}, "/var/lib/libvirt/images/web-01-cidata.iso")
	for _, fragment := range []string{"/var/lib/libvirt/images/web-01-cidata.iso", "dev='sdb'", "<serial>cloud-init</serial>"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("XML missing %q: %s", fragment, text)
		}
	}
	details, err := parseDomainXML(text)
	if err != nil {
		t.Fatalf("parseDomainXML() error = %v", err)
	}
	if !details.CloudInit || len(details.Disks) != 2 || !details.Disks[1].ReadOnly {
		t.Fatalf("cloud-init details = %+v", details)
	}
}

func TestCloudInitSeedVolumeName(t *testing.T) {
	if got := cloudInitSeedVolumeName("vm-01"); got != "vm-01-cidata.iso" {
		t.Fatalf("cloudInitSeedVolumeName() = %q", got)
	}
	longName := strings.Repeat("a", 64)
	if got := cloudInitSeedVolumeName(longName); len(got) > 127 || !strings.HasSuffix(got, "-cidata.iso") {
		t.Fatalf("long cloud-init volume name = %q (%d)", got, len(got))
	}
}
