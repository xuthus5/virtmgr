package vmshttp

import (
	"io"
	stdhttp "net/http"
	"net/url"
	"strings"
	"time"

	"virtmgr/internal/domain"
	vmsapp "virtmgr/internal/modules/vms/app"
	"virtmgr/internal/platform/authctx"
	"virtmgr/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
)

var consoleUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(r *stdhttp.Request) bool {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			return true
		}
		parsed, err := url.Parse(origin)
		return err == nil && strings.EqualFold(parsed.Host, r.Host)
	},
}

type sendConsoleKeyInput struct {
	Key string `json:"key"`
}

type consoleTokenInput struct {
	Kind   string `json:"kind"`
	Device string `json:"device,omitempty"`
}

func (h *Handler) IssueConsoleToken(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var input consoleTokenInput
	if r.Body != nil && r.ContentLength != 0 {
		if err := httpx.DecodeJSON(r, &input); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	ticket, err := h.service.IssueConsoleToken(r.Context(), vmsapp.ConsoleTokenCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Kind:    input.Kind,
		Device:  input.Device,
		Request: mutationContext(r, session),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, stdhttp.StatusCreated, ticket)
}

func (h *Handler) OpenConsole(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	connection, err := h.service.OpenConsole(r.Context(), vmsapp.ConsoleOpenCommand{
		UUID:  chi.URLParam(r, "uuid"),
		Token: r.URL.Query().Get("token"),
		Request: vmsapp.MutationContext{
			Session:   session,
			RequestID: middleware.GetReqID(r.Context()),
			SourceIP:  sourceIP(r),
		},
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	ws, err := consoleUpgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = connection.Close()
		return
	}
	bridgeConsole(connection, ws)
}

func (h *Handler) OpenSerialConsole(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	connection, err := h.service.OpenSerialConsole(r.Context(), vmsapp.ConsoleOpenCommand{
		UUID:  chi.URLParam(r, "uuid"),
		Token: r.URL.Query().Get("token"),
		Request: vmsapp.MutationContext{
			Session:   session,
			RequestID: middleware.GetReqID(r.Context()),
			SourceIP:  sourceIP(r),
		},
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	ws, err := consoleUpgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = connection.Close()
		return
	}
	bridgeConsole(connection, ws)
}

func (h *Handler) SendConsoleKey(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var input sendConsoleKeyInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(w, r, err)
		return
	}
	session, ok := authctx.Session(r.Context())
	if !ok {
		httpx.Error(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.service.SendConsoleKey(r.Context(), vmsapp.SendConsoleKeyCommand{
		UUID:    chi.URLParam(r, "uuid"),
		Key:     input.Key,
		Request: mutationContext(r, session),
	}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func bridgeConsole(connection io.ReadWriteCloser, ws *websocket.Conn) {
	defer func() { _ = connection.Close() }()
	defer func() { _ = ws.Close() }()
	ws.SetReadLimit(2 << 20)
	done := make(chan struct{}, 2)

	go func() {
		buffer := make([]byte, 32*1024)
		for {
			count, readErr := connection.Read(buffer)
			if count > 0 {
				_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if writeErr := ws.WriteMessage(websocket.BinaryMessage, buffer[:count]); writeErr != nil {
					done <- struct{}{}
					return
				}
			}
			if readErr != nil {
				done <- struct{}{}
				return
			}
		}
	}()

	go func() {
		for {
			_, reader, readErr := ws.NextReader()
			if readErr != nil {
				done <- struct{}{}
				return
			}
			if _, copyErr := io.Copy(connection, reader); copyErr != nil {
				done <- struct{}{}
				return
			}
		}
	}()

	<-done
}
