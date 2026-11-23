package auth

import (
	"context"
)

func Owner(ctx context.Context) (string, bool) {
	session := ctx.Value("SESSION")
	if session != nil {
		return session.(Session).Username, true
	}
	return "", false
}

func WithSession(ctx context.Context, session Session) context.Context {
	return context.WithValue(ctx, "SESSION", session)
}

func ReadSession(ctx context.Context) (Session, bool) {
	session, ok := ctx.Value("SESSION").(Session)
	if !ok {
		return Session{}, false
	}
	return session, true
}
