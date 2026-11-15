package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type fakeBackend struct {
	info        domain.GuestAgentInfo
	actionSpec  domain.GuestAgentActionSpec
	actionCalls int
	channelErr  error
	fileContent domain.GuestAgentFileContent
	fileRead    domain.GuestAgentFileReadSpec
	fileWrite   domain.GuestAgentFileWriteSpec
	password    domain.GuestAgentPasswordSpec
	sshAdd      domain.GuestAgentSSHKeySpec
	sshRemove   domain.GuestAgentSSHKeySpec
	signal      domain.GuestAgentProcessSignalSpec
	processes   []domain.GuestAgentProcess
	users       []domain.GuestAgentUser
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

func (f *fakeBackend) ReadGuestAgentFile(_ context.Context, _ string, spec domain.GuestAgentFileReadSpec) (domain.GuestAgentFileContent, error) {
	f.fileRead = spec
	return f.fileContent, nil
}

func (f *fakeBackend) WriteGuestAgentFile(_ context.Context, _ string, spec domain.GuestAgentFileWriteSpec) (domain.GuestAgentFileWriteResult, error) {
	f.fileWrite = spec
	return domain.GuestAgentFileWriteResult{Path: spec.Path, BytesWritten: 3, SHA256: "abc"}, nil
}

func (f *fakeBackend) ResetGuestAgentPassword(_ context.Context, _ string, spec domain.GuestAgentPasswordSpec) error {
	f.password = spec
	return nil
}

func (f *fakeBackend) ListGuestAgentAuthorizedKeys(context.Context, string, string) (domain.GuestAgentAuthorizedKeys, error) {
	return domain.GuestAgentAuthorizedKeys{Username: "alice", Keys: []string{"ssh-ed25519 AAAATEST"}}, nil
}

func (f *fakeBackend) AddGuestAgentAuthorizedKeys(_ context.Context, _ string, spec domain.GuestAgentSSHKeySpec) error {
	f.sshAdd = spec
	return nil
}

func (f *fakeBackend) RemoveGuestAgentAuthorizedKeys(_ context.Context, _ string, spec domain.GuestAgentSSHKeySpec) error {
	f.sshRemove = spec
	return nil
}

func (f *fakeBackend) ListGuestAgentProcesses(context.Context, string) ([]domain.GuestAgentProcess, error) {
	return f.processes, nil
}

func (f *fakeBackend) ListGuestAgentServices(context.Context, string) ([]domain.GuestAgentService, error) {
	return []domain.GuestAgentService{{Unit: "nginx.service", Load: "loaded", Active: "active", Sub: "running"}}, nil
}

func (f *fakeBackend) SignalGuestAgentProcess(_ context.Context, _ string, spec domain.GuestAgentProcessSignalSpec) error {
	f.signal = spec
	return nil
}

func (f *fakeBackend) ListGuestAgentUsers(context.Context, string) ([]domain.GuestAgentUser, error) {
	return f.users, nil
}

type fakeTasks struct{ started, succeeded, failed int }

func (f *fakeTasks) Start(context.Context, string, string, string, string, string) (string, error) {
	f.started++
	return "task-1", nil
}
func (f *fakeTasks) Progress(context.Context, string, int, string) error    { return nil }
func (f *fakeTasks) Succeed(context.Context, string, string) error          { f.succeeded++; return nil }
func (f *fakeTasks) Fail(context.Context, string, int, string, error) error { f.failed++; return nil }

type fakePolicies struct {
	items   map[string]domain.GuestAgentPolicy
	samples []domain.GuestAgentHealthSample
}

func (f *fakePolicies) ListGuestAgentPolicies(context.Context) ([]domain.GuestAgentPolicy, error) {
	items := make([]domain.GuestAgentPolicy, 0, len(f.items))
	for _, item := range f.items {
		items = append(items, item)
	}
	return items, nil
}
func (f *fakePolicies) GetGuestAgentPolicy(_ context.Context, uuid string) (domain.GuestAgentPolicy, error) {
	item, ok := f.items[uuid]
	if !ok {
		return domain.GuestAgentPolicy{}, domain.ErrNotFound
	}
	return item, nil
}
func (f *fakePolicies) UpsertGuestAgentPolicy(_ context.Context, policy domain.GuestAgentPolicy) (domain.GuestAgentPolicy, error) {
	f.items[policy.VMUUID] = policy
	return policy, nil
}
func (f *fakePolicies) DeleteGuestAgentPolicy(_ context.Context, uuid string) error {
	delete(f.items, uuid)
	return nil
}
func (f *fakePolicies) InsertGuestAgentHealthSample(_ context.Context, sample domain.GuestAgentHealthSample) error {
	f.samples = append(f.samples, sample)
	return nil
}
func (f *fakePolicies) ListGuestAgentHealthSamples(context.Context, string, time.Time, int) ([]domain.GuestAgentHealthSample, error) {
	return f.samples, nil
}
func (f *fakePolicies) DeleteGuestAgentHealthSamplesBefore(context.Context, time.Time) error {
	return nil
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
	if _, err := service.Action(context.Background(), ActionCommand{UUID: "vm-1", Spec: domain.GuestAgentActionSpec{Action: domain.GuestAgentActionSyncTime}, Request: request}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("viewer action error = %v", err)
	}
	if backend.actionCalls != 0 {
		t.Fatalf("viewer reached backend %d times", backend.actionCalls)
	}

	readOnly := New(backend, audit, true)
	if _, err := readOnly.Action(context.Background(), ActionCommand{UUID: "vm-1", Spec: domain.GuestAgentActionSpec{Action: domain.GuestAgentActionSyncTime}, Request: operatorRequest()}); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only action error = %v", err)
	}
}

func TestGuestAgentActionValidationAndAudit(t *testing.T) {
	backend := &fakeBackend{}
	audit := &fakeAudit{}
	service := New(backend, audit, false)
	if _, err := service.Action(context.Background(), ActionCommand{UUID: "vm-1", Spec: domain.GuestAgentActionSpec{Action: "arbitrary-command"}, Request: operatorRequest()}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid action error = %v", err)
	}
	if _, err := service.Action(context.Background(), ActionCommand{UUID: "vm-1", Spec: domain.GuestAgentActionSpec{Action: domain.GuestAgentActionTrim, Mount: "relative"}, Request: operatorRequest()}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("relative trim mount error = %v", err)
	}
	spec := domain.GuestAgentActionSpec{Action: domain.GuestAgentActionTrim, Mount: "/", MinimumBytes: 4096}
	if _, err := service.Action(context.Background(), ActionCommand{UUID: "vm-1", Spec: spec, Request: operatorRequest()}); err != nil {
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
	if _, err := service.Action(context.Background(), ActionCommand{
		UUID: "vm-1", Spec: domain.GuestAgentActionSpec{Action: " SYNC-TIME "}, Request: operatorRequest(),
	}); err != nil {
		t.Fatal(err)
	}
	if backend.actionSpec.Action != domain.GuestAgentActionSyncTime {
		t.Fatalf("normalized action = %q", backend.actionSpec.Action)
	}
}

func TestGuestAgentSensitiveOperationsAuditWithoutSecrets(t *testing.T) {
	backend := &fakeBackend{}
	audit := &fakeAudit{}
	tasks := &fakeTasks{}
	service := New(backend, audit, false, WithTaskRecorder(tasks))
	request := operatorRequest()

	if _, err := service.ResetPassword(context.Background(), PasswordCommand{
		UUID: "vm-1", Spec: domain.GuestAgentPasswordSpec{Username: "alice", Password: "super-secret"}, Request: request,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddSSHKeys(context.Background(), SSHKeyChangeCommand{
		UUID: "vm-1", Spec: domain.GuestAgentSSHKeySpec{Username: "alice", Keys: []string{"ssh-ed25519 AAAASECRET"}}, Request: request,
	}); err != nil {
		t.Fatal(err)
	}
	encoded := "aGVsbG8="
	if _, err := service.WriteFile(context.Background(), FileWriteCommand{
		UUID: "vm-1", Spec: domain.GuestAgentFileWriteSpec{Path: "/tmp/test", DataBase64: encoded, Overwrite: true}, Request: request,
	}); err != nil {
		t.Fatal(err)
	}
	if tasks.started != 3 || tasks.succeeded != 3 {
		t.Fatalf("task counts = %+v", tasks)
	}
	for _, event := range audit.events {
		for key, value := range event.Detail {
			text, ok := value.(string)
			if !ok || key == "fingerprints" {
				continue
			}
			if text == "super-secret" || text == "ssh-ed25519 AAAASECRET" || text == encoded || text == "aGVsbG8=" {
				t.Fatalf("audit leaked secret in %s: %+v", key, event.Detail)
			}
		}
	}
}

func TestGuestAgentFileAndProcessGuards(t *testing.T) {
	backend := &fakeBackend{}
	service := New(backend, nil, false)
	if _, err := service.ReadFile(context.Background(), FileReadCommand{UUID: "vm-1", Spec: domain.GuestAgentFileReadSpec{Path: "relative"}, Request: operatorRequest()}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("relative read error = %v", err)
	}
	if _, err := service.WriteFile(context.Background(), FileWriteCommand{UUID: "vm-1", Spec: domain.GuestAgentFileWriteSpec{Path: "/tmp/x", DataBase64: "aGVsbG8="}, Request: operatorRequest()}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("non-overwrite write error = %v", err)
	}
	if _, err := service.SignalProcess(context.Background(), ProcessSignalCommand{UUID: "vm-1", Spec: domain.GuestAgentProcessSignalSpec{PID: 1, Signal: "term"}, Request: operatorRequest()}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("pid 1 error = %v", err)
	}
	viewer := operatorRequest()
	viewer.Session.User.Role = domain.RoleViewer
	if _, err := service.AddSSHKeys(context.Background(), SSHKeyChangeCommand{UUID: "vm-1", Spec: domain.GuestAgentSSHKeySpec{Username: "alice", Keys: []string{"ssh-ed25519 AAAA"}}, Request: viewer}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("viewer ssh error = %v", err)
	}
}

func TestGuestAgentHealthTransitionsAndHistory(t *testing.T) {
	backend := &fakeBackend{info: domain.GuestAgentInfo{State: domain.GuestAgentStateDisconnected}}
	policies := &fakePolicies{items: map[string]domain.GuestAgentPolicy{
		"vm-1": {VMUUID: "vm-1", Expected: true, LastState: domain.GuestAgentStateConnected},
	}}
	service := New(backend, nil, false, WithPolicyRepository(policies))
	for attempt := 1; attempt <= 3; attempt++ {
		changes, err := service.CheckHealth(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if attempt < 3 && len(changes) != 0 {
			t.Fatalf("unexpected alert on attempt %d: %+v", attempt, changes)
		}
		if attempt == 3 && (len(changes) != 1 || changes[0].Kind != "degraded") {
			t.Fatalf("degraded alert = %+v", changes)
		}
	}
	if policy := policies.items["vm-1"]; policy.ConsecutiveFailures != 3 || policy.LastCheckedAt == nil {
		t.Fatalf("policy after failures = %+v", policy)
	}
	backend.info = domain.GuestAgentInfo{State: domain.GuestAgentStateConnected}
	changes, err := service.CheckHealth(context.Background())
	if err != nil || len(changes) != 1 || changes[0].Kind != "recovered" {
		t.Fatalf("recovery = %+v, %v", changes, err)
	}
	history, err := service.Health(context.Background(), HealthQuery{
		UUID: "vm-1", Hours: 1, Request: operatorRequest(),
	})
	if err != nil || history.Total != 4 || history.ConnectivityPercent != 25 {
		t.Fatalf("history = %+v, %v", history, err)
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
