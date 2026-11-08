package hosthttp

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"virtmgr/internal/domain"
	hostapp "virtmgr/internal/modules/host/app"

	"github.com/go-chi/chi/v5"
)

type handlerRepository struct{}

type handlerMetrics struct{}

func (handlerMetrics) HostNamed(context.Context, string) (domain.HostMetrics, error) {
	return domain.HostMetrics{HostName: "node-01", ResourceMetrics: domain.ResourceMetrics{
		CPUPercent: 12.5, MemoryUsedBytes: 2 << 30, MemoryTotalBytes: 8 << 30,
		NetworkRxBytesPerSecond: 1024, NetworkTxBytesPerSecond: 2048,
	}}, nil
}

func (handlerRepository) Host(context.Context) (domain.Host, error) {
	return domain.Host{Name: "node-01", CPUs: 8}, nil
}
func (handlerRepository) ListHostDevices(context.Context) ([]domain.HostDevice, error) {
	return []domain.HostDevice{
		{Name: "pci_0000_00_02_0", Type: "pci", Vendor: "Intel", Address: "0000:00:02.0", Active: true},
		{Name: "usb_device_1_2", Type: "usb_device", Product: "Receiver", Address: "bus 1 · device 2", Active: true},
	}, nil
}
func (handlerRepository) ListVMs(context.Context) ([]domain.VM, error) {
	return []domain.VM{{Name: "web-01", State: domain.VMStateRunning}}, nil
}
func (handlerRepository) ListStoragePools(context.Context) ([]domain.StoragePool, error) {
	return []domain.StoragePool{{Name: "default", State: domain.StoragePoolStateActive}}, nil
}
func (handlerRepository) ListNetworks(context.Context) ([]domain.VirtualNetwork, error) {
	return []domain.VirtualNetwork{{Name: "default", Active: true}}, nil
}

func TestGetHost(t *testing.T) {
	router := chi.NewRouter()
	New(hostapp.New(handlerRepository{})).Mount(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/host", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var details domain.HostDetails
	if err := json.NewDecoder(recorder.Body).Decode(&details); err != nil {
		t.Fatal(err)
	}
	if details.Host.Name != "node-01" || details.Counts.Running != 1 || len(details.StoragePools) != 1 || len(details.Networks) != 1 {
		t.Fatalf("details = %+v", details)
	}
}

func TestGetHostDevices(t *testing.T) {
	router := chi.NewRouter()
	New(hostapp.New(handlerRepository{})).Mount(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/host/devices", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var devices domain.HostDeviceList
	if err := json.NewDecoder(recorder.Body).Decode(&devices); err != nil {
		t.Fatal(err)
	}
	if devices.Total != 2 || devices.Items[0].Name != "pci_0000_00_02_0" || devices.Items[1].Type != "usb_device" {
		t.Fatalf("devices = %+v", devices)
	}
}

func TestGetHostMetrics(t *testing.T) {
	router := chi.NewRouter()
	New(hostapp.New(handlerRepository{}, hostapp.WithMetrics(handlerMetrics{}))).Mount(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/host/metrics", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var metrics domain.HostMetrics
	if err := json.NewDecoder(recorder.Body).Decode(&metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.HostName != "node-01" || metrics.CPUPercent != 12.5 || metrics.MemoryTotalBytes != 8<<30 || metrics.NetworkTxBytesPerSecond != 2048 {
		t.Fatalf("metrics = %+v", metrics)
	}
}
