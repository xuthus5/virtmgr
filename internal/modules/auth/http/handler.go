package authhttp

import (
	"context"
	"crypto/subtle"
	stdhttp "net/http"
	"time"

	"virtmgr/internal/domain"
	authapp "virtmgr/internal/modules/auth/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

const CookieName = "virtmgr_session"

type Handler struct {
	service      *authapp.Service
	cookieSecure bool
	sessionTTL   time.Duration
	readOnly     bool
	readOnlyMode domain.ReadOnlyProvider
}

type Option func(*Handler)

func WithReadOnlyMode(mode domain.ReadOnlyProvider) Option {
	return func(handler *Handler) {
		handler.readOnlyMode = mode
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionResponse struct {
	User      domain.User `json:"user"`
	AuthMode  string      `json:"authMode"`
	ReadOnly  bool        `json:"readOnly"`
	CSRFToken string      `json:"csrfToken"`
	ExpiresAt time.Time   `json:"expiresAt"`
}

func New(service *authapp.Service, cookieSecure bool, sessionTTL time.Duration, readOnly bool, options ...Option) *Handler {
	handler := &Handler{service: service, cookieSecure: cookieSecure, sessionTTL: sessionTTL, readOnly: readOnly}
	for _, option := range options {
		option(handler)
	}
	return handler
}

func (h *Handler) isReadOnly() bool {
	if h.readOnlyMode != nil {
		return h.readOnlyMode.ReadOnly()
	}
	return h.readOnly
}

func (h *Handler) Mount(r chi.Router) {
	r.Post("/session", h.Login)
	r.Get("/session", h.Get)
	r.Delete("/session", h.Logout)
}

func (h *Handler) Login(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var request loginRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.Error(w, r, err)
		return
	}
	result, err := h.service.Login(r.Context(), request.Username, request.Password, httpx.SourceIP(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	stdhttp.SetCookie(w, &stdhttp.Cookie{
		Name:     CookieName,
		Value:    result.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: stdhttp.SameSiteStrictMode,
		MaxAge:   int(h.sessionTTL.Seconds()),
		Expires:  result.Session.ExpiresAt,
	})
	httpx.JSON(w, stdhttp.StatusOK, h.response(result.Session))
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, err := h.SessionFromRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusOK, h.response(session))
}

func (h *Handler) Logout(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if cookie, err := r.Cookie(CookieName); err == nil {
		if err := h.service.Logout(r.Context(), cookie.Value); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	stdhttp.SetCookie(w, &stdhttp.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: stdhttp.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *Handler) SessionFromRequest(r *stdhttp.Request) (domain.Session, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		return domain.Session{}, domain.ErrUnauthorized
	}
	return h.service.Authenticate(r.Context(), cookie.Value)
}

func (h *Handler) RequireSession(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		session, err := h.SessionFromRequest(r)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(authctx.WithSession(r.Context(), session)))
	})
}

func SessionFromContext(ctx context.Context) (domain.Session, bool) {
	return authctx.Session(ctx)
}

func (h *Handler) RequireCSRF(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		session, ok := authctx.Session(r.Context())
		if !ok {
			httpx.Error(w, r, domain.ErrUnauthorized)
			return
		}
		provided := r.Header.Get("X-CSRF-Token")
		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(session.CSRFToken)) != 1 {
			httpx.Error(w, r, domain.ErrCSRF)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) response(session domain.Session) sessionResponse {
	session.User.PasswordHash = ""
	return sessionResponse{
		User:      session.User,
		AuthMode:  "local",
		ReadOnly:  h.isReadOnly(),
		CSRFToken: session.CSRFToken,
		ExpiresAt: session.ExpiresAt,
	}
}
