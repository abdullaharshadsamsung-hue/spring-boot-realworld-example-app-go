package httpapi

import (
	"context"
	"net/http"
	"strings"

	"migrated-app/internal/auth"
	"migrated-app/internal/core"
)

// This file replaces io.spring.api.security.WebSecurityConfig and the
// JwtTokenFilter bean it registers.
//
// Spring's security filter chain becomes ordinary net/http middleware:
//
//	CORS -> Authenticate (JWT filter) -> Authorize (ordered access rules) -> router
//
// Mapping of the Spring configuration:
//   - csrf().disable(): nothing to do. Go has no implicit CSRF protection.
//   - sessionCreationPolicy(STATELESS): nothing to do. No sessions are created.
//   - addFilterBefore(jwtTokenFilter, UsernamePasswordAuthenticationFilter):
//     Authenticate runs before Authorize, so the request's user is resolved
//     from the token before any authorization rule is evaluated.
//   - HttpStatusEntryPoint(UNAUTHORIZED): denied requests get a bare 401 with
//     an empty body.
//   - corsConfigurationSource(): CORS middleware registered for every path
//     ("/**").
//   - passwordEncoder() (BCryptPasswordEncoder, cost 10): this lives in
//     internal/auth/password.go, per the agreed plan.
//
// MIGRATION_NOTE: Spring's implicit anonymous authentication has no Go
// equivalent and needs none. A request without a resolvable user has no user
// in its context, and Authorize treats it as anonymous.

// SubjectResolver extracts the subject (user id) from a raw JWT. The JWT
// service in internal/auth satisfies it.
type SubjectResolver interface {
	// GetSubFromToken returns the token's subject. The bool is false when
	// the token is invalid, expired, or has no subject.
	GetSubFromToken(token string) (string, bool)
}

// UserByIDFinder loads a user by id. core.UserRepository satisfies it.
type UserByIDFinder interface {
	// FindByID returns the user, or an error (matching core.ErrNotFound when
	// the user does not exist).
	FindByID(ctx context.Context, id string) (*core.User, error)
}

// Authenticate is the port of JwtTokenFilter. It reads
// "Authorization: Token <jwt>", resolves the subject, loads the user, and
// stores the user in the request context.
//
// Any failure (missing header, malformed header, invalid token, unknown user,
// lookup error) leaves the request anonymous and passes it on. This matches
// the source filter, which never rejects a request on its own. Rejection is
// decided by Authorize.
func Authenticate(tokens SubjectResolver, users UserByIDFinder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tokens == nil || users == nil {
				next.ServeHTTP(w, r)
				return
			}

			token, ok := tokenFromHeader(r.Header.Get("Authorization"))
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			id, ok := tokens.GetSubFromToken(token)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			// The source only authenticates when no authentication is
			// already present in the security context.
			if _, already := auth.UserFrom(r.Context()); already {
				next.ServeHTTP(w, r)
				return
			}

			user, err := users.FindByID(r.Context(), id)
			if err != nil || user == nil {
				// MIGRATION_NOTE: the source swallows lookup failures
				// (Optional.empty / ifPresent). Errors other than not-found
				// are also treated as anonymous here.
				next.ServeHTTP(w, r)
				return
			}

			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), user)))
		})
	}
}

// tokenFromHeader mirrors JwtTokenFilter.getTokenString. The header is split
// on single spaces and the second element is used. The scheme word itself is
// not checked, which preserves the source's behavior.
func tokenFromHeader(header string) (string, bool) {
	if header == "" {
		return "", false
	}
	parts := strings.Split(header, " ")
	if len(parts) < 2 {
		return "", false
	}
	return parts[1], true
}

// accessLevel is the outcome of a matching authorization rule.
type accessLevel int

const (
	accessPermitAll accessLevel = iota + 1
	accessAuthenticated
)

// accessRule is one antMatchers(...) entry. An empty method matches any
// method. An empty patterns slice matches any path.
type accessRule struct {
	method   string
	patterns []string
	access   accessLevel
}

// accessRules is the ordered rule table from WebSecurityConfig.configure.
// The first matching rule wins. Anything that matches no rule falls through
// to anyRequest().authenticated().
var accessRules = []accessRule{
	{method: http.MethodOptions, access: accessPermitAll},
	{patterns: []string{"/graphiql"}, access: accessPermitAll},
	{patterns: []string{"/graphql"}, access: accessPermitAll},
	{method: http.MethodGet, patterns: []string{"/articles/feed"}, access: accessAuthenticated},
	{method: http.MethodPost, patterns: []string{"/users", "/users/login"}, access: accessPermitAll},
	{method: http.MethodGet, patterns: []string{"/articles/**", "/profiles/**", "/tags"}, access: accessPermitAll},
}

// requiredAccess evaluates accessRules in order for method and path.
func requiredAccess(method, path string) accessLevel {
	for _, rule := range accessRules {
		if rule.method != "" && rule.method != method {
			continue
		}
		if len(rule.patterns) == 0 {
			return rule.access
		}
		for _, p := range rule.patterns {
			if antMatch(p, path) {
				return rule.access
			}
		}
	}
	return accessAuthenticated // anyRequest().authenticated()
}

// antMatch implements the subset of Spring's AntPathMatcher used by this
// configuration: exact paths, plus a trailing "/**" that matches the prefix
// itself and anything below it.
func antMatch(pattern, path string) bool {
	if prefix, ok := strings.CutSuffix(pattern, "/**"); ok {
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	return path == pattern
}

// Authorize enforces the ordered access rules. Requests that need
// authentication and carry no user get 401 with an empty body, matching
// HttpStatusEntryPoint(HttpStatus.UNAUTHORIZED).
func Authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requiredAccess(r.Method, r.URL.Path) == accessAuthenticated {
			if _, ok := auth.UserFrom(r.Context()); !ok {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuth wraps a single handler so it rejects anonymous requests with a
// bare 401. It is useful for routes mounted outside the Authorize chain.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.UserFrom(r.Context()); !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CORS policy from corsConfigurationSource(), registered for "/**".
var (
	corsAllowedMethods = []string{"HEAD", "GET", "POST", "PUT", "DELETE", "PATCH"}
	corsAllowedHeaders = []string{"Authorization", "Cache-Control", "Content-Type"}
)

// CORS applies the global CORS policy: Allow-Origin "*", the allowed methods
// HEAD/GET/POST/PUT/DELETE/PATCH, the allowed headers
// Authorization/Cache-Control/Content-Type, and no credentials.
//
// Valid preflight requests are answered with 200 and are not forwarded.
// Invalid ones get 403 "Invalid CORS request", like Spring's
// DefaultCorsProcessor. Requests without an Origin header pass through
// untouched.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Add("Vary", "Origin")
		h.Add("Vary", "Access-Control-Request-Method")
		h.Add("Vary", "Access-Control-Request-Headers")

		if r.Header.Get("Origin") == "" {
			next.ServeHTTP(w, r)
			return
		}

		reqMethod := r.Header.Get("Access-Control-Request-Method")
		isPreflight := r.Method == http.MethodOptions && reqMethod != ""

		if !isPreflight {
			// MIGRATION_NOTE: Spring also rejects actual cross-origin
			// requests whose method is not allowed (e.g. OPTIONS without
			// preflight headers falls through as permitted). Here only
			// allowed methods receive the CORS header and nothing is
			// rejected, so the browser enforces the policy.
			if containsFold(corsAllowedMethods, r.Method) {
				h.Set("Access-Control-Allow-Origin", "*")
			}
			next.ServeHTTP(w, r)
			return
		}

		if !containsFold(corsAllowedMethods, reqMethod) {
			http.Error(w, "Invalid CORS request", http.StatusForbidden)
			return
		}

		var allowed []string
		for _, raw := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			if !containsFold(corsAllowedHeaders, name) {
				http.Error(w, "Invalid CORS request", http.StatusForbidden)
				return
			}
			allowed = append(allowed, name)
		}

		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", strings.Join(corsAllowedMethods, ","))
		if len(allowed) > 0 {
			h.Set("Access-Control-Allow-Headers", strings.Join(allowed, ", "))
		}
		w.WriteHeader(http.StatusOK)
	})
}

// containsFold reports whether list contains s, ignoring case.
func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// SecurityChain wraps h with the full security filter chain in Spring's
// order: CORS, then JWT authentication, then the ordered authorization rules.
func SecurityChain(h http.Handler, tokens SubjectResolver, users UserByIDFinder) http.Handler {
	return CORS(Authenticate(tokens, users)(Authorize(h)))
}
