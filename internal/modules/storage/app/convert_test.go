package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type convertRepository struct {
	*fakeRepository
	calls   atomic.Int32
	convert func(context.Context, domain.StorageVolumeConvertSpec) (domain.StorageVolume, error)
}

func (r *convertRepository) ConvertStorageVolume(ctx context.Context, spec domain.StorageVolumeConvertSpec) (domain.StorageVolume, error) {
	r.calls.Add(1)
	if r.convert != nil {
		return r.convert(ctx, spec)
	}
	return domain.StorageVolume{Name: spec.TargetName, Format: spec.TargetFormat, Path: "/var/lib/libvirt/images/" + spec.TargetName, CapacityBytes: 10 << 30}, nil
}

type convertTaskRecorder struct {
	mu        sync.Mutex
	starts    []string
	succeeded []string
	failed    []string
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
}

func newConvertTaskRecorder() *convertTaskRecorder {
	return &convertTaskRecorder{done: make(chan struct{})}
}

func (r *convertTaskRecorder) Start(_ context.Context, kind, targetKind, targetID, actor, step string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts = append(r.starts, kind+"|"+targetKind+"|"+targetID+"|"+actor+"|"+step)
	return "task-convert", nil
}

func (r *convertTaskRecorder) Progress(context.Context, string, int, string) error { return nil }

func (r *convertTaskRecorder) RegisterCancel(_ string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancel = cancel
}

func (r *convertTaskRecorder) triggerCancel() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *convertTaskRecorder) Succeed(_ context.Context, id, step string) error {
	r.mu.Lock()
	r.succeeded = append(r.succeeded, id+"|"+step)
	r.mu.Unlock()
	r.finish()
	return nil
}

func (r *convertTaskRecorder) Fail(_ context.Context, id string, _ int, step string, cause error) error {
	r.mu.Lock()
	r.failed = append(r.failed, id+"|"+step+"|"+cause.Error())
	r.mu.Unlock()
	r.finish()
	return nil
}

func (r *convertTaskRecorder) finish() {
	r.closeOnce.Do(func() { close(r.done) })
}

func (r *convertTaskRecorder) snapshot() (starts, succeeded, failed []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.starts...), append([]string(nil), r.succeeded...), append([]string(nil), r.failed...)
}

func waitConvertTask(t *testing.T, recorder *convertTaskRecorder) {
	t.Helper()
	select {
	case <-recorder.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for conversion task")
	}
}

type contextCheckingAuditRecorder struct {
	mu         sync.Mutex
	events     []domain.AuditEvent
	contextErr []error
}

func (r *contextCheckingAuditRecorder) RecordAudit(ctx context.Context, event domain.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	r.contextErr = append(r.contextErr, ctx.Err())
	return nil
}

func (r *contextCheckingAuditRecorder) snapshot() ([]domain.AuditEvent, []error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.AuditEvent(nil), r.events...), append([]error(nil), r.contextErr...)
}

func successfulConvertRepository() *convertRepository {
	return &convertRepository{fakeRepository: &fakeRepository{
		volumes: []domain.StorageVolume{{
			Name: "source.qcow2", Path: "/var/lib/libvirt/images/source.qcow2", Format: "qcow2",
			CapacityBytes: 10 << 30, AllocationBytes: 2 << 30,
		}},
		getPool: domain.StoragePool{Name: "default", State: domain.StoragePoolStateActive, AvailableBytes: 1 << 40},
	}}
}

func TestConvertVolumeSubmitsTaskAndAuditsSuccess(t *testing.T) {
	repository := successfulConvertRepository()
	audit := &fakeAuditRecorder{}
	tasks := newConvertTaskRecorder()
	service := New(repository, audit, false, WithTaskRecorder(tasks))
	task, err := service.ConvertVolume(context.Background(), ConvertVolumeCommand{
		Pool: "default", Name: "source.qcow2",
		Spec:    domain.StorageVolumeConvertSpec{TargetPool: "default", TargetName: "source.raw", TargetFormat: " RAW "},
		Request: MutationContext{Session: operatorSession(), RequestID: "req-convert"},
	})
	if err != nil {
		t.Fatalf("ConvertVolume() error = %v", err)
	}
	if task.ID != "task-convert" || task.Kind != "storage.volume.convert" || task.TargetID != "default/source.qcow2" || task.Status != domain.TaskStatusRunning {
		t.Fatalf("task = %+v", task)
	}
	waitConvertTask(t, tasks)
	starts, succeeded, failed := tasks.snapshot()
	if len(starts) != 1 || starts[0] != "storage.volume.convert|storage_volume|default/source.qcow2|operator|正在将 default/source.qcow2 转换为 raw" {
		t.Fatalf("starts = %+v", starts)
	}
	if len(succeeded) != 1 || len(failed) != 0 || succeeded[0] != "task-convert|存储卷已转换为 raw" {
		t.Fatalf("succeeded=%+v failed=%+v", succeeded, failed)
	}
	if repository.calls.Load() != 1 {
		t.Fatalf("converter calls = %d", repository.calls.Load())
	}
	if len(audit.events) != 1 || audit.events[0].Action != "storage.volume.convert" || audit.events[0].Result != "success" || audit.events[0].Detail["targetFormat"] != "raw" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestConvertVolumeMarksTaskAndAuditFailed(t *testing.T) {
	repository := successfulConvertRepository()
	repository.convert = func(context.Context, domain.StorageVolumeConvertSpec) (domain.StorageVolume, error) {
		return domain.StorageVolume{}, errors.New("libvirt conversion failed")
	}
	audit := &fakeAuditRecorder{}
	tasks := newConvertTaskRecorder()
	service := New(repository, audit, false, WithTaskRecorder(tasks))
	if _, err := service.ConvertVolume(context.Background(), ConvertVolumeCommand{
		Pool: "default", Name: "source.qcow2",
		Spec:    domain.StorageVolumeConvertSpec{TargetPool: "default", TargetName: "source.raw", TargetFormat: "raw"},
		Request: MutationContext{Session: operatorSession()},
	}); err != nil {
		t.Fatalf("ConvertVolume() error = %v", err)
	}
	waitConvertTask(t, tasks)
	_, succeeded, failed := tasks.snapshot()
	if len(succeeded) != 0 || len(failed) != 1 {
		t.Fatalf("succeeded=%+v failed=%+v", succeeded, failed)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestConvertVolumeFinalizesAuditAfterJobCancellation(t *testing.T) {
	repository := successfulConvertRepository()
	audit := &contextCheckingAuditRecorder{}
	tasks := newConvertTaskRecorder()
	repository.convert = func(ctx context.Context, _ domain.StorageVolumeConvertSpec) (domain.StorageVolume, error) {
		tasks.triggerCancel()
		return domain.StorageVolume{}, ctx.Err()
	}
	service := New(repository, audit, false, WithTaskRecorder(tasks))
	if _, err := service.ConvertVolume(context.Background(), ConvertVolumeCommand{
		Pool: "default", Name: "source.qcow2",
		Spec:    domain.StorageVolumeConvertSpec{TargetPool: "default", TargetName: "source.raw", TargetFormat: "raw"},
		Request: MutationContext{Session: operatorSession()},
	}); err != nil {
		t.Fatalf("ConvertVolume() error = %v", err)
	}
	waitConvertTask(t, tasks)
	events, contextErrs := audit.snapshot()
	if len(events) != 1 || events[0].Result != "failure" || events[0].Detail["error"] != context.Canceled.Error() {
		t.Fatalf("audit events = %+v", events)
	}
	for _, err := range contextErrs {
		if err != nil {
			t.Fatalf("final audit used canceled context: %v", err)
		}
	}
	_, succeeded, failed := tasks.snapshot()
	if len(succeeded) != 0 || len(failed) != 1 {
		t.Fatalf("succeeded=%+v failed=%+v", succeeded, failed)
	}
}

func TestConvertVolumeRejectsInvalidRequestsBeforeTask(t *testing.T) {
	baseVolume := domain.StorageVolume{Name: "source.qcow2", Path: "/var/lib/libvirt/images/source.qcow2", Format: "qcow2", CapacityBytes: 10 << 30, AllocationBytes: 2 << 30}
	conflictVolume := domain.StorageVolume{Name: "taken.raw", Path: "/var/lib/libvirt/images/taken.raw", Format: "raw", CapacityBytes: 10 << 30}
	request := MutationContext{Session: operatorSession()}
	for _, test := range []struct {
		name       string
		repository *convertRepository
		readOnly   bool
		request    MutationContext
		command    ConvertVolumeCommand
		want       error
	}{
		{
			name: "viewer", repository: successfulConvertRepository(), request: MutationContext{Session: domain.Session{User: domain.User{Username: "viewer", Role: domain.RoleViewer}}},
			command: ConvertVolumeCommand{Pool: "default", Name: "source.qcow2", Spec: domain.StorageVolumeConvertSpec{TargetPool: "default", TargetName: "source.raw", TargetFormat: "raw"}},
			want:    domain.ErrForbidden,
		},
		{
			name: "read only", repository: successfulConvertRepository(), readOnly: true, request: request,
			command: ConvertVolumeCommand{Pool: "default", Name: "source.qcow2", Spec: domain.StorageVolumeConvertSpec{TargetPool: "default", TargetName: "source.raw", TargetFormat: "raw"}},
			want:    domain.ErrReadOnly,
		},
		{
			name: "unsupported format", repository: successfulConvertRepository(), request: request,
			command: ConvertVolumeCommand{Pool: "default", Name: "source.qcow2", Spec: domain.StorageVolumeConvertSpec{TargetPool: "default", TargetName: "source.vmdk", TargetFormat: "vmdk"}},
			want:    domain.ErrInvalidInput,
		},
		{
			name: "extension mismatch", repository: successfulConvertRepository(), request: request,
			command: ConvertVolumeCommand{Pool: "default", Name: "source.qcow2", Spec: domain.StorageVolumeConvertSpec{TargetPool: "default", TargetName: "source.qcow2", TargetFormat: "raw"}},
			want:    domain.ErrInvalidInput,
		},
		{
			name:       "target conflict",
			repository: &convertRepository{fakeRepository: &fakeRepository{volumes: []domain.StorageVolume{baseVolume, conflictVolume}, getPool: domain.StoragePool{Name: "default", State: domain.StoragePoolStateActive, AvailableBytes: 1 << 40}}},
			request:    request,
			command:    ConvertVolumeCommand{Pool: "default", Name: "source.qcow2", Spec: domain.StorageVolumeConvertSpec{TargetPool: "default", TargetName: "taken.raw", TargetFormat: "raw"}},
			want:       domain.ErrConflict,
		},
		{
			name: "source in use",
			repository: &convertRepository{fakeRepository: &fakeRepository{
				volumes: []domain.StorageVolume{baseVolume},
				vms:     []domain.VM{{UUID: "vm-1", Name: "api-01", Disks: []domain.Disk{{Device: "disk", Target: "vda", Source: baseVolume.Path}}}},
				getPool: domain.StoragePool{Name: "default", State: domain.StoragePoolStateActive, AvailableBytes: 1 << 40},
			}},
			request: request,
			command: ConvertVolumeCommand{Pool: "default", Name: "source.qcow2", Spec: domain.StorageVolumeConvertSpec{TargetPool: "default", TargetName: "source.raw", TargetFormat: "raw"}},
			want:    domain.ErrVolumeInUse,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			audit := &fakeAuditRecorder{}
			tasks := newConvertTaskRecorder()
			_, err := New(test.repository, audit, test.readOnly, WithTaskRecorder(tasks)).ConvertVolume(context.Background(), ConvertVolumeCommand{
				Pool: test.command.Pool, Name: test.command.Name, Spec: test.command.Spec, Request: test.request,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			starts, _, _ := tasks.snapshot()
			if len(starts) != 0 || test.repository.calls.Load() != 0 {
				t.Fatalf("rejected request started task=%+v convertCalls=%d", starts, test.repository.calls.Load())
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}
