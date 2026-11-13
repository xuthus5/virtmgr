package hosthttp

import (
	"fmt"
	stdhttp "net/http"
	"strings"

	hostapp "virtmgr/internal/modules/host/app"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *hostapp.Service
}

func New(service *hostapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	details, err := h.service.Get(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, details)
}

func (h *Handler) Devices(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	devices, err := h.service.Devices(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, devices)
}

func (h *Handler) Metrics(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	metrics, err := h.service.Metrics(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, metrics)
}

func (h *Handler) Prometheus(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	details, err := h.service.Get(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	metrics, err := h.service.Metrics(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	counts := map[string]int{"running": 0, "paused": 0, "shutoff": 0, "failed": 0}
	for _, vm := range details.VMs {
		switch vm.State {
		case "running", "blocked":
			counts["running"]++
		case "paused", "pmsuspended":
			counts["paused"]++
		case "shutoff":
			counts["shutoff"]++
		case "crashed":
			counts["failed"]++
		}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	writeMetric := func(name, help string, value any) {
		_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %v\n", name, help, name, name, value)
	}
	writeMetric("virtmgr_up", "Whether VirtMgr can read the host and libvirt.", 1)
	writeMetric("virtmgr_host_cpu_percent", "Current host CPU usage percentage.", metrics.CPUPercent)
	writeMetric("virtmgr_host_memory_used_bytes", "Current host used memory in bytes.", metrics.MemoryUsedBytes)
	writeMetric("virtmgr_host_memory_total_bytes", "Total host memory in bytes.", metrics.MemoryTotalBytes)
	writeMetric("virtmgr_host_network_receive_bytes_per_second", "Current host receive throughput.", metrics.NetworkRxBytesPerSecond)
	writeMetric("virtmgr_host_network_transmit_bytes_per_second", "Current host transmit throughput.", metrics.NetworkTxBytesPerSecond)
	writeMetric("virtmgr_vms_total", "Total visible virtual machines.", len(details.VMs))
	for _, state := range []string{"running", "paused", "shutoff", "failed"} {
		_, _ = fmt.Fprintf(w, "virtmgr_vms_by_state{state=%q} %d\n", strings.TrimSpace(state), counts[state])
	}
	for _, pool := range details.StoragePools {
		_, _ = fmt.Fprintf(w, "virtmgr_storage_pool_active{pool=%q} %d\n", metricLabel(pool.Name), boolMetric(pool.State == "running"))
	}
}

func metricLabel(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n").Replace(value)
}

func boolMetric(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/host", h.Get)
	r.Get("/host/devices", h.Devices)
	r.Get("/host/metrics", h.Metrics)
}
