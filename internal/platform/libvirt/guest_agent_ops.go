package libvirtbackend

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

const (
	guestAgentFileChunkBytes  = 64 * 1024
	guestAgentMaxReadBytes    = 1 << 20
	guestAgentMaxWriteBytes   = 1 << 20
	guestAgentExecTimeout     = 10 * time.Second
	guestAgentResizeTimeout   = 2 * time.Minute
	guestAgentDiagnosticBytes = 24 << 10
	guestAgentDiagnosticLines = 80
)

var (
	guestDevicePathPattern  = regexp.MustCompile(`^/dev/[A-Za-z0-9._/-]+$`)
	guestDeviceNamePattern  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	guestServiceUnitPattern = regexp.MustCompile(`^[A-Za-z0-9@_.:-]{1,128}\.service$`)
	errGuestCommandTimeout  = errors.New("guest command timed out")
)

type guestCommandTimeoutError struct {
	path    string
	timeout time.Duration
}

func (e *guestCommandTimeoutError) Error() string {
	return fmt.Sprintf("guest command %s timed out after %s", e.path, e.timeout)
}

func (e *guestCommandTimeoutError) Unwrap() error { return errGuestCommandTimeout }

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
	for index := range spec.Keys {
		spec.Keys[index] = strings.TrimSpace(spec.Keys[index])
		if _, valid := domain.ParseSSHAuthorizedKey(spec.Keys[index]); !valid {
			return domain.ErrInvalidInput
		}
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
		output, execErr = runFirstGuestCommand(ctx, dom, []guestExecCommand{
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

func (c *Client) ListGuestAgentServices(ctx context.Context, uuid string) ([]domain.GuestAgentService, error) {
	info, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.Processes
	})
	if err != nil {
		return nil, err
	}
	if isWindowsGuest(info.OS) {
		return nil, errors.Join(domain.ErrGuestAgent, errors.New("service diagnostics are currently supported only on Linux guests"))
	}
	var output string
	err = c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		var execErr error
		output, execErr = runFirstGuestCommand(ctx, dom, []guestExecCommand{
			{path: "/bin/systemctl", args: []string{"list-units", "--type=service", "--all", "--no-legend", "--no-pager", "--plain"}},
			{path: "/usr/bin/systemctl", args: []string{"list-units", "--type=service", "--all", "--no-legend", "--no-pager", "--plain"}},
		})
		return execErr
	})
	if err != nil {
		return nil, mapGuestAgentOperationError(uuid, "list-services", err)
	}
	services := parseGuestServices(output)
	if len(services) > 500 {
		services = services[:500]
	}
	return services, nil
}

func (c *Client) GetGuestAgentDiagnostics(ctx context.Context, uuid string) (domain.GuestAgentDiagnostics, error) {
	info, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.Processes
	})
	if err != nil {
		return domain.GuestAgentDiagnostics{}, err
	}
	if isWindowsGuest(info.OS) {
		return domain.GuestAgentDiagnostics{}, errors.Join(domain.ErrGuestAgent, errors.New("boot diagnostics are currently supported only on Linux guests"))
	}
	result := domain.GuestAgentDiagnostics{
		CheckedAt: time.Now().UTC(), FailedServices: []domain.GuestAgentService{},
		ServiceLogs: []domain.GuestAgentServiceLog{}, KernelWarnings: []string{},
		SystemErrors: []string{}, OOMEvents: []string{}, Recommendations: []string{}, CommandErrors: []string{},
	}
	err = c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		failedOutput, failedErr := runFirstGuestCommand(ctx, dom, []guestExecCommand{
			{path: "/bin/systemctl", args: []string{"list-units", "--type=service", "--state=failed", "--no-legend", "--no-pager", "--plain"}},
			{path: "/usr/bin/systemctl", args: []string{"list-units", "--type=service", "--state=failed", "--no-legend", "--no-pager", "--plain"}},
		})
		if failedErr != nil {
			result.CommandErrors = append(result.CommandErrors, "无法读取失败服务: "+boundedGuestAgentError(failedErr))
		} else {
			result.FailedServices = parseGuestServices(failedOutput)
			if len(result.FailedServices) > 20 {
				result.FailedServices = result.FailedServices[:20]
			}
		}

		kernelOutput, kernelErr := runFirstGuestCommand(ctx, dom, []guestExecCommand{
			{path: "/usr/bin/journalctl", args: []string{"-k", "-b", "-p", "warning", "--no-pager", "-n", "80", "-o", "short-iso"}},
			{path: "/bin/journalctl", args: []string{"-k", "-b", "-p", "warning", "--no-pager", "-n", "80", "-o", "short-iso"}},
		})
		if kernelErr != nil {
			result.CommandErrors = append(result.CommandErrors, "无法读取内核日志: "+boundedGuestAgentError(kernelErr))
		} else {
			result.KernelWarnings = boundedDiagnosticLines(kernelOutput, guestAgentDiagnosticLines)
		}

		systemOutput, systemErr := runFirstGuestCommand(ctx, dom, []guestExecCommand{
			{path: "/usr/bin/journalctl", args: []string{"-b", "-p", "err", "--no-pager", "-n", "80", "-o", "short-iso"}},
			{path: "/bin/journalctl", args: []string{"-b", "-p", "err", "--no-pager", "-n", "80", "-o", "short-iso"}},
		})
		if systemErr != nil {
			result.CommandErrors = append(result.CommandErrors, "无法读取系统错误日志: "+boundedGuestAgentError(systemErr))
		} else {
			result.SystemErrors = boundedDiagnosticLines(systemOutput, guestAgentDiagnosticLines)
		}
		for _, line := range append(append([]string{}, result.KernelWarnings...), result.SystemErrors...) {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "out of memory") || strings.Contains(lower, "oom-kill") || strings.Contains(lower, "invoked oom-killer") {
				result.OOMEvents = append(result.OOMEvents, line)
				if len(result.OOMEvents) >= 20 {
					break
				}
			}
		}
		for index, service := range result.FailedServices {
			if index >= 5 {
				break
			}
			if !guestServiceUnitPattern.MatchString(service.Unit) {
				continue
			}
			logOutput, logErr := runFirstGuestCommand(ctx, dom, []guestExecCommand{
				{path: "/usr/bin/journalctl", args: []string{"-u", service.Unit, "-b", "--no-pager", "-n", "30", "-o", "short-iso"}},
				{path: "/bin/journalctl", args: []string{"-u", service.Unit, "-b", "--no-pager", "-n", "30", "-o", "short-iso"}},
			})
			if logErr != nil {
				result.CommandErrors = append(result.CommandErrors, "无法读取 "+service.Unit+" 日志: "+boundedGuestAgentError(logErr))
				continue
			}
			content, truncated := truncateGuestDiagnostic(logOutput)
			result.ServiceLogs = append(result.ServiceLogs, domain.GuestAgentServiceLog{Unit: service.Unit, Content: content, Truncated: truncated})
		}
		return nil
	})
	if err != nil {
		return domain.GuestAgentDiagnostics{}, mapGuestAgentOperationError(uuid, "boot-diagnostics", err)
	}
	result.Recommendations = guestDiagnosticRecommendations(result)
	return result, nil
}

func (c *Client) GrowGuestAgentFilesystem(ctx context.Context, uuid string, spec domain.GuestAgentFilesystemResizeSpec) (domain.GuestAgentFilesystemResizeResult, error) {
	spec.MountPoint = strings.TrimSpace(spec.MountPoint)
	if spec.MountPoint == "" || !filepath.IsAbs(spec.MountPoint) || strings.ContainsRune(spec.MountPoint, '\x00') || len(spec.MountPoint) > 4096 {
		return domain.GuestAgentFilesystemResizeResult{}, domain.ErrInvalidInput
	}
	info, err := c.requireGuestAgentCapability(ctx, uuid, func(capabilities domain.GuestAgentCapabilities) bool {
		return capabilities.FilesystemResize
	})
	if err != nil {
		return domain.GuestAgentFilesystemResizeResult{}, err
	}
	if isWindowsGuest(info.OS) {
		return domain.GuestAgentFilesystemResizeResult{}, errors.Join(domain.ErrGuestAgent, errors.New("online filesystem growth is currently supported only on Linux guests"))
	}
	var filesystem *domain.GuestAgentFilesystem
	for index := range info.Filesystems {
		if info.Filesystems[index].MountPoint == spec.MountPoint {
			filesystem = &info.Filesystems[index]
			break
		}
	}
	if filesystem == nil {
		return domain.GuestAgentFilesystemResizeResult{}, domain.ErrNotFound
	}
	fsType := strings.ToLower(strings.TrimSpace(filesystem.Type))
	if fsType != "ext4" && fsType != "xfs" {
		return domain.GuestAgentFilesystemResizeResult{}, fmt.Errorf("%w: only ext4 and xfs filesystems can be grown online", domain.ErrActionNotAllowed)
	}
	result := domain.GuestAgentFilesystemResizeResult{
		MountPoint: spec.MountPoint, Device: filesystem.Device, Filesystem: fsType,
		PreviousTotalBytes: filesystem.TotalBytes, TotalBytes: filesystem.TotalBytes, FreeBytes: filesystem.FreeBytes,
	}
	err = c.withGuestAgentDomain(ctx, uuid, func(dom *libvirt.Domain) error {
		mountOutput, err := runFirstGuestCommandWithTimeout(ctx, dom, guestAgentExecTimeout, []guestExecCommand{
			{path: "/usr/bin/findmnt", args: []string{"-n", "-o", "SOURCE,FSTYPE", "--target", spec.MountPoint}},
			{path: "/bin/findmnt", args: []string{"-n", "-o", "SOURCE,FSTYPE", "--target", spec.MountPoint}},
		})
		if err != nil {
			return fmt.Errorf("inspect mounted filesystem: %w", err)
		}
		fields := strings.Fields(strings.TrimSpace(mountOutput))
		if len(fields) < 2 {
			return errors.New("findmnt returned an unexpected response")
		}
		source := strings.TrimSpace(fields[0])
		actualType := strings.ToLower(strings.TrimSpace(fields[1]))
		if !guestDevicePathPattern.MatchString(source) || strings.Contains(source, "..") {
			return fmt.Errorf("%w: filesystem source %q is not a directly resizable block device", domain.ErrActionNotAllowed, source)
		}
		if actualType != fsType {
			return fmt.Errorf("%w: filesystem type changed from %s to %s", domain.ErrConflict, fsType, actualType)
		}
		parentOutput, err := runFirstGuestCommandWithTimeout(ctx, dom, guestAgentExecTimeout, []guestExecCommand{
			{path: "/usr/bin/lsblk", args: []string{"-n", "-o", "PKNAME", source}},
			{path: "/bin/lsblk", args: []string{"-n", "-o", "PKNAME", source}},
		})
		if err != nil {
			return fmt.Errorf("inspect parent block device: %w", err)
		}
		parent := strings.TrimSpace(parentOutput)
		if !guestDeviceNamePattern.MatchString(parent) {
			return fmt.Errorf("%w: root or LVM filesystems cannot be grown through this operation", domain.ErrActionNotAllowed)
		}
		partitionOutput, err := runFirstGuestCommandWithTimeout(ctx, dom, guestAgentExecTimeout, []guestExecCommand{
			{path: "/usr/bin/lsblk", args: []string{"-n", "-o", "PARTN", source}},
			{path: "/bin/lsblk", args: []string{"-n", "-o", "PARTN", source}},
		})
		if err != nil {
			return fmt.Errorf("inspect partition number: %w", err)
		}
		partition := strings.TrimSpace(partitionOutput)
		if partition == "" {
			return fmt.Errorf("%w: filesystem is not backed by a partitioned block device", domain.ErrActionNotAllowed)
		}
		if len(partition) > 3 || strings.Trim(partition, "0123456789") != "" || partition == "0" {
			return errors.New("lsblk returned an invalid partition number")
		}
		parentPath := "/dev/" + parent
		if _, err := runFirstGuestCommandWithTimeout(ctx, dom, guestAgentResizeTimeout, []guestExecCommand{
			{path: "/usr/bin/growpart", args: []string{parentPath, partition}},
			{path: "/sbin/growpart", args: []string{parentPath, partition}},
			{path: "/usr/sbin/growpart", args: []string{parentPath, partition}},
			{path: "/bin/growpart", args: []string{parentPath, partition}},
		}); err != nil {
			return fmt.Errorf("grow partition: %w", err)
		}
		resizeCommands := []guestExecCommand{
			{path: "/usr/sbin/resize2fs", args: []string{source}},
			{path: "/sbin/resize2fs", args: []string{source}},
		}
		if fsType == "xfs" {
			resizeCommands = []guestExecCommand{
				{path: "/usr/sbin/xfs_growfs", args: []string{spec.MountPoint}},
				{path: "/sbin/xfs_growfs", args: []string{spec.MountPoint}},
			}
		}
		if _, err := runFirstGuestCommandWithTimeout(ctx, dom, guestAgentResizeTimeout, resizeCommands); err != nil {
			return fmt.Errorf("grow filesystem: %w", err)
		}
		var fsResponse guestFSInfoResponse
		if err := guestAgentCall(dom, "guest-get-fsinfo", nil, guestAgentProbeTimeoutSeconds, &fsResponse); err != nil {
			return fmt.Errorf("read filesystem capacity after growth: %w", err)
		}
		for _, item := range fsResponse.Return {
			if strings.TrimSpace(item.MountPoint) != spec.MountPoint {
				continue
			}
			if item.TotalBytes <= result.PreviousTotalBytes {
				return fmt.Errorf("%w: filesystem capacity did not increase", domain.ErrActionNotAllowed)
			}
			result.TotalBytes = item.TotalBytes
			if item.FreeBytes != nil {
				result.FreeBytes = *item.FreeBytes
			}
			return nil
		}
		return domain.ErrNotFound
	})
	if err != nil {
		return domain.GuestAgentFilesystemResizeResult{}, mapGuestAgentOperationError(uuid, "grow-filesystem", err)
	}
	return result, nil
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
		_, execErr := runFirstGuestCommand(ctx, dom, []guestExecCommand{
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

func runFirstGuestCommand(ctx context.Context, dom *libvirt.Domain, commands []guestExecCommand) (string, error) {
	return runFirstGuestCommandWithTimeout(ctx, dom, guestAgentExecTimeout, commands)
}

func runFirstGuestCommandWithTimeout(ctx context.Context, dom *libvirt.Domain, timeout time.Duration, commands []guestExecCommand) (string, error) {
	var failures []error
	for _, command := range commands {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		output, err := runGuestCommand(ctx, dom, timeout, command.path, command.args)
		if err == nil {
			return output, nil
		}
		if errors.Is(err, errGuestCommandTimeout) {
			return "", err
		}
		failures = append(failures, err)
	}
	return "", errors.Join(failures...)
}

func runGuestCommand(ctx context.Context, dom *libvirt.Domain, timeout time.Duration, path string, args []string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var started guestExecStartResponse
	if err := guestAgentCall(dom, "guest-exec", map[string]any{
		"path": path, "arg": args, "capture-output": true,
	}, guestAgentProbeTimeoutSeconds, &started); err != nil {
		return "", err
	}
	if started.Return.PID <= 0 {
		return "", errors.New("guest command did not return a pid")
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			terminateGuestCommand(dom, started.Return.PID)
			return "", ctx.Err()
		case <-timer.C:
		}
		var status guestExecStatusResponse
		if err := guestAgentCall(dom, "guest-exec-status", map[string]any{"pid": started.Return.PID}, guestAgentProbeTimeoutSeconds, &status); err != nil {
			terminateGuestCommand(dom, started.Return.PID)
			return "", err
		}
		if !status.Return.Exited {
			continue
		}
		stdout, err := decodeGuestExecOutput("stdout", status.Return.OutData)
		if err != nil {
			return "", err
		}
		stderr, err := decodeGuestExecOutput("stderr", status.Return.ErrData)
		if err != nil {
			return "", err
		}
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
	terminateGuestCommand(dom, started.Return.PID)
	return "", &guestCommandTimeoutError{path: path, timeout: timeout}
}

func decodeGuestExecOutput(stream, encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode guest command %s: %w", stream, err)
	}
	return decoded, nil
}

func terminateGuestCommand(dom *libvirt.Domain, pid int64) {
	if dom == nil || pid <= 0 {
		return
	}
	pidText := strconv.FormatInt(pid, 10)
	for _, path := range []string{"/bin/kill", "/usr/bin/kill"} {
		var started guestExecStartResponse
		err := guestAgentCall(dom, "guest-exec", map[string]any{
			"path": path, "arg": []string{"-KILL", pidText}, "capture-output": true,
		}, guestAgentProbeTimeoutSeconds, &started)
		if err == nil && started.Return.PID > 0 {
			return
		}
	}
}

func boundedDiagnosticLines(output string, limit int) []string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	result := make([]string, 0, limit)
	for _, line := range lines {
		line = strings.TrimSpace(strings.ToValidUTF8(line, "�"))
		if line == "" {
			continue
		}
		line, _ = truncateGuestText(line, 2048)
		result = append(result, line)
		if len(result) >= limit {
			break
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}

func truncateGuestDiagnostic(output string) (string, bool) {
	return truncateGuestText(output, guestAgentDiagnosticBytes)
}

func truncateGuestText(value string, limit int) (string, bool) {
	value = strings.TrimSpace(strings.ToValidUTF8(value, "�"))
	if len(value) <= limit {
		return value, false
	}
	truncated := value[:limit]
	for len(truncated) > 0 && !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return strings.TrimSpace(truncated), true
}

func guestDiagnosticRecommendations(result domain.GuestAgentDiagnostics) []string {
	recommendations := make([]string, 0, 4)
	if len(result.FailedServices) > 0 {
		recommendations = append(recommendations, "检查失败服务的 systemd 日志，确认依赖、配置和端口冲突。")
	}
	if len(result.OOMEvents) > 0 {
		recommendations = append(recommendations, "检测到 OOM 事件，请检查客户机内存压力、进程上限或内存气球配置。")
	}
	if len(result.KernelWarnings) > 0 || len(result.SystemErrors) > 0 {
		recommendations = append(recommendations, "结合内核与系统错误日志检查驱动、存储和网络设备状态。")
	}
	if len(result.CommandErrors) > 0 {
		recommendations = append(recommendations, "部分固定诊断命令不可用，请确认 systemd/journalctl 已安装且 QGA 有足够权限。")
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "未发现失败的 systemd 服务、OOM 或错误级系统日志。")
	}
	return recommendations
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

func parseGuestServices(output string) []domain.GuestAgentService {
	result := make([]domain.GuestAgentService, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 4 {
			continue
		}
		unit, load, active, sub := fields[0], fields[1], fields[2], fields[3]
		if !strings.HasSuffix(unit, ".service") || (load != "loaded" && load != "not-found" && load != "masked" && load != "error") {
			continue
		}
		description := ""
		if len(fields) > 4 {
			description = strings.Join(fields[4:], " ")
		}
		result = append(result, domain.GuestAgentService{
			Unit: unit, Load: load, Active: active, Sub: sub, Description: description,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Unit < result[j].Unit })
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
