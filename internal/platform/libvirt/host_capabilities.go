package libvirtbackend

import (
	"context"
	"encoding/xml"
	"fmt"
	"slices"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

const hostCapabilitiesTTL = 5 * time.Minute

type hostPlatform struct {
	capabilities domain.HostCapabilities
	known        bool
	guestArches  map[string]libvirtGuestArchXML
}

type libvirtCapabilitiesXML struct {
	Host struct {
		CPU struct {
			Arch string `xml:"arch"`
		} `xml:"cpu"`
	} `xml:"host"`
	Guests []libvirtGuestXML `xml:"guest"`
}

type libvirtGuestXML struct {
	OSType string                `xml:"os_type"`
	Arches []libvirtGuestArchXML `xml:"arch"`
}

type libvirtGuestArchXML struct {
	Name     string   `xml:"name,attr"`
	Emulator string   `xml:"emulator"`
	Machines []string `xml:"machine"`
	Domains  []struct {
		Type string `xml:"type,attr"`
	} `xml:"domain"`
}

type libvirtDomainCapabilitiesXML struct {
	Machine string `xml:"machine"`
	OS      struct {
		Loader struct {
			Supported string `xml:"supported,attr"`
			Enums     []struct {
				Name string `xml:"name,attr"`
				Text string `xml:",chardata"`
			} `xml:"enum"`
		} `xml:"loader"`
	} `xml:"os"`
	Devices struct {
		TPM struct {
			Supported string `xml:"supported,attr"`
			Enums     []struct {
				Name string `xml:"name,attr"`
				Text string `xml:",chardata"`
			} `xml:"enum"`
		} `xml:"tpm"`
	} `xml:"devices"`
}

func (c *Client) hostPlatformLocked(ctx context.Context) hostPlatform {
	if c.platform != nil && time.Since(c.platformAt) < hostCapabilitiesTTL {
		return *c.platform
	}
	platform := hostPlatform{capabilities: domain.HostCapabilities{Architecture: domain.HostArchitectureFallback()}}
	xmlText, err := c.conn.GetCapabilities()
	if err == nil {
		parsed, parseErr := parseLibvirtCapabilities(xmlText)
		if parseErr == nil && parsed.Architecture != "" {
			platform.capabilities = parsed
			platform.known = true
			platform.guestArches = parseGuestArchitectures(xmlText)
			platform.capabilities.UEFI, platform.capabilities.SecureBoot, platform.capabilities.TPM = c.domainFirmwareCapabilitiesLocked(ctx, platform)
		}
	}
	if err := ctx.Err(); err != nil {
		return hostPlatform{capabilities: domain.HostCapabilities{Architecture: domain.HostArchitectureFallback()}}
	}
	c.platform = &platform
	c.platformAt = time.Now()
	return platform
}

func (c *Client) domainFirmwareCapabilitiesLocked(ctx context.Context, platform hostPlatform) (bool, bool, bool) {
	emulator := ""
	machine := ""
	archXML, ok := platform.guestArches[platform.capabilities.Architecture]
	if ok {
		emulator = archXML.Emulator
		machine = domain.DefaultVMMachine(platform.capabilities.Architecture, domain.VMFirmwareUEFI)
		if machine != "" && !domain.VMMachineSupported(machine, archXML.Machines) {
			machine = ""
		}
		if machine == "" && len(archXML.Machines) > 0 {
			machine = archXML.Machines[0]
		}
	}
	if emulator == "" || ctx.Err() != nil {
		return false, false, false
	}
	xmlText, err := c.conn.GetDomainCapabilities(emulator, platform.capabilities.Architecture, machine, "kvm", 0)
	if err != nil || ctx.Err() != nil {
		return false, false, false
	}
	return parseDomainCapabilities(xmlText)
}

func parseGuestArchitectures(xmlText string) map[string]libvirtGuestArchXML {
	var document libvirtCapabilitiesXML
	if err := xml.Unmarshal([]byte(xmlText), &document); err != nil {
		return nil
	}
	result := make(map[string]libvirtGuestArchXML)
	for _, guest := range document.Guests {
		if !strings.EqualFold(strings.TrimSpace(guest.OSType), "hvm") {
			continue
		}
		for _, arch := range guest.Arches {
			name, ok := domain.NormalizeVMArchitecture(arch.Name)
			if !ok || name == "" || !guestSupportsKVM(arch) {
				continue
			}
			result[name] = arch
		}
	}
	return result
}

func parseLibvirtCapabilities(xmlText string) (domain.HostCapabilities, error) {
	var document libvirtCapabilitiesXML
	if err := xml.Unmarshal([]byte(xmlText), &document); err != nil {
		return domain.HostCapabilities{}, fmt.Errorf("decode libvirt capabilities: %w", err)
	}
	hostArchitecture, ok := domain.NormalizeVMArchitecture(document.Host.CPU.Arch)
	if !ok || hostArchitecture == "" {
		hostArchitecture = domain.HostArchitectureFallback()
	}
	architectureSet := make([]string, 0)
	machineSet := make([]string, 0)
	for _, guest := range document.Guests {
		if !strings.EqualFold(strings.TrimSpace(guest.OSType), "hvm") {
			continue
		}
		for _, arch := range guest.Arches {
			name, valid := domain.NormalizeVMArchitecture(arch.Name)
			if !valid || name == "" || !guestSupportsKVM(arch) {
				continue
			}
			if !slices.Contains(architectureSet, name) {
				architectureSet = append(architectureSet, name)
			}
			if name != hostArchitecture {
				continue
			}
			for _, machine := range arch.Machines {
				machine = strings.TrimSpace(machine)
				if machine != "" && !slices.Contains(machineSet, machine) {
					machineSet = append(machineSet, machine)
				}
			}
		}
	}
	if !slices.Contains(architectureSet, hostArchitecture) {
		architectureSet = append([]string{hostArchitecture}, architectureSet...)
	}
	slices.Sort(machineSet)
	return domain.HostCapabilities{
		Architecture: hostArchitecture, Architectures: architectureSet, MachineTypes: machineSet, Virtualization: "kvm",
	}, nil
}

func guestSupportsKVM(arch libvirtGuestArchXML) bool {
	for _, domainType := range arch.Domains {
		if strings.EqualFold(strings.TrimSpace(domainType.Type), "kvm") {
			return true
		}
	}
	return false
}

func parseDomainCapabilities(xmlText string) (bool, bool, bool) {
	var document libvirtDomainCapabilitiesXML
	if err := xml.Unmarshal([]byte(xmlText), &document); err != nil {
		return false, false, false
	}
	uefi := strings.EqualFold(strings.TrimSpace(document.OS.Loader.Supported), "yes")
	secureBoot := false
	for _, enum := range document.OS.Loader.Enums {
		if strings.EqualFold(strings.TrimSpace(enum.Name), "secure") && strings.EqualFold(strings.TrimSpace(enum.Text), "yes") {
			secureBoot = true
		}
	}
	tpm := strings.EqualFold(strings.TrimSpace(document.Devices.TPM.Supported), "yes")
	return uefi, secureBoot, tpm
}

func resolveVMPlatform(platform hostPlatform, architecture, machine string, firmware domain.VMFirmware) (string, string, error) {
	if firmware == "" {
		firmware = domain.VMFirmwareBIOS
	}
	normalizedArchitecture, ok := domain.NormalizeVMArchitecture(architecture)
	if !ok {
		return "", "", fmt.Errorf("%w: unsupported VM architecture %q", domain.ErrInvalidInput, architecture)
	}
	if normalizedArchitecture == "" {
		normalizedArchitecture = platform.capabilities.Architecture
		if normalizedArchitecture == "" {
			normalizedArchitecture = domain.HostArchitectureFallback()
		}
	}
	if platform.known && len(platform.capabilities.Architectures) > 0 && !slices.Contains(platform.capabilities.Architectures, normalizedArchitecture) {
		return "", "", fmt.Errorf("%w: architecture %s is not supported by the host", domain.ErrInvalidInput, normalizedArchitecture)
	}
	if firmware == domain.VMFirmwareUEFI && platform.known && !platform.capabilities.UEFI {
		return "", "", fmt.Errorf("%w: UEFI firmware is not available on this host", domain.ErrInvalidInput)
	}
	normalizedMachine := strings.TrimSpace(machine)
	if normalizedMachine == "" {
		normalizedMachine = domain.DefaultVMMachine(normalizedArchitecture, firmware)
	}
	if normalizedMachine != "" && platform.known && len(platform.capabilities.MachineTypes) > 0 {
		resolvedMachine, supported := domain.ResolveVMMachine(normalizedMachine, platform.capabilities.MachineTypes)
		if !supported {
			return "", "", fmt.Errorf("%w: machine type %s is not supported by the host", domain.ErrInvalidInput, normalizedMachine)
		}
		normalizedMachine = resolvedMachine
	}
	return normalizedArchitecture, normalizedMachine, nil
}
