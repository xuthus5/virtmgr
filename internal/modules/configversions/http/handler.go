package configversionshttp

import (
	stdhttp "net/http"
	"strconv"

	"virtmgr/internal/domain"
	configversionsapp "virtmgr/internal/modules/configversions/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *configversionsapp.Service
}

type rollbackInput struct {
	XMLSHA256 string `json:"xmlSha256"`
	Note      string `json:"note,omitempty"`
}

type protectInput struct {
	Protected *bool `json:"protected"`
}

func New(service *configversionsapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	result, err := h.service.List(r.Context(), chi.URLParam(r, "uuid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	versionNo, err := parseVersion(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	result, err := h.service.Get(r.Context(), chi.URLParam(r, "uuid"), versionNo)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Protect(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	versionNo, err := parseVersion(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var input protectInput
	if err := httpx.DecodeJSON(r, &input); err != nil || input.Protected == nil {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	result, err := h.service.SetProtected(r.Context(), configversionsapp.ProtectCommand{
		UUID:      chi.URLParam(r, "uuid"),
		VersionNo: versionNo,
		Protected: *input.Protected,
		Request:   mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Rollback(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	versionNo, err := parseVersion(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var input rollbackInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	result, err := h.service.Rollback(r.Context(), configversionsapp.RollbackCommand{
		UUID:           chi.URLParam(r, "uuid"),
		VersionNo:      versionNo,
		ExpectedSHA256: input.XMLSHA256,
		Note:           input.Note,
		Request:        mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/vms/{uuid}/config-versions", h.List)
	r.Get("/vms/{uuid}/config-versions/{version}", h.Get)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Patch("/vms/{uuid}/config-versions/{version}", h.Protect)
		mutating.Post("/vms/{uuid}/config-versions/{version}/rollback", h.Rollback)
	})
}

func parseVersion(r *stdhttp.Request) (int, error) {
	versionNo, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || versionNo < 1 {
		return 0, domain.ErrInvalidInput
	}
	return versionNo, nil
}

func mutationContext(r *stdhttp.Request, session domain.Session) configversionsapp.MutationContext {
	return configversionsapp.MutationContext{
		Session:   session,
		RequestID: middleware.GetReqID(r.Context()),
		SourceIP:  httpx.SourceIP(r),
	}
}
