package core

// TokenService issues and verifies auth tokens (JWTs) for users. It is the
// core-layer port for the source's io.spring.core.service.JwtService; the
// concrete HS512 implementation (secret, expiry, signing) lives in
// internal/auth/jwt.go and is wired in cmd/server/main.go via constructor
// injection.
//
// MIGRATION_NOTE: the source annotated this interface with @Service, which
// Spring ignores on interfaces. Go needs no equivalent; consumers (auth
// middleware, users and current-user handlers) receive a TokenService
// explicitly through their constructors.
type TokenService interface {
	// ToToken issues a signed token for u whose subject (sub) claim is the
	// user's ID. It returns an error if u is nil or signing fails.
	ToToken(u *User) (string, error)

	// SubjectFromToken parses and verifies token and returns its subject
	// claim (the user ID). ok is false when the token is malformed, has an
	// invalid signature, is expired, or carries no subject.
	//
	// MIGRATION_NOTE: replaces Java's Optional<String> with Go's (T, bool).
	SubjectFromToken(token string) (sub string, ok bool)
}
