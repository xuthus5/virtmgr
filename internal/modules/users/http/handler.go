package usershttp

import (
	"net"
	stdhttp "net/http"
	"strconv"
	"strings"

	"virtmgr/internal/domain"
	usersapp "virtmgr/internal/modules/users/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service *usersapp.Service
}

type createRequest struct {
	Username string      `json:"username"`
	Password string      `json:"password"`
	Role     domain.Role `json:"role"`
}

type updateRequest struct {
	Role     domain.Role `json:"role"`
	Disabled bool        `json:"disabled"`
}

func New(service *usersapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
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
	var request createRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	user, err := h.service.Create(r.Context(), usersapp.CreateUserCommand{
		Username: request.Username,
		Password: request.Password,
		Role:     request.Role,
		Request:  mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, user)
}

func (h *Handler) Update(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		httpx.Error(w, r, domain.ErrInvalidInput)
		return
	}
	var request updateRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	user, err := h.service.Update(r.Context(), usersapp.UpdateUserCommand{
		ID:       id,
		Role:     request.Role,
		Disabled: request.Disabled,
		Request:  mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, user)
}

func (h *Handler) Mount(r chi.Router, csrf func(stdhttp.Handler) stdhttp.Handler) {
	r.Get("/users", h.List)
	r.Group(func(mutating chi.Router) {
		mutating.Use(csrf)
		mutating.Post("/users", h.Create)
		mutating.Patch("/users/{id}", h.Update)
	})
}

func mutationContext(r *stdhttp.Request, session domain.Session) usersapp.MutationContext {
	return usersapp.MutationContext{
		Session:   session,
		RequestID: middleware.GetReqID(r.Context()),
		SourceIP:  sourceIP(r),
	}
}

func sourceIP(r *stdhttp.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
