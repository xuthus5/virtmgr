package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type fakeRepository struct {
	pools               []domain.StoragePool
	volumes             []domain.StorageVolume
	poolsErr            error
	volumesErr          error
	getPool             domain.StoragePool
	getPoolErr          error
	createdPoolSpec     domain.StoragePoolCreateSpec
	createPoolErr       error
	performedPool       string
	performedPoolAction domain.StoragePoolAction
	poolActionErr       error
	deletedPoolName     string
	deletePoolErr       error
	createErr           error
	deleteErr           error
	uploadErr           error
	seenPool            string
	createdPool         string
	createdSpec         domain.StorageVolumeCreateSpec
	uploadPool          string
	uploadSpec          domain.StorageVolumeCreateSpec
	uploadBytes         []byte
	deletedPool         string
	deletedName         string
}

func (f *fakeRepository) ListStoragePools(context.Context) ([]domain.StoragePool, error) {
	return f.pools, f.poolsErr
}

func (f *fakeRepository) GetStoragePool(_ context.Context, name string) (domain.StoragePool, error) {
	if f.getPoolErr != nil {
		return domain.StoragePool{}, f.getPoolErr
	}
	if f.getPool.Name == "" {
		return domain.StoragePool{Name: name, State: domain.StoragePoolStateActive}, nil
	}
	return f.getPool, nil
}

func (f *fakeRepository) CreateStoragePool(_ context.Context, spec domain.StoragePoolCreateSpec) (domain.StoragePool, error) {
	f.createdPoolSpec = spec
	if f.createPoolErr != nil {
		return domain.StoragePool{}, f.createPoolErr
	}
	return domain.StoragePool{Name: spec.Name, Type: spec.Type, TargetPath: spec.TargetPath, State: domain.StoragePoolStateInactive}, nil
}

func (f *fakeRepository) PerformStoragePoolAction(_ context.Context, name string, action domain.StoragePoolAction) error {
	f.performedPool = name
	f.performedPoolAction = action
	return f.poolActionErr
}

func (f *fakeRepository) DeleteStoragePool(_ context.Context, name string) error {
	f.deletedPoolName = name
	return f.deletePoolErr
}

func (f *fakeRepository) ListStorageVolumes(_ context.Context, poolName string) ([]domain.StorageVolume, error) {
	f.seenPool = poolName
	return f.volumes, f.volumesErr
}

func (f *fakeRepository) CreateStorageVolume(_ context.Context, poolName string, spec domain.StorageVolumeCreateSpec) (domain.StorageVolume, error) {
	f.createdPool = poolName
	f.createdSpec = spec
	if f.createErr != nil {
		return domain.StorageVolume{}, f.createErr
	}
	return domain.StorageVolume{Name: spec.Name, Path: "/var/lib/libvirt/images/" + spec.Name, Type: "file", Format: spec.Format, CapacityBytes: spec.CapacityBytes}, nil
}

func (f *fakeRepository) UploadStorageVolume(_ context.Context, poolName string, spec domain.StorageVolumeCreateSpec, reader io.Reader) (domain.StorageVolume, error) {
	f.uploadPool = poolName
	f.uploadSpec = spec
	f.uploadBytes, _ = io.ReadAll(reader)
	if f.uploadErr != nil {
		return domain.StorageVolume{}, f.uploadErr
	}
	return domain.StorageVolume{Name: spec.Name, Path: "/var/lib/libvirt/images/" + spec.Name, Type: "file", Format: spec.Format, CapacityBytes: spec.CapacityBytes}, nil
}

func (f *fakeRepository) DeleteStorageVolume(_ context.Context, poolName, volumeName string) error {
	f.deletedPool = poolName
	f.deletedName = volumeName
	return f.deleteErr
}

type fakeAuditRecorder struct {
	events []domain.AuditEvent
	err    error
}

type dynamicReadOnly struct{ value bool }

func (m *dynamicReadOnly) ReadOnly() bool { return m.value }

func TestServiceUsesDynamicReadOnlyMode(t *testing.T) {
	mode := &dynamicReadOnly{value: true}
	service := New(&fakeRepository{}, nil, false, WithReadOnlyMode(mode))
	command := CreatePoolCommand{
		Spec:    domain.StoragePoolCreateSpec{Name: "data-pool", Type: "dir", TargetPath: "/srv/data"},
		Request: MutationContext{Session: operatorSession()},
	}
	if _, err := service.CreatePool(context.Background(), command); !errors.Is(err, domain.ErrReadOnly) {
		t.Fatalf("read-only error = %v", err)
	}
	mode.value = false
	if _, err := service.CreatePool(context.Background(), command); err != nil {
		t.Fatalf("writable error = %v", err)
	}
}

func (f *fakeAuditRecorder) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	f.events = append(f.events, event)
	return f.err
}

func TestListPoolsSorts(t *testing.T) {
	service := New(&fakeRepository{pools: []domain.StoragePool{{Name: "zfs"}, {Name: "Default"}, {Name: "archive"}}}, nil, false)
	result, err := service.ListPools(context.Background())
	if err != nil {
		t.Fatalf("ListPools() error = %v", err)
	}
	if result.Total != 3 || result.Items[0].Name != "archive" || result.Items[1].Name != "Default" {
		t.Fatalf("result = %+v", result)
	}
}

func TestListVolumesSortsAndValidates(t *testing.T) {
	repository := &fakeRepository{volumes: []domain.StorageVolume{{Name: "z.qcow2"}, {Name: "a.qcow2"}}}
	service := New(repository, nil, false)
	result, err := service.ListVolumes(context.Background(), " default ")
	if err != nil {
		t.Fatalf("ListVolumes() error = %v", err)
	}
	if result.Pool != "default" || result.Total != 2 || result.Items[0].Name != "a.qcow2" || repository.seenPool != "default" {
		t.Fatalf("result = %+v seen=%q", result, repository.seenPool)
	}
	if _, err := service.ListVolumes(context.Background(), " "); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("empty pool error = %v", err)
	}
	if _, err := service.ListVolumes(context.Background(), "../default"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid pool error = %v", err)
	}
}

func TestStorageErrorsPropagate(t *testing.T) {
	want := errors.New("libvirt unavailable")
	service := New(&fakeRepository{poolsErr: want, volumesErr: want}, nil, false)
	if _, err := service.ListPools(context.Background()); !errors.Is(err, want) {
		t.Fatalf("ListPools() error = %v", err)
	}
	if _, err := service.ListVolumes(context.Background(), "default"); !errors.Is(err, want) {
		t.Fatalf("ListVolumes() error = %v", err)
	}
}

func TestCreateVolumeNormalizesAndAudits(t *testing.T) {
	repository := &fakeRepository{}
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false)
	result, err := service.CreateVolume(context.Background(), CreateVolumeCommand{
		Pool: " default ",
		Spec: domain.StorageVolumeCreateSpec{Name: " data-01.qcow2 ", Format: " QCOW2 ", CapacityBytes: 20 << 30},
		Request: MutationContext{
			Session:   operatorSession(),
			RequestID: "req-1",
			SourceIP:  "127.0.0.1",
		},
	})
	if err != nil {
		t.Fatalf("CreateVolume() error = %v", err)
	}
	if repository.createdPool != "default" || repository.createdSpec.Name != "data-01.qcow2" || repository.createdSpec.Format != "qcow2" {
		t.Fatalf("created pool=%q spec=%+v", repository.createdPool, repository.createdSpec)
	}
	if result.Path != "/var/lib/libvirt/images/data-01.qcow2" {
		t.Fatalf("result = %+v", result)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	event := audit.events[0]
	if event.Action != "storage.volume.create" || event.TargetID != "default/data-01.qcow2" || event.Result != "success" || event.Actor != "operator" {
		t.Fatalf("audit event = %+v", event)
	}
	if event.Detail["capacityBytes"] != uint64(20<<30) || event.Detail["path"] != result.Path {
		t.Fatalf("audit detail = %+v", event.Detail)
	}
}

func TestCreateVolumeRejectsInvalidSpecs(t *testing.T) {
	tests := []struct {
		name string
		spec domain.StorageVolumeCreateSpec
	}{
		{name: "unsupported format", spec: domain.StorageVolumeCreateSpec{Name: "data.vmdk", Format: "vmdk", CapacityBytes: 1 << 30}},
		{name: "extension mismatch", spec: domain.StorageVolumeCreateSpec{Name: "data.qcow2", Format: "raw", CapacityBytes: 1 << 30}},
		{name: "path traversal", spec: domain.StorageVolumeCreateSpec{Name: "../data.qcow2", Format: "qcow2", CapacityBytes: 1 << 30}},
		{name: "too small", spec: domain.StorageVolumeCreateSpec{Name: "data.qcow2", Format: "qcow2", CapacityBytes: 1}},
		{name: "too large", spec: domain.StorageVolumeCreateSpec{Name: "data.qcow2", Format: "qcow2", CapacityBytes: 17 << 40}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			audit := &fakeAuditRecorder{}
			_, err := New(repository, audit, false).CreateVolume(context.Background(), CreateVolumeCommand{
				Pool: "default", Spec: test.spec, Request: MutationContext{Session: operatorSession()},
			})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("CreateVolume() error = %v", err)
			}
			if repository.createdPool != "" {
				t.Fatal("invalid request reached repository")
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestCreateVolumeRejectsInvalidPoolName(t *testing.T) {
	repository := &fakeRepository{}
	_, err := New(repository, nil, false).CreateVolume(context.Background(), CreateVolumeCommand{
		Pool:    "../default",
		Spec:    domain.StorageVolumeCreateSpec{Name: "data-01.qcow2", Format: "qcow2", CapacityBytes: 1 << 30},
		Request: MutationContext{Session: operatorSession()},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("CreateVolume() error = %v", err)
	}
	if repository.createdPool != "" {
		t.Fatal("invalid pool reached repository")
	}
}

func TestCreateVolumeRejectsViewerAndReadOnly(t *testing.T) {
	for _, test := range []struct {
		name     string
		role     domain.Role
		readOnly bool
		want     error
	}{
		{name: "viewer", role: domain.RoleViewer, want: domain.ErrForbidden},
		{name: "read only", role: domain.RoleOperator, readOnly: true, want: domain.ErrReadOnly},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			audit := &fakeAuditRecorder{}
			_, err := New(repository, audit, test.readOnly).CreateVolume(context.Background(), CreateVolumeCommand{
				Pool:    "default",
				Spec:    domain.StorageVolumeCreateSpec{Name: "data.qcow2", Format: "qcow2", CapacityBytes: 1 << 30},
				Request: MutationContext{Session: domain.Session{User: domain.User{Username: "test", Role: test.role}}},
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("CreateVolume() error = %v, want %v", err, test.want)
			}
			if repository.createdPool != "" {
				t.Fatal("rejected request reached repository")
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestCreateVolumePropagatesRepositoryError(t *testing.T) {
	repository := &fakeRepository{createErr: domain.ErrConflict}
	audit := &fakeAuditRecorder{}
	_, err := New(repository, audit, false).CreateVolume(context.Background(), CreateVolumeCommand{
		Pool:    "default",
		Spec:    domain.StorageVolumeCreateSpec{Name: "data.qcow2", Format: "qcow2", CapacityBytes: 1 << 30},
		Request: MutationContext{Session: operatorSession()},
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("CreateVolume() error = %v", err)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" || !strings.Contains(audit.events[0].Detail["error"].(string), "conflict") {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestUploadVolumeStreamsAndAudits(t *testing.T) {
	repository := &fakeRepository{}
	audit := &fakeAuditRecorder{}
	size := int64(minVolumeBytes)
	body := bytes.NewReader(bytes.Repeat([]byte{0x5a}, int(size)))
	result, err := New(repository, audit, false).UploadVolume(context.Background(), UploadVolumeCommand{
		Pool: " iso-store ", Name: " install.iso ", Size: size, Body: body,
		Request: MutationContext{Session: operatorSession(), RequestID: "req-upload", SourceIP: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("UploadVolume() error = %v", err)
	}
	if repository.uploadPool != "iso-store" || repository.uploadSpec.Name != "install.iso" || repository.uploadSpec.Format != "raw" || repository.uploadSpec.CapacityBytes != uint64(size) {
		t.Fatalf("upload pool=%q spec=%+v", repository.uploadPool, repository.uploadSpec)
	}
	if len(repository.uploadBytes) != int(size) || result.Path != "/var/lib/libvirt/images/install.iso" {
		t.Fatalf("uploaded bytes=%d result=%+v", len(repository.uploadBytes), result)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	event := audit.events[0]
	if event.Action != "storage.volume.upload" || event.TargetID != "iso-store/install.iso" || event.Result != "success" || event.Actor != "operator" {
		t.Fatalf("audit event = %+v", event)
	}
	if event.Detail["capacityBytes"] != uint64(size) || event.Detail["path"] != result.Path {
		t.Fatalf("audit detail = %+v", event.Detail)
	}
}

func TestUploadVolumeAllowsSafeSpacesInName(t *testing.T) {
	repository := &fakeRepository{}
	size := int64(minVolumeBytes)
	result, err := New(repository, nil, false).UploadVolume(context.Background(), UploadVolumeCommand{
		Pool: "iso-store", Name: "Ubuntu 24.04 installer.iso", Size: size, Body: bytes.NewReader(make([]byte, size)),
		Request: MutationContext{Session: operatorSession()},
	})
	if err != nil {
		t.Fatalf("UploadVolume() error = %v", err)
	}
	if repository.uploadSpec.Name != "Ubuntu 24.04 installer.iso" || result.Name != "Ubuntu 24.04 installer.iso" {
		t.Fatalf("upload spec=%+v result=%+v", repository.uploadSpec, result)
	}
}

func TestUploadVolumeRejectsInvalidRequests(t *testing.T) {
	validBody := func() io.Reader { return bytes.NewReader(make([]byte, minVolumeBytes)) }
	for _, test := range []struct {
		name     string
		command  UploadVolumeCommand
		readOnly bool
		want     error
	}{
		{name: "viewer", command: UploadVolumeCommand{Pool: "iso-store", Name: "install.iso", Size: int64(minVolumeBytes), Body: validBody(), Request: MutationContext{Session: domain.Session{User: domain.User{Username: "viewer", Role: domain.RoleViewer}}}}, want: domain.ErrForbidden},
		{name: "read only", command: UploadVolumeCommand{Pool: "iso-store", Name: "install.iso", Size: int64(minVolumeBytes), Body: validBody(), Request: MutationContext{Session: operatorSession()}}, readOnly: true, want: domain.ErrReadOnly},
		{name: "not iso", command: UploadVolumeCommand{Pool: "iso-store", Name: "install.img", Size: int64(minVolumeBytes), Body: validBody(), Request: MutationContext{Session: operatorSession()}}, want: domain.ErrInvalidInput},
		{name: "path traversal", command: UploadVolumeCommand{Pool: "iso-store", Name: "../install.iso", Size: int64(minVolumeBytes), Body: validBody(), Request: MutationContext{Session: operatorSession()}}, want: domain.ErrInvalidInput},
		{name: "invalid pool", command: UploadVolumeCommand{Pool: "../iso-store", Name: "install.iso", Size: int64(minVolumeBytes), Body: validBody(), Request: MutationContext{Session: operatorSession()}}, want: domain.ErrInvalidInput},
		{name: "missing pool", command: UploadVolumeCommand{Name: "install.iso", Size: int64(minVolumeBytes), Body: validBody(), Request: MutationContext{Session: operatorSession()}}, want: domain.ErrInvalidInput},
		{name: "missing body", command: UploadVolumeCommand{Pool: "iso-store", Name: "install.iso", Size: int64(minVolumeBytes), Request: MutationContext{Session: operatorSession()}}, want: domain.ErrInvalidInput},
		{name: "missing size", command: UploadVolumeCommand{Pool: "iso-store", Name: "install.iso", Body: validBody(), Request: MutationContext{Session: operatorSession()}}, want: domain.ErrInvalidInput},
		{name: "too small", command: UploadVolumeCommand{Pool: "iso-store", Name: "install.iso", Size: 1, Body: validBody(), Request: MutationContext{Session: operatorSession()}}, want: domain.ErrInvalidInput},
		{name: "too large", command: UploadVolumeCommand{Pool: "iso-store", Name: "install.iso", Size: int64(maxUploadBytes) + 1, Body: validBody(), Request: MutationContext{Session: operatorSession()}}, want: domain.ErrTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			audit := &fakeAuditRecorder{}
			_, err := New(repository, audit, test.readOnly).UploadVolume(context.Background(), test.command)
			if !errors.Is(err, test.want) {
				t.Fatalf("UploadVolume() error = %v, want %v", err, test.want)
			}
			if repository.uploadPool != "" {
				t.Fatal("rejected request reached repository")
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestUploadVolumeDerivesDiskImageFormat(t *testing.T) {
	for _, test := range []struct {
		name       string
		volumeName string
		format     string
	}{
		{name: "iso", volumeName: "installer.iso", format: "raw"},
		{name: "qcow2", volumeName: "base-image.qcow2", format: "qcow2"},
		{name: "raw", volumeName: "base-image.raw", format: "raw"},
		{name: "uppercase", volumeName: "BASE-IMAGE.QCOW2", format: "qcow2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			audit := &fakeAuditRecorder{}
			size := int64(minVolumeBytes)
			result, err := New(repository, audit, false).UploadVolume(context.Background(), UploadVolumeCommand{
				Pool: "images", Name: test.volumeName, Size: size, Body: bytes.NewReader(make([]byte, size)),
				Request: MutationContext{Session: operatorSession()},
			})
			if err != nil {
				t.Fatalf("UploadVolume() error = %v", err)
			}
			if repository.uploadSpec.Format != test.format || result.Format != test.format {
				t.Fatalf("format = %q, result = %+v", repository.uploadSpec.Format, result)
			}
			if len(audit.events) != 1 || audit.events[0].Detail["format"] != test.format {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestUploadVolumePropagatesRepositoryError(t *testing.T) {
	repository := &fakeRepository{uploadErr: domain.ErrConflict}
	audit := &fakeAuditRecorder{}
	_, err := New(repository, audit, false).UploadVolume(context.Background(), UploadVolumeCommand{
		Pool: "iso-store", Name: "install.iso", Size: int64(minVolumeBytes), Body: bytes.NewReader(make([]byte, minVolumeBytes)),
		Request: MutationContext{Session: operatorSession()},
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("UploadVolume() error = %v", err)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestDeleteVolumeNormalizesAndAudits(t *testing.T) {
	repository := &fakeRepository{}
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false)
	err := service.DeleteVolume(context.Background(), DeleteVolumeCommand{
		Pool: " default ", Name: " data-01.qcow2 ",
		Request: MutationContext{Session: operatorSession(), RequestID: "req-delete", SourceIP: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("DeleteVolume() error = %v", err)
	}
	if repository.deletedPool != "default" || repository.deletedName != "data-01.qcow2" {
		t.Fatalf("deleted pool=%q name=%q", repository.deletedPool, repository.deletedName)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	event := audit.events[0]
	if event.Action != "storage.volume.delete" || event.TargetID != "default/data-01.qcow2" || event.Result != "success" {
		t.Fatalf("audit event = %+v", event)
	}
}

func TestDeleteVolumeRejectsInvalidRequests(t *testing.T) {
	for _, test := range []struct {
		name     string
		command  DeleteVolumeCommand
		readOnly bool
		want     error
	}{
		{name: "viewer", command: DeleteVolumeCommand{Pool: "default", Name: "data.qcow2"}, want: domain.ErrForbidden},
		{name: "read only", command: DeleteVolumeCommand{Pool: "default", Name: "data.qcow2", Request: MutationContext{Session: operatorSession()}}, readOnly: true, want: domain.ErrReadOnly},
		{name: "path traversal", command: DeleteVolumeCommand{Pool: "default", Name: "../data.qcow2", Request: MutationContext{Session: operatorSession()}}, want: domain.ErrInvalidInput},
		{name: "invalid pool", command: DeleteVolumeCommand{Pool: "../default", Name: "data.qcow2", Request: MutationContext{Session: operatorSession()}}, want: domain.ErrInvalidInput},
		{name: "empty pool", command: DeleteVolumeCommand{Name: "data.qcow2", Request: MutationContext{Session: operatorSession()}}, want: domain.ErrInvalidInput},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			audit := &fakeAuditRecorder{}
			err := New(repository, audit, test.readOnly).DeleteVolume(context.Background(), test.command)
			if !errors.Is(err, test.want) {
				t.Fatalf("DeleteVolume() error = %v, want %v", err, test.want)
			}
			if repository.deletedPool != "" {
				t.Fatal("rejected request reached repository")
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestDeleteVolumePropagatesInUseError(t *testing.T) {
	repository := &fakeRepository{deleteErr: domain.ErrVolumeInUse}
	audit := &fakeAuditRecorder{}
	err := New(repository, audit, false).DeleteVolume(context.Background(), DeleteVolumeCommand{
		Pool: "default", Name: "attached.qcow2", Request: MutationContext{Session: operatorSession()},
	})
	if !errors.Is(err, domain.ErrVolumeInUse) {
		t.Fatalf("DeleteVolume() error = %v", err)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestGetPoolNormalizesAndValidates(t *testing.T) {
	service := New(&fakeRepository{getPool: domain.StoragePool{Name: "default", State: domain.StoragePoolStateActive}}, nil, false)
	pool, err := service.GetPool(context.Background(), " default ")
	if err != nil || pool.Name != "default" {
		t.Fatalf("GetPool() = %+v, %v", pool, err)
	}
	if _, err := service.GetPool(context.Background(), "../default"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid name error = %v", err)
	}
}

func TestCreatePoolNormalizesAndAudits(t *testing.T) {
	repository := &fakeRepository{}
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false)
	pool, err := service.CreatePool(context.Background(), CreatePoolCommand{
		Spec: domain.StoragePoolCreateSpec{
			Name: " data-pool ", Type: " DIR ", TargetPath: " /srv/virtmgr/data/../data ",
			Autostart: true, Start: true,
		},
		Request: MutationContext{Session: operatorSession(), RequestID: "req-pool", SourceIP: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("CreatePool() error = %v", err)
	}
	if pool.Name != "data-pool" || repository.createdPoolSpec.Name != "data-pool" || repository.createdPoolSpec.Type != "dir" || repository.createdPoolSpec.TargetPath != "/srv/virtmgr/data" {
		t.Fatalf("created pool=%+v spec=%+v", pool, repository.createdPoolSpec)
	}
	if !repository.createdPoolSpec.Autostart || !repository.createdPoolSpec.Start {
		t.Fatalf("create flags = %+v", repository.createdPoolSpec)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	event := audit.events[0]
	if event.Action != "storage.pool.create" || event.TargetKind != "storage_pool" || event.TargetID != "data-pool" || event.Result != "success" {
		t.Fatalf("audit event = %+v", event)
	}
}

func TestCreatePoolRejectsInvalidRequests(t *testing.T) {
	for _, test := range []struct {
		name     string
		spec     domain.StoragePoolCreateSpec
		request  MutationContext
		readOnly bool
		want     error
	}{
		{name: "viewer", spec: domain.StoragePoolCreateSpec{Name: "pool-a", TargetPath: "/srv/pool-a"}, want: domain.ErrForbidden},
		{name: "read only", spec: domain.StoragePoolCreateSpec{Name: "pool-a", TargetPath: "/srv/pool-a"}, request: MutationContext{Session: operatorSession()}, readOnly: true, want: domain.ErrReadOnly},
		{name: "bad name", spec: domain.StoragePoolCreateSpec{Name: "../pool", TargetPath: "/srv/pool-a"}, request: MutationContext{Session: operatorSession()}, want: domain.ErrInvalidInput},
		{name: "relative path", spec: domain.StoragePoolCreateSpec{Name: "pool-a", TargetPath: "srv/pool-a"}, request: MutationContext{Session: operatorSession()}, want: domain.ErrInvalidInput},
		{name: "root path", spec: domain.StoragePoolCreateSpec{Name: "pool-a", TargetPath: "/"}, request: MutationContext{Session: operatorSession()}, want: domain.ErrInvalidInput},
		{name: "unsupported type", spec: domain.StoragePoolCreateSpec{Name: "pool-a", Type: "zfs", TargetPath: "/srv/pool-a"}, request: MutationContext{Session: operatorSession()}, want: domain.ErrInvalidInput},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			audit := &fakeAuditRecorder{}
			_, err := New(repository, audit, test.readOnly).CreatePool(context.Background(), CreatePoolCommand{Spec: test.spec, Request: test.request})
			if !errors.Is(err, test.want) {
				t.Fatalf("CreatePool() error = %v, want %v", err, test.want)
			}
			if repository.createdPoolSpec.Name != "" {
				t.Fatal("rejected request reached repository")
			}
			if len(audit.events) != 1 || audit.events[0].Result != "failure" || audit.events[0].TargetKind != "storage_pool" {
				t.Fatalf("audit events = %+v", audit.events)
			}
		})
	}
}

func TestStoragePoolActionsAudit(t *testing.T) {
	repository := &fakeRepository{}
	audit := &fakeAuditRecorder{}
	service := New(repository, audit, false)
	for _, action := range []domain.StoragePoolAction{domain.StoragePoolActionStart, domain.StoragePoolActionStop, domain.StoragePoolActionRefresh} {
		if err := service.PerformPoolAction(context.Background(), StoragePoolActionCommand{
			Name: " data-pool ", Action: action, Request: MutationContext{Session: operatorSession()},
		}); err != nil {
			t.Fatalf("PerformPoolAction(%s) error = %v", action, err)
		}
	}
	if repository.performedPool != "data-pool" || repository.performedPoolAction != domain.StoragePoolActionRefresh {
		t.Fatalf("performed pool=%q action=%q", repository.performedPool, repository.performedPoolAction)
	}
	if len(audit.events) != 3 {
		t.Fatalf("audit events = %+v", audit.events)
	}
	for i, action := range []domain.StoragePoolAction{domain.StoragePoolActionStart, domain.StoragePoolActionStop, domain.StoragePoolActionRefresh} {
		event := audit.events[i]
		if event.Action != "storage.pool."+string(action) || event.TargetKind != "storage_pool" || event.TargetID != "data-pool" || event.Result != "success" {
			t.Fatalf("audit event %d = %+v", i, event)
		}
	}
}

func TestStoragePoolActionRejectsInvalidRequests(t *testing.T) {
	repository := &fakeRepository{}
	audit := &fakeAuditRecorder{}
	err := New(repository, audit, false).PerformPoolAction(context.Background(), StoragePoolActionCommand{
		Name: "data-pool", Action: "destroy", Request: MutationContext{Session: operatorSession()},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid action error = %v", err)
	}
	if repository.performedPool != "" {
		t.Fatal("invalid action reached repository")
	}
	if len(audit.events) != 1 || audit.events[0].Result != "failure" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestDeletePoolAudits(t *testing.T) {
	repository := &fakeRepository{}
	audit := &fakeAuditRecorder{}
	err := New(repository, audit, false).DeletePool(context.Background(), DeletePoolCommand{
		Name: " data-pool ", Request: MutationContext{Session: operatorSession()},
	})
	if err != nil {
		t.Fatalf("DeletePool() error = %v", err)
	}
	if repository.deletedPoolName != "data-pool" {
		t.Fatalf("deleted pool = %q", repository.deletedPoolName)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "storage.pool.delete" || audit.events[0].TargetKind != "storage_pool" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func operatorSession() domain.Session {
	return domain.Session{User: domain.User{Username: "operator", Role: domain.RoleOperator}, ExpiresAt: time.Now().Add(time.Hour)}
}
