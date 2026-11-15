package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

const (
	maxGuestAgentReadBytes   = 1 << 20
	maxGuestAgentWriteBytes  = 1 << 20
	maxGuestAgentSSHKeyCount = 16
)

type VMBackend interface {
	GetGuestAgentInfo(ctx context.Context, uuid string, refresh bool) (domain.GuestAgentInfo, error)
	PerformGuestAgentAction(ctx context.Context, uuid string, spec domain.GuestAgentActionSpec) error
	EnsureGuestAgentChannel(ctx context.Context, uuid string) (domain.VM, error)
	ReadGuestAgentFile(ctx context.Context, uuid string, spec domain.GuestAgentFileReadSpec) (domain.GuestAgentFileContent, error)
	WriteGuestAgentFile(ctx context.Context, uuid string, spec domain.GuestAgentFileWriteSpec) (domain.GuestAgentFileWriteResult, error)
	ResetGuestAgentPassword(ctx context.Context, uuid string, spec domain.GuestAgentPasswordSpec) error
	ListGuestAgentAuthorizedKeys(ctx context.Context, uuid, username string) (domain.GuestAgentAuthorizedKeys, error)
	AddGuestAgentAuthorizedKeys(ctx context.Context, uuid string, spec domain.GuestAgentSSHKeySpec) error
	RemoveGuestAgentAuthorizedKeys(ctx context.Context, uuid string, spec domain.GuestAgentSSHKeySpec) error
	ListGuestAgentProcesses(ctx context.Context, uuid string) ([]domain.GuestAgentProcess, error)
	SignalGuestAgentProcess(ctx context.Context, uuid string, spec domain.GuestAgentProcessSignalSpec) error
	ListGuestAgentUsers(ctx context.Context, uuid string) ([]domain.GuestAgentUser, error)
}

type VMInventory interface {
	ListVMs(ctx context.Context) ([]domain.VM, error)
}

type PolicyRepository interface {
	ListGuestAgentPolicies(ctx context.Context) ([]domain.GuestAgentPolicy, error)
	GetGuestAgentPolicy(ctx context.Context, vmUUID string) (domain.GuestAgentPolicy, error)
	UpsertGuestAgentPolicy(ctx context.Context, policy domain.GuestAgentPolicy) (domain.GuestAgentPolicy, error)
	DeleteGuestAgentPolicy(ctx context.Context, vmUUID string) error
	InsertGuestAgentHealthSample(ctx context.Context, sample domain.GuestAgentHealthSample) error
	ListGuestAgentHealthSamples(ctx context.Context, vmUUID string, since time.Time, limit int) ([]domain.GuestAgentHealthSample, error)
	DeleteGuestAgentHealthSamplesBefore(ctx context.Context, before time.Time) error
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type TaskRecorder interface {
	Start(ctx context.Context, kind, targetKind, targetID, actor, step string) (string, error)
	Progress(ctx context.Context, id string, progress int, step string) error
	Succeed(ctx context.Context, id, step string) error
	Fail(ctx context.Context, id string, progress int, step string, cause error) error
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type ActionCommand struct {
	UUID    string
	Spec    domain.GuestAgentActionSpec
	Request MutationContext
}

type ChannelCommand struct {
	UUID    string
	Request MutationContext
}

type FileReadCommand struct {
	UUID    string
	Spec    domain.GuestAgentFileReadSpec
	Request MutationContext
}

type FileWriteCommand struct {
	UUID    string
	Spec    domain.GuestAgentFileWriteSpec
	Request MutationContext
}

type FileWriteResponse struct {
	Task   domain.Task                      `json:"task"`
	Result domain.GuestAgentFileWriteResult `json:"result"`
}

type PasswordCommand struct {
	UUID    string
	Spec    domain.GuestAgentPasswordSpec
	Request MutationContext
}

type SSHKeyListCommand struct {
	UUID     string
	Username string
	Request  MutationContext
}

type SSHKeyChangeCommand struct {
	UUID    string
	Spec    domain.GuestAgentSSHKeySpec
	Request MutationContext
}

type ProcessCommand struct {
	UUID    string
	Request MutationContext
}

type ProcessSignalCommand struct {
	UUID    string
	Spec    domain.GuestAgentProcessSignalSpec
	Request MutationContext
}

type PolicyCommand struct {
	UUID    string
	Spec    domain.GuestAgentPolicySpec
	Request MutationContext
}

type PolicyQuery struct {
	UUID    string
	Request MutationContext
}

type HealthQuery struct {
	UUID    string
	Hours   int
	Limit   int
	Request MutationContext
}

type HealthChange struct {
	VMUUID    string                 `json:"vmUuid"`
	Kind      string                 `json:"kind"`
	State     domain.GuestAgentState `json:"state"`
	Failures  int                    `json:"failures"`
	LastError string                 `json:"lastError,omitempty"`
}

type Service struct {
	backend      VMBackend
	audit        AuditRecorder
	policies     PolicyRepository
	tasks        TaskRecorder
	readOnly     bool
	readOnlyMode domain.ReadOnlyProvider
}

type Option func(*Service)

func WithReadOnlyMode(mode domain.ReadOnlyProvider) Option {
	return func(service *Service) {
		service.readOnlyMode = mode
	}
}

func WithPolicyRepository(repository PolicyRepository) Option {
	return func(service *Service) {
		service.policies = repository
	}
}

func WithTaskRecorder(recorder TaskRecorder) Option {
	return func(service *Service) {
		service.tasks = recorder
	}
}

func New(backend VMBackend, audit AuditRecorder, readOnly bool, options ...Option) *Service {
	service := &Service{backend: backend, audit: audit, readOnly: readOnly}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) isReadOnly() bool {
	if s.readOnlyMode != nil {
		return s.readOnlyMode.ReadOnly()
	}
	return s.readOnly
}

func (s *Service) Get(ctx context.Context, uuid string, refresh bool) (domain.GuestAgentInfo, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return domain.GuestAgentInfo{}, domain.ErrInvalidInput
	}
	return s.backend.GetGuestAgentInfo(ctx, uuid, refresh)
}

func (s *Service) Action(ctx context.Context, command ActionCommand) (domain.Task, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Spec.Mount = strings.TrimSpace(command.Spec.Mount)
	action, ok := domain.ValidGuestAgentAction(string(command.Spec.Action))
	if !ok {
		return domain.Task{}, domain.ErrInvalidInput
	}
	command.Spec.Action = action
	detail := map[string]any{
		"action": command.Spec.Action, "mount": command.Spec.Mount, "minimumBytes": command.Spec.MinimumBytes,
	}
	if command.UUID == "" || !validActionSpec(command.Spec) {
		return domain.Task{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.Task{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.action", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Task{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.action", command.UUID, detail, domain.ErrReadOnly)
	}
	return s.runTask(ctx, command.Request, "guest-agent."+string(action), "vm.guest_agent.action", command.UUID, "执行 Guest Agent 动作", detail, "Guest Agent 动作已完成", func() (any, error) {
		return nil, s.backend.PerformGuestAgentAction(ctx, command.UUID, command.Spec)
	})
}

func (s *Service) EnsureChannel(ctx context.Context, command ChannelCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	if command.UUID == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.channel.add", command.UUID, nil, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.channel.add", command.UUID, nil, domain.ErrReadOnly)
	}
	vm, err := s.backend.EnsureGuestAgentChannel(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.channel.add", command.UUID, nil, err)
	}
	detail := map[string]any{"channel": "org.qemu.guest_agent.0"}
	if err := s.recordAudit(ctx, command.Request, "vm.guest_agent.channel.add", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record guest agent channel audit: %w", err)
	}
	return vm, nil
}

func (s *Service) ReadFile(ctx context.Context, command FileReadCommand) (domain.GuestAgentFileContent, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Spec.Path = strings.TrimSpace(command.Spec.Path)
	detail := map[string]any{"path": command.Spec.Path, "offset": command.Spec.Offset, "maxBytes": command.Spec.MaxBytes}
	if command.UUID == "" || !validFilePath(command.Spec.Path) || command.Spec.MaxBytes > maxGuestAgentReadBytes {
		return domain.GuestAgentFileContent{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.GuestAgentFileContent{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.file.read", command.UUID, detail, domain.ErrForbidden)
	}
	content, err := s.backend.ReadGuestAgentFile(ctx, command.UUID, command.Spec)
	if err != nil {
		return domain.GuestAgentFileContent{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.file.read", command.UUID, detail, err)
	}
	detail["size"] = content.Size
	detail["sha256"] = content.SHA256
	detail["sensitive"] = true
	if err := s.recordAudit(ctx, command.Request, "vm.guest_agent.file.read", command.UUID, "success", detail, nil); err != nil {
		return domain.GuestAgentFileContent{}, fmt.Errorf("record guest agent file read audit: %w", err)
	}
	return content, nil
}

func (s *Service) WriteFile(ctx context.Context, command FileWriteCommand) (FileWriteResponse, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Spec.Path = strings.TrimSpace(command.Spec.Path)
	command.Spec.Mode = strings.ToLower(strings.TrimSpace(command.Spec.Mode))
	if command.Spec.Mode == "" {
		command.Spec.Mode = "replace"
	}
	data, decodeErr := base64.StdEncoding.DecodeString(command.Spec.DataBase64)
	if command.UUID == "" || !validFilePath(command.Spec.Path) || decodeErr != nil || len(data) > maxGuestAgentWriteBytes ||
		(command.Spec.Mode != "replace" && command.Spec.Mode != "append") || (command.Spec.Mode == "replace" && !command.Spec.Overwrite) {
		return FileWriteResponse{}, domain.ErrInvalidInput
	}
	sum := sha256.Sum256(data)
	detail := map[string]any{
		"path": command.Spec.Path, "mode": command.Spec.Mode, "bytes": len(data), "sha256": hex.EncodeToString(sum[:]),
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return FileWriteResponse{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.file.write", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return FileWriteResponse{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.file.write", command.UUID, detail, domain.ErrReadOnly)
	}
	var result domain.GuestAgentFileWriteResult
	task, err := s.runTask(ctx, command.Request, "guest-agent.file.write", "vm.guest_agent.file.write", command.UUID, "写入客户机文件", detail, "客户机文件写入完成", func() (any, error) {
		var opErr error
		result, opErr = s.backend.WriteGuestAgentFile(ctx, command.UUID, command.Spec)
		return result, opErr
	})
	return FileWriteResponse{Task: task, Result: result}, err
}

func (s *Service) ResetPassword(ctx context.Context, command PasswordCommand) (domain.Task, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Spec.Username = strings.TrimSpace(command.Spec.Username)
	detail := map[string]any{"username": command.Spec.Username}
	if command.UUID == "" || !domain.ValidGuestAgentUsername(command.Spec.Username) || len(command.Spec.Password) == 0 || len(command.Spec.Password) > 256 {
		return domain.Task{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.Task{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.password.reset", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Task{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.password.reset", command.UUID, detail, domain.ErrReadOnly)
	}
	return s.runTask(ctx, command.Request, "guest-agent.password.reset", "vm.guest_agent.password.reset", command.UUID, "重置客户机用户密码", detail, "客户机用户密码已重置", func() (any, error) {
		return nil, s.backend.ResetGuestAgentPassword(ctx, command.UUID, command.Spec)
	})
}

func (s *Service) ListSSHKeys(ctx context.Context, command SSHKeyListCommand) (domain.GuestAgentAuthorizedKeys, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Username = strings.TrimSpace(command.Username)
	detail := map[string]any{"username": command.Username}
	if command.UUID == "" || !domain.ValidGuestAgentUsername(command.Username) {
		return domain.GuestAgentAuthorizedKeys{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.GuestAgentAuthorizedKeys{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.ssh_keys.list", command.UUID, detail, domain.ErrForbidden)
	}
	keys, err := s.backend.ListGuestAgentAuthorizedKeys(ctx, command.UUID, command.Username)
	if err != nil {
		return domain.GuestAgentAuthorizedKeys{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.ssh_keys.list", command.UUID, detail, err)
	}
	detail["keyCount"] = len(keys.Keys)
	detail["fingerprints"] = sshKeyFingerprints(keys.Keys)
	detail["sensitive"] = true
	if err := s.recordAudit(ctx, command.Request, "vm.guest_agent.ssh_keys.list", command.UUID, "success", detail, nil); err != nil {
		return domain.GuestAgentAuthorizedKeys{}, fmt.Errorf("record guest agent SSH key list audit: %w", err)
	}
	return keys, nil
}

func (s *Service) AddSSHKeys(ctx context.Context, command SSHKeyChangeCommand) (domain.Task, error) {
	return s.changeSSHKeys(ctx, command, true)
}

func (s *Service) RemoveSSHKeys(ctx context.Context, command SSHKeyChangeCommand) (domain.Task, error) {
	return s.changeSSHKeys(ctx, command, false)
}

func (s *Service) changeSSHKeys(ctx context.Context, command SSHKeyChangeCommand, add bool) (domain.Task, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Spec.Username = strings.TrimSpace(command.Spec.Username)
	verb := "remove"
	action := "vm.guest_agent.ssh_keys.remove"
	step := "移除客户机 SSH 公钥"
	successStep := "客户机 SSH 公钥已移除"
	if add {
		verb = "add"
		action = "vm.guest_agent.ssh_keys.add"
		step = "添加客户机 SSH 公钥"
		successStep = "客户机 SSH 公钥已添加"
	}
	detail := map[string]any{
		"username": command.Spec.Username, "keyCount": len(command.Spec.Keys),
		"fingerprints": sshKeyFingerprints(command.Spec.Keys),
	}
	if command.UUID == "" || !domain.ValidGuestAgentUsername(command.Spec.Username) || len(command.Spec.Keys) == 0 || len(command.Spec.Keys) > maxGuestAgentSSHKeyCount {
		return domain.Task{}, domain.ErrInvalidInput
	}
	for _, key := range command.Spec.Keys {
		if strings.TrimSpace(key) == "" || len(key) > 16<<10 || strings.ContainsRune(key, '\x00') {
			return domain.Task{}, domain.ErrInvalidInput
		}
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.Task{}, s.recordFailure(ctx, command.Request, action, command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Task{}, s.recordFailure(ctx, command.Request, action, command.UUID, detail, domain.ErrReadOnly)
	}
	return s.runTask(ctx, command.Request, "guest-agent.ssh-keys."+verb, action, command.UUID, step, detail, successStep, func() (any, error) {
		if add {
			return nil, s.backend.AddGuestAgentAuthorizedKeys(ctx, command.UUID, command.Spec)
		}
		return nil, s.backend.RemoveGuestAgentAuthorizedKeys(ctx, command.UUID, command.Spec)
	})
}

func (s *Service) ListProcesses(ctx context.Context, command ProcessCommand) ([]domain.GuestAgentProcess, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	if command.UUID == "" {
		return nil, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return nil, s.recordFailure(ctx, command.Request, "vm.guest_agent.processes.list", command.UUID, nil, domain.ErrForbidden)
	}
	processes, err := s.backend.ListGuestAgentProcesses(ctx, command.UUID)
	if err != nil {
		return nil, s.recordFailure(ctx, command.Request, "vm.guest_agent.processes.list", command.UUID, nil, err)
	}
	return processes, nil
}

func (s *Service) SignalProcess(ctx context.Context, command ProcessSignalCommand) (domain.Task, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	signal, ok := domain.ValidGuestAgentProcessSignal(command.Spec.Signal)
	if !ok {
		return domain.Task{}, domain.ErrInvalidInput
	}
	command.Spec.Signal = signal
	detail := map[string]any{"pid": command.Spec.PID, "signal": signal}
	if command.UUID == "" || command.Spec.PID <= 1 {
		return domain.Task{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.Task{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.process.signal", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Task{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.process.signal", command.UUID, detail, domain.ErrReadOnly)
	}
	return s.runTask(ctx, command.Request, "guest-agent.process.signal", "vm.guest_agent.process.signal", command.UUID, "向客户机进程发送信号", detail, "客户机进程信号已发送", func() (any, error) {
		return nil, s.backend.SignalGuestAgentProcess(ctx, command.UUID, command.Spec)
	})
}

func (s *Service) ListUsers(ctx context.Context, command ProcessCommand) ([]domain.GuestAgentUser, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	if command.UUID == "" {
		return nil, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return nil, s.recordFailure(ctx, command.Request, "vm.guest_agent.users.list", command.UUID, nil, domain.ErrForbidden)
	}
	users, err := s.backend.ListGuestAgentUsers(ctx, command.UUID)
	if err != nil {
		return nil, s.recordFailure(ctx, command.Request, "vm.guest_agent.users.list", command.UUID, nil, err)
	}
	return users, nil
}

func (s *Service) GetPolicy(ctx context.Context, query PolicyQuery) (domain.GuestAgentPolicy, error) {
	query.UUID = strings.TrimSpace(query.UUID)
	if query.UUID == "" {
		return domain.GuestAgentPolicy{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(query.Request.Session.User.Role) {
		return domain.GuestAgentPolicy{}, s.recordFailure(ctx, query.Request, "vm.guest_agent.policy.get", query.UUID, nil, domain.ErrForbidden)
	}
	if s.policies == nil {
		return domain.GuestAgentPolicy{}, domain.ErrUnavailable
	}
	policy, err := s.policies.GetGuestAgentPolicy(ctx, query.UUID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.GuestAgentPolicy{
			VMUUID: query.UUID, Expected: true, LastState: domain.GuestAgentStateUnknown,
			UpdatedAt: time.Now().UTC(),
		}, nil
	}
	return policy, err
}

func (s *Service) SetPolicy(ctx context.Context, command PolicyCommand) (domain.GuestAgentPolicy, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	detail := map[string]any{"expected": command.Spec.Expected}
	if command.UUID == "" {
		return domain.GuestAgentPolicy{}, domain.ErrInvalidInput
	}
	if command.Request.Session.User.Role != domain.RoleAdmin {
		return domain.GuestAgentPolicy{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.policy.update", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.GuestAgentPolicy{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.policy.update", command.UUID, detail, domain.ErrReadOnly)
	}
	if s.policies == nil {
		return domain.GuestAgentPolicy{}, domain.ErrUnavailable
	}
	now := time.Now().UTC()
	policy, err := s.policies.UpsertGuestAgentPolicy(ctx, domain.GuestAgentPolicy{
		VMUUID: command.UUID, Expected: command.Spec.Expected, LastState: domain.GuestAgentStateUnknown,
		UpdatedBy: command.Request.Session.User.Username, UpdatedAt: now,
	})
	if err != nil {
		return domain.GuestAgentPolicy{}, s.recordFailure(ctx, command.Request, "vm.guest_agent.policy.update", command.UUID, detail, err)
	}
	detail["lastState"] = policy.LastState
	if err := s.recordAudit(ctx, command.Request, "vm.guest_agent.policy.update", command.UUID, "success", detail, nil); err != nil {
		return domain.GuestAgentPolicy{}, fmt.Errorf("record guest agent policy audit: %w", err)
	}
	return policy, nil
}

func (s *Service) Health(ctx context.Context, query HealthQuery) (domain.GuestAgentHealthHistory, error) {
	query.UUID = strings.TrimSpace(query.UUID)
	if query.UUID == "" || s.policies == nil {
		return domain.GuestAgentHealthHistory{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(query.Request.Session.User.Role) {
		return domain.GuestAgentHealthHistory{}, s.recordFailure(ctx, query.Request, "vm.guest_agent.health.get", query.UUID, nil, domain.ErrForbidden)
	}
	if query.Hours < 1 || query.Hours > 24*30 {
		query.Hours = 24
	}
	if query.Limit < 1 || query.Limit > 1000 {
		query.Limit = 288
	}
	samples, err := s.policies.ListGuestAgentHealthSamples(ctx, query.UUID, time.Now().UTC().Add(-time.Duration(query.Hours)*time.Hour), query.Limit)
	if err != nil {
		return domain.GuestAgentHealthHistory{}, err
	}
	connected := 0
	for _, sample := range samples {
		if sample.Connected {
			connected++
		}
	}
	percent := 0.0
	if len(samples) > 0 {
		percent = float64(connected) * 100 / float64(len(samples))
	}
	return domain.GuestAgentHealthHistory{Items: samples, Total: len(samples), ConnectivityPercent: percent}, nil
}

func (s *Service) ListPolicies(ctx context.Context) ([]domain.GuestAgentPolicy, error) {
	if s.policies == nil {
		return nil, domain.ErrUnavailable
	}
	return s.policies.ListGuestAgentPolicies(ctx)
}

// CheckHealth samples every policy that expects a guest agent. It returns only
// threshold transitions so callers can publish an event without alerting on
// every failed probe.
func (s *Service) CheckHealth(ctx context.Context) ([]HealthChange, error) {
	if s.policies == nil {
		return nil, domain.ErrUnavailable
	}
	policies, err := s.policies.ListGuestAgentPolicies(ctx)
	if err != nil {
		return nil, err
	}
	running := map[string]domain.VMState(nil)
	if inventory, ok := s.backend.(VMInventory); ok {
		if vms, inventoryErr := inventory.ListVMs(ctx); inventoryErr == nil {
			running = make(map[string]domain.VMState, len(vms))
			for _, vm := range vms {
				running[vm.UUID] = vm.State
			}
		}
	}
	changes := make([]HealthChange, 0)
	for _, policy := range policies {
		if !policy.Expected {
			continue
		}
		if running != nil {
			state, exists := running[policy.VMUUID]
			if !exists || (state != domain.VMStateRunning && state != domain.VMStateBlocked) {
				continue
			}
		}
		if err := ctx.Err(); err != nil {
			return changes, err
		}
		started := time.Now()
		info, probeErr := s.backend.GetGuestAgentInfo(ctx, policy.VMUUID, true)
		latency := time.Since(started).Milliseconds()
		connected := probeErr == nil && info.State == domain.GuestAgentStateConnected
		state := info.State
		errorText := ""
		if probeErr != nil {
			state = domain.GuestAgentStateError
			errorText = probeErr.Error()
		} else if info.LastError != "" && !connected {
			errorText = info.LastError
		}
		now := time.Now().UTC()
		_ = s.policies.InsertGuestAgentHealthSample(ctx, domain.GuestAgentHealthSample{
			VMUUID: policy.VMUUID, SampledAt: now, State: state, LatencyMS: latency,
			Connected: connected, Error: errorText,
		})
		previousFailures := policy.ConsecutiveFailures
		policy.LastCheckedAt = &now
		policy.LastState = state
		policy.LastError = errorText
		policy.UpdatedAt = now
		if connected {
			policy.ConsecutiveFailures = 0
			policy.LastHealthyAt = &now
			policy.LastError = ""
			if previousFailures >= 3 {
				changes = append(changes, HealthChange{VMUUID: policy.VMUUID, Kind: "recovered", State: state})
			}
		} else {
			policy.ConsecutiveFailures++
			if policy.ConsecutiveFailures == 3 {
				changes = append(changes, HealthChange{
					VMUUID: policy.VMUUID, Kind: "degraded", State: state,
					Failures: policy.ConsecutiveFailures, LastError: errorText,
				})
			}
		}
		if _, err := s.policies.UpsertGuestAgentPolicy(ctx, policy); err != nil {
			return changes, err
		}
	}
	return changes, nil
}

// EnsureHealthPolicies creates default expectations for configured agents.
// Existing operator choices are never overwritten.
func (s *Service) EnsureHealthPolicies(ctx context.Context) error {
	if s.policies == nil {
		return domain.ErrUnavailable
	}
	inventory, ok := s.backend.(VMInventory)
	if !ok {
		return nil
	}
	vms, err := inventory.ListVMs(ctx)
	if err != nil {
		return err
	}
	for _, vm := range vms {
		if !vm.AgentConfigured {
			continue
		}
		if _, err := s.policies.GetGuestAgentPolicy(ctx, vm.UUID); err == nil {
			continue
		} else if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if _, err := s.policies.UpsertGuestAgentPolicy(ctx, domain.GuestAgentPolicy{
			VMUUID: vm.UUID, Expected: true, LastState: domain.GuestAgentStateUnknown,
			UpdatedBy: "system", UpdatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) PruneHealthHistory(ctx context.Context, before time.Time) error {
	if s.policies == nil {
		return domain.ErrUnavailable
	}
	return s.policies.DeleteGuestAgentHealthSamplesBefore(ctx, before)
}

func (s *Service) runTask(ctx context.Context, request MutationContext, kind, auditAction, uuid, startStep string, detail map[string]any, successStep string, operation func() (any, error)) (domain.Task, error) {
	now := time.Now().UTC()
	task := domain.Task{Kind: kind, TargetKind: "vm", TargetID: uuid, Actor: request.Session.User.Username, CreatedAt: now, UpdatedAt: now}
	if s.tasks != nil {
		id, err := s.tasks.Start(ctx, kind, "vm", uuid, request.Session.User.Username, startStep)
		if err != nil {
			return domain.Task{}, s.recordFailure(ctx, request, auditAction, uuid, detail, err)
		}
		task.ID = id
		task.Status = domain.TaskStatusRunning
		task.Step = startStep
	}
	result, err := operation()
	if err != nil {
		if task.ID != "" {
			_ = s.tasks.Fail(ctx, task.ID, 0, startStep+"失败", err)
		}
		return task, s.recordFailure(ctx, request, auditAction, uuid, detail, err)
	}
	if result != nil {
		switch value := result.(type) {
		case domain.GuestAgentFileWriteResult:
			detail["bytesWritten"] = value.BytesWritten
			detail["sha256"] = value.SHA256
		}
	}
	if err := s.recordAudit(ctx, request, auditAction, uuid, "success", detail, nil); err != nil {
		if task.ID != "" {
			_ = s.tasks.Fail(ctx, task.ID, 100, "操作完成但审计写入失败", err)
		}
		return task, fmt.Errorf("record guest agent task audit: %w", err)
	}
	task.Status = domain.TaskStatusSucceeded
	task.Progress = 100
	task.Step = successStep
	task.UpdatedAt = time.Now().UTC()
	if task.ID != "" {
		if err := s.tasks.Succeed(ctx, task.ID, successStep); err != nil {
			return task, fmt.Errorf("complete guest agent task: %w", err)
		}
	}
	return task, nil
}

func validActionSpec(spec domain.GuestAgentActionSpec) bool {
	switch spec.Action {
	case domain.GuestAgentActionShutdown, domain.GuestAgentActionReboot, domain.GuestAgentActionSyncTime:
		return spec.Mount == "" && spec.MinimumBytes == 0
	case domain.GuestAgentActionTrim:
		return spec.Mount == "" || (filepath.IsAbs(spec.Mount) && !strings.ContainsRune(spec.Mount, '\x00'))
	default:
		return false
	}
}

func validFilePath(path string) bool {
	return path != "" && len(path) <= 4096 && filepath.IsAbs(path) && !strings.ContainsRune(path, '\x00')
}

func sshKeyFingerprints(keys []string) []string {
	fingerprints := make([]string, 0, len(keys))
	for _, key := range keys {
		sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
		fingerprints = append(fingerprints, "SHA256:"+hex.EncodeToString(sum[:]))
	}
	sort.Strings(fingerprints)
	return fingerprints
}

func (s *Service) recordFailure(ctx context.Context, request MutationContext, action, uuid string, detail map[string]any, cause error) error {
	return s.recordAudit(ctx, request, action, uuid, "failure", detail, cause)
}

func (s *Service) recordAudit(ctx context.Context, request MutationContext, action, uuid, result string, detail map[string]any, cause error) error {
	if cause != nil {
		if detail == nil {
			detail = make(map[string]any)
		}
		detail["error"] = cause.Error()
	}
	if s.audit != nil {
		if err := s.audit.RecordAudit(ctx, domain.AuditEvent{
			OccurredAt: time.Now().UTC(), RequestID: request.RequestID,
			Actor: request.Session.User.Username, Role: request.Session.User.Role,
			Action: action, TargetKind: "vm", TargetID: uuid,
			SourceIP: request.SourceIP, Result: result, Detail: detail,
		}); err != nil {
			return errors.Join(cause, fmt.Errorf("record %s audit: %w", action, err))
		}
	}
	return cause
}
