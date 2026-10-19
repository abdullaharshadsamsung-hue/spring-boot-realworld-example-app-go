package auth

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"migrated-app/internal/core"
)

// ErrEmptySecret is returned by NewJWTService when no signing secret is given.
var ErrEmptySecret = errors.New("auth: jwt secret is empty")

// ErrInvalidSessionTime is returned by NewJWTService when the session time is
// not positive.
var ErrInvalidSessionTime = errors.New("auth: jwt session time must be positive")

// ErrNilUser is returned by JWTService.ToToken when given a nil user.
var ErrNilUser = errors.New("auth: cannot issue token for nil user")

// jwtAlg is the only JWS algorithm this service issues or accepts.
const jwtAlg = "HS512"

// b64 is the unpadded base64url encoding mandated by RFC 7515.
var b64 = base64.RawURLEncoding

// jwtHeader is the JOSE header of a compact JWS.
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ,omitempty"`
}

// jwtClaims holds the registered claims this service reads and writes.
// Pointers distinguish "absent" from zero values while parsing.
type jwtClaims struct {
	Sub *string `json:"sub,omitempty"`
	Exp *int64  `json:"exp,omitempty"`
	Nbf *int64  `json:"nbf,omitempty"`
}

// JWTService issues and verifies HS512-signed JWTs. It is the Go replacement
// for the source's DefaultJwtService and implements core.TokenService.
//
// It is immutable after construction and safe for concurrent use.
type JWTService struct {
	key         []byte
	sessionTime time.Duration
	now         func() time.Time
}

// Compile-time proof that JWTService satisfies the core port.
var _ core.TokenService = (*JWTService)(nil)

// NewJWTService creates a JWTService that signs with the raw bytes of secret
// (as the source's SecretKeySpec(secret.getBytes())) and issues tokens valid
// for sessionSeconds seconds (source property jwt.sessionTime).
func NewJWTService(secret string, sessionSeconds int) (*JWTService, error) {
	if secret == "" {
		return nil, ErrEmptySecret
	}
	if sessionSeconds <= 0 {
		return nil, ErrInvalidSessionTime
	}
	// MIGRATION_NOTE: jjwt's signWith(Key) rejects keys shorter than 256 bits
	// and silently downgrades to HS256/HS384 for keys shorter than 512 bits.
	// This port always uses HS512 (per the agreed plan) and accepts any
	// non-empty secret; operators should configure a >= 64-byte secret.
	key := make([]byte, len(secret))
	copy(key, secret)
	return &JWTService{
		key:         key,
		sessionTime: time.Duration(sessionSeconds) * time.Second,
		now:         time.Now,
	}, nil
}

// ToToken issues a compact JWS whose subject is u.ID and whose expiration is
// now + sessionTime.
func (s *JWTService) ToToken(u *core.User) (string, error) {
	if u == nil {
		return "", ErrNilUser
	}
	header, err := json.Marshal(jwtHeader{Alg: jwtAlg})
	if err != nil {
		return "", fmt.Errorf("auth: encode jwt header: %w", err)
	}
	sub := u.ID
	exp := s.now().Add(s.sessionTime).Unix()
	claims, err := json.Marshal(jwtClaims{Sub: &sub, Exp: &exp})
	if err != nil {
		return "", fmt.Errorf("auth: encode jwt claims: %w", err)
	}
	signingInput := b64.EncodeToString(header) + "." + b64.EncodeToString(claims)
	return signingInput + "." + b64.EncodeToString(s.sign(signingInput)), nil
}

// SubjectFromToken verifies token's HS512 signature, expiry and not-before
// claims and returns its subject. Any failure — malformed input, wrong
// algorithm, bad signature, expired token, or missing subject — yields
// ("", false), mirroring the source's catch-all returning Optional.empty().
func (s *JWTService) SubjectFromToken(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", false
	}

	headerJSON, err := b64.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	var h jwtHeader
	if err := json.Unmarshal(headerJSON, &h); err != nil || h.Alg != jwtAlg {
		return "", false
	}

	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return "", false
	}
	if !hmac.Equal(sig, s.sign(parts[0]+"."+parts[1])) {
		return "", false
	}

	claimsJSON, err := b64.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	var c jwtClaims
	if err := json.Unmarshal(claimsJSON, &c); err != nil {
		return "", false
	}

	now := s.now().Unix()
	// jjwt treats a token as expired once now >= exp (zero clock skew).
	if c.Exp != nil && now >= *c.Exp {
		return "", false
	}
	if c.Nbf != nil && now < *c.Nbf {
		return "", false
	}
	if c.Sub == nil {
		return "", false
	}
	return *c.Sub, true
}

// sign computes the HMAC-SHA512 of signingInput with the service key.
func (s *JWTService) sign(signingInput string) []byte {
	mac := hmac.New(sha512.New, s.key)
	mac.Write([]byte(signingInput)) // hash.Hash.Write never returns an error
	return mac.Sum(nil)
}
