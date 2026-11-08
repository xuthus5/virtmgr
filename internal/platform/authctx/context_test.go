package authctx

import (
	"context"
	"testing"

	"virtmgr/internal/domain"
)

func TestSessionContext(t *testing.T) {
	if _, ok := Session(context.Background()); ok {
		t.Fatal("empty context returned a session")
	}
	session := domain.Session{User: domain.User{ID: 1, Username: "admin"}}
	ctx := WithSession(context.Background(), session)
	got, ok := Session(ctx)
	if !ok || got.User.Username != "admin" {
		t.Fatalf("Session() = %+v, %v", got, ok)
	}
}
