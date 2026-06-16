package auth

import "context"

type contextKey struct{}

func WithToken(ctx context.Context, token *Token) context.Context {
	return context.WithValue(ctx, contextKey{}, token)
}

func TokenFromContext(ctx context.Context) *Token {
	t, _ := ctx.Value(contextKey{}).(*Token)
	return t
}
