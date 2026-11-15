package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

type VMBackend interface {
	GetGuestAgentInfo(ctx context.Context, uuid string, refresh bool) (domain.GuestAgentInfo, error)
	PerformGuestAgentAction(ctx context.Context, uuid string, spec domain.GuestAgentActionSpec) error
	EnsureGuestAgentChannel(ctx context.Context, uuid string) (domain.VM, error)
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
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

type Service struct {
	backend      VMBackend
	audit        AuditRecorder
	readOnly     bool
	readOnlyMode domain.ReadOnlyProvider
}

type Option func(*Service)

func WithReadOnlyMode(mode domain.ReadOnlyProvider) Option {
	return func(service *Service) {
		service.readOnlyMode = mode
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

func (s *Service) Action(ctx context.Context, command ActionCommand) error {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Spec.Mount = strings.TrimSpace(command.Spec.Mount)
	action, ok := domain.ValidGuestAgentAction(string(command.Spec.Action))
	if !ok {
		return domain.ErrInvalidInput
	}
	command.Spec.Action = action
	detail := map[string]any{
		"action": command.Spec.Action, "mount": command.Spec.Mount, "minimumBytes": command.Spec.MinimumBytes,
	}
	if command.UUID == "" || !validActionSpec(command.Spec) {
		return domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return s.recordFailure(ctx, command.Request, "vm.guest_agent.action", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return s.recordFailure(ctx, command.Request, "vm.guest_agent.action", command.UUID, detail, domain.ErrReadOnly)
	}
	if err := s.backend.PerformGuestAgentAction(ctx, command.UUID, command.Spec); err != nil {
		return s.recordFailure(ctx, command.Request, "vm.guest_agent.action", command.UUID, detail, err)
	}
	if err := s.recordAudit(ctx, command.Request, "vm.guest_agent.action", command.UUID, "success", detail, nil); err != nil {
		return fmt.Errorf("record guest agent action audit: %w", err)
	}
	return nil
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
