package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"migrated-app/internal/auth"
	"migrated-app/internal/core"
)

type fakeTokens struct {
	calls []string
	sub   string
	ok    bool
}

func (f *fakeTokens) SubjectFromToken(token string) (string, bool) {
	f.calls = append(f.calls, token)
	return f.sub, f.ok
}

type fakeUsers struct {
	calls []string
	user  *core.User
	err   error
}

func (f *fakeUsers) FindByID(ctx context.Context, id string) (*core.User, error) {
	f.calls = append(f.calls, id)
	return f.user, f.err
}

func TestAuthenticate(t *testing.T) {
	john := &core.User{ID: "user-id-1", Email: "john@jacob.com", Username: "johnjacob"}
	existing := &core.User{ID: "existing-id", Username: "existing"}

	tests := []struct {
		name          string
		header        string
		setHeader     bool
		tokenSub      string
		tokenOK       bool
		repoUser      *core.User
		repoErr       error
		preUser       *core.User
		wantTokenArgs []string
		wantRepoArgs  []string
		wantUser      *core.User
	}{
		{
			name: "valid token authenticates user", header: "Token token", setHeader: true,
			tokenSub: john.ID, tokenOK: true, repoUser: john,
			wantTokenArgs: []string{"token"}, wantRepoArgs: []string{john.ID}, wantUser: john,
		},
		{
			name: "any scheme accepted", header: "Bearer token", setHeader: true,
			tokenSub: john.ID, tokenOK: true, repoUser: john,
			wantTokenArgs: []string{"token"}, wantRepoArgs: []string{john.ID}, wantUser: john,
		},
		{name: "no header", setHeader: false},
		{name: "empty header", header: "", setHeader: true},
		{name: "single part token", header: "token", setHeader: true},
		{name: "single part scheme", header: "Token", setHeader: true},
		{name: "trailing space empty token", header: "Token ", setHeader: true},
		{
			name: "more than two parts uses second", header: "Token abc extra", setHeader: true,
			tokenSub: john.ID, tokenOK: true, repoUser: john,
			wantTokenArgs: []string{"abc"}, wantRepoArgs: []string{john.ID}, wantUser: john,
		},
		{
			name: "invalid token", header: "Token bad", setHeader: true,
			tokenOK: false, wantTokenArgs: []string{"bad"},
		},
		{
			name: "user not found", header: "Token token", setHeader: true,
			tokenSub: "missing", tokenOK: true, repoErr: core.ErrNotFound,
			wantTokenArgs: []string{"token"}, wantRepoArgs: []string{"missing"},
		},
		{
			name: "wrapped not found", header: "Token token", setHeader: true,
			tokenSub: "missing", tokenOK: true, repoErr: fmt.Errorf("find: %w", core.ErrNotFound),
			wantTokenArgs: []string{"token"}, wantRepoArgs: []string{"missing"},
		},
		{
			name: "repository error fails open", header: "Token token", setHeader: true,
			tokenSub: john.ID, tokenOK: true, repoErr: errors.New("db down"),
			wantTokenArgs: []string{"token"}, wantRepoArgs: []string{john.ID},
		},
		{
			name: "nil user without error", header: "Token token", setHeader: true,
			tokenSub: john.ID, tokenOK: true,
			wantTokenArgs: []string{"token"}, wantRepoArgs: []string{john.ID},
		},
		{
			name: "existing authentication preserved", header: "Token token", setHeader: true,
			tokenSub: john.ID, tokenOK: true, repoUser: john, preUser: existing,
			wantTokenArgs: []string{"token"}, wantUser: existing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := &fakeTokens{sub: tt.tokenSub, ok: tt.tokenOK}
			users := &fakeUsers{user: tt.repoUser, err: tt.repoErr}

			nextCalls := 0
			var gotUser *core.User
			var gotOK bool
			var gotW http.ResponseWriter
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalls++
				gotW = w
				gotUser, gotOK = auth.UserFrom(r.Context())
				w.WriteHeader(http.StatusTeapot)
			})

			req := httptest.NewRequest(http.MethodGet, "/user", nil)
			if tt.setHeader {
				req.Header.Set("Authorization", tt.header)
			}
			if tt.preUser != nil {
				req = req.WithContext(auth.WithUser(req.Context(), tt.preUser))
			}
			rec := httptest.NewRecorder()

			Authenticate(users, tokens)(next).ServeHTTP(rec, req)

			if nextCalls != 1 {
				t.Fatalf("next called %d times, want 1", nextCalls)
			}
			if gotW != rec {
				t.Errorf("next received a different ResponseWriter")
			}
			if rec.Code != http.StatusTeapot {
				t.Errorf("status = %d, want %d (middleware must not write)", rec.Code, http.StatusTeapot)
			}
			if !equalStrings(tokens.calls, tt.wantTokenArgs) {
				t.Errorf("token calls = %v, want %v", tokens.calls, tt.wantTokenArgs)
			}
			if !equalStrings(users.calls, tt.wantRepoArgs) {
				t.Errorf("repo calls = %v, want %v", users.calls, tt.wantRepoArgs)
			}
			if tt.wantUser == nil {
				if gotOK {
					t.Errorf("expected anonymous request, got user %v", gotUser)
				}
			} else {
				if !gotOK || gotUser != tt.wantUser {
					t.Errorf("user = %v (ok=%v), want %v", gotUser, gotOK, tt.wantUser)
				}
			}
		})
	}
}

func TestAuthenticateNilDependencies(t *testing.T) {
	tests := []struct {
		name   string
		users  UserByIDFinder
		tokens SubjectResolver
	}{
		{"nil users", nil, &fakeTokens{sub: "x", ok: true}},
		{"nil tokens", &fakeUsers{user: &core.User{ID: "x"}}, nil},
		{"both nil", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if _, ok := auth.UserFrom(r.Context()); ok {
					t.Errorf("expected anonymous request")
				}
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Token token")
			rec := httptest.NewRecorder()
			Authenticate(tt.users, tt.tokens)(next).ServeHTTP(rec, req)
			if calls != 1 {
				t.Errorf("next called %d times, want 1", calls)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
		})
	}
}

func TestTokenFromHeader(t *testing.T) {
	tests := []struct {
		header string
		want   string
		ok     bool
	}{
		{"", "", false},
		{"token", "", false},
		{"Token", "", false},
		{"Token ", "", false},
		{"Token token", "token", true},
		{"Token abc extra", "abc", true},
		{"Bearer xyz", "xyz", true},
		{"Token  double", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got, ok := tokenFromHeader(tt.header)
			if got != tt.want || ok != tt.ok {
				t.Errorf("tokenFromHeader(%q) = (%q, %v), want (%q, %v)", tt.header, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}