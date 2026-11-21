package libvirtbackend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"

	"virtmgr/internal/domain"
)

const portForwardTable = "virtmgr_pf"

// ApplyPortForwards atomically rebuilds the single firewall table owned
// by VirtMgr. The caller must validate guest addresses and persist the catalog
// only after this method succeeds.
func (c *Client) ApplyPortForwards(ctx context.Context, forwards []domain.PortForward) error {
	path, err := exec.LookPath("nft")
	if err != nil {
		return fmt.Errorf("%w: nftables is required for inbound port forwarding", domain.ErrUnavailable)
	}
	enabled := make([]domain.PortForward, 0, len(forwards))
	for _, item := range forwards {
		if item.Enabled {
			enabled = append(enabled, item)
		}
	}
	if len(enabled) == 0 {
		output, deleteErr := commandOutput(ctx, path, "delete", "table", "inet", portForwardTable)
		if deleteErr != nil && !missingPortForwardTable(output+" "+deleteErr.Error()) {
			return fmt.Errorf("remove port forwarding table: %w", deleteErr)
		}
		return nil
	}
	ruleset, err := buildPortForwardRuleset(enabled)
	if err != nil {
		return err
	}
	_, listErr := commandOutput(ctx, path, "list", "table", "inet", portForwardTable)
	switch {
	case listErr == nil:
		ruleset = "delete table inet " + portForwardTable + "\n" + ruleset
	case missingPortForwardTable(listErr.Error()):
	default:
		return fmt.Errorf("inspect port forwarding table: %w", listErr)
	}
	command := exec.CommandContext(ctx, path, "-f", "-")
	command.Stdin = strings.NewReader(ruleset)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("apply port forwarding rules: %s", message)
	}
	return nil
}

func missingPortForwardTable(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "no such file or directory") || strings.Contains(lower, "does not exist")
}

func buildPortForwardRuleset(forwards []domain.PortForward) (string, error) {
	var builder strings.Builder
	builder.WriteString("table inet " + portForwardTable + " {\n")
	builder.WriteString("  chain prerouting {\n")
	builder.WriteString("    type nat hook prerouting priority dstnat; policy accept;\n")
	for _, item := range forwards {
		protocol := string(item.Protocol)
		if protocol != "tcp" && protocol != "udp" {
			return "", fmt.Errorf("%w: invalid port forwarding protocol", domain.ErrInvalidInput)
		}
		hostAddress := strings.TrimSpace(item.HostAddress)
		guest, err := netip.ParseAddr(strings.TrimSpace(item.GuestAddress))
		if err != nil || !guest.IsValid() || item.HostPort == 0 || item.GuestPort == 0 {
			return "", fmt.Errorf("%w: invalid port forwarding address or port", domain.ErrInvalidInput)
		}
		if hostAddress != "" {
			address, err := netip.ParseAddr(hostAddress)
			if err != nil || address.IsUnspecified() || address.IsMulticast() || address.IsLoopback() {
				return "", fmt.Errorf("%w: invalid host address", domain.ErrInvalidInput)
			}
			if address.Is4() {
				builder.WriteString("    ip daddr " + address.String())
			} else {
				builder.WriteString("    ip6 daddr " + address.String())
			}
		} else if guest.Is4() {
			builder.WriteString("    meta nfproto ipv4")
		} else {
			builder.WriteString("    meta nfproto ipv6")
		}
		builder.WriteString(" " + protocol + " dport " + strconv.Itoa(int(item.HostPort)) + " dnat to ")
		if guest.Is6() {
			builder.WriteString("[")
			builder.WriteString(guest.String())
			builder.WriteString("]:")
		} else {
			builder.WriteString(guest.String())
			builder.WriteString(":")
		}
		builder.WriteString(strconv.Itoa(int(item.GuestPort)))
		builder.WriteString(portForwardComment(item.Network))
		builder.WriteString("\n")
	}
	builder.WriteString("  }\n")
	builder.WriteString("  chain forward {\n")
	builder.WriteString("    type filter hook forward priority filter; policy accept;\n")
	for _, item := range forwards {
		protocol := string(item.Protocol)
		guest, err := netip.ParseAddr(strings.TrimSpace(item.GuestAddress))
		if err != nil || !guest.IsValid() || item.GuestPort == 0 {
			return "", fmt.Errorf("%w: invalid guest address or port", domain.ErrInvalidInput)
		}
		if guest.Is4() {
			builder.WriteString("    ip daddr " + guest.String())
		} else {
			builder.WriteString("    ip6 daddr " + guest.String())
		}
		builder.WriteString(" " + protocol + " dport " + strconv.Itoa(int(item.GuestPort)) + " accept")
		builder.WriteString(portForwardComment(item.Network))
		builder.WriteString("\n")
	}
	builder.WriteString("  }\n")
	builder.WriteString("}\n")
	return builder.String(), nil
}

func portForwardComment(network string) string {
	network = strings.TrimSpace(network)
	if network == "" {
		return ""
	}
	return ` comment "virtmgr-pf:` + network + `"`
}

type portForwardInspection struct {
	count int
	ready bool
	error string
}

func inspectPortForwardFirewall(ctx context.Context, network string) portForwardInspection {
	path, err := exec.LookPath("nft")
	if err != nil {
		return portForwardInspection{error: "未找到 nftables"}
	}
	output, commandErr := commandOutput(ctx, path, "list", "table", "inet", portForwardTable)
	if commandErr != nil {
		if missingPortForwardTable(commandErr.Error()) {
			return portForwardInspection{ready: true}
		}
		return portForwardInspection{error: commandErr.Error()}
	}
	count := 0
	marker := `comment "virtmgr-pf:` + strings.TrimSpace(network) + `"`
	for line := range strings.SplitSeq(output, "\n") {
		if strings.Contains(line, " dnat to ") && strings.Contains(line, marker) {
			count++
		}
	}
	return portForwardInspection{count: count, ready: true}
}

func applyOutputError(output []byte, err error) error {
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(bytes.TrimSpace(output)))
	if message == "" {
		message = err.Error()
	}
	return errors.New(message)
}
