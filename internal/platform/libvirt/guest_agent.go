package libvirtbackend

import (
	"cmp"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

const (
	guestAgentProbeTimeoutSeconds = 2
	guestAgentInfoTimeoutSeconds  = 1
	guestAgentCacheTTL            = 15 * time.Second
	guestAgentFailureCacheTTL     = 8 * time.Second
)

type guestAgentCacheEntry struct {
	info    domain.GuestAgentInfo
	expires time.Time
}

type guestAgentProbeCall struct {
	done chan struct{}
	info domain.GuestAgentInfo
	err  error
}

type guestInfoResponse struct {
	Return struct {
		Version           string `json:"version"`
		SupportedCommands []struct {
			Name    string `json:"name"`
			Enabled *bool  `json:"enabled"`
		} `json:"supported_commands"`
	} `json:"return"`
}

type guestOSInfoResponse struct {
	Return struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		PrettyName    string `json:"pretty-name"`
		Version       string `json:"version"`
		KernelRelease string `json:"kernel-release"`
		Machine       string `json:"machine"`
		Hostname      string `json:"hostname"`
	} `json:"return"`
}

type guestFSInfoResponse struct {
	Return []struct {
		Name       string  `json:"name"`
		MountPoint string  `json:"mountpoint"`
		Type       string  `json:"type"`
		TotalBytes uint64  `json:"total-bytes"`
		UsedBytes  uint64  `json:"used-bytes"`
		FreeBytes  *uint64 `json:"free-bytes"`
	} `json:"return"`
}

type guestTimezoneResponse struct {
	Return struct {
		Zone   string `json:"zone"`
		Offset int    `json:"offset"`
	} `json:"return"`
}

type guestRouteResponse struct {
	Return []struct {
		Destination string `json:"destination"`
		Gateway     string `json:"gateway"`
		Interface   string `json:"interface"`
		Family      string `json:"family"`
		Metric      int    `json:"metric"`
	} `json:"return"`
}

func (c *Client) GetGuestAgentInfo(ctx context.Context, uuid string, refresh bool) (domain.GuestAgentInfo, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return domain.GuestAgentInfo{}, domain.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return domain.GuestAgentInfo{}, err
	}

	now := time.Now().UTC()
	c.guestCacheMu.Lock()
	if c.guestCache == nil {
		c.guestCache = make(map[string]guestAgentCacheEntry)
	}
	if c.guestInflight == nil {
		c.guestInflight = make(map[string]*guestAgentProbeCall)
	}
	entry, hasEntry := c.guestCache[uuid]
	if !refresh && hasEntry && now.Before(entry.expires) {
		c.guestCacheMu.Unlock()
		return cloneGuestAgentInfo(entry.info), nil
	}
	if call := c.guestInflight[uuid]; call != nil {
		c.guestCacheMu.Unlock()
		select {
		case <-ctx.Done():
			return domain.GuestAgentInfo{}, ctx.Err()
		case <-call.done:
			return cloneGuestAgentInfo(call.info), call.err
		}
	}
	call := &guestAgentProbeCall{done: make(chan struct{})}
	c.guestInflight[uuid] = call
	previous := entry.info
	c.guestCacheMu.Unlock()

	info, err := c.probeGuestAgent(ctx, uuid, previous, hasEntry)
	if err != nil && hasEntry {
		info = cloneGuestAgentInfo(previous)
		info.State = domain.GuestAgentStateError
		info.Connected = false
		info.Stale = true
		info.LastError = boundedGuestAgentError(err)
		info.ProbedAt = now
		err = nil
	}
	if err == nil {
		if info.ProbedAt.IsZero() {
			info.ProbedAt = now
		}
		if hasEntry && info.LastSeenAt == nil && previous.LastSeenAt != nil {
			lastSeen := *previous.LastSeenAt
			info.LastSeenAt = &lastSeen
		}
		if hasEntry && !info.Connected && previous.LastSeenAt != nil {
			info.Stale = true
		}
		ttl := guestAgentCacheTTL
		if !info.Connected {
			ttl = guestAgentFailureCacheTTL
		}
		c.guestCacheMu.Lock()
		c.guestCache[uuid] = guestAgentCacheEntry{info: cloneGuestAgentInfo(info), expires: time.Now().UTC().Add(ttl)}
		c.guestCacheMu.Unlock()
	}

	call.info = cloneGuestAgentInfo(info)
	call.err = err
	c.guestCacheMu.Lock()
	delete(c.guestInflight, uuid)
	close(call.done)
	c.guestCacheMu.Unlock()
	if err != nil {
		return domain.GuestAgentInfo{}, err
	}
	return cloneGuestAgentInfo(info), nil
}

func (c *Client) probeGuestAgent(ctx context.Context, uuid string, previous domain.GuestAgentInfo, hasPrevious bool) (domain.GuestAgentInfo, error) {
	c.guestMu.Lock()
	defer c.guestMu.Unlock()
	if err := c.ensureGuestConnectionLocked(); err != nil {
		return domain.GuestAgentInfo{}, err
	}
	dom, err := c.guestConn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return domain.GuestAgentInfo{}, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()

	state, _, err := dom.GetState()
	if err != nil {
		return domain.GuestAgentInfo{}, fmt.Errorf("read domain %s state for guest agent: %w", uuid, err)
	}
	xmlText, err := guestAgentDomainXML(dom)
	if err != nil {
		return domain.GuestAgentInfo{}, fmt.Errorf("read domain %s xml for guest agent: %w", uuid, err)
	}
	info := domain.GuestAgentInfo{
		State:             domain.GuestAgentStateUnknown,
		ChannelConfigured: hasGuestAgentChannelXML(xmlText),
		Interfaces:        []domain.GuestAgentNetworkInterface{},
		Filesystems:       []domain.GuestAgentFilesystem{},
		ProbedAt:          time.Now().UTC(),
	}
	if hasPrevious && previous.LastSeenAt != nil {
		lastSeen := *previous.LastSeenAt
		info.LastSeenAt = &lastSeen
		info.Stale = true
	}
	if !info.ChannelConfigured {
		info.State = domain.GuestAgentStateNotConfigured
		info.LastError = "虚拟机配置中没有 org.qemu.guest_agent.0 通道"
		info.DiagnosticCode = "channel_missing"
		info.DiagnosticHint = "关机后可为持久域补充 QGA 通道；新建虚拟机默认会挂载该通道。"
		return info, nil
	}
	if state != libvirt.DOMAIN_RUNNING && state != libvirt.DOMAIN_BLOCKED {
		info.State = domain.GuestAgentStateDisconnected
		info.LastError = "虚拟机当前未运行，客户机代理无法响应"
		info.DiagnosticCode = "domain_not_running"
		info.DiagnosticHint = "启动虚拟机后再探测 Guest Agent。"
		return info, nil
	}
	return probeGuestAgentOnDomain(dom, info)
}

func guestAgentDomainXML(dom *libvirt.Domain) (string, error) {
	if persistent, err := dom.IsPersistent(); err == nil && persistent {
		if text, err := dom.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE); err == nil {
			return text, nil
		}
	}
	return dom.GetXMLDesc(0)
}

func probeGuestAgentOnDomain(dom *libvirt.Domain, info domain.GuestAgentInfo) (domain.GuestAgentInfo, error) {
	rawInfo, err := guestAgentCommand(dom, "guest-info", nil, guestAgentProbeTimeoutSeconds)
	if err != nil {
		info.State = guestAgentStateFromError(err)
		info.Connected = false
		info.LastError = boundedGuestAgentError(err)
		info.DiagnosticCode, info.DiagnosticHint = guestAgentDiagnosticFromError(err)
		return info, nil
	}
	var parsed guestInfoResponse
	if err := decodeGuestAgentResponse(rawInfo, &parsed); err != nil {
		info.State = domain.GuestAgentStateError
		info.LastError = "无法解析 guest-info 响应: " + boundedGuestAgentError(err)
		info.DiagnosticCode = "protocol_error"
		info.DiagnosticHint = "Agent 返回了无法识别的 guest-info 响应，请检查 qemu-guest-agent 版本。"
		return info, nil
	}
	info.State = domain.GuestAgentStateConnected
	info.Connected = true
	info.Version = strings.TrimSpace(parsed.Return.Version)
	info.Stale = false
	now := time.Now().UTC()
	info.LastSeenAt = &now
	info.ProbedAt = now
	info.Capabilities = guestAgentCapabilities(parsed)

	var collectionErrors []string
	if info.Capabilities.Hostname {
		if raw, commandErr := guestAgentCommand(dom, "guest-get-host-name", nil, guestAgentInfoTimeoutSeconds); commandErr == nil {
			if hostname, parseErr := parseGuestHostname(raw); parseErr == nil {
				info.Hostname = hostname
			}
		}
	}
	if info.Capabilities.OSInfo {
		if raw, commandErr := guestAgentCommand(dom, "guest-get-osinfo", nil, guestAgentInfoTimeoutSeconds); commandErr == nil {
			osInfo, parseErr := parseGuestOSInfo(raw)
			if parseErr != nil {
				collectionErrors = append(collectionErrors, "osinfo")
			} else {
				info.OS = osInfo
				if info.Hostname == "" {
					info.Hostname = osInfo.Hostname
				}
			}
		} else {
			collectionErrors = append(collectionErrors, "osinfo")
		}
	}
	if info.Capabilities.Network {
		if raw, commandErr := guestAgentCommand(dom, "guest-network-get-interfaces", nil, guestAgentInfoTimeoutSeconds); commandErr == nil {
			interfaces, parseErr := parseGuestAgentNetworkInterfaces(raw)
			if parseErr != nil {
				collectionErrors = append(collectionErrors, "network")
			} else {
				info.Interfaces = interfaces
			}
		} else {
			collectionErrors = append(collectionErrors, "network")
		}
	}
	if info.Capabilities.FSInfo {
		if raw, commandErr := guestAgentCommand(dom, "guest-get-fsinfo", nil, guestAgentInfoTimeoutSeconds); commandErr == nil {
			filesystems, parseErr := parseGuestAgentFilesystems(raw)
			if parseErr != nil {
				collectionErrors = append(collectionErrors, "fsinfo")
			} else {
				info.Filesystems = filesystems
			}
		} else {
			collectionErrors = append(collectionErrors, "fsinfo")
		}
	}
	if info.Capabilities.Timezone {
		if raw, commandErr := guestAgentCommand(dom, "guest-get-timezone", nil, guestAgentInfoTimeoutSeconds); commandErr == nil {
			timezone, parseErr := parseGuestTimezone(raw)
			if parseErr != nil {
				collectionErrors = append(collectionErrors, "timezone")
			} else {
				info.Timezone = timezone
			}
		} else {
			collectionErrors = append(collectionErrors, "timezone")
		}
	}
	if info.Capabilities.NetworkRoutes {
		if raw, commandErr := guestAgentCommand(dom, "guest-network-get-route", nil, guestAgentInfoTimeoutSeconds); commandErr == nil {
			routes, parseErr := parseGuestRoutes(raw)
			if parseErr != nil {
				collectionErrors = append(collectionErrors, "routes")
			} else {
				info.Routes = routes
			}
		} else {
			collectionErrors = append(collectionErrors, "routes")
		}
	}
	if isWindowsGuest(info.OS) {
		info.Capabilities.Processes = false
	}
	if len(collectionErrors) > 0 {
		info.LastError = "以下代理信息读取失败: " + strings.Join(collectionErrors, ", ")
		info.DiagnosticCode = "partial_data"
		info.DiagnosticHint = "Agent 已连接，但部分信息读取失败；可刷新重试或检查客户机负载。"
	}
	return info, nil
}

func guestAgentCommand(dom *libvirt.Domain, command string, arguments any, timeoutSeconds int) (string, error) {
	payload := map[string]any{"execute": command}
	if arguments != nil {
		payload["arguments"] = arguments
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode guest agent command %s: %w", command, err)
	}
	return dom.QemuAgentCommand(string(encoded), libvirt.DomainQemuAgentCommandTimeout(timeoutSeconds), 0)
}

func decodeGuestAgentResponse(raw string, target any) error {
	var envelope struct {
		Error *struct {
			Class string `json:"class"`
			Desc  string `json:"desc"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return err
	}
	if envelope.Error != nil {
		message := strings.TrimSpace(envelope.Error.Desc)
		if message == "" {
			message = strings.TrimSpace(envelope.Error.Class)
		}
		if message == "" {
			message = "guest agent command failed"
		}
		return errors.New(message)
	}
	return json.Unmarshal([]byte(raw), target)
}

func guestAgentCapabilities(response guestInfoResponse) domain.GuestAgentCapabilities {
	commands := make(map[string]bool, len(response.Return.SupportedCommands))
	for _, command := range response.Return.SupportedCommands {
		if command.Enabled == nil || *command.Enabled {
			commands[strings.TrimSpace(command.Name)] = true
		}
	}
	capabilities := domain.GuestAgentCapabilities{
		Hostname: commands["guest-get-host-name"],
		Network:  commands["guest-network-get-interfaces"],
		OSInfo:   commands["guest-get-osinfo"],
		FSInfo:   commands["guest-get-fsinfo"],
		Shutdown: commands["guest-shutdown"],
		Reboot:   commands["guest-shutdown"],
		TimeSync: commands["guest-set-time"],
		Trim:     commands["guest-fstrim"],
		Quiesce:  commands["guest-fsfreeze-freeze"] && commands["guest-fsfreeze-thaw"],
		FileAccess: commands["guest-file-open"] && commands["guest-file-read"] &&
			commands["guest-file-write"] && commands["guest-file-close"],
		PasswordReset: commands["guest-set-user-password"],
		SSHKeys: commands["guest-ssh-get-authorized-keys"] && commands["guest-ssh-add-authorized-keys"] &&
			commands["guest-ssh-remove-authorized-keys"],
		Processes:     commands["guest-exec"] && commands["guest-exec-status"],
		Users:         commands["guest-get-users"],
		Timezone:      commands["guest-get-timezone"],
		NetworkRoutes: commands["guest-network-get-route"],
	}
	capabilities.FilesystemResize = capabilities.Processes && capabilities.FSInfo
	return capabilities
}

func parseGuestTimezone(raw string) (string, error) {
	var response guestTimezoneResponse
	if err := decodeGuestAgentResponse(raw, &response); err != nil {
		return "", fmt.Errorf("decode guest timezone response: %w", err)
	}
	return strings.TrimSpace(response.Return.Zone), nil
}

func parseGuestRoutes(raw string) ([]domain.GuestAgentRoute, error) {
	var response guestRouteResponse
	if err := decodeGuestAgentResponse(raw, &response); err != nil {
		return nil, fmt.Errorf("decode guest route response: %w", err)
	}
	result := make([]domain.GuestAgentRoute, 0, len(response.Return))
	for _, route := range response.Return {
		destination := strings.TrimSpace(route.Destination)
		family := strings.ToLower(strings.TrimSpace(route.Family))
		if family == "" {
			if strings.Contains(destination, ":") {
				family = "ipv6"
			} else if destination != "" {
				family = "ipv4"
			}
		}
		result = append(result, domain.GuestAgentRoute{
			Destination: destination, Gateway: strings.TrimSpace(route.Gateway),
			Interface: strings.TrimSpace(route.Interface), Family: family, Metric: route.Metric,
		})
	}
	slices.SortFunc(result, func(left, right domain.GuestAgentRoute) int {
		if left.Family != right.Family {
			if left.Family == "ipv4" {
				return -1
			}
			if right.Family == "ipv4" {
				return 1
			}
			return cmp.Compare(left.Family, right.Family)
		}
		return cmp.Or(cmp.Compare(left.Destination, right.Destination), cmp.Compare(left.Interface, right.Interface))
	})
	return result, nil
}

func isWindowsGuest(info domain.GuestAgentOSInfo) bool {
	value := strings.ToLower(strings.TrimSpace(info.ID + " " + info.Name + " " + info.PrettyName))
	return strings.Contains(value, "windows") || strings.Contains(value, "microsoft")
}

func guestAgentDiagnosticFromError(err error) (string, string) {
	message := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, libvirt.ERR_AGENT_UNRESPONSIVE), errors.Is(err, libvirt.ERR_AGENT_COMMAND_TIMEOUT):
		return "agent_unresponsive", "Guest Agent 无响应；请检查客户机 qemu-guest-agent 服务、virtio-serial 驱动和客户机负载。"
	case errors.Is(err, libvirt.ERR_AGENT_UNSYNCED):
		return "agent_unsynced", "Guest Agent 尚未完成同步；请稍后刷新或重启客户机代理服务。"
	case errors.Is(err, libvirt.ERR_OPERATION_UNSUPPORTED):
		return "protocol_unsupported", "当前 Agent 或 QEMU 版本不支持该能力，请升级 qemu-guest-agent。"
	case strings.Contains(message, "permission"), strings.Contains(message, "denied"):
		return "permission_denied", "客户机代理拒绝了请求；请检查 Agent 运行权限或相关策略。"
	default:
		return "agent_not_running", "Agent 未响应；请在客户机内安装并启动 qemu-guest-agent，并确认 virtio-serial 驱动已加载。"
	}
}

func parseGuestOSInfo(raw string) (domain.GuestAgentOSInfo, error) {
	var response guestOSInfoResponse
	if err := decodeGuestAgentResponse(raw, &response); err != nil {
		return domain.GuestAgentOSInfo{}, fmt.Errorf("decode guest osinfo response: %w", err)
	}
	return domain.GuestAgentOSInfo{
		ID:            strings.TrimSpace(response.Return.ID),
		Name:          strings.TrimSpace(response.Return.Name),
		PrettyName:    strings.TrimSpace(response.Return.PrettyName),
		Version:       strings.TrimSpace(response.Return.Version),
		KernelRelease: strings.TrimSpace(response.Return.KernelRelease),
		Arch:          strings.TrimSpace(response.Return.Machine),
		Hostname:      strings.TrimSpace(response.Return.Hostname),
	}, nil
}

func parseGuestAgentNetworkInterfaces(raw string) ([]domain.GuestAgentNetworkInterface, error) {
	var response guestNetworkResponse
	if err := decodeGuestAgentResponse(raw, &response); err != nil {
		return nil, fmt.Errorf("decode guest network response: %w", err)
	}
	result := make([]domain.GuestAgentNetworkInterface, 0, len(response.Return))
	for _, networkInterface := range response.Return {
		item := domain.GuestAgentNetworkInterface{
			Name:      strings.TrimSpace(networkInterface.Name),
			MAC:       strings.TrimSpace(networkInterface.MAC),
			Addresses: []domain.GuestAgentIPAddress{},
		}
		for _, address := range networkInterface.IPAddresses {
			value := strings.TrimSpace(address.Address)
			if zone := strings.LastIndexByte(value, '%'); zone >= 0 {
				value = value[:zone]
			}
			ip := net.ParseIP(value)
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || isIPv6LinkLocal(ip) {
				continue
			}
			normalized := ip.String()
			if slices.ContainsFunc(item.Addresses, func(existing domain.GuestAgentIPAddress) bool {
				return existing.Address == normalized
			}) {
				continue
			}
			family := "ipv6"
			if ip.To4() != nil {
				family = "ipv4"
			}
			item.Addresses = append(item.Addresses, domain.GuestAgentIPAddress{Address: normalized, Family: family, Prefix: address.Prefix})
		}
		if len(item.Addresses) == 0 {
			continue
		}
		slices.SortFunc(item.Addresses, func(left, right domain.GuestAgentIPAddress) int {
			if left.Family != right.Family {
				if left.Family == "ipv4" {
					return -1
				}
				if right.Family == "ipv4" {
					return 1
				}
				return cmp.Compare(left.Family, right.Family)
			}
			return cmp.Compare(left.Address, right.Address)
		})
		result = append(result, item)
	}
	slices.SortFunc(result, func(left, right domain.GuestAgentNetworkInterface) int {
		return cmp.Compare(left.Name, right.Name)
	})
	return result, nil
}

func parseGuestAgentFilesystems(raw string) ([]domain.GuestAgentFilesystem, error) {
	var response guestFSInfoResponse
	if err := decodeGuestAgentResponse(raw, &response); err != nil {
		return nil, fmt.Errorf("decode guest fsinfo response: %w", err)
	}
	result := make([]domain.GuestAgentFilesystem, 0, len(response.Return))
	for _, filesystem := range response.Return {
		freeBytes := uint64(0)
		if filesystem.FreeBytes != nil {
			freeBytes = *filesystem.FreeBytes
		} else if filesystem.TotalBytes >= filesystem.UsedBytes {
			freeBytes = filesystem.TotalBytes - filesystem.UsedBytes
		}
		result = append(result, domain.GuestAgentFilesystem{
			Device:     strings.TrimSpace(filesystem.Name),
			MountPoint: strings.TrimSpace(filesystem.MountPoint),
			Type:       strings.TrimSpace(filesystem.Type),
			TotalBytes: filesystem.TotalBytes,
			UsedBytes:  filesystem.UsedBytes,
			FreeBytes:  freeBytes,
		})
	}
	slices.SortFunc(result, func(left, right domain.GuestAgentFilesystem) int {
		return cmp.Compare(left.MountPoint, right.MountPoint)
	})
	return result, nil
}

func guestAgentStateFromError(err error) domain.GuestAgentState {
	switch {
	case errors.Is(err, libvirt.ERR_AGENT_UNRESPONSIVE), errors.Is(err, libvirt.ERR_AGENT_COMMAND_TIMEOUT):
		return domain.GuestAgentStateUnresponsive
	case errors.Is(err, libvirt.ERR_OPERATION_UNSUPPORTED), errors.Is(err, libvirt.ERR_OPERATION_INVALID), errors.Is(err, libvirt.ERR_AGENT_COMMAND_FAILED):
		return domain.GuestAgentStateDisconnected
	default:
		return domain.GuestAgentStateError
	}
}

func boundedGuestAgentError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 600 {
		message = message[:600] + "..."
	}
	return message
}

func cloneGuestAgentInfo(info domain.GuestAgentInfo) domain.GuestAgentInfo {
	cloned := info
	if info.LastSeenAt != nil {
		lastSeen := *info.LastSeenAt
		cloned.LastSeenAt = &lastSeen
	}
	cloned.Interfaces = append([]domain.GuestAgentNetworkInterface(nil), info.Interfaces...)
	for index := range cloned.Interfaces {
		cloned.Interfaces[index].Addresses = append([]domain.GuestAgentIPAddress(nil), cloned.Interfaces[index].Addresses...)
	}
	cloned.Filesystems = append([]domain.GuestAgentFilesystem(nil), info.Filesystems...)
	return cloned
}

func (c *Client) PerformGuestAgentAction(ctx context.Context, uuid string, spec domain.GuestAgentActionSpec) error {
	uuid = strings.TrimSpace(uuid)
	action, ok := domain.ValidGuestAgentAction(string(spec.Action))
	if !ok {
		return domain.ErrInvalidInput
	}
	spec.Action = action
	spec.Mount = strings.TrimSpace(spec.Mount)
	if uuid == "" || !validGuestAgentActionSpec(spec) {
		return domain.ErrInvalidInput
	}
	info, err := c.GetGuestAgentInfo(ctx, uuid, false)
	if err != nil {
		return err
	}
	if info.State != domain.GuestAgentStateConnected || !spec.SupportedBy(info.Capabilities) {
		return domain.ErrGuestAgent
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	switch spec.Action {
	case domain.GuestAgentActionShutdown:
		err = dom.ShutdownFlags(libvirt.DOMAIN_SHUTDOWN_GUEST_AGENT)
	case domain.GuestAgentActionReboot:
		err = dom.Reboot(libvirt.DOMAIN_REBOOT_GUEST_AGENT)
	case domain.GuestAgentActionSyncTime:
		now := time.Now()
		err = dom.SetTime(now.Unix(), uint(now.Nanosecond()), libvirt.DOMAIN_TIME_SYNC)
	case domain.GuestAgentActionTrim:
		err = dom.FSTrim(spec.Mount, spec.MinimumBytes, 0)
	default:
		return domain.ErrInvalidInput
	}
	if err != nil {
		return mapGuestAgentActionError(uuid, spec.Action, err)
	}
	c.invalidateGuestAgentCache(uuid)
	return nil
}

func (c *Client) invalidateGuestAgentCache(uuid string) {
	c.guestCacheMu.Lock()
	if c.guestCache != nil {
		delete(c.guestCache, uuid)
	}
	c.guestCacheMu.Unlock()
}

func validGuestAgentActionSpec(spec domain.GuestAgentActionSpec) bool {
	switch spec.Action {
	case domain.GuestAgentActionShutdown, domain.GuestAgentActionReboot, domain.GuestAgentActionSyncTime:
		return strings.TrimSpace(spec.Mount) == "" && spec.MinimumBytes == 0
	case domain.GuestAgentActionTrim:
		return spec.Mount == "" || (filepath.IsAbs(spec.Mount) && !strings.ContainsRune(spec.Mount, '\x00'))
	default:
		return false
	}
}

func mapGuestAgentActionError(uuid string, action domain.GuestAgentAction, err error) error {
	switch {
	case errors.Is(err, libvirt.ERR_NO_DOMAIN):
		return domain.ErrNotFound
	case errors.Is(err, libvirt.ERR_AGENT_UNRESPONSIVE),
		errors.Is(err, libvirt.ERR_AGENT_COMMAND_TIMEOUT),
		errors.Is(err, libvirt.ERR_AGENT_UNSYNCED),
		errors.Is(err, libvirt.ERR_AGENT_COMMAND_FAILED),
		errors.Is(err, libvirt.ERR_OPERATION_UNSUPPORTED):
		return errors.Join(domain.ErrGuestAgent, fmt.Errorf("guest agent action %s on domain %s: %w", action, uuid, err))
	case errors.Is(err, libvirt.ERR_OPERATION_INVALID):
		return errors.Join(domain.ErrActionNotAllowed, fmt.Errorf("guest agent action %s on domain %s: %w", action, uuid, err))
	default:
		return fmt.Errorf("guest agent action %s on domain %s: %w", action, uuid, err)
	}
}

func (c *Client) EnsureGuestAgentChannel(ctx context.Context, uuid string) (domain.VM, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VM{}, err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return domain.VM{}, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	persistent, err := dom.IsPersistent()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s persistence: %w", uuid, err)
	}
	if !persistent {
		return domain.VM{}, errors.Join(domain.ErrActionNotAllowed, errors.New("transient domains cannot be updated"))
	}
	state, _, err := dom.GetState()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s state: %w", uuid, err)
	}
	if state != libvirt.DOMAIN_SHUTOFF && state != libvirt.DOMAIN_CRASHED {
		return domain.VM{}, errors.Join(domain.ErrActionNotAllowed, errors.New("guest agent channel can only be added to a stopped domain"))
	}
	xmlText, err := dom.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE)
	if err != nil {
		return domain.VM{}, fmt.Errorf("read persistent domain %s xml: %w", uuid, err)
	}
	updatedXML, err := addGuestAgentChannelXML(xmlText)
	if err != nil {
		return domain.VM{}, err
	}
	resultDomain := dom
	if updatedXML != xmlText {
		defined, err := c.conn.DomainDefineXMLFlags(updatedXML, libvirt.DOMAIN_DEFINE_VALIDATE)
		if err != nil {
			return domain.VM{}, fmt.Errorf("define domain %s with guest agent channel: %w", uuid, err)
		}
		defer func() { _ = defined.Free() }()
		definedUUID, err := defined.GetUUIDString()
		if err != nil {
			return domain.VM{}, fmt.Errorf("read updated domain uuid: %w", err)
		}
		definedName, err := defined.GetName()
		if err != nil {
			return domain.VM{}, fmt.Errorf("read updated domain name: %w", err)
		}
		originalName, _ := dom.GetName()
		if !strings.EqualFold(definedUUID, uuid) || definedName != originalName {
			return domain.VM{}, fmt.Errorf("%w: libvirt returned a different domain identity", domain.ErrConflict)
		}
		resultDomain = defined
	}
	c.invalidateGuestAgentCache(uuid)
	vm, err := domainFromLibvirt(resultDomain)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&vm)
	return vm, nil
}

func hasGuestAgentChannelXML(text string) bool {
	var parsed domainXML
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		return false
	}
	for _, channel := range parsed.Devices.Channels {
		if strings.EqualFold(strings.TrimSpace(channel.Type), "unix") &&
			strings.EqualFold(strings.TrimSpace(channel.Target.Type), "virtio") &&
			strings.TrimSpace(channel.Target.Name) == "org.qemu.guest_agent.0" {
			return true
		}
	}
	return false
}

func addGuestAgentChannelXML(text string) (string, error) {
	var parsed domainXML
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		return "", fmt.Errorf("decode domain xml before adding guest agent channel: %w", err)
	}
	if hasGuestAgentChannelXML(text) {
		return text, nil
	}
	controllerIndex := -1
	for _, controller := range parsed.Devices.Controllers {
		if strings.EqualFold(strings.TrimSpace(controller.Type), "virtio-serial") {
			controllerIndex = controller.Index
			break
		}
	}
	var fragment strings.Builder
	if controllerIndex < 0 {
		controllerIndex = 0
		fragment.WriteString("<controller type='virtio-serial' index='0'/>")
	}
	fragment.WriteString("<channel type='unix'><target type='virtio' name='org.qemu.guest_agent.0'/>")
	if controllerIndex != 0 {
		fmt.Fprintf(&fragment, "<address type='virtio-serial' controller='%d'/>", controllerIndex)
	}
	fragment.WriteString("</channel>")
	end := strings.LastIndex(text, "</devices>")
	if end < 0 {
		return "", errors.New("domain xml has no devices section")
	}
	return text[:end] + fragment.String() + text[end:], nil
}
