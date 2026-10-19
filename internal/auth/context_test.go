package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"migrated-app/internal/core"
)

type otherKey struct{}

func TestUserFrom(t *testing.T) {
	alice := &core.User{ID: "id-1", Email: "a@x.io", Username: "alice"}

	tests := []struct {
		name    string
		ctx     func() context.Context
		want    *core.User
		wantOK  bool
	}{
		{
			name:   "authenticated user returns same reference",
			ctx:    func() context.Context { return WithUser(context.Background(), alice) },
			want:   alice,
			wantOK: true,
		},
		{
			name:   "anonymous context without user",
			ctx:    func() context.Context { return context.Background() },
			wantOK: false,
		},
		{
			name:   "nil user attached is anonymous",
			ctx:    func() context.Context { return WithUser(context.Background(), nil) },
			wantOK: false,
		},
		{
			name: "typed nil stored directly under key",
			ctx: func() context.Context {
				var u *core.User
				return context.WithValue(context.Background(), userContextKey{}, u)
			},
			wantOK: false,
		},
		{
			name: "non-user value under key does not panic",
			ctx: func() context.Context {
				return context.WithValue(context.Background(), userContextKey{}, "anonymousUser")
			},
			wantOK: false,
		},
		{
			name: "core.User value (not pointer) under key is rejected",
			ctx: func() context.Context {
				return context.WithValue(context.Background(), userContextKey{}, core.User{ID: "x"})
			},
			wantOK: false,
		},
		{
			name: "unrelated key with user is ignored",
			ctx: func() context.Context {
				return context.WithValue(context.Background(), otherKey{}, alice)
			},
			wantOK: false,
		},
		{
			name: "nil context is safe",
			//nolint:staticcheck
			ctx:    func() context.Context { return nil },
			wantOK: false,
		},
		{
			name: "user survives derived contexts",
			ctx: func() context.Context {
				c, cancel := context.WithCancel(WithUser(context.Background(), alice))
				_ = cancel
				return context.WithValue(c, otherKey{}, "v")
			},
			want:   alice,
			wantOK: true,
		},
		{
			name: "latest WithUser wins",
			ctx: func() context.Context {
				return WithUser(WithUser(context.Background(), &core.User{ID: "old"}), alice)
			},
			want:   alice,
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := UserFrom(tt.ctx())
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("user = %v, want %v (same pointer)", got, tt.want)
			}
		})
	}
}

func TestWithUserNilReturnsSameContext(t *testing.T) {
	base := context.WithValue(context.Background(), otherKey{}, "v")
	if got := WithUser(base, nil); got != base {
		t.Fatal("WithUser(nil) should return ctx unchanged")
	}
}

func TestWithUserDoesNotModifyParent(t *testing.T) {
	base := context.Background()
	_ = WithUser(base, &core.User{ID: "1"})
	if _, ok := UserFrom(base); ok {
		t.Fatal("parent context must not be modified")
	}
}

func TestUserFromIsRepeatable(t *testing.T) {
	u := &core.User{ID: "1"}
	ctx := WithUser(context.Background(), u)
	for i := 0; i < 3; i++ {
		got, ok := UserFrom(ctx)
		if !ok || got != u {
			t.Fatalf("call %d: got %v, %v", i, got, ok)
		}
	}
}

func TestUserFromHTTPRequestContext(t *testing.T) {
	u := &core.User{ID: "id-9", Username: "bob"}

	tests := []struct {
		name     string
		attach   *core.User
		wantCode int
		wantBody string
	}{
		{"authenticated", u, http.StatusOK, "bob"},
		{"anonymous", nil, http.StatusUnauthorized, "anonymous"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cu, ok := UserFrom(r.Context())
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("anonymous"))
			return
		}
		_, _ = w.Write([]byte(cu.Username))
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/user", nil)
			req = req.WithContext(WithUser(req.Context(), tt.attach))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			if rec.Body.String() != tt.wantBody {
				t.Fatalf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}