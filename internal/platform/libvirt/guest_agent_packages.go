package libvirtbackend

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

const (
	guestAgentPackageLimit    = 500
	guestAgentPackageRawBytes = 4 << 20
	guestAgentPackageTimeout  = 45 * time.Second
)

// ListGuestAgentPackages returns a read-only package inventory from Linux
// guests. It never refreshes metadata or changes packages.
func (c *Client) ListGuestAgentPackages(ctx context.Context, uuid string) (domain.GuestAgentPackageInventory, error) {
	info, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.Packages
	})
	if err != nil {
		return domain.GuestAgentPackageInventory{}, err
	}
	if isWindowsGuest(info.OS) {
		return domain.GuestAgentPackageInventory{}, errors.Join(domain.ErrGuestAgent, errors.New("package inventory is currently supported only on Linux guests"))
	}
	result := domain.GuestAgentPackageInventory{
		Items: []domain.GuestAgentPackage{}, Warnings: []string{}, CheckedAt: time.Now().UTC(),
	}
	err = c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		var listErr error
		switch strings.ToLower(strings.TrimSpace(info.OS.ID)) {
		case "debian", "ubuntu", "linuxmint", "pop", "raspbian":
			listErr = listDebianPackages(ctx, dom, &result)
		case "rhel", "fedora", "centos", "rocky", "almalinux", "ol", "opensuse", "opensuse-leap", "opensuse-tumbleweed", "sles", "sled":
			listErr = listRPMPackages(ctx, dom, info.OS.ID, &result)
		default:
			listErr = listDebianPackages(ctx, dom, &result)
			if listErr != nil {
				result = domain.GuestAgentPackageInventory{Items: []domain.GuestAgentPackage{}, Warnings: []string{}, CheckedAt: time.Now().UTC()}
				listErr = listRPMPackages(ctx, dom, info.OS.ID, &result)
			}
		}
		return listErr
	})
	if err != nil {
		return domain.GuestAgentPackageInventory{}, mapGuestAgentOperationError(uuid, "list-packages", err)
	}
	slices.SortFunc(result.Items, func(left, right domain.GuestAgentPackage) int {
		if left.Upgradable != right.Upgradable {
			if left.Upgradable {
				return -1
			}
			return 1
		}
		return cmp.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	})
	result.Total = len(result.Items)
	for _, item := range result.Items {
		if item.Upgradable {
			result.Upgradable++
		}
		if item.SecurityUpdate {
			result.SecurityUpdates++
		}
	}
	return result, nil
}

func listDebianPackages(ctx context.Context, dom *libvirt.Domain, result *domain.GuestAgentPackageInventory) error {
	output, err := runFirstGuestCommandWithTimeout(ctx, dom, guestAgentPackageTimeout, []guestExecCommand{
		{path: "/usr/bin/dpkg-query", args: []string{"-W", "-f=${binary:Package}\t${Version}\t${Architecture}\t${db:Status-Abbrev}\t${binary:Summary}\n"}},
		{path: "/bin/dpkg-query", args: []string{"-W", "-f=${binary:Package}\t${Version}\t${Architecture}\t${db:Status-Abbrev}\t${binary:Summary}\n"}},
	})
	if err != nil {
		return fmt.Errorf("query dpkg packages: %w", err)
	}
	result.Manager = "dpkg"
	output, truncated := truncateGuestText(output, guestAgentPackageRawBytes)
	if truncated {
		result.Truncated = true
		result.Warnings = append(result.Warnings, "dpkg 输出超过读取上限，已截断。")
	}
	installed := parseDpkgPackages(output)
	updates := map[string]string{}
	security := map[string]bool{}
	if upgradeOutput, upgradeErr := runFirstGuestCommandWithTimeout(ctx, dom, guestAgentPackageTimeout, []guestExecCommand{
		{path: "/usr/bin/apt-get", args: []string{"-s", "-o", "APT::Get::List-Cleanup=0", "upgrade"}},
		{path: "/bin/apt-get", args: []string{"-s", "-o", "APT::Get::List-Cleanup=0", "upgrade"}},
	}); upgradeErr == nil {
		result.Manager = "dpkg/apt"
		updates, security = parseAPTUpgrades(upgradeOutput)
	} else {
		result.Warnings = append(result.Warnings, "无法读取 apt 可升级列表；仅返回已安装软件包。")
	}
	addPackageItems(result, installed, updates, security)
	return nil
}

func listRPMPackages(ctx context.Context, dom *libvirt.Domain, osID string, result *domain.GuestAgentPackageInventory) error {
	output, err := runFirstGuestCommandWithTimeout(ctx, dom, guestAgentPackageTimeout, []guestExecCommand{
		{path: "/usr/bin/rpm", args: []string{"-qa", "--qf", "%{NAME}\t%{VERSION}-%{RELEASE}\t%{ARCH}\t%{SUMMARY}\n"}},
		{path: "/bin/rpm", args: []string{"-qa", "--qf", "%{NAME}\t%{VERSION}-%{RELEASE}\t%{ARCH}\t%{SUMMARY}\n"}},
	})
	if err != nil {
		return fmt.Errorf("query rpm packages: %w", err)
	}
	result.Manager = "rpm"
	output, truncated := truncateGuestText(output, guestAgentPackageRawBytes)
	if truncated {
		result.Truncated = true
		result.Warnings = append(result.Warnings, "rpm 输出超过读取上限，已截断。")
	}
	installed := parseRPMPackages(output)
	updates := map[string]string{}
	security := map[string]bool{}
	if strings.Contains(strings.ToLower(osID), "suse") || strings.Contains(strings.ToLower(osID), "sles") {
		if upgradeOutput, upgradeErr := runFirstGuestCommandWithTimeout(ctx, dom, guestAgentPackageTimeout, []guestExecCommand{
			{path: "/usr/bin/zypper", args: []string{"--no-refresh", "--quiet", "list-updates"}},
			{path: "/sbin/zypper", args: []string{"--no-refresh", "--quiet", "list-updates"}},
		}); upgradeErr == nil {
			result.Manager = "rpm/zypper"
			updates = parseZypperUpdates(upgradeOutput)
		} else {
			result.Warnings = append(result.Warnings, "无法读取 zypper 可升级列表；仅返回已安装软件包。")
		}
	} else {
		if upgradeOutput, upgradeErr := runFirstGuestCommandWithTimeout(ctx, dom, guestAgentPackageTimeout, []guestExecCommand{
			{path: "/usr/bin/dnf", args: []string{"-C", "-q", "list", "--upgrades"}},
			{path: "/bin/dnf", args: []string{"-C", "-q", "list", "--upgrades"}},
			{path: "/usr/bin/yum", args: []string{"-C", "-q", "list", "updates"}},
			{path: "/usr/bin/dnf5", args: []string{"-C", "-q", "list", "--upgrades"}},
		}); upgradeErr == nil {
			result.Manager = "rpm/dnf"
			updates = parseDNFUpdates(upgradeOutput)
			if securityOutput, securityErr := runFirstGuestCommandWithTimeout(ctx, dom, guestAgentPackageTimeout, []guestExecCommand{
				{path: "/usr/bin/dnf", args: []string{"-C", "-q", "updateinfo", "list", "--security"}},
				{path: "/bin/dnf", args: []string{"-C", "-q", "updateinfo", "list", "--security"}},
				{path: "/usr/bin/dnf5", args: []string{"-C", "-q", "advisory", "list", "--security"}},
			}); securityErr == nil {
				security = parseDNFSecurity(securityOutput, updates)
			}
		} else {
			result.Warnings = append(result.Warnings, "无法读取 dnf/yum 可升级列表；仅返回已安装软件包。")
		}
	}
	addPackageItems(result, installed, updates, security)
	return nil
}

func addPackageItems(result *domain.GuestAgentPackageInventory, installed []domain.GuestAgentPackage, updates map[string]string, security map[string]bool) {
	pending := make([]domain.GuestAgentPackage, 0, len(installed))
	for _, item := range installed {
		key := normalizePackageName(item.Name)
		if version, ok := updates[key]; ok {
			item.Upgradable = true
			item.UpdateVersion = version
			item.SecurityUpdate = security[key]
			pending = append(pending, item)
		}
	}
	seen := make(map[string]struct{}, len(pending))
	for _, item := range pending {
		seen[normalizePackageName(item.Name)] = struct{}{}
	}
	result.Items = append(result.Items, pending...)
	for _, item := range installed {
		if _, ok := seen[normalizePackageName(item.Name)]; ok {
			continue
		}
		if len(result.Items) >= guestAgentPackageLimit {
			result.Truncated = true
			break
		}
		result.Items = append(result.Items, item)
	}
	if len(result.Items) > guestAgentPackageLimit {
		result.Items = result.Items[:guestAgentPackageLimit]
		result.Truncated = true
	}
	if result.Truncated {
		result.Warnings = append(result.Warnings, fmt.Sprintf("软件包列表超过 %d 项，已截断显示。", guestAgentPackageLimit))
	}
}

func parseDpkgPackages(output string) []domain.GuestAgentPackage {
	result := make([]domain.GuestAgentPackage, 0)
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 5)
		if len(fields) < 4 {
			continue
		}
		status := strings.TrimSpace(fields[3])
		if !strings.HasPrefix(status, "ii") {
			continue
		}
		name := strings.TrimSpace(fields[0])
		if name == "" {
			continue
		}
		item := domain.GuestAgentPackage{Name: name, Version: strings.TrimSpace(fields[1]), Architecture: strings.TrimSpace(fields[2])}
		if len(fields) == 5 {
			item.Summary = strings.TrimSpace(fields[4])
		}
		result = append(result, item)
	}
	return result
}

func parseAPTUpgrades(output string) (map[string]string, map[string]bool) {
	updates := map[string]string{}
	security := map[string]bool{}
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "Inst" {
			continue
		}
		name := normalizePackageName(fields[1])
		version := ""
		for _, field := range fields[3:] {
			candidate := strings.TrimPrefix(field, "(")
			if candidate == "" || strings.HasPrefix(candidate, "[") {
				continue
			}
			version = strings.Trim(candidate, "()")
			break
		}
		if name == "" || version == "" {
			continue
		}
		updates[name] = version
		lower := strings.ToLower(line)
		if strings.Contains(lower, "security") || strings.Contains(lower, "-security") {
			security[name] = true
		}
	}
	return updates, security
}

func parseRPMPackages(output string) []domain.GuestAgentPackage {
	result := make([]domain.GuestAgentPackage, 0)
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 4)
		if len(fields) < 3 || strings.TrimSpace(fields[0]) == "" {
			continue
		}
		item := domain.GuestAgentPackage{Name: strings.TrimSpace(fields[0]), Version: strings.TrimSpace(fields[1]), Architecture: strings.TrimSpace(fields[2])}
		if len(fields) == 4 {
			item.Summary = strings.TrimSpace(fields[3])
		}
		result = append(result, item)
	}
	return result
}

func parseDNFUpdates(output string) map[string]string {
	updates := map[string]string{}
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "Last metadata") || strings.HasPrefix(fields[0], "Available") {
			continue
		}
		name, version := splitRPMNameVersion(fields[0], fields[1])
		if name != "" && version != "" {
			updates[name] = version
		}
	}
	return updates
}

func parseZypperUpdates(output string) map[string]string {
	updates := map[string]string{}
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Split(line, "|")
		if len(fields) < 6 || strings.TrimSpace(fields[0]) != "v" {
			continue
		}
		name := normalizePackageName(fields[2])
		version := strings.TrimSpace(fields[4])
		if name != "" && version != "" {
			updates[name] = version
		}
	}
	return updates
}

func parseDNFSecurity(output string, updates map[string]string) map[string]bool {
	result := map[string]bool{}
	lower := strings.ToLower(output)
	for name := range updates {
		if name != "" && strings.Contains(lower, strings.ToLower(name)) {
			result[name] = true
		}
	}
	return result
}

func splitRPMNameVersion(nameVersion, fallbackVersion string) (string, string) {
	nameVersion = strings.TrimSpace(nameVersion)
	version := strings.TrimSpace(fallbackVersion)
	if nameVersion == "" {
		return "", ""
	}
	if dot := strings.LastIndex(nameVersion, "."); dot > 0 {
		potentialArch := nameVersion[dot+1:]
		if len(potentialArch) <= 12 && !strings.ContainsAny(potentialArch, "0123456789") {
			nameVersion = nameVersion[:dot]
		} else if len(potentialArch) <= 12 {
			nameVersion = nameVersion[:dot]
		}
	}
	return normalizePackageName(nameVersion), version
}

func normalizePackageName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if colon := strings.LastIndex(value, ":"); colon > 0 {
		value = value[:colon]
	}
	return value
}

func packageOutputSummary(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= 160 {
		return output
	}
	output = output[:160]
	for len(output) > 0 && !utf8.ValidString(output) {
		output = output[:len(output)-1]
	}
	return output + "…"
}
