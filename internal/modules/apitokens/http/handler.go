package http

import (
	_ "embed"
	stdhttp "net/http"
	"strings"

	"virtmgr/internal/domain"
	apitokensapp "virtmgr/internal/modules/apitokens/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed openapi.json
var openAPIContract []byte

type Handler struct {
	service *apitokensapp.Service
}

func New(service *apitokensapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) MountPublic(r chi.Router) {
	r.Get("/openapi.json", h.OpenAPI)
}

func (h *Handler) MountProtected(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/api-tokens", h.List)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/api-tokens", h.Create)
		mutating.Delete("/api-tokens/{id}", h.Revoke)
	})
}

func (h *Handler) OpenAPI(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="virtmgr-openapi.json"`)
	w.WriteHeader(stdhttp.StatusOK)
	_, _ = w.Write(openAPIContract)
}

func (h *Handler) List(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := browserSession(r)
	if !ok {
		httpx.Error(w, r, domain.ErrForbidden)
		return
	}
	result, err := h.service.List(r.Context(), session)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, result)
}

func (h *Handler) Create(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := browserSession(r)
	if !ok {
		httpx.Error(w, r, domain.ErrForbidden)
		return
	}
	var spec domain.APITokenCreateSpec
	if err := httpx.DecodeJSON(r, &spec); err != nil {
		httpx.Error(w, r, err)
		return
	}
	result, err := h.service.Create(r.Context(), spec, mutationContext(r, session))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, result)
}

func (h *Handler) Revoke(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := browserSession(r)
	if !ok {
		httpx.Error(w, r, domain.ErrForbidden)
		return
	}
	if err := h.service.Revoke(r.Context(), chi.URLParam(r, "id"), mutationContext(r, session)); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func browserSession(r *stdhttp.Request) (domain.Session, bool) {
	session, ok := authctx.Session(r.Context())
	if !ok || session.IsAPIToken() {
		return domain.Session{}, false
	}
	return session, true
}

func mutationContext(r *stdhttp.Request, session domain.Session) apitokensapp.MutationContext {
	return apitokensapp.MutationContext{
		Session: session, RequestID: middleware.GetReqID(r.Context()), SourceIP: httpx.SourceIP(r),
	}
}

func bearerToken(r *stdhttp.Request) (string, bool) {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) < 7 || !strings.EqualFold(value[:7], "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(value[7:])
	return token, token != ""
}
