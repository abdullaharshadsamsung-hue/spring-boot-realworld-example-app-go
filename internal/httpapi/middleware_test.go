package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"migrated-app/internal/auth"
	"migrated-app/internal/core"
)

type fakeResolver struct {
	subs  map[string]string
	calls int
}

func (f *fakeResolver) GetSubFromToken(token string) (string, bool) {
	f.calls++
	s, ok := f.subs[token]
	return s, ok
}

type fakeUsers struct {
	users map[string]*core.User
	err   error
	calls int
}

func (f *fakeUsers) FindByID(ctx context.Context, id string) (*core.User, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	u, ok := f.users[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	return u, nil
}

func newFakes() (*fakeResolver, *fakeUsers) {
	return &fakeResolver{subs: map[string]string{"good": "u1", "ghost": "u404"}},
		&fakeUsers{users: map[string]*core.User{"u1": {ID: "u1"}}}
}

type captured struct {
	called bool
	userID string
	hasU   bool
}

func captureHandler(c *captured) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.called = true
		if u, ok := auth.UserFromContext(r.Context()); ok && u != nil {
			c.hasU = true
			c.userID = u.ID
		}
		w.WriteHeader(http.StatusTeapot)
	})
}

func TestTokenFromHeader(t *testing.T) {
	tests := []struct {
		name, header, want string
		ok                 bool
	}{
		{"empty", "", "", false},
		{"no space", "Token", "", false},
		{"token scheme", "Token abc", "abc", true},
		{"scheme not checked", "Bearer abc", "abc", true},
		{"extra parts", "Token abc def", "abc", true},
		{"double space", "Token  abc", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tokenFromHeader(tt.header)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("got (%q,%v), want (%q,%v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestAntMatch(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"/tags", "/tags", true},
		{"/tags", "/tags/x", false},
		{"/articles/**", "/articles", true},
		{"/articles/**", "/articles/slug", true},
		{"/articles/**", "/articles/slug/comments", true},
		{"/articles/**", "/articlesfoo", false},
		{"/users", "/users/login", false},
	}
	for _, tt := range tests {
		if got := antMatch(tt.pattern, tt.path); got != tt.want {
			t.Errorf("antMatch(%q,%q)=%v want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestRequiredAccess(t *testing.T) {
	tests := []struct {
		method, path string
		want         accessLevel
	}{
		{http.MethodOptions, "/user", accessPermitAll},
		{http.MethodOptions, "/articles/feed", accessPermitAll},
		{http.MethodPost, "/graphql", accessPermitAll},
		{http.MethodGet, "/graphiql", accessPermitAll},
		{http.MethodDelete, "/graphql", accessPermitAll},
		{http.MethodGet, "/articles/feed", accessAuthenticated},
		{http.MethodPost, "/users", accessPermitAll},
		{http.MethodPost, "/users/login", accessPermitAll},
		{http.MethodGet, "/users", accessAuthenticated},
		{http.MethodGet, "/articles", accessPermitAll},
		{http.MethodGet, "/articles/slug", accessPermitAll},
		{http.MethodGet, "/articles/slug/comments", accessPermitAll},
		{http.MethodGet, "/profiles/jake", accessPermitAll},
		{http.MethodGet, "/tags", accessPermitAll},
		{http.MethodGet, "/user", accessAuthenticated},
		{http.MethodPut, "/user", accessAuthenticated},
		{http.MethodPost, "/articles", accessAuthenticated},
		{http.MethodDelete, "/articles/slug", accessAuthenticated},
		{http.MethodPost, "/articles/slug/favorite", accessAuthenticated},
		{http.MethodPost, "/profiles/u/follow", accessAuthenticated},
	}
	for _, tt := range tests {
		if got := requiredAccess(tt.method, tt.path); got != tt.want {
			t.Errorf("%s %s = %v want %v", tt.method, tt.path, got, tt.want)
		}
	}
}

func TestAuthorize(t *testing.T) {
	tests := []struct {
		name, method, path string
		withUser           bool
		wantStatus         int
	}{
		{"options anon", http.MethodOptions, "/user", false, http.StatusTeapot},
		{"graphql anon", http.MethodPost, "/graphql", false, http.StatusTeapot},
		{"graphiql anon", http.MethodGet, "/graphiql", false, http.StatusTeapot},
		{"feed anon", http.MethodGet, "/articles/feed", false, http.StatusUnauthorized},
		{"feed auth", http.MethodGet, "/articles/feed", true, http.StatusTeapot},
		{"register anon", http.MethodPost, "/users", false, http.StatusTeapot},
		{"login anon", http.MethodPost, "/users/login", false, http.StatusTeapot},
		{"articles anon", http.MethodGet, "/articles", false, http.StatusTeapot},
		{"profile anon", http.MethodGet, "/profiles/jake", false, http.StatusTeapot},
		{"tags anon", http.MethodGet, "/tags", false, http.StatusTeapot},
		{"user anon", http.MethodGet, "/user", false, http.StatusUnauthorized},
		{"put user anon", http.MethodPut, "/user", false, http.StatusUnauthorized},
		{"create article anon", http.MethodPost, "/articles", false, http.StatusUnauthorized},
		{"delete article anon", http.MethodDelete, "/articles/s", false, http.StatusUnauthorized},
		{"favorite anon", http.MethodPost, "/articles/s/favorite", false, http.StatusUnauthorized},
		{"follow anon", http.MethodPost, "/profiles/u/follow", false, http.StatusUnauthorized},
		{"create article auth", http.MethodPost, "/articles", true, http.StatusTeapot},
		{"delete no csrf auth", http.MethodDelete, "/articles/s", true, http.StatusTeapot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c captured
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.withUser {
				req = req.WithContext(auth.WithUser(req.Context(), &core.User{ID: "u1"}))
			}
			rec := httptest.NewRecorder()
			Authorize(captureHandler(&c)).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusUnauthorized {
				if c.called {
					t.Fatal("next called on 401")
				}
				if rec.Body.Len() != 0 {
					t.Fatalf("expected empty body, got %q", rec.Body.String())
				}
				if rec.Header().Get("Location") != "" {
					t.Fatal("unexpected redirect")
				}
			}
			if rec.Header().Get("Set-Cookie") != "" {
				t.Fatal("session cookie issued")
			}
		})
	}
}

func TestRequireAuth(t *testing.T) {
	for _, withUser := range []bool{false, true} {
		var c captured
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if withUser {
			req = req.WithContext(auth.WithUser(req.Context(), &core.User{ID: "u1"}))
		}
		rec := httptest.NewRecorder()
		RequireAuth(captureHandler(&c)).ServeHTTP(rec, req)
		want := http.StatusUnauthorized
		if withUser {
			want = http.StatusTeapot
		}
		if rec.Code != want || c.called != withUser {
			t.Errorf("withUser=%v: code %d called %v", withUser, rec.Code, c.called)
		}
	}
}

func TestAuthenticate(t *testing.T) {
	tests := []struct {
		name        string
		header      string
		usersErr    error
		nilDeps     bool
		preUser     bool
		wantUser    bool
		wantUserID  string
		wantLookups int
	}{
		{name: "no header"},
		{name: "malformed header", header: "Token"},
		{name: "invalid token", header: "Token bad"},
		{name: "valid token", header: "Token good", wantUser: true, wantUserID: "u1", wantLookups: 1},
		{name: "unknown user", header: "Token ghost", wantLookups: 1},
		{name: "lookup error", header: "Token good", usersErr: errors.New("db down"), wantLookups: 1},
		{name: "nil deps", header: "Token good", nilDeps: true},
		{name: "already authenticated", header: "Token good", preUser: true, wantUser: true, wantUserID: "pre"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, users := newFakes()
			users.err = tt.usersErr
			var mw func(http.Handler) http.Handler
			if tt.nilDeps {
				mw = Authenticate(nil, nil)
			} else {
				mw = Authenticate(res, users)
			}
			var c captured
			req := httptest.NewRequest(http.MethodGet, "/user", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			if tt.preUser {
				req = req.WithContext(auth.WithUser(req.Context(), &core.User{ID: "pre"}))
			}
			rec := httptest.NewRecorder()
			mw(captureHandler(&c)).ServeHTTP(rec, req)
			if !c.called {
				t.Fatal("next not called; Authenticate must never reject")
			}
			if c.hasU != tt.wantUser || c.userID != tt.wantUserID {
				t.Fatalf("user=%v id=%q, want %v %q", c.hasU, c.userID, tt.wantUser, tt.wantUserID)
			}
			if users.calls != tt.wantLookups {
				t.Fatalf("lookups %d want %d", users.calls, tt.wantLookups)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		headers     map[string]string
		wantStatus  int
		wantNext    bool
		wantOrigin  string
		wantMethods bool
		wantHeaders string
		wantBody    string
	}{
		{name: "no origin", method: http.MethodGet, wantStatus: http.StatusTeapot, wantNext: true},
		{name: "simple get", method: http.MethodGet, headers: map[string]string{"Origin": "http://a.com"},
			wantStatus: http.StatusTeapot, wantNext: true, wantOrigin: "*"},
		{name: "options non-preflight", method: http.MethodOptions, headers: map[string]string{"Origin": "http://a.com"},
			wantStatus: http.StatusTeapot, wantNext: true},
		{name: "preflight ok", method: http.MethodOptions, headers: map[string]string{
			"Origin": "http://a.com", "Access-Control-Request-Method": "PUT"},
			wantStatus: http.StatusOK, wantOrigin: "*", wantMethods: true},
		{name: "preflight allowed headers", method: http.MethodOptions, headers: map[string]string{
			"Origin": "http://b.org", "Access-Control-Request-Method": "POST",
			"Access-Control-Request-Headers": "Authorization, cache-control,Content-Type"},
			wantStatus: http.StatusOK, wantOrigin: "*", wantMethods: true,
			wantHeaders: "Authorization, cache-control, Content-Type"},
		{name: "preflight bad header", method: http.MethodOptions, headers: map[string]string{
			"Origin": "http://a.com", "Access-Control-Request-Method": "GET",
			"Access-Control-Request-Headers": "Authorization, X-Custom"},
			wantStatus: http.StatusForbidden, wantBody: "Invalid CORS request"},
		{name: "preflight bad method", method: http.MethodOptions, headers: map[string]string{
			"Origin": "http://a.com", "Access-Control-Request-Method": "TRACE"},
			wantStatus: http.StatusForbidden, wantBody: "Invalid CORS request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c captured
			req := httptest.NewRequest(tt.method, "/articles", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			CORS(captureHandler(&c)).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d want %d", rec.Code, tt.wantStatus)
			}
			if c.called != tt.wantNext {
				t.Fatalf("next called %v want %v", c.called, tt.wantNext)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tt.wantOrigin {
				t.Fatalf("allow-origin %q want %q", got, tt.wantOrigin)
			}
			if tt.wantMethods {
				got := rec.Header().Get("Access-Control-Allow-Methods")
				if got != "HEAD,GET,POST,PUT,DELETE,PATCH" {
					t.Fatalf("allow-methods %q", got)
				}
			}
			if got := rec.Header().Get("Access-Control-Allow-Headers"); got != tt.wantHeaders {
				t.Fatalf("allow-headers %q want %q", got, tt.wantHeaders)
			}
			if rec.Header().Get("Access-Control-Allow-Credentials") == "true" {
				t.Fatal("credentials must not be allowed")
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body %q", rec.Body.String())
			}
		})
	}
}

func TestSecurityChain(t *testing.T) {
	tests := []struct {
		name, method, path, authz string
		headers                   map[string]string
		wantStatus                int
		wantUserID                string
	}{
		{name: "feed anon", method: http.MethodGet, path: "/articles/feed", wantStatus: http.StatusUnauthorized},
		{name: "feed invalid token", method: http.MethodGet, path: "/articles/feed", authz: "Token bad", wantStatus: http.StatusUnauthorized},
		{name: "feed valid token", method: http.MethodGet, path: "/articles/feed", authz: "Token good", wantStatus: http.StatusTeapot, wantUserID: "u1"},
		{name: "public tags", method: http.MethodGet, path: "/tags", wantStatus: http.StatusTeapot},
		{name: "put user valid", method: http.MethodPut, path: "/user", authz: "Token good", wantStatus: http.StatusTeapot, wantUserID: "u1"},
		{name: "preflight protected path", method: http.MethodOptions, path: "/user",
			headers: map[string]string{"Origin": "http://a.com", "Access-Control-Request-Method": "PUT"}, wantStatus: http.StatusOK},
		{name: "cors anon protected still 401", method: http.MethodPost, path: "/articles",
			headers: map[string]string{"Origin": "http://a.com"}, wantStatus: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, users := newFakes()
			var c captured
			h := SecurityChain(captureHandler(&c), res, users)
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.authz != "" {
				req.Header.Set("Authorization", tt.authz)
			}
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d want %d", rec.Code, tt.wantStatus)
			}
			if c.userID != tt.wantUserID {
				t.Fatalf("user %q want %q", c.userID, tt.wantUserID)
			}
			if rec.Header().Get("Set-Cookie") != "" {
				t.Fatal("no session cookie expected")
			}
		})
	}

	// Stateless: a later request without token is anonymous again.
	res, users := newFakes()
	var c captured
	h := SecurityChain(captureHandler(&c), res, users)
	r1 := httptest.NewRequest(http.MethodGet, "/user", nil)
	r1.Header.Set("Authorization", "Token good")
	h.ServeHTTP(httptest.NewRecorder(), r1)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/user", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("second request should be 401, got %d", rec.Code)
	}
}