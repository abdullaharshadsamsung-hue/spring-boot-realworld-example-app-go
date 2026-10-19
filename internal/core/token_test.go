package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// fakeTokenService is a test implementation of TokenService. It produces
// HS256-signed compact JWTs so the interface contract can be exercised
// without depending on internal/auth.
type fakeTokenService struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

var _ TokenService = (*fakeTokenService)(nil)

func (f *fakeTokenService) sign(data string) string {
	m := hmac.New(sha256.New, f.secret)
	m.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (f *fakeTokenService) ToToken(u *User) (string, error) {
	if u == nil {
		return "", ErrUnauthenticated
	}
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	pb, err := json.Marshal(map[string]any{"sub": u.ID, "exp": f.now().Add(f.ttl).Unix()})
	if err != nil {
		return "", err
	}
	p := base64.RawURLEncoding.EncodeToString(pb)
	return h + "." + p + "." + f.sign(h+"."+p), nil
}

func (f *fakeTokenService) SubjectFromToken(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", false
	}
	if !hmac.Equal([]byte(parts[2]), []byte(f.sign(parts[0]+"."+parts[1]))) {
		return "", false
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	var claims struct {
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(pb, &claims); err != nil {
		return "", false
	}
	if claims.Sub == "" || f.now().Unix() >= claims.Exp {
		return "", false
	}
	return claims.Sub, true
}

func newFake(secret string) *fakeTokenService {
	return &fakeTokenService{secret: []byte(secret), ttl: time.Hour, now: time.Now}
}

func TestToToken(t *testing.T) {
	svc := newFake("s3cret")
	u1, err := NewUser("a@x.io", "a", "p", "", "")
	if err != nil {
		t.Fatal(err)
	}
	u2, err := NewUser("b@x.io", "b", "p", "", "")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		user    *User
		wantErr bool
	}{
		{"user one", u1, false},
		{"user two", u2, false},
		{"fixed id", &User{ID: "123"}, false},
		{"nil user", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok, err := svc.ToToken(tt.user)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok == "" || len(strings.Split(tok, ".")) != 3 {
				t.Fatalf("not a compact JWT: %q", tok)
			}
			sub, ok := svc.SubjectFromToken(tok)
			if !ok || sub != tt.user.ID {
				t.Fatalf("round trip got (%q,%v), want %q", sub, ok, tt.user.ID)
			}
		})
	}
	t1, _ := svc.ToToken(u1)
	t2, _ := svc.ToToken(u2)
	s1, _ := svc.SubjectFromToken(t1)
	s2, _ := svc.SubjectFromToken(t2)
	if s1 == s2 || s1 != u1.ID || s2 != u2.ID {
		t.Fatalf("subjects not distinct per user: %q %q", s1, s2)
	}
}

func TestSubjectFromToken(t *testing.T) {
	svc := newFake("s3cret")
	u := &User{ID: "user-1"}
	valid, err := svc.ToToken(u)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := newFake("different").ToToken(u)
	parts := strings.Split(valid, ".")
	sig := []byte(parts[2])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	tamperedSig := parts[0] + "." + parts[1] + "." + string(sig)
	evilPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"admin","exp":9999999999}`))
	tamperedPayload := parts[0] + "." + evilPayload + "." + parts[2]

	expiredSvc := newFake("s3cret")
	expiredSvc.ttl = -time.Minute
	expired, _ := expiredSvc.ToToken(u)
	noSub, _ := svc.ToToken(&User{})

	tests := []struct {
		name    string
		token   string
		wantSub string
		wantOK  bool
	}{
		{"valid", valid, "user-1", true},
		{"empty", "", "", false},
		{"garbage", "not-a-jwt", "", false},
		{"garbage segments", "a.b.c", "", false},
		{"tampered signature", tamperedSig, "", false},
		{"tampered payload", tamperedPayload, "", false},
		{"different secret", other, "", false},
		{"expired", expired, "", false},
		{"no subject", noSub, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked: %v", r)
				}
			}()
			sub, ok := svc.SubjectFromToken(tt.token)
			if ok != tt.wantOK || sub != tt.wantSub {
				t.Fatalf("got (%q,%v), want (%q,%v)", sub, ok, tt.wantSub, tt.wantOK)
			}
			sub2, ok2 := svc.SubjectFromToken(tt.token)
			if sub2 != sub || ok2 != ok {
				t.Fatal("not deterministic")
			}
		})
	}
}