package app

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
)

func TestCreateVolumeRejectsInsufficientCapacity(t *testing.T) {
	repository := &fakeRepository{getPool: domain.StoragePool{Name: "default", State: domain.StoragePoolStateActive, AvailableBytes: 1 << 20}}
	_, err := New(repository, &fakeAuditRecorder{}, false).CreateVolume(context.Background(), CreateVolumeCommand{
		Pool: "default", Spec: domain.StorageVolumeCreateSpec{Name: "data.qcow2", Format: "qcow2", CapacityBytes: 20 << 30},
		Request: MutationContext{Session: operatorSession()},
	})
	if !errors.Is(err, domain.ErrStorageInsufficient) {
		t.Fatalf("CreateVolume() error = %v", err)
	}
	if repository.createdSpec.Name != "" {
		t.Fatalf("created spec = %+v", repository.createdSpec)
	}
}

func TestUploadVolumeRejectsInsufficientCapacityBeforeTask(t *testing.T) {
	repository := &fakeRepository{getPool: domain.StoragePool{Name: "iso-store", State: domain.StoragePoolStateActive, AvailableBytes: 1 << 20}}
	tasks := &fakeTaskRecorder{taskID: "task-upload"}
	_, err := New(repository, &fakeAuditRecorder{}, false, WithTaskRecorder(tasks)).UploadVolume(context.Background(), UploadVolumeCommand{
		Pool: "iso-store", Name: "install.iso", Size: int64(minVolumeBytes), Body: bytes.NewReader(make([]byte, minVolumeBytes)),
		Request: MutationContext{Session: operatorSession()},
	})
	if !errors.Is(err, domain.ErrStorageInsufficient) {
		t.Fatalf("UploadVolume() error = %v", err)
	}
	if len(tasks.starts) != 0 || repository.uploadPool != "" {
		t.Fatalf("task starts=%v uploadPool=%q", tasks.starts, repository.uploadPool)
	}
}
