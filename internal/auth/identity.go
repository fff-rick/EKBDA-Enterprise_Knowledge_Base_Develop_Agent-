package auth

import (
	"context"
	"errors"
	"net/http"
)

var ErrUnauthenticated = errors.New("request is not authenticated")

type Identity struct {
	UserID string
	Roles  []string
	Source string
}

type Authenticator interface {
	Authenticate(*http.Request) (Identity, error)
	Mode() string
}

type identityContextKey struct{}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, identity)
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	return identity, ok
}
