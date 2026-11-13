package vmshttp

import (
	stdhttp "net/http"
	"strings"

	"virtmgr/internal/domain"
	vmsapp "virtmgr/internal/modules/vms/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *vmsapp.Service
}

type hostDeviceInput struct {
	Device string `json:"device"`
}

type updateXMLInput struct {
	XML string `json:"xml"`
}

type attachDiskInput struct {
	Pool   string `json:"pool"`
	Volume string `json:"volume"`
	Target string `json:"target,omitempty"`
	domain.VMDiskBus
}

func New(service *vmsapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.List(r.Context(), domain.ListQuery{
		Query: r.URL.Query().Get("q"),
		State: r.URL.Query().Get("state"),
		Tag:   r.URL.Query().Get("tag"),
		Owner: r.URL.Query().Get("owner"),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	vm, err := h.service.Get(r.Context(), chi.URLParam(r, "uuid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) GetMetadata(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.GetMetadata(r.Context(), chi.URLParam(r, "uuid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) UpdateMetadata(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.VMMetadataSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	result, err := h.service.UpdateMetadata(r.Context(), vmsapp.VMMetadataCommand{
		UUID: chi.URLParam(r, "uuid"), Spec: spec, Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) GetXML(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.GetXML(r.Context(), chi.URLParam(r, "uuid"), r.URL.Query().Get("format"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) UpdateXML(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var input updateXMLInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	result, err := h.service.UpdateXML(r.Context(), vmsapp.XMLUpdateCommand{
		UUID:    chi.URLParam(r, "uuid"),
		XML:     input.XML,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Export(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	result, err := h.service.Export(r.Context(), vmsapp.ExportCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+exportFilename(result.VM.Name)+`"`)
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Metrics(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	metrics, err := h.service.Metrics(r.Context(), chi.URLParam(r, "uuid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, metrics)
}

func (h *Handler) Create(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.VMCreateSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	vm, err := h.service.Create(r.Context(), vmsapp.CreateCommand{
		Spec:    spec,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, vm)
}

func (h *Handler) Clone(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.VMCloneSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	task, err := h.service.Clone(r.Context(), vmsapp.CloneCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Spec:    spec,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusAccepted, task)
}

func (h *Handler) Backup(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.VMBackupSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	task, err := h.service.Backup(r.Context(), vmsapp.BackupCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Spec:    spec,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusAccepted, task)
}

func (h *Handler) Delete(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.service.Delete(r.Context(), vmsapp.DeleteCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Request: mutationContext(r, session),
	}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) SetAutostart(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if input.Enabled == nil {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	vm, err := h.service.SetAutostart(r.Context(), vmsapp.SetAutostartCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Enabled: *input.Enabled,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) UpdateResources(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.VMResourcesSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	vm, err := h.service.UpdateResources(r.Context(), vmsapp.UpdateResourcesCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Spec:    spec,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) ResizeDisk(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.VMDiskResizeSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	vm, err := h.service.ResizeDisk(r.Context(), vmsapp.ResizeDiskCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Target:  chi.URLParam(r, "target"),
		Spec:    spec,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) AttachDisk(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var input attachDiskInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	vm, err := h.service.AttachDisk(r.Context(), vmsapp.AttachDiskCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Spec:    domain.VMDiskAttachSpec{Pool: input.Pool, Volume: input.Volume, Target: input.Target, Bus: input.VMDiskBus},
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) DetachDisk(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	vm, err := h.service.DetachDisk(r.Context(), vmsapp.DetachDiskCommand{
		UUID: chi.URLParam(r, "uuid"), Target: chi.URLParam(r, "target"), Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) AttachNetwork(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.VMNetworkAttachSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	vm, err := h.service.AttachNetwork(r.Context(), vmsapp.AttachNetworkCommand{
		UUID: chi.URLParam(r, "uuid"), Spec: spec, Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) DetachNetwork(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	vm, err := h.service.DetachNetwork(r.Context(), vmsapp.DetachNetworkCommand{
		UUID: chi.URLParam(r, "uuid"), MAC: chi.URLParam(r, "mac"), Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) AttachHostDevice(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var input hostDeviceInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	vm, err := h.service.AttachHostDevice(r.Context(), vmsapp.HostDeviceCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Device:  input.Device,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) DetachHostDevice(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	vm, err := h.service.DetachHostDevice(r.Context(), vmsapp.HostDeviceCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Device:  chi.URLParam(r, "device"),
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) Action(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	action, ok := domain.ValidVMAction(chi.URLParam(r, "action"))
	if !ok {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	vm, err := h.service.PerformAction(r.Context(), vmsapp.ActionCommand{
		UUID:      chi.URLParam(r, "uuid"),
		Action:    action,
		Session:   session,
		RequestID: middleware.GetReqID(r.Context()),
		SourceIP:  httpx.SourceIP(r),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) BatchAction(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.VMBatchActionSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	result, err := h.service.BatchAction(r.Context(), vmsapp.BatchActionCommand{
		Action:    spec.Action,
		UUIDs:     spec.UUIDs,
		Session:   session,
		RequestID: middleware.GetReqID(r.Context()),
		SourceIP:  httpx.SourceIP(r),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) ListSnapshots(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.ListSnapshots(r.Context(), chi.URLParam(r, "uuid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) CreateSnapshot(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.VMSnapshotCreateSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	snapshot, err := h.service.CreateSnapshot(r.Context(), vmsapp.SnapshotCreateCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Spec:    spec,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, snapshot)
}

func (h *Handler) RevertSnapshot(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.service.RevertSnapshot(r.Context(), vmsapp.SnapshotCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Name:    chi.URLParam(r, "snapshot"),
		Request: mutationContext(r, session),
	}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) DeleteSnapshot(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.service.DeleteSnapshot(r.Context(), vmsapp.SnapshotCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Name:    chi.URLParam(r, "snapshot"),
		Request: mutationContext(r, session),
	}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/vms", h.List)
	r.Get("/vms/{uuid}/xml", h.GetXML)
	r.Get("/vms/{uuid}/export", h.Export)
	r.Get("/vms/{uuid}/snapshots", h.ListSnapshots)
	r.Get("/vms/{uuid}/metrics", h.Metrics)
	r.Get("/vms/{uuid}/console", h.OpenConsole)
	r.Get("/vms/{uuid}/serial-console", h.OpenSerialConsole)
	r.Get("/vms/{uuid}", h.Get)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/vms", h.Create)
		mutating.Post("/vms/batch-actions", h.BatchAction)
		mutating.Post("/vms/{uuid}/clone", h.Clone)
		mutating.Post("/vms/{uuid}/backups", h.Backup)
		mutating.Put("/vms/{uuid}/xml", h.UpdateXML)
		mutating.Delete("/vms/{uuid}", h.Delete)
		mutating.Patch("/vms/{uuid}/autostart", h.SetAutostart)
		mutating.Patch("/vms/{uuid}/resources", h.UpdateResources)
		mutating.Patch("/vms/{uuid}/disks/{target}", h.ResizeDisk)
		mutating.Put("/vms/{uuid}/metadata", h.UpdateMetadata)
		mutating.Post("/vms/{uuid}/disks", h.AttachDisk)
		mutating.Delete("/vms/{uuid}/disks/{target}", h.DetachDisk)
		mutating.Post("/vms/{uuid}/network-interfaces", h.AttachNetwork)
		mutating.Delete("/vms/{uuid}/network-interfaces/{mac}", h.DetachNetwork)
		mutating.Post("/vms/{uuid}/actions/{action}", h.Action)
		mutating.Post("/vms/{uuid}/devices", h.AttachHostDevice)
		mutating.Delete("/vms/{uuid}/devices/{device}", h.DetachHostDevice)
		mutating.Post("/vms/{uuid}/snapshots", h.CreateSnapshot)
		mutating.Post("/vms/{uuid}/snapshots/{snapshot}/revert", h.RevertSnapshot)
		mutating.Delete("/vms/{uuid}/snapshots/{snapshot}", h.DeleteSnapshot)
		mutating.Post("/vms/{uuid}/console-token", h.IssueConsoleToken)
		mutating.Post("/vms/{uuid}/console/send-key", h.SendConsoleKey)
	})
	r.Get("/vms/{uuid}/metadata", h.GetMetadata)
}

func exportFilename(name string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(name) {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9', char == '.', char == '_', char == '-':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	cleaned := strings.Trim(builder.String(), "._-")
	if cleaned == "" {
		cleaned = "vm"
	}
	return cleaned + "-config-export.json"
}

func mutationContext(r *stdhttp.Request, session domain.Session) vmsapp.MutationContext {
	return vmsapp.MutationContext{
		Session:   session,
		RequestID: middleware.GetReqID(r.Context()),
		SourceIP:  httpx.SourceIP(r),
	}
}
