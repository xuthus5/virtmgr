package http

import (
	stdhttp "net/http"
	"strconv"
	"strings"

	"virtmgr/internal/domain"
	guestagentapp "virtmgr/internal/modules/guestagent/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *guestagentapp.Service
}

func New(service *guestagentapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	refresh := r.URL.Query().Get("refresh") == "true"
	result, err := h.service.Get(r.Context(), chi.URLParam(r, "uuid"), refresh)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Action(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.GuestAgentActionSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	task, err := h.service.Action(r.Context(), guestagentapp.ActionCommand{
		UUID: chi.URLParam(r, "uuid"), Spec: spec, Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, task)
}

func (h *Handler) EnsureChannel(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	vm, err := h.service.EnsureChannel(r.Context(), guestagentapp.ChannelCommand{
		UUID: chi.URLParam(r, "uuid"), Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, vm)
}

func (h *Handler) ReadFile(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	offset, err := queryUint(r, "offset")
	if err != nil {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	maxBytes, err := queryUint(r, "maxBytes")
	if err != nil {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	result, err := h.service.ReadFile(r.Context(), guestagentapp.FileReadCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Spec:    domain.GuestAgentFileReadSpec{Path: r.URL.Query().Get("path"), Offset: offset, MaxBytes: maxBytes},
		Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) WriteFile(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.GuestAgentFileWriteSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	result, err := h.service.WriteFile(r.Context(), guestagentapp.FileWriteCommand{
		UUID: chi.URLParam(r, "uuid"), Spec: spec, Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) ResetPassword(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.GuestAgentPasswordSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	task, err := h.service.ResetPassword(r.Context(), guestagentapp.PasswordCommand{
		UUID: chi.URLParam(r, "uuid"), Spec: spec, Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, task)
}

func (h *Handler) ListSSHKeys(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	result, err := h.service.ListSSHKeys(r.Context(), guestagentapp.SSHKeyListCommand{
		UUID: chi.URLParam(r, "uuid"), Username: r.URL.Query().Get("username"), Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) AddSSHKeys(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.changeSSHKeys(w, r, true)
}

func (h *Handler) RemoveSSHKeys(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.changeSSHKeys(w, r, false)
}

func (h *Handler) changeSSHKeys(w stdhttp.ResponseWriter, r *stdhttp.Request, add bool) {
	var spec domain.GuestAgentSSHKeySpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	command := guestagentapp.SSHKeyChangeCommand{UUID: chi.URLParam(r, "uuid"), Spec: spec, Request: context}
	var task domain.Task
	var err error
	if add {
		task, err = h.service.AddSSHKeys(r.Context(), command)
	} else {
		task, err = h.service.RemoveSSHKeys(r.Context(), command)
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, task)
}

func (h *Handler) ListProcesses(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	result, err := h.service.ListProcesses(r.Context(), guestagentapp.ProcessCommand{
		UUID: chi.URLParam(r, "uuid"), Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, map[string]any{"items": result, "total": len(result)})
}

func (h *Handler) ListServices(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	result, err := h.service.ListServices(r.Context(), guestagentapp.ProcessCommand{
		UUID: chi.URLParam(r, "uuid"), Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, map[string]any{"items": result, "total": len(result)})
}

func (h *Handler) CloudInit(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	result, err := h.service.CloudInit(r.Context(), guestagentapp.ProcessCommand{
		UUID: chi.URLParam(r, "uuid"), Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) SignalProcess(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.GuestAgentProcessSignalSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	task, err := h.service.SignalProcess(r.Context(), guestagentapp.ProcessSignalCommand{
		UUID: chi.URLParam(r, "uuid"), Spec: spec, Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, task)
}

func (h *Handler) ListUsers(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	result, err := h.service.ListUsers(r.Context(), guestagentapp.ProcessCommand{
		UUID: chi.URLParam(r, "uuid"), Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, map[string]any{"items": result, "total": len(result)})
}

func (h *Handler) GetPolicy(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	policy, err := h.service.GetPolicy(r.Context(), guestagentapp.PolicyQuery{
		UUID: chi.URLParam(r, "uuid"), Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, policy)
}

func (h *Handler) SetPolicy(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var spec domain.GuestAgentPolicySpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	policy, err := h.service.SetPolicy(r.Context(), guestagentapp.PolicyCommand{
		UUID: chi.URLParam(r, "uuid"), Spec: spec, Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, policy)
}

func (h *Handler) Health(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	context, ok := h.mutationContext(w, r)
	if !ok {
		return
	}
	hours, err := queryInt(r, "hours")
	if err != nil {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	limit, err := queryInt(r, "limit")
	if err != nil {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	history, err := h.service.Health(r.Context(), guestagentapp.HealthQuery{
		UUID: chi.URLParam(r, "uuid"), Hours: hours, Limit: limit, Request: context,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, history)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/vms/{uuid}/guest-agent", h.Get)
	r.Get("/vms/{uuid}/guest-agent/files", h.ReadFile)
	r.Get("/vms/{uuid}/guest-agent/ssh-keys", h.ListSSHKeys)
	r.Get("/vms/{uuid}/guest-agent/processes", h.ListProcesses)
	r.Get("/vms/{uuid}/guest-agent/services", h.ListServices)
	r.Get("/vms/{uuid}/guest-agent/cloud-init", h.CloudInit)
	r.Get("/vms/{uuid}/guest-agent/users", h.ListUsers)
	r.Get("/vms/{uuid}/guest-agent/policy", h.GetPolicy)
	r.Get("/vms/{uuid}/guest-agent/health", h.Health)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/vms/{uuid}/guest-agent/actions", h.Action)
		mutating.Post("/vms/{uuid}/guest-agent/channel", h.EnsureChannel)
		mutating.Post("/vms/{uuid}/guest-agent/files", h.WriteFile)
		mutating.Post("/vms/{uuid}/guest-agent/password", h.ResetPassword)
		mutating.Post("/vms/{uuid}/guest-agent/ssh-keys/add", h.AddSSHKeys)
		mutating.Post("/vms/{uuid}/guest-agent/ssh-keys/remove", h.RemoveSSHKeys)
		mutating.Post("/vms/{uuid}/guest-agent/processes/signal", h.SignalProcess)
		mutating.Put("/vms/{uuid}/guest-agent/policy", h.SetPolicy)
	})
}

func (h *Handler) mutationContext(w stdhttp.ResponseWriter, r *stdhttp.Request) (guestagentapp.MutationContext, bool) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return guestagentapp.MutationContext{}, false
	}
	return mutationContext(r, session), true
}

func mutationContext(r *stdhttp.Request, session domain.Session) guestagentapp.MutationContext {
	return guestagentapp.MutationContext{
		Session: session, RequestID: middleware.GetReqID(r.Context()), SourceIP: httpx.SourceIP(r),
	}
}

func queryUint(r *stdhttp.Request, name string) (uint64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseUint(raw, 10, 64)
}

func queryInt(r *stdhttp.Request, name string) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0, nil
	}
	return strconv.Atoi(raw)
}
