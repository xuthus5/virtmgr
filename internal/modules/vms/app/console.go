package app

import (
	"context"
	"io"
	"net/url"
	"strings"
	"sync"

	"virtmgr/internal/domain"
)

type ConsoleTokenCommand struct {
	UUID    string
	Kind    string
	Device  string
	Request MutationContext
}

type ConsoleOpenCommand struct {
	UUID    string
	Token   string
	Request MutationContext
}

type SendConsoleKeyCommand struct {
	UUID    string
	Key     string
	Request MutationContext
}

func (s *Service) IssueConsoleToken(ctx context.Context, command ConsoleTokenCommand) (domain.ConsoleTicket, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	kindValue := command.Kind
	if strings.TrimSpace(kindValue) == "" {
		kindValue = string(domain.ConsoleKindVNC)
	}
	kind, validKind := domain.ValidConsoleKind(kindValue)
	detail := map[string]any{"vmUuid": command.UUID, "kind": string(kind)}
	if !validKind {
		return domain.ConsoleTicket{}, s.mutationFailure(ctx, command.Request, "vm.console.token", command.UUID, detail, domain.ErrInvalidInput)
	}
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
	device := ""
	switch kind {
	case domain.ConsoleKindVNC:
		index, ok := vncGraphicsIndex(vm)
		if !ok || !consoleState(vm.State) {
			return domain.ConsoleTicket{}, s.mutationFailure(ctx, command.Request, "vm.console.token", command.UUID, detail, domain.ErrConsoleUnavailable)
		}
		detail["graphicsIndex"] = index
	case domain.ConsoleKindSerial:
		serial, ok := serialConsole(vm, command.Device)
		if !ok || !consoleState(vm.State) {
			return domain.ConsoleTicket{}, s.mutationFailure(ctx, command.Request, "vm.console.token", command.UUID, detail, domain.ErrConsoleUnavailable)
		}
		device = serial.Device
		detail["device"] = device
		detail["label"] = serial.Label
	}
	grant, err := s.consoleTokens.Issue(command.UUID, command.Request.Session.User.ID, kind, device)
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
		Path:      consoleTicketPath(command.UUID, kind, grant.Token),
		Kind:      kind,
		Device:    device,
	}, nil
}

func consoleTicketPath(uuid string, kind domain.ConsoleKind, token string) string {
	base := "/api/v1/vms/" + url.PathEscape(uuid)
	if kind == domain.ConsoleKindSerial {
		base += "/serial-console"
	} else {
		base += "/console"
	}
	return base + "?token=" + url.QueryEscape(token)
}

func (s *Service) OpenConsole(ctx context.Context, command ConsoleOpenCommand) (io.ReadWriteCloser, error) {
	grant, vm, err := s.consumeConsoleGrant(ctx, command, domain.ConsoleKindVNC, "vm.console.open")
	if err != nil {
		return nil, err
	}
	index, ok := vncGraphicsIndex(vm)
	if !ok || !consoleState(vm.State) {
		return nil, s.mutationFailure(ctx, command.Request, "vm.console.open", command.UUID, map[string]any{"vmUuid": command.UUID, "kind": "vnc"}, domain.ErrConsoleUnavailable)
	}
	detail := map[string]any{
		"vmUuid":        command.UUID,
		"kind":          "vnc",
		"state":         string(vm.State),
		"graphicsIndex": index,
	}
	connection, err := s.repository.OpenConsole(ctx, command.UUID, uint(index))
	if err != nil {
		return nil, s.mutationFailure(ctx, command.Request, "vm.console.open", command.UUID, detail, err)
	}
	if err := s.recordMutationAudit(ctx, command.Request, "vm.console.open", command.UUID, "success", detail, nil); err != nil {
		_ = connection.Close()
		return nil, err
	}
	_ = grant
	return connection, nil
}

func (s *Service) OpenSerialConsole(ctx context.Context, command ConsoleOpenCommand) (io.ReadWriteCloser, error) {
	grant, vm, err := s.consumeConsoleGrant(ctx, command, domain.ConsoleKindSerial, "vm.serial-console.open")
	if err != nil {
		return nil, err
	}
	serial, ok := serialConsole(vm, grant.Device)
	if !ok || !consoleState(vm.State) {
		return nil, s.mutationFailure(ctx, command.Request, "vm.serial-console.open", command.UUID, map[string]any{"vmUuid": command.UUID, "kind": "serial", "device": grant.Device}, domain.ErrConsoleUnavailable)
	}
	sessionKey := command.UUID + "\x00" + serial.Device
	if !s.reserveSerialSession(sessionKey) {
		return nil, s.mutationFailure(ctx, command.Request, "vm.serial-console.open", command.UUID, map[string]any{"vmUuid": command.UUID, "kind": "serial", "device": serial.Device}, domain.ErrConsoleBusy)
	}
	detail := map[string]any{
		"vmUuid": command.UUID,
		"kind":   "serial",
		"device": serial.Device,
		"label":  serial.Label,
		"state":  string(vm.State),
	}
	connection, err := s.repository.OpenSerialConsole(ctx, command.UUID, serial.Device)
	if err != nil {
		s.releaseSerialSession(sessionKey)
		return nil, s.mutationFailure(ctx, command.Request, "vm.serial-console.open", command.UUID, detail, err)
	}
	if err := s.recordMutationAudit(ctx, command.Request, "vm.serial-console.open", command.UUID, "success", detail, nil); err != nil {
		_ = connection.Close()
		s.releaseSerialSession(sessionKey)
		return nil, err
	}
	return &serialConsoleConnection{ReadWriteCloser: connection, release: func() { s.releaseSerialSession(sessionKey) }}, nil
}

func (s *Service) SendConsoleKey(ctx context.Context, command SendConsoleKeyCommand) error {
	command.UUID = strings.TrimSpace(command.UUID)
	detail := map[string]any{"vmUuid": command.UUID, "key": strings.TrimSpace(command.Key)}
	keycodes, ok := domain.ConsoleKeycodes(command.Key)
	if !ok {
		return s.mutationFailure(ctx, command.Request, "vm.console.send-key", command.UUID, detail, domain.ErrInvalidInput)
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return s.mutationFailure(ctx, command.Request, "vm.console.send-key", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return s.mutationFailure(ctx, command.Request, "vm.console.send-key", command.UUID, detail, domain.ErrReadOnly)
	}
	if command.UUID == "" {
		return s.mutationFailure(ctx, command.Request, "vm.console.send-key", command.UUID, detail, domain.ErrInvalidInput)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return s.mutationFailure(ctx, command.Request, "vm.console.send-key", command.UUID, detail, err)
	}
	if _, ok := vncGraphicsIndex(vm); !ok || !consoleState(vm.State) {
		return s.mutationFailure(ctx, command.Request, "vm.console.send-key", command.UUID, detail, domain.ErrConsoleUnavailable)
	}
	if err := s.repository.SendConsoleKey(ctx, command.UUID, keycodes); err != nil {
		return s.mutationFailure(ctx, command.Request, "vm.console.send-key", command.UUID, detail, err)
	}
	return s.recordMutationAudit(ctx, command.Request, "vm.console.send-key", command.UUID, "success", detail, nil)
}

func (s *Service) consumeConsoleGrant(ctx context.Context, command ConsoleOpenCommand, kind domain.ConsoleKind, action string) (domain.ConsoleToken, domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Token = strings.TrimSpace(command.Token)
	detail := map[string]any{"vmUuid": command.UUID, "kind": string(kind)}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.ConsoleToken{}, domain.VM{}, s.mutationFailure(ctx, command.Request, action, command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.ConsoleToken{}, domain.VM{}, s.mutationFailure(ctx, command.Request, action, command.UUID, detail, domain.ErrReadOnly)
	}
	if s.consoleTokens == nil || command.UUID == "" || command.Token == "" {
		return domain.ConsoleToken{}, domain.VM{}, s.mutationFailure(ctx, command.Request, action, command.UUID, detail, domain.ErrConsoleToken)
	}
	grant, err := s.consoleTokens.Consume(command.Token)
	if err != nil || grant.VMUUID != command.UUID || grant.UserID != command.Request.Session.User.ID || grant.Kind != kind {
		return domain.ConsoleToken{}, domain.VM{}, s.mutationFailure(ctx, command.Request, action, command.UUID, detail, domain.ErrConsoleToken)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.ConsoleToken{}, domain.VM{}, s.mutationFailure(ctx, command.Request, action, command.UUID, detail, err)
	}
	return grant, vm, nil
}

func (s *Service) reserveSerialSession(key string) bool {
	s.serialMu.Lock()
	defer s.serialMu.Unlock()
	if s.serialSessions == nil {
		s.serialSessions = make(map[string]struct{})
	}
	if _, exists := s.serialSessions[key]; exists {
		return false
	}
	s.serialSessions[key] = struct{}{}
	return true
}

func (s *Service) releaseSerialSession(key string) {
	s.serialMu.Lock()
	defer s.serialMu.Unlock()
	delete(s.serialSessions, key)
}

type serialConsoleConnection struct {
	io.ReadWriteCloser
	once    sync.Once
	release func()
}

func (c *serialConsoleConnection) Close() error {
	err := c.ReadWriteCloser.Close()
	c.once.Do(c.release)
	return err
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

func serialConsole(vm domain.VM, device string) (domain.SerialConsole, bool) {
	device = strings.TrimSpace(device)
	for _, serial := range vm.Serials {
		if device == "" || serial.Device == device {
			return serial, true
		}
	}
	return domain.SerialConsole{}, false
}
