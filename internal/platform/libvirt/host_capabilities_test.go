package libvirtbackend

import (
	"testing"

	"virtmgr/internal/domain"
)

func TestParseLibvirtCapabilitiesBuildsHostPlatform(t *testing.T) {
	xmlText := `<capabilities>
  <host><cpu><arch>x86_64</arch></cpu></host>
  <guest><os_type>hvm</os_type><arch name="x86_64">
    <emulator>/usr/bin/qemu-system-x86_64</emulator>
    <machine>pc-i440fx-9.2</machine><machine>pc-q35-9.2</machine>
    <domain type="kvm"/><domain type="qemu"/>
  </arch></guest>
</capabilities>`
	got, err := parseLibvirtCapabilities(xmlText)
	if err != nil {
		t.Fatalf("parseLibvirtCapabilities() error = %v", err)
	}
	if got.Architecture != "x86_64" || got.Virtualization != "kvm" {
		t.Fatalf("capabilities = %+v", got)
	}
	if len(got.MachineTypes) != 2 || !domain.VMMachineSupported("q35", got.MachineTypes) {
		t.Fatalf("machine types = %+v", got.MachineTypes)
	}
}

func TestResolveVMPlatformUsesHostArchitectureAndDefaults(t *testing.T) {
	arm := hostPlatform{
		known: true,
		capabilities: domain.HostCapabilities{
			Architecture: "aarch64", Architectures: []string{"aarch64"}, MachineTypes: []string{"virt-9.2"}, UEFI: true, SecureBoot: true,
		},
	}
	architecture, machine, err := resolveVMPlatform(arm, "", "", domain.VMFirmwareBIOS)
	if err != nil {
		t.Fatalf("resolveVMPlatform() error = %v", err)
	}
	if architecture != "aarch64" || machine != "virt-9.2" {
		t.Fatalf("architecture=%q machine=%q", architecture, machine)
	}
	if _, _, err := resolveVMPlatform(arm, "x86_64", "", domain.VMFirmwareBIOS); err == nil {
		t.Fatal("unsupported architecture was accepted")
	}
	if _, _, err := resolveVMPlatform(arm, "aarch64", "pc-q35-9.2", domain.VMFirmwareBIOS); err == nil {
		t.Fatal("unsupported machine type was accepted")
	}
}

func TestResolveVMPlatformRejectsUnavailableFirmware(t *testing.T) {
	platform := hostPlatform{
		known: true,
		capabilities: domain.HostCapabilities{
			Architecture: "x86_64", Architectures: []string{"x86_64"}, MachineTypes: []string{"pc-q35-9.2"},
		},
	}
	if _, _, err := resolveVMPlatform(platform, "", "", domain.VMFirmwareUEFI); err == nil {
		t.Fatal("UEFI was accepted without host support")
	}
}

func TestParseDomainFirmwareCapabilities(t *testing.T) {
	uefi, secure, tpm := parseDomainCapabilities(`<domainCapabilities><os><loader supported="yes">
	  <enum name="secure">yes</enum>
	</loader></os><devices><tpm supported="yes"/></devices></domainCapabilities>`)
	if !uefi || !secure || !tpm {
		t.Fatalf("uefi=%v secure=%v tpm=%v", uefi, secure, tpm)
	}
}
