package libvirtbackend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

const (
	guestCloudInitLogBytes = 32 << 10
	guestCloudInitMaxErrs  = 50
)

type guestCloudInitStatusResponse struct {
	Status            string          `json:"status"`
	ExtendedStatus    string          `json:"extended_status"`
	BootStatusCode    string          `json:"boot_status_code"`
	Stage             string          `json:"stage"`
	Datasource        string          `json:"datasource"`
	LastUpdate        string          `json:"last_update"`
	Errors            []string        `json:"errors"`
	RecoverableErrors json.RawMessage `json:"recoverable_errors"`
}

func (c *Client) GetGuestAgentCloudInit(ctx context.Context, uuid string) (domain.GuestAgentCloudInit, error) {
	info, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.Processes
	})
	if err != nil {
		return domain.GuestAgentCloudInit{}, err
	}
	if isWindowsGuest(info.OS) {
		return domain.GuestAgentCloudInit{}, errors.Join(domain.ErrGuestAgent, errors.New("cloud-init diagnostics are currently supported only on Linux guests"))
	}

	var result domain.GuestAgentCloudInit
	err = c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		statusOutput, statusErr := runFirstGuestCommand(ctx, dom, []guestExecCommand{
			{path: "/usr/bin/cloud-init", args: []string{"status", "--long", "--format", "json"}},
			{path: "/usr/sbin/cloud-init", args: []string{"status", "--long", "--format", "json"}},
		})
		if statusErr != nil {
			statusOutput, statusErr = runFirstGuestCommand(ctx, dom, []guestExecCommand{
				{path: "/usr/bin/cloud-init", args: []string{"status", "--long"}},
				{path: "/usr/sbin/cloud-init", args: []string{"status", "--long"}},
			})
		}
		if statusErr != nil {
			result = domain.GuestAgentCloudInit{
				Installed: false, State: "unknown", Errors: []string{boundedGuestAgentError(statusErr)},
				RecoverableErrors: []string{}, LogPath: "", CheckedAt: time.Now().UTC(),
			}
		} else {
			result = parseGuestCloudInitStatus(statusOutput)
			result.Installed = true
			result.CheckedAt = time.Now().UTC()
		}
		logText, logPath := readGuestCloudInitLog(ctx, dom)
		if logText != "" {
			result.LogPath = logPath
			if len(logText) > guestCloudInitLogBytes {
				logText = logText[len(logText)-guestCloudInitLogBytes:]
				result.LogTruncated = true
			}
			result.Log = strings.ToValidUTF8(logText, "�")
		}
		return nil
	})
	if err != nil {
		return domain.GuestAgentCloudInit{}, mapGuestAgentOperationError(uuid, "cloud-init-status", err)
	}
	return result, nil
}

func readGuestCloudInitLog(ctx context.Context, dom *libvirt.Domain) (string, string) {
	for _, path := range []string{"/var/log/cloud-init-output.log", "/var/log/cloud-init.log"} {
		output, err := runFirstGuestCommand(ctx, dom, []guestExecCommand{
			{path: "/usr/bin/tail", args: []string{"-c", fmt.Sprintf("%d", guestCloudInitLogBytes), path}},
			{path: "/bin/tail", args: []string{"-c", fmt.Sprintf("%d", guestCloudInitLogBytes), path}},
		})
		if err == nil && strings.TrimSpace(output) != "" {
			return output, path
		}
	}
	output, err := runFirstGuestCommand(ctx, dom, []guestExecCommand{
		{path: "/usr/bin/journalctl", args: []string{"-u", "cloud-init", "--no-pager", "-n", "200", "-o", "short-iso"}},
		{path: "/bin/journalctl", args: []string{"-u", "cloud-init", "--no-pager", "-n", "200", "-o", "short-iso"}},
	})
	if err == nil {
		return output, "journalctl -u cloud-init"
	}
	return "", ""
}

func parseGuestCloudInitStatus(raw string) domain.GuestAgentCloudInit {
	result := domain.GuestAgentCloudInit{
		State: "unknown", Errors: []string{}, RecoverableErrors: []string{}, CheckedAt: time.Now().UTC(),
	}
	var parsed guestCloudInitStatusResponse
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed) == nil {
		result.Status = strings.TrimSpace(parsed.Status)
		result.ExtendedStatus = strings.TrimSpace(parsed.ExtendedStatus)
		result.Stage = strings.TrimSpace(parsed.Stage)
		result.Datasource = strings.TrimSpace(parsed.Datasource)
		result.LastUpdate = strings.TrimSpace(parsed.LastUpdate)
		result.Errors = boundedCloudInitErrors(parsed.Errors)
		result.RecoverableErrors = decodeCloudInitRecoverableErrors(parsed.RecoverableErrors)
		if result.Status != "" || result.ExtendedStatus != "" || parsed.BootStatusCode != "" {
			result.State = classifyGuestCloudInitState(parsed.Status, parsed.ExtendedStatus, parsed.BootStatusCode, result.Errors)
			return result
		}
	}

	recoverable := false
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "." {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			if recoverable && strings.HasPrefix(line, "-") {
				result.RecoverableErrors = append(result.RecoverableErrors, strings.TrimSpace(strings.TrimPrefix(line, "-")))
			}
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		recoverable = key == "recoverable errors"
		switch key {
		case "status":
			result.Status = value
		case "extended status", "extended_status":
			result.ExtendedStatus = value
		case "stage":
			result.Stage = value
		case "datasource":
			result.Datasource = value
		case "last update", "last_update":
			result.LastUpdate = value
		case "errors":
			if value != "" && value != "none" {
				result.Errors = append(result.Errors, value)
			}
		}
	}
	result.Errors = boundedCloudInitErrors(result.Errors)
	result.RecoverableErrors = boundedCloudInitErrors(result.RecoverableErrors)
	result.State = classifyGuestCloudInitState(result.Status, result.ExtendedStatus, "", result.Errors)
	return result
}

func classifyGuestCloudInitState(status, extended, bootCode string, failures []string) string {
	value := strings.ToLower(strings.TrimSpace(status + " " + extended + " " + bootCode))
	switch {
	case strings.Contains(value, "error") || len(failures) > 0:
		return "error"
	case strings.Contains(value, "disabled"):
		return "disabled"
	case strings.Contains(value, "running"):
		return "running"
	case strings.Contains(value, "done"):
		return "done"
	case strings.Contains(value, "not run") || strings.Contains(value, "not-run") || strings.Contains(value, "not_run"):
		return "not_run"
	default:
		return "unknown"
	}
}

func decodeCloudInitRecoverableErrors(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return []string{}
	}
	var values []string
	if json.Unmarshal(raw, &values) == nil {
		return boundedCloudInitErrors(values)
	}
	var grouped map[string][]string
	if json.Unmarshal(raw, &grouped) == nil {
		values = make([]string, 0)
		for _, key := range slices.Sorted(maps.Keys(grouped)) {
			for _, item := range grouped[key] {
				item = strings.TrimSpace(item)
				if item != "" {
					values = append(values, key+": "+item)
				}
			}
		}
		return boundedCloudInitErrors(values)
	}
	var single map[string]string
	if json.Unmarshal(raw, &single) == nil {
		keys := slices.Sorted(maps.Keys(single))
		values = make([]string, 0, len(keys))
		for _, key := range keys {
			if item := strings.TrimSpace(single[key]); item != "" {
				values = append(values, key+": "+item)
			}
		}
	}
	return boundedCloudInitErrors(values)
}

func boundedCloudInitErrors(values []string) []string {
	capacity := min(len(values), guestCloudInitMaxErrs)
	result := make([]string, 0, capacity)
	for _, value := range values {
		value = strings.TrimSpace(strings.ToValidUTF8(value, "�"))
		if value == "" {
			continue
		}
		if len(value) > 2048 {
			value = value[:2048]
		}
		result = append(result, value)
		if len(result) >= guestCloudInitMaxErrs {
			break
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}
