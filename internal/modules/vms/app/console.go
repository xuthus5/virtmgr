package app

import (
	"context"
	"io"
	"strings"

	"virtmgr/internal/domain"
)

type ConsoleTokenCommand struct {
	UUID    string
	Request MutationContext
}

type ConsoleOpenCommand struct {
	UUID    string
	Token   string
	Request MutationContext
}

func (s *Service) IssueConsoleToken(ctx context.Context, command ConsoleTokenCommand) (domain.ConsoleTicket, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	detail := map[string]any{"vmUuid": command.UUID}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.ConsoleTicket{}, s.mutationFailure(ctx, command.Request, "vm.console.token", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.ConsoleTicket{}, s.mutationFailure(ctx, command.Request, "vm.console.token", command.UUID, detail, domain.ErrReadOnly)
	}
	if s.consoleTokens == nil {
		return domain.ConsoleTicket{}, s.mutationFailure(ctx, command.Request, "vm.console.token", command.UUID, detail, domain.ErrUnavailable)
	}
	if command.UUID == "" {
		return domain.ConsoleTicket{}, s.mutationFailure(ctx, command.Request, "vm.console.token", command.UUID, detail, domain.ErrInvalidInput)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.ConsoleTicket{}, s.mutationFailure(ctx, command.Request, "vm.console.token", command.UUID, detail, err)
	}
	if !consoleCapable(vm) {
		return domain.ConsoleTicket{}, s.mutationFailure(ctx, command.Request, "vm.console.token", command.UUID, detail, domain.ErrConsoleUnavailable)
	}
	grant, err := s.consoleTokens.Issue(command.UUID, command.Request.Session.User.ID)
	if err != nil {
		return domain.ConsoleTicket{}, s.mutationFailure(ctx, command.Request, "vm.console.token", command.UUID, detail, err)
	}
	detail["state"] = string(vm.State)
	if err := s.recordMutationAudit(ctx, command.Request, "vm.console.token", command.UUID, "success", detail, nil); err != nil {
		return domain.ConsoleTicket{}, err
	}
	return domain.ConsoleTicket{
		Token:     grant.Token,
		ExpiresAt: grant.ExpiresAt,
		Path:      "/api/v1/vms/" + command.UUID + "/console?token=" + grant.Token,
	}, nil
}

func (s *Service) OpenConsole(ctx context.Context, command ConsoleOpenCommand) (io.ReadWriteCloser, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Token = strings.TrimSpace(command.Token)
	detail := map[string]any{"vmUuid": command.UUID}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return nil, s.mutationFailure(ctx, command.Request, "vm.console.open", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return nil, s.mutationFailure(ctx, command.Request, "vm.console.open", command.UUID, detail, domain.ErrReadOnly)
	}
	if s.consoleTokens == nil || command.UUID == "" || command.Token == "" {
		return nil, s.mutationFailure(ctx, command.Request, "vm.console.open", command.UUID, detail, domain.ErrConsoleToken)
	}
	grant, err := s.consoleTokens.Consume(command.Token)
	if err != nil {
		return nil, s.mutationFailure(ctx, command.Request, "vm.console.open", command.UUID, detail, domain.ErrConsoleToken)
	}
	if grant.VMUUID != command.UUID || grant.UserID != command.Request.Session.User.ID {
		return nil, s.mutationFailure(ctx, command.Request, "vm.console.open", command.UUID, detail, domain.ErrConsoleToken)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return nil, s.mutationFailure(ctx, command.Request, "vm.console.open", command.UUID, detail, err)
	}
	index, ok := vncGraphicsIndex(vm)
	if !ok || !consoleState(vm.State) {
		return nil, s.mutationFailure(ctx, command.Request, "vm.console.open", command.UUID, detail, domain.ErrConsoleUnavailable)
	}
	detail["state"] = string(vm.State)
	detail["graphicsIndex"] = index
	connection, err := s.repository.OpenConsole(ctx, command.UUID, uint(index))
	if err != nil {
		return nil, s.mutationFailure(ctx, command.Request, "vm.console.open", command.UUID, detail, err)
	}
	if err := s.recordMutationAudit(ctx, command.Request, "vm.console.open", command.UUID, "success", detail, nil); err != nil {
		_ = connection.Close()
		return nil, err
	}
	return connection, nil
}

func consoleCapable(vm domain.VM) bool {
	_, ok := vncGraphicsIndex(vm)
	return ok && consoleState(vm.State)
}

func consoleState(state domain.VMState) bool {
	return state == domain.VMStateRunning || state == domain.VMStateBlocked || state == domain.VMStatePaused
}

func vncGraphicsIndex(vm domain.VM) (int, bool) {
	for index, graphics := range vm.Graphics {
		if strings.EqualFold(graphics.Type, "vnc") {
			return index, true
		}
	}
	return 0, false
}
