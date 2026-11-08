package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"virtmgr/internal/domain"
	"virtmgr/internal/platform/consoletoken"
)

type consoleRepository struct {
	fakeVMRepository
	openedIndex uint
	openErr     error
}

func (f *consoleRepository) OpenConsole(_ context.Context, _ string, graphicsIndex uint) (io.ReadWriteCloser, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	f.openedIndex = graphicsIndex
	return consoleTestConn{Reader: strings.NewReader("RFB 003.003\n")}, nil
}

type consoleTestConn struct{ *strings.Reader }

func (consoleTestConn) Write(buffer []byte) (int, error) { return len(buffer), nil }
func (consoleTestConn) Close() error                     { return nil }

func consoleVM() domain.VM {
	return domain.VM{
		UUID: "uuid-console", Name: "console-01", State: domain.VMStateRunning,
		Graphics: []domain.Graphics{{Type: "spice", Port: 5900}, {Type: "vnc", Port: 5901}},
	}
}

func TestIssueConsoleTokenAndOpen(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &consoleRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{consoleVM()}}}
	tokens := consoletoken.New(time.Minute)
	service := New(repository, audit, false, WithConsoleTokens(tokens))
	request := MutationContext{Session: operatorSession(), RequestID: "req-console", SourceIP: "127.0.0.1"}

	ticket, err := service.IssueConsoleToken(context.Background(), ConsoleTokenCommand{UUID: " uuid-console ", Request: request})
	if err != nil {
		t.Fatalf("IssueConsoleToken() error = %v", err)
	}
	if ticket.Token == "" || ticket.ExpiresAt.IsZero() || ticket.Path != "/api/v1/vms/uuid-console/console?token="+ticket.Token {
		t.Fatalf("ticket = %+v", ticket)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.console.token" || audit.events[0].Result != "success" {
		t.Fatalf("audit = %+v", audit.events)
	}

	connection, err := service.OpenConsole(context.Background(), ConsoleOpenCommand{UUID: "uuid-console", Token: ticket.Token, Request: request})
	if err != nil {
		t.Fatalf("OpenConsole() error = %v", err)
	}
	_ = connection.Close()
	if repository.openedIndex != 1 {
		t.Fatalf("openedIndex = %d", repository.openedIndex)
	}
	if len(audit.events) != 2 || audit.events[1].Action != "vm.console.open" || audit.events[1].Detail["graphicsIndex"] != 1 {
		t.Fatalf("audit = %+v", audit.events)
	}

	if _, err := service.OpenConsole(context.Background(), ConsoleOpenCommand{UUID: "uuid-console", Token: ticket.Token, Request: request}); !errors.Is(err, domain.ErrConsoleToken) {
		t.Fatalf("reused token error = %v", err)
	}
}

func TestIssueConsoleTokenRejectsUnavailable(t *testing.T) {
	tests := []struct {
		name     string
		vm       domain.VM
		session  domain.Session
		readOnly bool
		want     error
	}{
		{name: "viewer", vm: consoleVM(), session: domain.Session{User: domain.User{Role: domain.RoleViewer}}, want: domain.ErrForbidden},
		{name: "read only", vm: consoleVM(), session: operatorSession(), readOnly: true, want: domain.ErrReadOnly},
		{name: "shutoff", vm: domain.VM{UUID: "uuid-console", State: domain.VMStateShutoff, Graphics: []domain.Graphics{{Type: "vnc"}}}, session: operatorSession(), want: domain.ErrConsoleUnavailable},
		{name: "no vnc", vm: domain.VM{UUID: "uuid-console", State: domain.VMStateRunning, Graphics: []domain.Graphics{{Type: "spice"}}}, session: operatorSession(), want: domain.ErrConsoleUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			audit := &fakeAuditRecorder{}
			repository := &consoleRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{test.vm}}}
			service := New(repository, audit, test.readOnly, WithConsoleTokens(consoletoken.New(time.Minute)))
			_, err := service.IssueConsoleToken(context.Background(), ConsoleTokenCommand{
				UUID: "uuid-console", Request: MutationContext{Session: test.session},
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("IssueConsoleToken() error = %v, want %v", err, test.want)
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("audit = %+v", audit.events)
			}
		})
	}
}

func TestOpenConsoleRejectsTokenForAnotherUser(t *testing.T) {
	audit := &fakeAuditRecorder{}
	repository := &consoleRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{consoleVM()}}}
	tokens := consoletoken.New(time.Minute)
	service := New(repository, audit, false, WithConsoleTokens(tokens))
	grant, err := tokens.Issue("uuid-console", 99)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.OpenConsole(context.Background(), ConsoleOpenCommand{
		UUID: "uuid-console", Token: grant.Token,
		Request: MutationContext{Session: operatorSession()},
	})
	if !errors.Is(err, domain.ErrConsoleToken) {
		t.Fatalf("OpenConsole() error = %v", err)
	}
}
