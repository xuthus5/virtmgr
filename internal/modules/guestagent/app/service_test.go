package app

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
)

type fakeBackend struct {
	info        domain.GuestAgentInfo
	actionSpec  domain.GuestAgentActionSpec
	actionCalls int
	channelErr  error
}

func (f *fakeBackend) GetGuestAgentInfo(context.Context, string, bool) (domain.GuestAgentInfo, error) {
	return f.info, nil
}

func (f *fakeBackend) PerformGuestAgentAction(_ context.Context, _ string, spec domain.GuestAgentActionSpec) error {
	f.actionCalls++
	f.actionSpec = spec
	return nil
}

func (f *fakeBackend) EnsureGuestAgentChannel(context.Context, string) (domain.VM, error) {
	if f.channelErr != nil {
		return domain.VM{}, f.channelErr
	}
	return domain.VM{UUID: "vm-1"}, nil
}

type fakeAudit struct {
	events []domain.AuditEvent
}

func (f *fakeAudit) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	f.events = append(f.events, event)
	return nil
}

func operatorRequest() MutationContext {
	return MutationContext{Session: domain.Session{User: domain.User{Username: "operator", Role: domain.RoleOperator}}}
}

func TestGuestAgentActionPermissionsAndReadOnly(t *testing.T) {
	backend := &fakeBackend{}
	audit := &fakeAudit{}
	service := New(backend, audit, false)

	request := operatorRequest()
	request.Session.User.Role = domain.RoleViewer
	if err := service.Action(context.Background(), ActionCommand{UUID: "vm-1", Spec: domain.GuestAgentActionSpec{Action: domain.GuestAgentActionSyncTime}, Request: request}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("viewer action error = %v", err)
	}
	if backend.actionCalls != 0 {
		t.Fatalf("viewer reached backend %d times", backend.actionCalls)
	}

	readOnly := New(backend, audit, true)
	if err := readOnly.Action(context.Background(), ActionCommand{UUID: "vm-1", Spec: domain.GuestAgentActionSpec{Action: domain.GuestAgentActionSyncTime}, Request: operatorRequest()}); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only action error = %v", err)
	}
}

func TestGuestAgentActionValidationAndAudit(t *testing.T) {
	backend := &fakeBackend{}
	audit := &fakeAudit{}
	service := New(backend, audit, false)
	if err := service.Action(context.Background(), ActionCommand{UUID: "vm-1", Spec: domain.GuestAgentActionSpec{Action: "arbitrary-command"}, Request: operatorRequest()}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid action error = %v", err)
	}
	if err := service.Action(context.Background(), ActionCommand{UUID: "vm-1", Spec: domain.GuestAgentActionSpec{Action: domain.GuestAgentActionTrim, Mount: "relative"}, Request: operatorRequest()}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("relative trim mount error = %v", err)
	}
	spec := domain.GuestAgentActionSpec{Action: domain.GuestAgentActionTrim, Mount: "/", MinimumBytes: 4096}
	if err := service.Action(context.Background(), ActionCommand{UUID: "vm-1", Spec: spec, Request: operatorRequest()}); err != nil {
		t.Fatal(err)
	}
	if backend.actionCalls != 1 || backend.actionSpec.Mount != "/" || backend.actionSpec.MinimumBytes != 4096 {
		t.Fatalf("backend action = %+v calls=%d", backend.actionSpec, backend.actionCalls)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.guest_agent.action" || audit.events[0].Result != "success" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestGuestAgentActionNormalizesName(t *testing.T) {
	backend := &fakeBackend{}
	service := New(backend, nil, false)
	if err := service.Action(context.Background(), ActionCommand{
		UUID: "vm-1", Spec: domain.GuestAgentActionSpec{Action: " SYNC-TIME "}, Request: operatorRequest(),
	}); err != nil {
		t.Fatal(err)
	}
	if backend.actionSpec.Action != domain.GuestAgentActionSyncTime {
		t.Fatalf("normalized action = %q", backend.actionSpec.Action)
	}
}

func TestGuestAgentChannelAudit(t *testing.T) {
	backend := &fakeBackend{}
	audit := &fakeAudit{}
	service := New(backend, audit, false)
	vm, err := service.EnsureChannel(context.Background(), ChannelCommand{UUID: "vm-1", Request: operatorRequest()})
	if err != nil || vm.UUID != "vm-1" {
		t.Fatalf("EnsureChannel() = %+v, %v", vm, err)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.guest_agent.channel.add" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}
