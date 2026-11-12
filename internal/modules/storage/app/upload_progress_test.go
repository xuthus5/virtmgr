package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type fakeTaskRecorder struct {
	starts      []string
	progresses  []int
	succeeded   []string
	failed      []string
	taskID      string
	startErr    error
	progressErr error
	succeedErr  error
	failErr     error
}

func (f *fakeTaskRecorder) Start(_ context.Context, kind, targetKind, targetID, actor, step string) (string, error) {
	f.starts = append(f.starts, kind+"|"+targetKind+"|"+targetID+"|"+actor+"|"+step)
	if f.taskID == "" {
		f.taskID = "task-upload"
	}
	return f.taskID, f.startErr
}

func (f *fakeTaskRecorder) Progress(_ context.Context, id string, progress int, step string) error {
	if id != f.taskID {
		return errors.New("unexpected task id")
	}
	f.progresses = append(f.progresses, progress)
	return f.progressErr
}

func (f *fakeTaskRecorder) Succeed(_ context.Context, id, step string) error {
	if id != f.taskID {
		return errors.New("unexpected task id")
	}
	f.succeeded = append(f.succeeded, step)
	return f.succeedErr
}

func (f *fakeTaskRecorder) Fail(_ context.Context, id string, progress int, step string, cause error) error {
	if id != f.taskID {
		return errors.New("unexpected task id")
	}
	f.failed = append(f.failed, step)
	return f.failErr
}

func TestUploadProgressTrackerReportsBoundedProgress(t *testing.T) {
	recorder := &fakeTaskRecorder{taskID: "task-upload"}
	tracker := &uploadProgressTracker{
		ctx: context.Background(), reader: bytes.NewReader(make([]byte, 100)), total: 100,
		recorder: recorder, taskID: "task-upload", name: "install.iso", lastProgress: -1,
	}
	data, err := io.ReadAll(tracker)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(data) != 100 || len(recorder.progresses) != 1 || recorder.progresses[0] != 99 || tracker.Progress() != 99 {
		t.Fatalf("data=%d progresses=%v progress=%d", len(data), recorder.progresses, tracker.Progress())
	}
}

func TestUploadVolumeRecordsTaskLifecycle(t *testing.T) {
	repository := &fakeRepository{}
	audit := &fakeAuditRecorder{}
	tasks := &fakeTaskRecorder{taskID: "task-upload"}
	result, err := New(repository, audit, false, WithTaskRecorder(tasks)).UploadVolume(context.Background(), UploadVolumeCommand{
		Pool: "iso-store", Name: "install.iso", Size: int64(minVolumeBytes), Body: bytes.NewReader(make([]byte, minVolumeBytes)),
		Request: MutationContext{Session: operatorSession()},
	})
	if err != nil {
		t.Fatalf("UploadVolume() error = %v", err)
	}
	if result.Name != "install.iso" || len(tasks.starts) != 1 || len(tasks.progresses) == 0 || len(tasks.succeeded) != 1 || tasks.succeeded[0] != "存储卷上传完成" {
		t.Fatalf("result=%+v starts=%v progresses=%v succeeded=%v", result, tasks.starts, tasks.progresses, tasks.succeeded)
	}
	if tasks.starts[0] != "storage.volume.upload|storage_volume|iso-store/install.iso|operator|正在上传 install.iso" {
		t.Fatalf("start = %q", tasks.starts[0])
	}
}

func TestUploadVolumeMarksTaskFailed(t *testing.T) {
	repository := &fakeRepository{uploadErr: errors.New("libvirt write failed")}
	audit := &fakeAuditRecorder{}
	tasks := &fakeTaskRecorder{taskID: "task-upload"}
	_, err := New(repository, audit, false, WithTaskRecorder(tasks)).UploadVolume(context.Background(), UploadVolumeCommand{
		Pool: "iso-store", Name: "install.iso", Size: int64(minVolumeBytes), Body: bytes.NewReader(make([]byte, minVolumeBytes)),
		Request: MutationContext{Session: operatorSession()},
	})
	if err == nil || len(tasks.failed) != 1 || tasks.failed[0] != "上传失败" {
		t.Fatalf("error=%v failed=%v", err, tasks.failed)
	}
}

func TestUploadVolumeRejectsWhenTaskStartFails(t *testing.T) {
	repository := &fakeRepository{}
	audit := &fakeAuditRecorder{}
	tasks := &fakeTaskRecorder{startErr: errors.New("sqlite unavailable")}
	_, err := New(repository, audit, false, WithTaskRecorder(tasks)).UploadVolume(context.Background(), UploadVolumeCommand{
		Pool: "iso-store", Name: "install.iso", Size: int64(minVolumeBytes), Body: bytes.NewReader(make([]byte, minVolumeBytes)),
		Request: MutationContext{Session: operatorSession()},
	})
	if err == nil || repository.uploadPool != "" || len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("error=%v uploadPool=%q audit=%+v", err, repository.uploadPool, audit.events)
	}
}
