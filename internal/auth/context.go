// Package auth provides request-scoped authentication helpers: storing and
// retrieving the authenticated domain user on a context.Context, plus (in
// sibling files) token issuing/verification and HTTP middleware.
package auth

import (
	"context"

	"migrated-app/internal/core"
)

// userContextKey is an unexported key type so no other package can collide
// with or overwrite the authenticated user stored on a context.
type userContextKey struct{}

// WithUser returns a copy of ctx carrying u as the authenticated user.
//
// It replaces Spring Security's thread-local SecurityContextHolder: the JWT
// middleware calls WithUser after successfully resolving a token. Passing a
// nil u returns ctx unchanged, which is equivalent to an anonymous request.
func WithUser(ctx context.Context, u *core.User) context.Context {
	if u == nil {
		return ctx
	}
	return context.WithValue(ctx, userContextKey{}, u)
}

// UserFrom returns the authenticated user stored on ctx by WithUser.
//
// It is the Go equivalent of the source's SecurityUtil.getCurrentUser():
// the boolean is false (the Optional is empty) when the request is
// anonymous — i.e. no user was attached — or the stored principal is nil.
// It is safe to call with a nil context.
func UserFrom(ctx context.Context) (*core.User, bool) {
	if ctx == nil {
		return nil, false
	}
	u, ok := ctx.Value(userContextKey{}).(*core.User)
	if !ok || u == nil {
		return nil, false
	}
	return u, true
}
