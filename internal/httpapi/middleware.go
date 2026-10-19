package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"migrated-app/internal/auth"
	"migrated-app/internal/core"
)

// authorizationHeader is the request header the JWT is read from.
const authorizationHeader = "Authorization"

// UserByIDFinder loads a user by ID. core.UserRepository satisfies it.
// Taking this narrow interface keeps the middleware easy to test.
type UserByIDFinder interface {
	// FindByID returns the user with the given ID, or an error matching
	// core.ErrNotFound if there is none.
	FindByID(ctx context.Context, id string) (*core.User, error)
}

// SubjectResolver pulls the subject (user ID) out of a token.
// core.TokenService satisfies it.
type SubjectResolver interface {
	// SubjectFromToken returns the token's subject. ok is false if the
	// token is invalid.
	SubjectFromToken(token string) (sub string, ok bool)
}

// Compile-time checks that the core ports satisfy the narrow interfaces.
var (
	_ UserByIDFinder  = (core.UserRepository)(nil)
	_ SubjectResolver = (core.TokenService)(nil)
)

// Authenticate returns middleware that replaces the source's JwtTokenFilter.
//
// For each request it reads the Authorization header, splits it on " ", and
// treats the second part as the token. Any scheme word is accepted, as in
// the source. It resolves the token's subject through tokens and loads that
// user through users. If no user is on the request context yet, the user is
// attached with auth.WithUser.
//
// Authentication fails open. A missing, malformed or invalid token, an
// unknown user, or a repository error never produces an error response; the
// request just continues as anonymous. Authorization is enforced later by
// the handlers.
//
// MIGRATION_NOTE: Spring's thread-local SecurityContextHolder becomes the
// request's context.Context. A net/http middleware runs once per request, so
// it matches OncePerRequestFilter. The principal is the *core.User, like the
// source's UsernamePasswordAuthenticationToken(user, null, emptyList()).
// WebAuthenticationDetailsSource details (remote address, session id) are
// dropped: nothing in the source reads them, and handlers can still get them
// from *http.Request.
//
// If users or tokens is nil, every request passes through anonymously.
func Authenticate(users UserByIDFinder, tokens SubjectResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, authenticateRequest(r, users, tokens))
		})
	}
}

// authenticateRequest returns r with the authenticated user attached to its
// context, or r unchanged when the request stays anonymous.
func authenticateRequest(r *http.Request, users UserByIDFinder, tokens SubjectResolver) *http.Request {
	if users == nil || tokens == nil {
		return r
	}
	token, ok := tokenFromHeader(r.Header.Get(authorizationHeader))
	if !ok {
		return r
	}
	id, ok := tokens.SubjectFromToken(token)
	if !ok {
		return r
	}
	ctx := r.Context()
	// Matches the source's check that the security context has no
	// authentication yet.
	if _, already := auth.UserFrom(ctx); already {
		return r
	}
	user, err := users.FindByID(ctx, id)
	if err != nil {
		if !errors.Is(err, core.ErrNotFound) {
			// The source would have thrown here. We fail open instead and
			// log the error so infrastructure failures stay visible.
			log.Ctx(ctx).Warn().Err(err).Msg("auth middleware: load user by id")
		}
		return r
	}
	if user == nil {
		return r
	}
	return r.WithContext(auth.WithUser(ctx, user))
}

// tokenFromHeader returns the second space-separated part of the
// Authorization header value, like the source's getTokenString. ok is false
// when the header is empty, has fewer than two parts, or the token part is
// empty.
//
// Java's String.split drops trailing empty strings and strings.Split does
// not. The only case where they differ ("Scheme ") gives an empty token
// here, which is rejected, so the result is the same.
func tokenFromHeader(header string) (string, bool) {
	if header == "" {
		return "", false
	}
	parts := strings.Split(header, " ")
	if len(parts) < 2 || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
