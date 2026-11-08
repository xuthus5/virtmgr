package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

type xmlEditRepository struct {
	fakeVMRepository
	definedUUID string
	definedXML  string
	defineErr   error
	calls       int
}

func (f *xmlEditRepository) DefineVMXML(_ context.Context, uuid, xmlText string) (domain.VMXML, error) {
	f.calls++
	f.definedUUID = uuid
	f.definedXML = xmlText
	if f.defineErr != nil {
		return domain.VMXML{}, f.defineErr
	}
	for _, vm := range f.vms {
		if vm.UUID == uuid {
			return domain.VMXML{UUID: vm.UUID, Name: vm.Name, Format: domain.VMXMLFormatConfig, XML: xmlText}, nil
		}
	}
	return domain.VMXML{}, domain.ErrNotFound
}

func adminSession() domain.Session {
	return domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}
}

func updateXMLFor(name, uuid string) string {
	return `<?xml version="1.0"?>
<domain type="kvm">
  <name>` + name + `</name>
  <uuid>` + uuid + `</uuid>
  <vcpu>3</vcpu>
</domain>`
}

func TestUpdateXMLRecordsSuccessAndNormalizesResult(t *testing.T) {
	vm := persistentVM()
	repository := &xmlEditRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{vm}}}
	audit := &fakeAuditRecorder{}
	xmlText := "  " + updateXMLFor(vm.Name, vm.UUID) + "\n"
	result, err := New(repository, audit, false).UpdateXML(context.Background(), XMLUpdateCommand{
		UUID: " " + vm.UUID + " ", XML: xmlText,
		Request: MutationContext{Session: adminSession(), RequestID: "req-xml", SourceIP: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("UpdateXML() error = %v", err)
	}
	if result.Format != domain.VMXMLFormatConfig || result.UUID != vm.UUID || result.XML != strings.TrimSpace(xmlText) {
		t.Fatalf("result = %+v", result)
	}
	if repository.definedUUID != vm.UUID || repository.definedXML != strings.TrimSpace(xmlText) || repository.calls != 1 {
		t.Fatalf("repository uuid=%q xml=%q calls=%d", repository.definedUUID, repository.definedXML, repository.calls)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	event := audit.events[0]
	if event.Action != "vm.xml.update" || event.TargetKind != "vm" || event.TargetID != vm.UUID || event.Result != "success" || event.Actor != "admin" {
		t.Fatalf("audit event = %+v", event)
	}
	if event.Detail["name"] != vm.Name || event.Detail["persistent"] != true || event.Detail["xmlBytes"] != len(strings.TrimSpace(xmlText)) {
		t.Fatalf("audit detail = %+v", event.Detail)
	}
	if digest, _ := event.Detail["xmlSha256"].(string); len(digest) != 64 {
		t.Fatalf("xml digest = %q", digest)
	}
}

func TestUpdateXMLRejectsInvalidRequests(t *testing.T) {
	vm := persistentVM()
	viewer := adminSession()
	viewer.User.Role = domain.RoleViewer
	operator := adminSession()
	operator.User.Role = domain.RoleOperator
	transient := vm
	transient.Persistent = false

	tests := []struct {
		name     string
		readOnly bool
		vms      []domain.VM
		xml      string
		session  domain.Session
		want     error
	}{
		{name: "viewer", vms: []domain.VM{vm}, xml: updateXMLFor(vm.Name, vm.UUID), session: viewer, want: domain.ErrForbidden},
		{name: "operator", vms: []domain.VM{vm}, xml: updateXMLFor(vm.Name, vm.UUID), session: operator, want: domain.ErrForbidden},
		{name: "read only", readOnly: true, vms: []domain.VM{vm}, xml: updateXMLFor(vm.Name, vm.UUID), session: adminSession(), want: domain.ErrReadOnly},
		{name: "transient VM", vms: []domain.VM{transient}, xml: updateXMLFor(transient.Name, transient.UUID), session: adminSession(), want: domain.ErrActionNotAllowed},
		{name: "name mismatch", vms: []domain.VM{vm}, xml: updateXMLFor("other-vm", vm.UUID), session: adminSession(), want: domain.ErrInvalidInput},
		{name: "uuid mismatch", vms: []domain.VM{vm}, xml: updateXMLFor(vm.Name, "uuid-other"), session: adminSession(), want: domain.ErrInvalidInput},
		{name: "qemu commandline", vms: []domain.VM{vm}, xml: `<domain xmlns:qemu="http://libvirt.org/schemas/domain/qemu/1.0"><name>` + vm.Name + `</name><uuid>` + vm.UUID + `</uuid><qemu:commandline/></domain>`, session: adminSession(), want: domain.ErrInvalidInput},
		{name: "malformed XML", vms: []domain.VM{vm}, xml: `<domain><name>` + vm.Name + `</name>`, session: adminSession(), want: domain.ErrInvalidInput},
		{name: "too large", vms: []domain.VM{vm}, xml: strings.Repeat("x", maxVMXMLBytes+1), session: adminSession(), want: domain.ErrTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &xmlEditRepository{fakeVMRepository: fakeVMRepository{vms: test.vms}}
			audit := &fakeAuditRecorder{}
			_, err := New(repository, audit, test.readOnly).UpdateXML(context.Background(), XMLUpdateCommand{
				UUID: vm.UUID, XML: test.xml, Request: MutationContext{Session: test.session},
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if repository.calls != 0 {
				t.Fatalf("backend calls = %d", repository.calls)
			}
			if len(audit.events) != 1 || audit.events[0].Action != "vm.xml.update" || audit.events[0].Result != "failure" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestUpdateXMLValidatesRequiredInputWithoutAudit(t *testing.T) {
	service := New(&xmlEditRepository{fakeVMRepository: fakeVMRepository{vms: []domain.VM{persistentVM()}}}, &fakeAuditRecorder{}, false)
	for _, command := range []XMLUpdateCommand{
		{UUID: " ", XML: updateXMLFor("api-01", "uuid-a"), Request: MutationContext{Session: adminSession()}},
		{UUID: "uuid-a", XML: " ", Request: MutationContext{Session: adminSession()}},
	} {
		if _, err := service.UpdateXML(context.Background(), command); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestUpdateXMLAuditsBackendFailureAndFailsClosedOnAuditError(t *testing.T) {
	vm := persistentVM()
	repository := &xmlEditRepository{
		fakeVMRepository: fakeVMRepository{vms: []domain.VM{vm}},
		defineErr:        domain.ErrUnavailable,
	}
	audit := &fakeAuditRecorder{}
	_, err := New(repository, audit, false).UpdateXML(context.Background(), XMLUpdateCommand{
		UUID: vm.UUID, XML: updateXMLFor(vm.Name, vm.UUID), Request: MutationContext{Session: adminSession()},
	})
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("audit events = %+v", audit.events)
	}

	repository.defineErr = nil
	_, err = New(repository, &fakeAuditRecorder{err: errors.New("audit unavailable")}, false).UpdateXML(context.Background(), XMLUpdateCommand{
		UUID: vm.UUID, XML: updateXMLFor(vm.Name, vm.UUID), Request: MutationContext{Session: adminSession()},
	})
	if err == nil || !strings.Contains(err.Error(), "record VM XML update audit") {
		t.Fatalf("error = %v", err)
	}
}
