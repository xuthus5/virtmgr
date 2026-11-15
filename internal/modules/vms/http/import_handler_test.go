package vmshttp

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

type handlerImportVolumes struct{}

func (handlerImportVolumes) ListStorageVolumes(_ context.Context, poolName string) ([]domain.StorageVolume, error) {
	switch poolName {
	case "images":
		return []domain.StorageVolume{{Name: "base.qcow2", Path: "/images/base.qcow2", Format: "qcow2", CapacityBytes: 1 << 30}}, nil
	case "vms":
		return nil, nil
	default:
		return nil, domain.ErrNotFound
	}
}

func (handlerImportVolumes) CloneStorageVolume(_ context.Context, _, _, targetPool, targetName string) (domain.StorageVolume, error) {
	return domain.StorageVolume{Name: targetName, Path: "/vms/" + targetName, Format: "qcow2"}, nil
}

func (handlerImportVolumes) CreateStorageVolume(_ context.Context, poolName string, spec domain.StorageVolumeCreateSpec) (domain.StorageVolume, error) {
	return domain.StorageVolume{Name: spec.Name, Path: "/" + poolName + "/" + spec.Name, Format: spec.Format, CapacityBytes: spec.CapacityBytes}, nil
}

func (handlerImportVolumes) DeleteStorageVolume(context.Context, string, string) error { return nil }

func TestImportVMReturnsAcceptedTask(t *testing.T) {
	router := newHandlerRouter(&handlerRepository{}, &domain.Session{User: domain.User{ID: 1, Username: "operator", Role: domain.RoleOperator}}, true)
	body := `{"name":"web-import","sourcePool":"images","sourceVolume":"base.qcow2","targetPool":"vms","vcpus":2,"memoryMiB":2048,"network":"default","firmware":"bios","autostart":false,"start":false}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("POST", "/vms/import", strings.NewReader(body)))
	if recorder.Code != 202 {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"kind":"vm.import"`) || !strings.Contains(recorder.Body.String(), `"targetKind":"vm_import"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestImportVMRequiresCSRF(t *testing.T) {
	router := newHandlerRouter(&handlerRepository{}, &domain.Session{User: domain.User{ID: 1, Username: "operator", Role: domain.RoleOperator}}, false)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("POST", "/vms/import", strings.NewReader(`{}`)))
	if recorder.Code != 403 {
		t.Fatalf("status = %d", recorder.Code)
	}
}
