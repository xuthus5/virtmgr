package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type fakeImportVolumes struct {
	mu                 sync.Mutex
	pools              map[string][]domain.StorageVolume
	unavailable        map[string]bool
	cloneErr           error
	createErr          error
	deleteErr          error
	blockClone         bool
	clonedPool         string
	clonedSource       string
	targetPool         string
	targetName         string
	createdPool        string
	createdVolumeSpecs []domain.StorageVolumeCreateSpec
	deleted            []string
}

func (f *fakeImportVolumes) ListStorageVolumes(_ context.Context, poolName string) ([]domain.StorageVolume, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unavailable[poolName] {
		return nil, domain.ErrUnavailable
	}
	volumes, ok := f.pools[poolName]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return append([]domain.StorageVolume(nil), volumes...), nil
}

func (f *fakeImportVolumes) CloneStorageVolume(ctx context.Context, sourcePool, sourceVolume, targetPool, targetName string) (domain.StorageVolume, error) {
	f.mu.Lock()
	f.clonedPool, f.clonedSource, f.targetPool, f.targetName = sourcePool, sourceVolume, targetPool, targetName
	block := f.blockClone
	cloneErr := f.cloneErr
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return domain.StorageVolume{}, ctx.Err()
	}
	if cloneErr != nil {
		return domain.StorageVolume{}, cloneErr
	}
	format := strings.TrimPrefix(strings.ToLower(sourceVolume[strings.LastIndex(sourceVolume, "."):]), ".")
	return domain.StorageVolume{Name: targetName, Path: "/var/lib/libvirt/" + targetPool + "/" + targetName, Format: format}, nil
}

func (f *fakeImportVolumes) CreateStorageVolume(_ context.Context, poolName string, spec domain.StorageVolumeCreateSpec) (domain.StorageVolume, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return domain.StorageVolume{}, f.createErr
	}
	f.createdPool = poolName
	f.createdVolumeSpecs = append(f.createdVolumeSpecs, spec)
	f.targetPool = poolName
	f.targetName = spec.Name
	volume := domain.StorageVolume{
		Name: spec.Name, Path: "/var/lib/libvirt/" + poolName + "/" + spec.Name,
		Format: spec.Format, CapacityBytes: spec.CapacityBytes,
	}
	f.pools[poolName] = append(f.pools[poolName], volume)
	return volume, nil
}

func (f *fakeImportVolumes) DeleteStorageVolume(_ context.Context, poolName, volumeName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, poolName+"/"+volumeName)
	return f.deleteErr
}

func (f *fakeImportVolumes) deletedVolumes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

func validImportSpec() domain.VMImportSpec {
	return domain.VMImportSpec{
		Name: "web-import", SourcePool: "images", SourceVolume: "base.qcow2", TargetPool: "vms",
		VCPUs: 2, MemoryMiB: 2048, Network: "default", Firmware: domain.VMFirmwareBIOS,
	}
}

func newImportVolumes(format string) *fakeImportVolumes {
	return &fakeImportVolumes{
		pools: map[string][]domain.StorageVolume{
			"images": {{Name: "base." + format, Path: "/var/lib/libvirt/images/base." + format, Format: format, CapacityBytes: 20 << 30, AllocationBytes: 8 << 30}},
			"vms":    {},
		},
		unavailable: map[string]bool{},
	}
}

func newISOImportVolumes() *fakeImportVolumes {
	volumes := newImportVolumes("raw")
	volumes.pools["images"] = append(volumes.pools["images"], domain.StorageVolume{
		Name: "install.iso", Path: "/var/lib/libvirt/images/install.iso", Format: "raw", CapacityBytes: 700 << 20,
	})
	return volumes
}

func TestImportQCOW2AndRawStartsTaskAndKeepsSource(t *testing.T) {
	for _, format := range []string{"qcow2", "raw"} {
		t.Run(format, func(t *testing.T) {
			volumes := newImportVolumes(format)
			repository := &recordingRepository{}
			audit := &fakeAuditRecorder{}
			tasks := newFakeTaskRecorder()
			service := New(repository, audit, false, WithTaskRecorder(tasks), WithImportVolumes(volumes))
			spec := validImportSpec()
			spec.SourceVolume = "base." + format

			task, err := service.Import(context.Background(), ImportCommand{
				Spec: spec, Request: MutationContext{Session: operatorSession(), RequestID: "req-import", SourceIP: "127.0.0.1"},
			})
			if err != nil {
				t.Fatalf("Import() error = %v", err)
			}
			if task.Kind != "vm.import" || task.TargetKind != "vm_import" || task.TargetID != "images/base."+format {
				t.Fatalf("task = %+v", task)
			}
			tasks.wait(t)

			if len(repository.createdSpecs) != 1 {
				t.Fatalf("created specs = %+v", repository.createdSpecs)
			}
			created := repository.createdSpecs[0]
			expectedPath := fmt.Sprintf("/var/lib/libvirt/vms/web-import-disk.%s", format)
			if created.DiskPath != expectedPath || created.DiskFormat != format || created.TargetPool != "vms" {
				t.Fatalf("created spec = %+v", created)
			}
			if volumes.clonedSource != "base."+format || volumes.targetName != "web-import-disk."+format {
				t.Fatalf("clone = %s -> %s/%s", volumes.clonedSource, volumes.targetPool, volumes.targetName)
			}
			if len(volumes.deletedVolumes()) != 0 {
				t.Fatalf("source or target was deleted: %+v", volumes.deletedVolumes())
			}
			if len(audit.events) != 1 || audit.events[0].Action != "vm.import" || audit.events[0].Result != "success" || audit.events[0].TargetID != "uuid-created" {
				t.Fatalf("audit = %+v", audit.events)
			}
		})
	}
}

func TestImportISOCreatesBlankSystemDiskAndBootsFromCDROM(t *testing.T) {
	volumes := newISOImportVolumes()
	repository := &recordingRepository{}
	audit := &fakeAuditRecorder{}
	tasks := newFakeTaskRecorder()
	service := New(repository, audit, false, WithTaskRecorder(tasks), WithImportVolumes(volumes))
	spec := validImportSpec()
	spec.SourceVolume = "install.iso"

	task, err := service.Import(context.Background(), ImportCommand{
		Spec: spec, Request: MutationContext{Session: operatorSession(), RequestID: "req-iso-import", SourceIP: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if task.TargetID != "images/install.iso" {
		t.Fatalf("task = %+v", task)
	}
	tasks.wait(t)

	if volumes.clonedSource != "" || volumes.targetName != "web-import-disk.qcow2" {
		t.Fatalf("ISO must not be cloned: clone=%q target=%q", volumes.clonedSource, volumes.targetName)
	}
	if len(volumes.createdVolumeSpecs) != 1 {
		t.Fatalf("created volume specs = %+v", volumes.createdVolumeSpecs)
	}
	createdVolume := volumes.createdVolumeSpecs[0]
	if createdVolume.Name != "web-import-disk.qcow2" || createdVolume.Format != "qcow2" || createdVolume.CapacityBytes != defaultImportISODiskCapacity {
		t.Fatalf("created volume = %+v", createdVolume)
	}
	if len(repository.createdSpecs) != 1 {
		t.Fatalf("created specs = %+v", repository.createdSpecs)
	}
	created := repository.createdSpecs[0]
	if created.DiskPath != "/var/lib/libvirt/vms/web-import-disk.qcow2" || created.DiskFormat != "qcow2" ||
		created.ISOPath != "/var/lib/libvirt/images/install.iso" || created.BootOrder != domain.VMBootOrderCDROMFirst {
		t.Fatalf("created spec = %+v", created)
	}
	if len(volumes.deletedVolumes()) != 0 {
		t.Fatalf("source or target was deleted: %+v", volumes.deletedVolumes())
	}
	if len(audit.events) != 1 || audit.events[0].Action != "vm.import" || audit.events[0].Result != "success" || audit.events[0].TargetID != "uuid-created" {
		t.Fatalf("audit = %+v", audit.events)
	}
}

func TestImportPreflightRejectsUnsupportedOrConflictingSources(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeImportVolumes, *recordingRepository, *domain.VMImportSpec)
		want   error
	}{
		{name: "unsupported extension", mutate: func(_ *fakeImportVolumes, _ *recordingRepository, spec *domain.VMImportSpec) {
			spec.SourceVolume = "install.vmdk"
		}, want: domain.ErrInvalidInput},
		{name: "format mismatch", mutate: func(volumes *fakeImportVolumes, _ *recordingRepository, _ *domain.VMImportSpec) {
			volumes.pools["images"] = []domain.StorageVolume{{Name: "base.qcow2", Format: "raw", Path: "/images/base.qcow2"}}
		}, want: domain.ErrInvalidInput},
		{name: "iso format mismatch", mutate: func(volumes *fakeImportVolumes, _ *recordingRepository, spec *domain.VMImportSpec) {
			volumes.pools["images"] = []domain.StorageVolume{{Name: "install.iso", Format: "qcow2", Path: "/images/install.iso"}}
			spec.SourceVolume = "install.iso"
		}, want: domain.ErrInvalidInput},
		{name: "iso capacity too small", mutate: func(_ *fakeImportVolumes, _ *recordingRepository, spec *domain.VMImportSpec) {
			spec.SourceVolume = "install.iso"
			spec.DiskCapacityBytes = minImportISODiskCapacity - 1
		}, want: domain.ErrInvalidInput},
		{name: "missing volume", mutate: func(_ *fakeImportVolumes, _ *recordingRepository, spec *domain.VMImportSpec) {
			spec.SourceVolume = "missing.qcow2"
		}, want: domain.ErrNotFound},
		{name: "inactive target pool", mutate: func(volumes *fakeImportVolumes, _ *recordingRepository, _ *domain.VMImportSpec) {
			volumes.unavailable["vms"] = true
		}, want: domain.ErrUnavailable},
		{name: "target disk conflict", mutate: func(volumes *fakeImportVolumes, _ *recordingRepository, _ *domain.VMImportSpec) {
			volumes.pools["vms"] = []domain.StorageVolume{{Name: "web-import-disk.qcow2"}}
		}, want: domain.ErrConflict},
		{name: "vm name conflict", mutate: func(_ *fakeImportVolumes, repository *recordingRepository, _ *domain.VMImportSpec) {
			repository.vms = []domain.VM{{Name: "web-import"}}
		}, want: domain.ErrConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			volumes := newImportVolumes("qcow2")
			repository := &recordingRepository{}
			spec := validImportSpec()
			test.mutate(volumes, repository, &spec)
			tasks := newFakeTaskRecorder()
			service := New(repository, &fakeAuditRecorder{}, false, WithTaskRecorder(tasks), WithImportVolumes(volumes))
			_, err := service.Import(context.Background(), ImportCommand{Spec: spec, Request: MutationContext{Session: operatorSession()}})
			if !errors.Is(err, test.want) {
				t.Fatalf("Import() error = %v, want %v", err, test.want)
			}
			if len(repository.createdSpecs) != 0 || volumes.targetName != "" {
				t.Fatalf("preflight created resources: specs=%+v clone=%s", repository.createdSpecs, volumes.targetName)
			}
		})
	}
}

func TestImportCleansClonedVolumeWhenDomainCreationFails(t *testing.T) {
	volumes := newImportVolumes("qcow2")
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{createErr: domain.ErrConflict}}
	tasks := newFakeTaskRecorder()
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false, WithTaskRecorder(tasks), WithImportVolumes(volumes))
	_, err := service.Import(context.Background(), ImportCommand{Spec: validImportSpec(), Request: MutationContext{Session: operatorSession()}})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	tasks.wait(t)
	deleted := volumes.deletedVolumes()
	if len(deleted) != 1 || deleted[0] != "vms/web-import-disk.qcow2" {
		t.Fatalf("deleted volumes = %+v", deleted)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" || audit.events[0].Action != "vm.import" {
		t.Fatalf("audit = %+v", audit.events)
	}
}

func TestImportISOCleansBlankSystemDiskWhenDomainCreationFails(t *testing.T) {
	volumes := newISOImportVolumes()
	repository := &recordingRepository{fakeVMRepository: fakeVMRepository{createErr: domain.ErrConflict}}
	tasks := newFakeTaskRecorder()
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false, WithTaskRecorder(tasks), WithImportVolumes(volumes))
	spec := validImportSpec()
	spec.SourceVolume = "install.iso"
	_, err := service.Import(context.Background(), ImportCommand{Spec: spec, Request: MutationContext{Session: operatorSession()}})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	tasks.wait(t)
	deleted := volumes.deletedVolumes()
	if len(deleted) != 1 || deleted[0] != "vms/web-import-disk.qcow2" {
		t.Fatalf("deleted volumes = %+v", deleted)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" || audit.events[0].Action != "vm.import" {
		t.Fatalf("audit = %+v", audit.events)
	}
}

func TestImportRejectsViewerAndReadOnly(t *testing.T) {
	viewer := operatorSession()
	viewer.User.Role = domain.RoleViewer
	for _, test := range []struct {
		name     string
		session  domain.Session
		readOnly bool
		want     error
	}{
		{name: "viewer", session: viewer, want: domain.ErrForbidden},
		{name: "read only", session: operatorSession(), readOnly: true, want: domain.ErrReadOnly},
	} {
		t.Run(test.name, func(t *testing.T) {
			volumes := newImportVolumes("qcow2")
			tasks := newFakeTaskRecorder()
			service := New(&recordingRepository{}, &fakeAuditRecorder{}, test.readOnly, WithTaskRecorder(tasks), WithImportVolumes(volumes))
			_, err := service.Import(context.Background(), ImportCommand{Spec: validImportSpec(), Request: MutationContext{Session: test.session}})
			if !errors.Is(err, test.want) || volumes.targetName != "" {
				t.Fatalf("Import() error = %v clone = %s", err, volumes.targetName)
			}
		})
	}
}

type cancellableImportTaskRecorder struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	failErr  error
	done     chan struct{}
	finished bool
}

func newCancellableImportTaskRecorder() *cancellableImportTaskRecorder {
	return &cancellableImportTaskRecorder{done: make(chan struct{})}
}

func (r *cancellableImportTaskRecorder) Start(context.Context, string, string, string, string, string) (string, error) {
	return "task-import", nil
}

func (r *cancellableImportTaskRecorder) Progress(context.Context, string, int, string) error {
	return nil
}

func (r *cancellableImportTaskRecorder) Succeed(context.Context, string, string) error {
	r.finish(nil)
	return nil
}

func (r *cancellableImportTaskRecorder) Fail(_ context.Context, _ string, _ int, _ string, cause error) error {
	r.finish(cause)
	return nil
}

func (r *cancellableImportTaskRecorder) RegisterCancel(_ string, cancel context.CancelFunc) {
	r.mu.Lock()
	r.cancel = cancel
	r.mu.Unlock()
}

func (r *cancellableImportTaskRecorder) cancelImport() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *cancellableImportTaskRecorder) finish(cause error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return
	}
	r.finished = true
	r.failErr = cause
	close(r.done)
}

func TestImportCancellationStopsAtCloneCheckpoint(t *testing.T) {
	volumes := newImportVolumes("qcow2")
	volumes.blockClone = true
	repository := &recordingRepository{}
	tasks := newCancellableImportTaskRecorder()
	service := New(repository, &fakeAuditRecorder{}, false, WithTaskRecorder(tasks), WithImportVolumes(volumes))
	_, err := service.Import(context.Background(), ImportCommand{Spec: validImportSpec(), Request: MutationContext{Session: operatorSession()}})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		tasks.mu.Lock()
		ready := tasks.cancel != nil
		tasks.mu.Unlock()
		if ready {
			break
		}
	}
	tasks.cancelImport()
	select {
	case <-tasks.done:
	case <-time.After(time.Second):
		t.Fatal("cancelled import did not finish")
	}
	if !errors.Is(tasks.failErr, context.Canceled) {
		t.Fatalf("failure = %v", tasks.failErr)
	}
	if len(repository.createdSpecs) != 0 {
		t.Fatalf("cancelled import created domains: %+v", repository.createdSpecs)
	}
}

func TestVMImportIsCancellableTaskKind(t *testing.T) {
	if !domain.CancellableTaskKind("vm.import") {
		t.Fatal("vm.import must be cancellable")
	}
}
