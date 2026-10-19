package auth

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"migrated-app/internal/core"
)

const testSecret = "123123123123123123123123123123123123123123123123123123123123"

func newTestService(t *testing.T) *JWTService {
	t.Helper()
	s, err := NewJWTService(testSecret, 3600)
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}
	return s
}

func testUser(t *testing.T) *core.User {
	t.Helper()
	u, err := core.NewUser("email@email.com", "username", "123", "", "")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	return u
}

func signWith(secret string, header, claims []byte) string {
	in := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(in))
	return in + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestNewJWTService(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		seconds int
		wantErr error
	}{
		{"valid", testSecret, 3600, nil},
		{"empty secret", "", 3600, ErrEmptySecret},
		{"zero session", testSecret, 0, ErrInvalidSessionTime},
		{"negative session", testSecret, -1, ErrInvalidSessionTime},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewJWTService(tt.secret, tt.seconds)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && s == nil {
				t.Fatal("nil service")
			}
			if tt.wantErr != nil && s != nil {
				t.Fatal("expected nil service")
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	s := newTestService(t)
	u := testUser(t)
	tok, err := s.ToToken(u)
	if err != nil || tok == "" {
		t.Fatalf("ToToken: %q %v", tok, err)
	}
	sub, ok := s.SubjectFromToken(tok)
	if !ok || sub != u.ID {
		t.Fatalf("got (%q,%v), want (%q,true)", sub, ok, u.ID)
	}
}

func TestToTokenNilUser(t *testing.T) {
	s := newTestService(t)
	tok, err := s.ToToken(nil)
	if !errors.Is(err, ErrNilUser) || tok != "" {
		t.Fatalf("got (%q,%v)", tok, err)
	}
}

func TestToTokenStructure(t *testing.T) {
	s := newTestService(t)
	fixed := time.Unix(1700000000, 0)
	s.now = func() time.Time { return fixed }
	u := testUser(t)
	tok, err := s.ToToken(u)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 || parts[2] == "" {
		t.Fatalf("bad token %q", tok)
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var h map[string]any
	if err := json.Unmarshal(hb, &h); err != nil {
		t.Fatal(err)
	}
	if h["alg"] != "HS512" {
		t.Fatalf("alg = %v", h["alg"])
	}
	cb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(cb, &c); err != nil {
		t.Fatal(err)
	}
	if c.Sub != u.ID {
		t.Fatalf("sub = %q", c.Sub)
	}
	if c.Exp != fixed.Unix()+3600 {
		t.Fatalf("exp = %d", c.Exp)
	}
	// signature must be HMAC-SHA512 with raw secret bytes
	if want := signWith(testSecret, hb, cb); want != tok {
		t.Fatalf("signature mismatch")
	}
}

func TestToTokenExpRealClock(t *testing.T) {
	s := newTestService(t)
	before := time.Now().Unix()
	tok, _ := s.ToToken(testUser(t))
	after := time.Now().Unix()
	cb, _ := base64.RawURLEncoding.DecodeString(strings.Split(tok, ".")[1])
	var c struct{ Exp int64 `json:"exp"` }
	_ = json.Unmarshal(cb, &c)
	if c.Exp < before+3600 || c.Exp > after+3600 {
		t.Fatalf("exp %d out of range", c.Exp)
	}
}

func TestDifferentSecretRejects(t *testing.T) {
	s := newTestService(t)
	other, err := NewJWTService(testSecret+"x", 3600)
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := s.ToToken(testUser(t))
	if sub, ok := other.SubjectFromToken(tok); ok || sub != "" {
		t.Fatalf("got (%q,%v)", sub, ok)
	}
}

func TestSubjectFromTokenFailures(t *testing.T) {
	s := newTestService(t)
	fixed := time.Unix(1700000000, 0)
	s.now = func() time.Time { return fixed }
	valid, _ := s.ToToken(&core.User{ID: "user-1"})
	parts := strings.Split(valid, ".")

	hdr := []byte(`{"alg":"HS512"}`)
	exp := fixed.Unix() + 100
	tamperedClaims, _ := json.Marshal(map[string]any{"sub": "attacker", "exp": exp})
	tamperedPayload := parts[0] + "." + base64.RawURLEncoding.EncodeToString(tamperedClaims) + "." + parts[2]

	sigBytes, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sigBytes[0] ^= 0xff
	tamperedSig := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sigBytes)

	noneHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	unsigned := noneHeader + "." + parts[1] + "."
	noneSigned := signWith(testSecret, []byte(`{"alg":"none"}`), []byte(`{"sub":"user-1"}`))
	hs256 := signWith(testSecret, []byte(`{"alg":"HS256"}`), []byte(`{"sub":"user-1"}`))

	noSub := signWith(testSecret, hdr, []byte(`{"exp":`+itoa(exp)+`}`))
	expired := signWith(testSecret, hdr, []byte(`{"sub":"u","exp":`+itoa(fixed.Unix()-1)+`}`))
	expEqualNow := signWith(testSecret, hdr, []byte(`{"sub":"u","exp":`+itoa(fixed.Unix())+`}`))
	notYet := signWith(testSecret, hdr, []byte(`{"sub":"u","nbf":`+itoa(fixed.Unix()+10)+`}`))
	badClaimsJSON := signWith(testSecret, hdr, []byte(`not json`))
	badHeaderJSON := signWith(testSecret, []byte(`{`), []byte(`{"sub":"u"}`))

	tests := []struct {
		name  string
		token string
	}{
		{"malformed 123", "123"},
		{"empty", ""},
		{"source expired token", "eyJhbGciOiJIUzUxMiJ9.eyJzdWIiOiJhaXNlbnNpeSIsImV4cCI6MTUwMjE2MTIwNH0.SJB-U60WzxLYNomqLo4G3v3LzFxJKuVrIud8D8Lz3-mgpo9pN1i7C8ikU_jQPJGm8HsC1CquGMI-rSuM7j6LDA"},
		{"tampered payload", tamperedPayload},
		{"tampered signature", tamperedSig},
		{"unsigned alg none", unsigned},
		{"alg none with sig", noneSigned},
		{"alg HS256", hs256},
		{"no sub", noSub},
		{"expired", expired},
		{"exp equals now", expEqualNow},
		{"nbf in future", notYet},
		{"bad claims json", badClaimsJSON},
		{"bad header json", badHeaderJSON},
		{"four parts", valid + ".x"},
		{"empty header", "." + parts[1] + "." + parts[2]},
		{"invalid base64 header", "!!!." + parts[1] + "." + parts[2]},
		{"invalid base64 sig", parts[0] + "." + parts[1] + ".!!!"},
		{"invalid base64 claims", parts[0] + ".!!!." + parts[2]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic: %v", r)
				}
			}()
			if sub, ok := s.SubjectFromToken(tt.token); ok || sub != "" {
				t.Fatalf("got (%q,%v), want empty", sub, ok)
			}
		})
	}

	// sanity: valid token and valid hand-built tokens are accepted
	accepted := []struct {
		name, token, want string
	}{
		{"issued", valid, "user-1"},
		{"no exp", signWith(testSecret, hdr, []byte(`{"sub":"u"}`)), "u"},
		{"nbf past", signWith(testSecret, hdr, []byte(`{"sub":"u","nbf":`+itoa(fixed.Unix())+`}`)), "u"},
	}
	for _, tt := range accepted {
		t.Run(tt.name, func(t *testing.T) {
			if sub, ok := s.SubjectFromToken(tt.token); !ok || sub != tt.want {
				t.Fatalf("got (%q,%v)", sub, ok)
			}
		})
	}
}

func TestIssuedTokenExpiresAfterSession(t *testing.T) {
	s := newTestService(t)
	base := time.Unix(1700000000, 0)
	cur := base
	s.now = func() time.Time { return cur }
	tok, _ := s.ToToken(&core.User{ID: "id"})
	cur = base.Add(3599 * time.Second)
	if _, ok := s.SubjectFromToken(tok); !ok {
		t.Fatal("should be valid before expiry")
	}
	cur = base.Add(3600 * time.Second)
	if _, ok := s.SubjectFromToken(tok); ok {
		t.Fatal("should be expired")
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}