package graphql

import (
	"context"
	"errors"
	"testing"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
	"migrated-app/internal/httpapi"
)

type fakeUserFinder struct {
	data   app.UserData
	found  bool
	err    error
	calls  int
	lastID string
}

func (f *fakeUserFinder) FindByID(ctx context.Context, id string) (app.UserData, bool, error) {
	f.calls++
	f.lastID = id
	return f.data, f.found, f.err
}

type fakeTokenService struct {
	token  string
	err    error
	calls  int
	lastU  *core.User
}

func (f *fakeTokenService) ToToken(u *core.User) (string, error) {
	f.calls++
	f.lastU = u
	if f.err != nil {
		return "", f.err
	}
	return f.token + ":" + u.ID, nil
}

func (f *fakeTokenService) SubjectFromToken(token string) (string, bool) {
	return "", false
}

func TestNewMeDatafetcher(t *testing.T) {
	tests := []struct {
		name    string
		users   UserByIDFinder
		tokens  core.TokenService
		wantErr error
	}{
		{"ok", &fakeUserFinder{}, &fakeTokenService{}, nil},
		{"nil users", nil, &fakeTokenService{}, ErrNilUserByIDFinder},
		{"nil tokens", &fakeUserFinder{}, nil, ErrNilMeTokenService},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := NewMeDatafetcher(tt.users, tt.tokens)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && d == nil {
				t.Fatal("nil datafetcher")
			}
			if tt.wantErr != nil && d != nil {
				t.Fatal("expected nil datafetcher")
			}
		})
	}
}

func TestMe(t *testing.T) {
	principal := &core.User{ID: "u1", Email: "principal@x.io", Username: "principal"}
	stored := app.NewUserData("u1", "stored@x.io", "stored", "bio", "img")
	storeErr := errors.New("db down")

	tests := []struct {
		name       string
		ctx        context.Context
		authz      string
		finder     *fakeUserFinder
		wantNil    bool
		wantErr    error
		wantCalls  int
		wantToken  string
	}{
		{"token scheme", auth.WithUser(context.Background(), principal), "Token abc.def.ghi",
			&fakeUserFinder{data: stored, found: true}, false, nil, 1, "abc.def.ghi"},
		{"bearer scheme", auth.WithUser(context.Background(), principal), "Bearer xyz",
			&fakeUserFinder{data: stored, found: true}, false, nil, 1, "xyz"},
		{"anonymous", context.Background(), "Token abc",
			&fakeUserFinder{data: stored, found: true}, true, nil, 0, ""},
		{"nil principal", auth.WithUser(context.Background(), nil), "Token abc",
			&fakeUserFinder{data: stored, found: true}, true, nil, 0, ""},
		{"not found", auth.WithUser(context.Background(), principal), "Token abc",
			&fakeUserFinder{found: false}, true, httpapi.ErrResourceNotFound, 1, ""},
		{"store error", auth.WithUser(context.Background(), principal), "Token abc",
			&fakeUserFinder{err: storeErr}, true, storeErr, 1, ""},
		{"malformed header", auth.WithUser(context.Background(), principal), "Tokenabc",
			&fakeUserFinder{data: stored, found: true}, true, ErrMalformedAuthorization, 1, ""},
		{"missing header", auth.WithUser(context.Background(), principal), "",
			&fakeUserFinder{data: stored, found: true}, true, ErrMalformedAuthorization, 1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := &fakeTokenService{token: "new"}
			d, err := NewMeDatafetcher(tt.finder, tokens)
			if err != nil {
				t.Fatal(err)
			}
			res, err := d.Me(tt.ctx, tt.authz)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if tt.finder.calls != tt.wantCalls {
				t.Fatalf("finder calls = %d, want %d", tt.finder.calls, tt.wantCalls)
			}
			if tokens.calls != 0 {
				t.Fatal("Me must not generate a token")
			}
			if tt.wantNil {
				if res != nil {
					t.Fatalf("expected nil result, got %+v", res)
				}
				return
			}
			if tt.finder.lastID != principal.ID {
				t.Fatalf("looked up %q", tt.finder.lastID)
			}
			want := User{Email: "stored@x.io", Username: "stored", Token: tt.wantToken}
			if res == nil || res.Data == nil || *res.Data != want {
				t.Fatalf("data = %+v, want %+v", res, want)
			}
			if res.LocalContext != principal {
				t.Fatal("local context must be the principal")
			}
		})
	}
}

func TestUserPayloadUser(t *testing.T) {
	u := &core.User{ID: "u9", Email: "a@b.c", Username: "alice"}
	signErr := errors.New("sign fail")
	tests := []struct {
		name    string
		user    *core.User
		signErr error
		wantErr error
	}{
		{"ok", u, nil, nil},
		{"nil local context", nil, nil, ErrMissingLocalContext},
		{"token error", u, signErr, signErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finder := &fakeUserFinder{}
			tokens := &fakeTokenService{token: "jwt", err: tt.signErr}
			d, _ := NewMeDatafetcher(finder, tokens)
			res, err := d.UserPayloadUser(context.Background(), tt.user)
			if finder.calls != 0 {
				t.Fatal("no lookup expected")
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || res != nil {
					t.Fatalf("err=%v res=%+v", err, res)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tokens.calls != 1 || tokens.lastU != u {
				t.Fatal("token must be issued for context user")
			}
			want := User{Email: u.Email, Username: u.Username, Token: "jwt:u9"}
			if *res.Data != want {
				t.Fatalf("data = %+v, want %+v", *res.Data, want)
			}
			if res.LocalContext != u {
				t.Fatal("local context must be same user")
			}
		})
	}
}