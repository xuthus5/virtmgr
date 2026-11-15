package libvirtbackend

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

const (
	guestAgentFileChunkBytes = 64 * 1024
	guestAgentMaxReadBytes   = 1 << 20
	guestAgentMaxWriteBytes  = 1 << 20
	guestAgentExecTimeout    = 10 * time.Second
)

type guestFileOpenResponse struct {
	Return int `json:"return"`
}

type guestFileSeekResponse struct {
	Return struct {
		Position uint64 `json:"position"`
	} `json:"return"`
}

type guestFileReadResponse struct {
	Return struct {
		Count  uint64 `json:"count"`
		EOF    bool   `json:"eof"`
		BufB64 string `json:"buf-b64"`
	} `json:"return"`
}

type guestFileWriteResponse struct {
	Return struct {
		Count uint64 `json:"count"`
		EOF   bool   `json:"eof"`
	} `json:"return"`
}

type guestAuthorizedKeysResponse struct {
	Return struct {
		Keys []string `json:"keys"`
	} `json:"return"`
}

type guestExecStartResponse struct {
	Return struct {
		PID int64 `json:"pid"`
	} `json:"return"`
}

type guestExecStatusResponse struct {
	Return struct {
		Exited   bool   `json:"exited"`
		ExitCode int    `json:"exitcode"`
		OutData  string `json:"out-data"`
		ErrData  string `json:"err-data"`
	} `json:"return"`
}

type guestUsersResponse struct {
	Return []struct {
		User      string `json:"user"`
		Domain    string `json:"domain"`
		LoginTime int64  `json:"login-time"`
	} `json:"return"`
}

func (c *Client) ReadGuestAgentFile(ctx context.Context, uuid string, spec domain.GuestAgentFileReadSpec) (domain.GuestAgentFileContent, error) {
	spec.Path = strings.TrimSpace(spec.Path)
	if !validGuestAgentFilePath(spec.Path) || spec.MaxBytes > guestAgentMaxReadBytes {
		return domain.GuestAgentFileContent{}, domain.ErrInvalidInput
	}
	if spec.MaxBytes == 0 {
		spec.MaxBytes = guestAgentMaxReadBytes
	}
	if _, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.FileAccess
	}); err != nil {
		return domain.GuestAgentFileContent{}, err
	}

	var result domain.GuestAgentFileContent
	err := c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		handle, err := guestAgentOpenFile(dom, spec.Path, "rb")
		if err != nil {
			return err
		}
		defer guestAgentCloseFile(dom, handle)
		if spec.Offset > 0 {
			var seek guestFileSeekResponse
			if err := guestAgentCall(dom, "guest-file-seek", map[string]any{
				"handle": handle, "offset": spec.Offset, "whence": 0,
			}, guestAgentProbeTimeoutSeconds, &seek); err != nil {
				return fmt.Errorf("seek guest file %s: %w", spec.Path, err)
			}
		}
		data := make([]byte, 0)
		remaining := spec.MaxBytes
		for remaining > 0 {
			chunkSize := remaining
			if chunkSize > guestAgentFileChunkBytes {
				chunkSize = guestAgentFileChunkBytes
			}
			var response guestFileReadResponse
			if err := guestAgentCall(dom, "guest-file-read", map[string]any{
				"handle": handle, "count": chunkSize,
			}, guestAgentProbeTimeoutSeconds, &response); err != nil {
				return fmt.Errorf("read guest file %s: %w", spec.Path, err)
			}
			chunk, err := base64.StdEncoding.DecodeString(response.Return.BufB64)
			if err != nil {
				return fmt.Errorf("decode guest file %s data: %w", spec.Path, err)
			}
			data = append(data, chunk...)
			remaining -= uint64(len(chunk))
			if response.Return.EOF || response.Return.Count == 0 || len(chunk) == 0 {
				result.EOF = true
				break
			}
		}
		sum := sha256.Sum256(data)
		result = domain.GuestAgentFileContent{
			Path: spec.Path, DataBase64: base64.StdEncoding.EncodeToString(data), Offset: spec.Offset,
			Size: uint64(len(data)), EOF: result.EOF, SHA256: hex.EncodeToString(sum[:]),
		}
		return nil
	})
	if err != nil {
		return domain.GuestAgentFileContent{}, mapGuestAgentOperationError(uuid, "read-file", err)
	}
	return result, nil
}

func (c *Client) WriteGuestAgentFile(ctx context.Context, uuid string, spec domain.GuestAgentFileWriteSpec) (domain.GuestAgentFileWriteResult, error) {
	spec.Path = strings.TrimSpace(spec.Path)
	spec.Mode = strings.ToLower(strings.TrimSpace(spec.Mode))
	if spec.Mode == "" {
		spec.Mode = "replace"
	}
	if !validGuestAgentFilePath(spec.Path) || (spec.Mode != "replace" && spec.Mode != "append") || (spec.Mode == "replace" && !spec.Overwrite) {
		return domain.GuestAgentFileWriteResult{}, domain.ErrInvalidInput
	}
	data, err := base64.StdEncoding.DecodeString(spec.DataBase64)
	if err != nil || len(data) > guestAgentMaxWriteBytes {
		return domain.GuestAgentFileWriteResult{}, domain.ErrInvalidInput
	}
	if _, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.FileAccess
	}); err != nil {
		return domain.GuestAgentFileWriteResult{}, err
	}
	openMode := "wb"
	if spec.Mode == "append" {
		openMode = "ab"
	}
	err = c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		handle, err := guestAgentOpenFile(dom, spec.Path, openMode)
		if err != nil {
			return err
		}
		defer guestAgentCloseFile(dom, handle)
		for offset := 0; offset < len(data); offset += guestAgentFileChunkBytes {
			end := offset + guestAgentFileChunkBytes
			if end > len(data) {
				end = len(data)
			}
			chunk := data[offset:end]
			var response guestFileWriteResponse
			if err := guestAgentCall(dom, "guest-file-write", map[string]any{
				"handle": handle, "buf-b64": base64.StdEncoding.EncodeToString(chunk), "count": len(chunk),
			}, guestAgentProbeTimeoutSeconds, &response); err != nil {
				return fmt.Errorf("write guest file %s: %w", spec.Path, err)
			}
			if response.Return.Count != uint64(len(chunk)) {
				return fmt.Errorf("short write to guest file %s: %d of %d bytes", spec.Path, response.Return.Count, len(chunk))
			}
		}
		return nil
	})
	if err != nil {
		return domain.GuestAgentFileWriteResult{}, mapGuestAgentOperationError(uuid, "write-file", err)
	}
	sum := sha256.Sum256(data)
	return domain.GuestAgentFileWriteResult{
		Path: spec.Path, BytesWritten: uint64(len(data)), SHA256: hex.EncodeToString(sum[:]),
	}, nil
}

func (c *Client) ResetGuestAgentPassword(ctx context.Context, uuid string, spec domain.GuestAgentPasswordSpec) error {
	spec.Username = strings.TrimSpace(spec.Username)
	if !domain.ValidGuestAgentUsername(spec.Username) || len(spec.Password) == 0 || len(spec.Password) > 256 {
		return domain.ErrInvalidInput
	}
	if _, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.PasswordReset
	}); err != nil {
		return err
	}
	err := c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		var response struct{}
		return guestAgentCall(dom, "guest-set-user-password", map[string]any{
			"username": spec.Username, "password": spec.Password, "crypted": false,
		}, guestAgentProbeTimeoutSeconds, &response)
	})
	if err != nil {
		return mapGuestAgentOperationError(uuid, "reset-password", err)
	}
	return nil
}

func (c *Client) ListGuestAgentAuthorizedKeys(ctx context.Context, uuid, username string) (domain.GuestAgentAuthorizedKeys, error) {
	username = strings.TrimSpace(username)
	if !domain.ValidGuestAgentUsername(username) {
		return domain.GuestAgentAuthorizedKeys{}, domain.ErrInvalidInput
	}
	if _, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.SSHKeys
	}); err != nil {
		return domain.GuestAgentAuthorizedKeys{}, err
	}
	var result domain.GuestAgentAuthorizedKeys
	err := c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		var response guestAuthorizedKeysResponse
		if err := guestAgentCall(dom, "guest-ssh-get-authorized-keys", map[string]any{
			"username": username,
		}, guestAgentProbeTimeoutSeconds, &response); err != nil {
			return err
		}
		result = domain.GuestAgentAuthorizedKeys{Username: username, Keys: append([]string(nil), response.Return.Keys...)}
		return nil
	})
	if err != nil {
		return domain.GuestAgentAuthorizedKeys{}, mapGuestAgentOperationError(uuid, "list-ssh-keys", err)
	}
	return result, nil
}

func (c *Client) AddGuestAgentAuthorizedKeys(ctx context.Context, uuid string, spec domain.GuestAgentSSHKeySpec) error {
	return c.changeGuestAgentAuthorizedKeys(ctx, uuid, spec, "guest-ssh-add-authorized-keys", "add-ssh-keys")
}

func (c *Client) RemoveGuestAgentAuthorizedKeys(ctx context.Context, uuid string, spec domain.GuestAgentSSHKeySpec) error {
	return c.changeGuestAgentAuthorizedKeys(ctx, uuid, spec, "guest-ssh-remove-authorized-keys", "remove-ssh-keys")
}

func (c *Client) changeGuestAgentAuthorizedKeys(ctx context.Context, uuid string, spec domain.GuestAgentSSHKeySpec, command, action string) error {
	spec.Username = strings.TrimSpace(spec.Username)
	if !domain.ValidGuestAgentUsername(spec.Username) || len(spec.Keys) == 0 || len(spec.Keys) > 16 {
		return domain.ErrInvalidInput
	}
	if _, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.SSHKeys
	}); err != nil {
		return err
	}
	err := c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		var response struct{}
		return guestAgentCall(dom, command, map[string]any{"username": spec.Username, "keys": spec.Keys}, guestAgentProbeTimeoutSeconds, &response)
	})
	if err != nil {
		return mapGuestAgentOperationError(uuid, action, err)
	}
	return nil
}

func (c *Client) ListGuestAgentProcesses(ctx context.Context, uuid string) ([]domain.GuestAgentProcess, error) {
	info, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.Processes
	})
	if err != nil {
		return nil, err
	}
	if isWindowsGuest(info.OS) {
		return nil, errors.Join(domain.ErrGuestAgent, errors.New("process management is currently supported only on Linux guests"))
	}
	var output string
	err = c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		var execErr error
		output, execErr = runFirstGuestCommand(dom, []guestExecCommand{
			{path: "/bin/ps", args: []string{"-eo", "pid=,ppid=,user=,stat=,comm=,args="}},
			{path: "/usr/bin/ps", args: []string{"-eo", "pid=,ppid=,user=,stat=,comm=,args="}},
		})
		return execErr
	})
	if err != nil {
		return nil, mapGuestAgentOperationError(uuid, "list-processes", err)
	}
	processes := parseGuestProcesses(output)
	if len(processes) > 500 {
		processes = processes[:500]
	}
	return processes, nil
}

func (c *Client) SignalGuestAgentProcess(ctx context.Context, uuid string, spec domain.GuestAgentProcessSignalSpec) error {
	signal, ok := domain.ValidGuestAgentProcessSignal(spec.Signal)
	if !ok || spec.PID <= 1 {
		return domain.ErrInvalidInput
	}
	info, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.Processes
	})
	if err != nil {
		return err
	}
	if isWindowsGuest(info.OS) {
		return errors.Join(domain.ErrGuestAgent, errors.New("process management is currently supported only on Linux guests"))
	}
	signalFlag := "-TERM"
	if signal == "kill" {
		signalFlag = "-KILL"
	}
	pid := strconv.FormatInt(spec.PID, 10)
	err = c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		_, execErr := runFirstGuestCommand(dom, []guestExecCommand{
			{path: "/bin/kill", args: []string{signalFlag, pid}},
			{path: "/usr/bin/kill", args: []string{signalFlag, pid}},
		})
		return execErr
	})
	if err != nil {
		return mapGuestAgentOperationError(uuid, "signal-process", err)
	}
	return nil
}

func (c *Client) ListGuestAgentUsers(ctx context.Context, uuid string) ([]domain.GuestAgentUser, error) {
	if _, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.Users
	}); err != nil {
		return nil, err
	}
	var users []domain.GuestAgentUser
	err := c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		var response guestUsersResponse
		if err := guestAgentCall(dom, "guest-get-users", nil, guestAgentProbeTimeoutSeconds, &response); err != nil {
			return err
		}
		for _, user := range response.Return {
			username := strings.TrimSpace(user.User)
			if username == "" {
				continue
			}
			users = append(users, domain.GuestAgentUser{
				Username: username, Domain: strings.TrimSpace(user.Domain), LoginTime: user.LoginTime,
			})
		}
		return nil
	})
	if err != nil {
		return nil, mapGuestAgentOperationError(uuid, "list-users", err)
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].LoginTime != users[j].LoginTime {
			return users[i].LoginTime > users[j].LoginTime
		}
		return users[i].Username < users[j].Username
	})
	return users, nil
}

func (c *Client) requireGuestAgentCapability(ctx context.Context, uuid string, allowed func(domain.GuestAgentCapabilities) bool) (domain.GuestAgentInfo, error) {
	info, err := c.GetGuestAgentInfo(ctx, uuid, true)
	if err != nil {
		return domain.GuestAgentInfo{}, err
	}
	if info.State != domain.GuestAgentStateConnected || !allowed(info.Capabilities) {
		return info, domain.ErrGuestAgent
	}
	return info, nil
}

func (c *Client) withGuestAgentDomain(ctx context.Context, uuid string, operation func(*libvirt.Domain) error) error {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return domain.ErrInvalidInput
	}
	c.guestMu.Lock()
	defer c.guestMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.ensureGuestConnectionLocked(); err != nil {
		return err
	}
	dom, err := c.guestConn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	state, _, err := dom.GetState()
	if err != nil {
		return fmt.Errorf("read domain %s state for guest agent operation: %w", uuid, err)
	}
	if state != libvirt.DOMAIN_RUNNING && state != libvirt.DOMAIN_BLOCKED {
		return errors.Join(domain.ErrGuestAgent, errors.New("guest agent operations require a running domain"))
	}
	return operation(dom)
}

func guestAgentCall(dom *libvirt.Domain, command string, arguments any, timeoutSeconds int, target any) error {
	raw, err := guestAgentCommand(dom, command, arguments, timeoutSeconds)
	if err != nil {
		return err
	}
	return decodeGuestAgentResponse(raw, target)
}

func guestAgentOpenFile(dom *libvirt.Domain, path, mode string) (int, error) {
	var response guestFileOpenResponse
	if err := guestAgentCall(dom, "guest-file-open", map[string]any{"path": path, "mode": mode}, guestAgentProbeTimeoutSeconds, &response); err != nil {
		return 0, fmt.Errorf("open guest file %s: %w", path, err)
	}
	if response.Return < 0 {
		return 0, fmt.Errorf("open guest file %s: invalid handle", path)
	}
	return response.Return, nil
}

func guestAgentCloseFile(dom *libvirt.Domain, handle int) {
	var response struct{}
	_ = guestAgentCall(dom, "guest-file-close", map[string]any{"handle": handle}, guestAgentProbeTimeoutSeconds, &response)
}

func validGuestAgentFilePath(path string) bool {
	return path != "" && len(path) <= 4096 && filepath.IsAbs(path) && !strings.ContainsRune(path, '\x00')
}

type guestExecCommand struct {
	path string
	args []string
}

func runFirstGuestCommand(dom *libvirt.Domain, commands []guestExecCommand) (string, error) {
	var failures []error
	for _, command := range commands {
		output, err := runGuestCommand(dom, command.path, command.args)
		if err == nil {
			return output, nil
		}
		failures = append(failures, err)
	}
	return "", errors.Join(failures...)
}

func runGuestCommand(dom *libvirt.Domain, path string, args []string) (string, error) {
	var started guestExecStartResponse
	if err := guestAgentCall(dom, "guest-exec", map[string]any{
		"path": path, "arg": args, "capture-output": true,
	}, guestAgentProbeTimeoutSeconds, &started); err != nil {
		return "", err
	}
	if started.Return.PID <= 0 {
		return "", errors.New("guest command did not return a pid")
	}
	deadline := time.Now().Add(guestAgentExecTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		var status guestExecStatusResponse
		if err := guestAgentCall(dom, "guest-exec-status", map[string]any{"pid": started.Return.PID}, guestAgentProbeTimeoutSeconds, &status); err != nil {
			return "", err
		}
		if !status.Return.Exited {
			continue
		}
		stdout, _ := base64.StdEncoding.DecodeString(status.Return.OutData)
		stderr, _ := base64.StdEncoding.DecodeString(status.Return.ErrData)
		if status.Return.ExitCode != 0 {
			message := strings.TrimSpace(string(stderr))
			if message == "" {
				message = strings.TrimSpace(string(stdout))
			}
			if message == "" {
				message = fmt.Sprintf("guest command exited with status %d", status.Return.ExitCode)
			}
			return "", errors.New(message)
		}
		return string(stdout), nil
	}
	return "", fmt.Errorf("guest command %s timed out", path)
}

func parseGuestProcesses(output string) []domain.GuestAgentProcess {
	result := make([]domain.GuestAgentProcess, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 5 {
			continue
		}
		pid, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil || pid <= 0 {
			continue
		}
		ppid, _ := strconv.ParseInt(fields[1], 10, 64)
		args := ""
		if len(fields) > 5 {
			args = strings.Join(fields[5:], " ")
		}
		result = append(result, domain.GuestAgentProcess{
			PID: pid, PPID: ppid, User: fields[2], State: fields[3], Command: fields[4], Args: args,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PID < result[j].PID })
	return result
}

func mapGuestAgentOperationError(uuid, action string, err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidInput), errors.Is(err, domain.ErrNotFound):
		return err
	case errors.Is(err, domain.ErrGuestAgent):
		return err
	default:
		return errors.Join(domain.ErrGuestAgent, fmt.Errorf("guest agent %s on domain %s: %w", action, uuid, err))
	}
}
