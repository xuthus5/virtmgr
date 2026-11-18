package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"virtmgr/internal/domain"
)

type flattenRepository struct {
	*fakeRepository
	calls   atomic.Int32
	flatten func(context.Context, domain.StorageVolumeFlattenSpec) (domain.StorageVolume, error)
}

func (r *flattenRepository) FlattenStorageVolume(ctx context.Context, spec domain.StorageVolumeFlattenSpec) (domain.StorageVolume, error) {
	r.calls.Add(1)
	if r.flatten != nil {
		return r.flatten(ctx, spec)
	}
	return domain.StorageVolume{Name: spec.TargetName, Format: "qcow2", Path: "/var/lib/libvirt/images/" + spec.TargetName, CapacityBytes: 10 << 30}, nil
}

func linkedVolumeRepository() *flattenRepository {
	return &flattenRepository{fakeRepository: &fakeRepository{
		volumes: []domain.StorageVolume{{
			Name: "source.qcow2", Path: "/var/lib/libvirt/images/source.qcow2", Format: "qcow2", Linked: true,
			CapacityBytes: 10 << 30, AllocationBytes: 2 << 30,
		}},
		getPool: domain.StoragePool{Name: "default", State: domain.StoragePoolStateActive, AvailableBytes: 1 << 40},
	}}
}

func TestFlattenVolumeSubmitsTaskAndAuditsSuccess(t *testing.T) {
	repository := linkedVolumeRepository()
	audit := &fakeAuditRecorder{}
	tasks := newConvertTaskRecorder()
	service := New(repository, audit, false, WithTaskRecorder(tasks))
	task, err := service.FlattenVolume(context.Background(), FlattenVolumeCommand{
		Pool: "default", Name: "source.qcow2",
		Spec:    domain.StorageVolumeFlattenSpec{TargetPool: "default", TargetName: "source-flat.qcow2"},
		Request: MutationContext{Session: operatorSession(), RequestID: "req-flatten"},
	})
	if err != nil {
		t.Fatalf("FlattenVolume() error = %v", err)
	}
	if task.Kind != "storage.volume.flatten" || task.TargetID != "default/source.qcow2" || task.Status != domain.TaskStatusRunning {
		t.Fatalf("task = %+v", task)
	}
	waitConvertTask(t, tasks)
	starts, succeeded, failed := tasks.snapshot()
	if len(starts) != 1 || starts[0] != "storage.volume.flatten|storage_volume|default/source.qcow2|operator|正在展平链接克隆 default/source.qcow2" {
		t.Fatalf("starts = %+v", starts)
	}
	if len(succeeded) != 1 || len(failed) != 0 || succeeded[0] != "task-convert|链接克隆已展平为独立卷" {
		t.Fatalf("succeeded=%+v failed=%+v", succeeded, failed)
	}
	if repository.calls.Load() != 1 || len(audit.events) != 1 || audit.events[0].Action != "storage.volume.flatten" || audit.events[0].Result != "success" {
		t.Fatalf("calls=%d audit=%+v", repository.calls.Load(), audit.events)
	}
}

func TestFlattenVolumeRejectsBeforeTask(t *testing.T) {
	request := MutationContext{Session: operatorSession()}
	for _, test := range []struct {
		name       string
		repository *flattenRepository
		readOnly   bool
		command    FlattenVolumeCommand
		want       error
	}{
		{
			name: "not linked", repository: &flattenRepository{fakeRepository: &fakeRepository{
				volumes: []domain.StorageVolume{{Name: "source.qcow2", Path: "/var/lib/libvirt/images/source.qcow2", Format: "qcow2"}},
				getPool: domain.StoragePool{Name: "default", State: domain.StoragePoolStateActive, AvailableBytes: 1 << 40},
			}},
			command: FlattenVolumeCommand{Pool: "default", Name: "source.qcow2", Spec: domain.StorageVolumeFlattenSpec{TargetPool: "default", TargetName: "source-flat.qcow2"}},
			want:    domain.ErrInvalidInput,
		},
		{
			name: "in use", repository: &flattenRepository{fakeRepository: &fakeRepository{
				volumes: []domain.StorageVolume{{Name: "source.qcow2", Path: "/var/lib/libvirt/images/source.qcow2", Format: "qcow2", Linked: true}},
				vms:     []domain.VM{{UUID: "vm-1", Name: "web-01", Disks: []domain.Disk{{Source: "/var/lib/libvirt/images/source.qcow2"}}}},
				getPool: domain.StoragePool{Name: "default", State: domain.StoragePoolStateActive, AvailableBytes: 1 << 40},
			}},
			command: FlattenVolumeCommand{Pool: "default", Name: "source.qcow2", Spec: domain.StorageVolumeFlattenSpec{TargetPool: "default", TargetName: "source-flat.qcow2"}},
			want:    domain.ErrVolumeInUse,
		},
		{
			name: "wrong extension", repository: linkedVolumeRepository(), readOnly: false,
			command: FlattenVolumeCommand{Pool: "default", Name: "source.qcow2", Spec: domain.StorageVolumeFlattenSpec{TargetPool: "default", TargetName: "source-flat.raw"}},
			want:    domain.ErrInvalidInput,
		},
		{
			name: "read only", repository: linkedVolumeRepository(), readOnly: true,
			command: FlattenVolumeCommand{Pool: "default", Name: "source.qcow2", Spec: domain.StorageVolumeFlattenSpec{TargetPool: "default", TargetName: "source-flat.qcow2"}},
			want:    domain.ErrReadOnly,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			audit := &fakeAuditRecorder{}
			tasks := newConvertTaskRecorder()
			_, err := New(test.repository, audit, test.readOnly, WithTaskRecorder(tasks)).FlattenVolume(context.Background(), FlattenVolumeCommand{
				Pool: test.command.Pool, Name: test.command.Name, Spec: test.command.Spec, Request: request,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			starts, _, _ := tasks.snapshot()
			if len(starts) != 0 || test.repository.calls.Load() != 0 {
				t.Fatalf("rejected request started task=%+v flattenCalls=%d", starts, test.repository.calls.Load())
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}
