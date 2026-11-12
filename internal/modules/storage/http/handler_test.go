package storagehttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"virtmgr/internal/domain"
	storageapp "virtmgr/internal/modules/storage/app"
	"virtmgr/internal/platform/authctx"

	"github.com/go-chi/chi/v5"
)

type handlerRepository struct {
	pools               []domain.StoragePool
	volumes             []domain.StorageVolume
	err                 error
	getPool             domain.StoragePool
	getPoolErr          error
	createdPoolSpec     domain.StoragePoolCreateSpec
	createPoolErr       error
	performedPool       string
	performedPoolAction domain.StoragePoolAction
	poolActionErr       error
	deletedPoolName     string
	deletePoolErr       error
	createdPool         string
	createdSpec         domain.StorageVolumeCreateSpec
	uploadPool          string
	uploadSpec          domain.StorageVolumeCreateSpec
	uploadBytes         []byte
	deletedPool         string
	deletedName         string
}

func (f *handlerRepository) ListStoragePools(context.Context) ([]domain.StoragePool, error) {
	return f.pools, f.err
}

func (f *handlerRepository) GetStoragePool(_ context.Context, name string) (domain.StoragePool, error) {
	if f.getPoolErr != nil {
		return domain.StoragePool{}, f.getPoolErr
	}
	if f.getPool.Name == "" {
		return domain.StoragePool{Name: name, State: domain.StoragePoolStateActive}, nil
	}
	return f.getPool, nil
}

func (f *handlerRepository) CreateStoragePool(_ context.Context, spec domain.StoragePoolCreateSpec) (domain.StoragePool, error) {
	f.createdPoolSpec = spec
	if f.createPoolErr != nil {
		return domain.StoragePool{}, f.createPoolErr
	}
	return domain.StoragePool{Name: spec.Name, Type: spec.Type, TargetPath: spec.TargetPath, State: domain.StoragePoolStateInactive}, nil
}

func (f *handlerRepository) PerformStoragePoolAction(_ context.Context, name string, action domain.StoragePoolAction) error {
	f.performedPool = name
	f.performedPoolAction = action
	return f.poolActionErr
}

func (f *handlerRepository) DeleteStoragePool(_ context.Context, name string) error {
	f.deletedPoolName = name
	return f.deletePoolErr
}

func (f *handlerRepository) ListStorageVolumes(context.Context, string) ([]domain.StorageVolume, error) {
	return f.volumes, f.err
}

func (f *handlerRepository) CreateStorageVolume(_ context.Context, poolName string, spec domain.StorageVolumeCreateSpec) (domain.StorageVolume, error) {
	f.createdPool = poolName
	f.createdSpec = spec
	if f.err != nil {
		return domain.StorageVolume{}, f.err
	}
	return domain.StorageVolume{Name: spec.Name, Format: spec.Format, CapacityBytes: spec.CapacityBytes}, nil
}

func (f *handlerRepository) UploadStorageVolume(_ context.Context, poolName string, spec domain.StorageVolumeCreateSpec, reader io.Reader) (domain.StorageVolume, error) {
	f.uploadPool = poolName
	f.uploadSpec = spec
	f.uploadBytes, _ = io.ReadAll(reader)
	if f.err != nil {
		return domain.StorageVolume{}, f.err
	}
	return domain.StorageVolume{Name: spec.Name, Format: spec.Format, CapacityBytes: spec.CapacityBytes}, nil
}

func (f *handlerRepository) DeleteStorageVolume(_ context.Context, poolName, volumeName string) error {
	f.deletedPool = poolName
	f.deletedName = volumeName
	return f.err
}

func allowAll(next http.Handler) http.Handler { return next }

func TestStorageRoutes(t *testing.T) {
	router := chi.NewRouter()
	New(storageapp.New(&handlerRepository{
		pools:   []domain.StoragePool{{Name: "default", State: domain.StoragePoolStateActive}},
		volumes: []domain.StorageVolume{{Name: "demo.qcow2", Format: "qcow2"}},
	}, nil, false)).Mount(router, allowAll)

	for _, path := range []string{"/storage/pools", "/storage/pools/default", "/storage/pools/default/volumes"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestCreateStoragePoolRoute(t *testing.T) {
	repository := &handlerRepository{}
	router := chi.NewRouter()
	New(storageapp.New(repository, nil, false)).Mount(router, allowAll)
	request := httptest.NewRequest(http.MethodPost, "/storage/pools", strings.NewReader(`{"name":"data-pool","type":"dir","targetPath":"/srv/virtmgr/data","autostart":true,"start":true}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(authctx.WithSession(request.Context(), domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.createdPoolSpec.Name != "data-pool" || repository.createdPoolSpec.TargetPath != "/srv/virtmgr/data" || !repository.createdPoolSpec.Start {
		t.Fatalf("created spec = %+v", repository.createdPoolSpec)
	}
}

func TestStoragePoolActionRoute(t *testing.T) {
	repository := &handlerRepository{}
	router := chi.NewRouter()
	New(storageapp.New(repository, nil, false)).Mount(router, allowAll)
	request := httptest.NewRequest(http.MethodPost, "/storage/pools/data-pool/actions/refresh", nil)
	request = request.WithContext(authctx.WithSession(request.Context(), domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.performedPool != "data-pool" || repository.performedPoolAction != domain.StoragePoolActionRefresh {
		t.Fatalf("performed pool=%q action=%q", repository.performedPool, repository.performedPoolAction)
	}
}

func TestDeleteStoragePoolRoute(t *testing.T) {
	repository := &handlerRepository{}
	router := chi.NewRouter()
	New(storageapp.New(repository, nil, false)).Mount(router, allowAll)
	request := httptest.NewRequest(http.MethodDelete, "/storage/pools/data-pool", nil)
	request = request.WithContext(authctx.WithSession(request.Context(), domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.deletedPoolName != "data-pool" {
		t.Fatalf("deleted pool = %q", repository.deletedPoolName)
	}
}

func TestStoragePoolMutationRequiresSession(t *testing.T) {
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/storage/pools", strings.NewReader(`{"name":"data-pool","targetPath":"/srv/data"}`)),
		httptest.NewRequest(http.MethodPost, "/storage/pools/data-pool/actions/start", nil),
		httptest.NewRequest(http.MethodDelete, "/storage/pools/data-pool", nil),
	} {
		router := chi.NewRouter()
		New(storageapp.New(&handlerRepository{}, nil, false)).Mount(router, allowAll)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d body=%s", request.Method, request.URL.Path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestCreateStorageVolumeRoute(t *testing.T) {
	repository := &handlerRepository{}
	router := chi.NewRouter()
	New(storageapp.New(repository, nil, false)).Mount(router, allowAll)
	request := httptest.NewRequest(http.MethodPost, "/storage/pools/default/volumes", strings.NewReader(`{"name":"data-01.qcow2","format":"qcow2","capacityBytes":1073741824}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(authctx.WithSession(request.Context(), domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.createdPool != "default" || repository.createdSpec.Name != "data-01.qcow2" || repository.createdSpec.CapacityBytes != 1<<30 {
		t.Fatalf("created pool=%q spec=%+v", repository.createdPool, repository.createdSpec)
	}
}

func TestUploadStorageVolumeRoute(t *testing.T) {
	repository := &handlerRepository{}
	router := chi.NewRouter()
	New(storageapp.New(repository, nil, false)).Mount(router, allowAll)
	body := bytes.Repeat([]byte{0x42}, 1<<20)
	request := httptest.NewRequest(http.MethodPost, "/storage/pools/iso-store/volumes/upload?name=install.iso", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/octet-stream")
	request = request.WithContext(authctx.WithSession(request.Context(), domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.uploadPool != "iso-store" || repository.uploadSpec.Name != "install.iso" || repository.uploadSpec.Format != "raw" || repository.uploadSpec.CapacityBytes != 1<<20 {
		t.Fatalf("upload pool=%q spec=%+v", repository.uploadPool, repository.uploadSpec)
	}
	if len(repository.uploadBytes) != len(body) {
		t.Fatalf("uploaded bytes = %d", len(repository.uploadBytes))
	}
}

func TestUploadDiskImageRoutePreservesFormat(t *testing.T) {
	repository := &handlerRepository{}
	router := chi.NewRouter()
	New(storageapp.New(repository, nil, false)).Mount(router, allowAll)
	request := httptest.NewRequest(http.MethodPost, "/storage/pools/images/volumes/upload?name=base.qcow2", bytes.NewReader(make([]byte, 1<<20)))
	request.Header.Set("Content-Type", "application/octet-stream")
	request = request.WithContext(authctx.WithSession(request.Context(), domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.uploadSpec.Format != "qcow2" || repository.uploadSpec.Name != "base.qcow2" {
		t.Fatalf("upload spec=%+v", repository.uploadSpec)
	}
}

func TestUploadStorageVolumeRequiresSessionAndLength(t *testing.T) {
	repository := &handlerRepository{}
	router := chi.NewRouter()
	New(storageapp.New(repository, nil, false)).Mount(router, allowAll)

	request := httptest.NewRequest(http.MethodPost, "/storage/pools/iso-store/volumes/upload?name=install.iso", bytes.NewReader(make([]byte, 1<<20)))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/storage/pools/iso-store/volumes/upload?name=install.iso", nil)
	request = request.WithContext(authctx.WithSession(request.Context(), domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}))
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing length status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.uploadPool != "" {
		t.Fatal("invalid upload reached repository")
	}
}

func TestCreateStorageVolumeRequiresSession(t *testing.T) {
	router := chi.NewRouter()
	New(storageapp.New(&handlerRepository{}, nil, false)).Mount(router, allowAll)
	request := httptest.NewRequest(http.MethodPost, "/storage/pools/default/volumes", strings.NewReader(`{"name":"data-01.qcow2","format":"qcow2","capacityBytes":1073741824}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDeleteStorageVolumeRoute(t *testing.T) {
	repository := &handlerRepository{}
	router := chi.NewRouter()
	New(storageapp.New(repository, nil, false)).Mount(router, allowAll)
	request := httptest.NewRequest(http.MethodDelete, "/storage/pools/default/volumes/data-01.qcow2", nil)
	request = request.WithContext(authctx.WithSession(request.Context(), domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.deletedPool != "default" || repository.deletedName != "data-01.qcow2" {
		t.Fatalf("deleted pool=%q name=%q", repository.deletedPool, repository.deletedName)
	}
}

func TestStorageRouteError(t *testing.T) {
	router := chi.NewRouter()
	New(storageapp.New(&handlerRepository{err: domain.ErrUnavailable}, nil, false)).Mount(router, allowAll)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/storage/pools", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
