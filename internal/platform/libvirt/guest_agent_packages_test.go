package libvirtbackend

import (
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

func TestParsePackagesFromSupportedManagers(t *testing.T) {
	dpkg := parseDpkgPackages("openssl\t3.0.13\tamd64\tii \tSecure Sockets Layer toolkit\nremoved\t1.0\tamd64\trc \tremoved\n")
	if len(dpkg) != 1 || dpkg[0].Name != "openssl" || dpkg[0].Summary != "Secure Sockets Layer toolkit" {
		t.Fatalf("dpkg packages = %+v", dpkg)
	}
	rpm := parseRPMPackages("openssl\t3.0.13-1\tx86_64\tSecure Sockets Layer toolkit\nqemu-guest-agent\t8.2\tx86_64\tQEMU Guest Agent\n")
	if len(rpm) != 2 || rpm[0].Name != "openssl" || rpm[1].Name != "qemu-guest-agent" {
		t.Fatalf("rpm packages = %+v", rpm)
	}
	updates, security := parseAPTUpgrades("Inst openssl [3.0.12] (3.0.13 Ubuntu:24.04/jammy-security [amd64])\nInst curl [1.0] (1.1 Ubuntu:24.04/jammy-updates [amd64])\n")
	if updates["openssl"] != "3.0.13" || !security["openssl"] || security["curl"] {
		t.Fatalf("apt updates = %+v security = %+v", updates, security)
	}
	if updates := parseDNFUpdates("openssl.x86_64 1:3.0.13-1 updates\ncurl.x86_64 8.0-1 baseos\n"); updates["openssl"] != "1:3.0.13-1" || updates["curl"] != "8.0-1" {
		t.Fatalf("dnf updates = %+v", updates)
	}
	if updates := parseZypperUpdates("v | package | openssl | 3.0.12 | 3.0.13 | repo\n"); updates["openssl"] != "3.0.13" {
		t.Fatalf("zypper updates = %+v", updates)
	}
}

func TestAddPackageItemsPrioritizesUpgradesAndTruncates(t *testing.T) {
	installed := make([]domain.GuestAgentPackage, 0, guestAgentPackageLimit+10)
	for index := 0; index < guestAgentPackageLimit+10; index++ {
		installed = append(installed, domain.GuestAgentPackage{Name: "pkg-" + strings.Repeat("x", 1) + string(rune('a'+index%26)) + "-" + string(rune('a'+index/26)), Version: "1"})
	}
	installed[0].Name = "security-update"
	result := domain.GuestAgentPackageInventory{Items: []domain.GuestAgentPackage{}, Warnings: []string{}}
	addPackageItems(&result, installed, map[string]string{"security-update": "2"}, map[string]bool{"security-update": true})
	if !result.Truncated || len(result.Items) != guestAgentPackageLimit {
		t.Fatalf("items = %d truncated = %v", len(result.Items), result.Truncated)
	}
	if result.Items[0].Name != "security-update" || !result.Items[0].Upgradable || !result.Items[0].SecurityUpdate || result.Items[0].UpdateVersion != "2" {
		t.Fatalf("first package = %+v", result.Items[0])
	}
}

func TestGuestAgentPackagesCapabilityRequiresExecAndOSInfo(t *testing.T) {
	var parsed guestInfoResponse
	if err := decodeGuestAgentResponse(`{"return":{"supported_commands":[
  {"name":"guest-exec","enabled":true},{"name":"guest-exec-status","enabled":true},
  {"name":"guest-get-osinfo","enabled":true}
]}}`, &parsed); err != nil {
		t.Fatal(err)
	}
	capabilities := guestAgentCapabilities(parsed)
	if !capabilities.Packages || !capabilities.Processes || !capabilities.OSInfo {
		t.Fatalf("capabilities = %+v", capabilities)
	}
}
