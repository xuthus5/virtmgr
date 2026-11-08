package authctx

import (
	"context"

	"virtmgr/internal/domain"
)

type contextKey string

const sessionKey contextKey = "virtmgr-session"

func WithSession(ctx context.Context, session domain.Session) context.Context {
	return context.WithValue(ctx, sessionKey, session)
}

func Session(ctx context.Context) (domain.Session, bool) {
	session, ok := ctx.Value(sessionKey).(domain.Session)
	return session, ok && session.User.ID != 0
}
