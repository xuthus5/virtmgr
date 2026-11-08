package app

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
)

type fakeMode struct {
	readOnly bool
	err      error
}

func (m *fakeMode) ReadOnly() bool { return m.readOnly }
func (m *fakeMode) SetReadOnly(_ context.Context, readOnly bool) error {
	if m.err != nil {
		return m.err
	}
	m.readOnly = readOnly
	return nil
}

type fakeAudit struct {
	events []domain.AuditEvent
	err    error
}

func (f *fakeAudit) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	f.events = append(f.events, event)
	return f.err
}

func adminRequest() MutationContext {
	return MutationContext{
		Session:   domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}},
		RequestID: "req-settings",
		SourceIP:  "127.0.0.1",
	}
}

func TestUpdateReadOnlyPersistsAndAudits(t *testing.T) {
	mode := &fakeMode{}
	audit := &fakeAudit{}
	service := New(mode, audit)
	if service.Get().ReadOnly {
		t.Fatal("initial mode is read-only")
	}
	result, err := service.Update(context.Background(), UpdateCommand{ReadOnly: true, Request: adminRequest()})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !result.ReadOnly || !mode.ReadOnly() {
		t.Fatalf("result=%+v mode=%v", result, mode.ReadOnly())
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	event := audit.events[0]
	if event.Action != "settings.readonly.update" || event.TargetKind != "settings" || event.TargetID != "read_only" || event.Result != "success" {
		t.Fatalf("audit event = %+v", event)
	}
	if event.Detail["previousReadOnly"] != false || event.Detail["newReadOnly"] != true {
		t.Fatalf("audit detail = %+v", event.Detail)
	}
}

func TestUpdateReadOnlyRejectsNonAdmin(t *testing.T) {
	mode := &fakeMode{readOnly: false}
	audit := &fakeAudit{}
	service := New(mode, audit)
	request := adminRequest()
	request.Session.User.Role = domain.RoleOperator
	if _, err := service.Update(context.Background(), UpdateCommand{ReadOnly: true, Request: request}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Update() error = %v", err)
	}
	if mode.ReadOnly() || len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("mode=%v audit=%+v", mode.ReadOnly(), audit.events)
	}
}

func TestUpdateReadOnlyAuditsPersistenceFailure(t *testing.T) {
	want := errors.New("database unavailable")
	audit := &fakeAudit{}
	service := New(&fakeMode{err: want}, audit)
	if _, err := service.Update(context.Background(), UpdateCommand{ReadOnly: true, Request: adminRequest()}); !errors.Is(err, want) {
		t.Fatalf("Update() error = %v", err)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}
