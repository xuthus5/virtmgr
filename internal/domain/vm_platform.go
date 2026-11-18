package domain

import (
	"runtime"
	"slices"
	"strings"
)

func NormalizeVMArchitecture(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", true
	case "x86_64", "amd64", "x86-64", "x64":
		return "x86_64", true
	case "aarch64", "arm64":
		return "aarch64", true
	case "i386", "i486", "i586", "i686", "x86":
		return "i686", true
	case "ppc64le":
		return "ppc64le", true
	case "ppc64":
		return "ppc64", true
	case "s390x":
		return "s390x", true
	case "riscv64":
		return "riscv64", true
	default:
		return "", false
	}
}

func HostArchitectureFallback() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "ppc64le", "ppc64", "s390x", "riscv64":
		return runtime.GOARCH
	default:
		return runtime.GOARCH
	}
}

func DefaultVMMachine(architecture string, firmware VMFirmware) string {
	architecture, _ = NormalizeVMArchitecture(architecture)
	switch architecture {
	case "x86_64", "i686":
		if firmware == VMFirmwareUEFI {
			return "q35"
		}
		return ""
	case "", "aarch64", "ppc64le", "ppc64", "s390x", "riscv64":
		if architecture == "" {
			return ""
		}
		if slices.Contains([]string{"aarch64", "ppc64le", "ppc64", "s390x", "riscv64"}, architecture) {
			return "virt"
		}
	}
	return ""
}

func VMMachineSupported(requested string, machines []string) bool {
	_, supported := ResolveVMMachine(requested, machines)
	return supported
}

func ResolveVMMachine(requested string, machines []string) (string, bool) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", true
	}
	bestAlias := ""
	for _, machine := range machines {
		machine = strings.TrimSpace(machine)
		if machine == requested {
			return machine, true
		}
		matched := false
		switch requested {
		case "q35":
			if strings.HasPrefix(machine, "pc-q35-") || strings.HasPrefix(machine, "q35-") {
				matched = true
			}
		case "pc", "i440fx":
			if strings.HasPrefix(machine, "pc-i440fx-") {
				matched = true
			}
		case "virt":
			if strings.HasPrefix(machine, "virt-") {
				matched = true
			}
		}
		if matched && machine > bestAlias {
			bestAlias = machine
		}
	}
	if bestAlias != "" {
		return bestAlias, true
	}
	return "", false
}

func VMArchitecturesCompatible(hostArchitecture, imageArchitecture string) bool {
	hostArchitecture, _ = NormalizeVMArchitecture(hostArchitecture)
	imageArchitecture, _ = NormalizeVMArchitecture(imageArchitecture)
	return hostArchitecture == "" || imageArchitecture == "" || hostArchitecture == imageArchitecture
}
