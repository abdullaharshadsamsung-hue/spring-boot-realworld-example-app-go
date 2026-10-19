package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"migrated-app/internal/app"
	"migrated-app/internal/core"
)

type cuFakeUpdater struct {
	err    error
	called bool
	cmd    *app.UpdateUserCommand
}

func (f *cuFakeUpdater) UpdateUser(ctx context.Context, cmd *app.UpdateUserCommand) error {
	f.called = true
	f.cmd = cmd
	return f.err
}

type cuFakeLoader struct {
	data   app.UserData
	found  bool
	err    error
	gotID  string
	called bool
}

func (f *cuFakeLoader) FindByID(ctx context.Context, id string) (app.UserData, bool, error) {
	f.called = true
	f.gotID = id
	return f.data, f.found, f.err
}

const cuDefaultAvatar = "https://static.productionready.io/images/smiley-cyrus.jpg"

func cuTestUser() *core.User {
	return &core.User{ID: "u1", Email: "john@jacob.com", Username: "johnjacob", Password: "hash", Image: cuDefaultAvatar}
}

func cuResolverFor(u *core.User) CurrentUserResolver {
	return func(ctx context.Context) (*core.User, bool) {
		if u == nil {
			return nil, false
		}
		return u, true
	}
}

func TestNewCurrentUserHandler(t *testing.T) {
	up := &cuFakeUpdater{}
	ld := &cuFakeLoader{}
	res := cuResolverFor(nil)
	tests := []struct {
		name    string
		up      CurrentUserUpdater
		ld      UserDataLoader
		res     CurrentUserResolver
		wantErr error
	}{
		{"ok", up, ld, res, nil},
		{"nil updater", nil, ld, res, ErrNilCurrentUserUpdater},
		{"nil queries", up, nil, res, ErrNilCurrentUserQueries},
		{"nil resolver", up, ld, nil, ErrNilCurrentUserResolver},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewCurrentUserHandler(tt.up, tt.ld, tt.res)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && h == nil {
				t.Fatal("nil handler")
			}
			if tt.wantErr != nil && h != nil {
				t.Fatal("expected nil handler")
			}
		})
	}
}

func cuDecodeUser(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	u, ok := m["user"].(map[string]any)
	if !ok {
		t.Fatalf("no user key in %q", body)
	}
	return u
}

func TestCurrentUser(t *testing.T) {
	user := cuTestUser()
	tests := []struct {
		name       string
		user       *core.User
		auth       *string
		loader     *cuFakeLoader
		wantStatus int
		wantToken  string
	}{
		{
			name: "valid token", user: user, auth: cuStrPtr("Token abc.def.ghi"),
			loader:     &cuFakeLoader{data: app.NewUserData("u1", user.Email, user.Username, "", cuDefaultAvatar), found: true},
			wantStatus: http.StatusOK, wantToken: "abc.def.ghi",
		},
		{name: "no auth header unauthenticated", user: nil, auth: nil, loader: &cuFakeLoader{}, wantStatus: http.StatusUnauthorized},
		{name: "invalid token unresolved", user: nil, auth: cuStrPtr("Token asdfasd"), loader: &cuFakeLoader{}, wantStatus: http.StatusUnauthorized},
		{name: "authenticated but header missing", user: user, auth: nil, loader: &cuFakeLoader{found: true}, wantStatus: http.StatusBadRequest},
		{name: "header without space", user: user, auth: cuStrPtr("Tokenabc"), loader: &cuFakeLoader{found: true}, wantStatus: http.StatusInternalServerError},
		{name: "read model missing", user: user, auth: cuStrPtr("Token t"), loader: &cuFakeLoader{found: false}, wantStatus: http.StatusInternalServerError},
		{name: "loader error", user: user, auth: cuStrPtr("Token t"), loader: &cuFakeLoader{err: errors.New("db")}, wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewCurrentUserHandler(&cuFakeUpdater{}, tt.loader, cuResolverFor(tt.user))
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			h.Register(mux)
			req := httptest.NewRequest(http.MethodGet, "/user", nil)
			if tt.auth != nil {
				req.Header.Set("Authorization", *tt.auth)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			if tt.loader.gotID != user.ID {
				t.Errorf("loaded id %q, want %q", tt.loader.gotID, user.ID)
			}
			u := cuDecodeUser(t, rec.Body.String())
			if u["email"] != user.Email || u["username"] != user.Username {
				t.Errorf("unexpected user %v", u)
			}
			if u["image"] != cuDefaultAvatar {
				t.Errorf("image = %v", u["image"])
			}
			if b, ok := u["bio"]; ok && b != nil && b != "" {
				t.Errorf("bio = %v", b)
			}
			if u["token"] != tt.wantToken {
				t.Errorf("token = %v, want %s", u["token"], tt.wantToken)
			}
		})
	}
}

func TestUpdateProfile(t *testing.T) {
	updated := app.NewUserData("u1", "newemail@example.com", "newusernamee", "updated", cuDefaultAvatar)
	tests := []struct {
		name        string
		authed      bool
		auth        *string
		body        string
		updErr      error
		loader      *cuFakeLoader
		wantStatus  int
		wantCalled  bool
		wantField   string
		wantMessage string
	}{
		{
			name: "success", authed: true, auth: cuStrPtr("Token tok123"),
			body:   `{"user":{"email":"newemail@example.com","bio":"updated","username":"newusernamee"}}`,
			loader: &cuFakeLoader{data: updated, found: true}, wantStatus: http.StatusOK, wantCalled: true,
		},
		{
			name: "duplicate email", authed: true, auth: cuStrPtr("Token tok123"),
			body:      `{"user":{"email":"newemail@example.com"}}`,
			updErr:    errors.Join(&app.FieldError{Field: "email", Message: "email already exist"}),
			loader:    &cuFakeLoader{found: true}, wantStatus: http.StatusUnprocessableEntity, wantCalled: true,
			wantField: "email", wantMessage: "email already exist",
		},
		{
			name: "duplicate username", authed: true, auth: cuStrPtr("Token tok123"),
			body:      `{"user":{"username":"taken"}}`,
			updErr:    errors.Join(&app.FieldError{Field: "username", Message: "username already exist"}),
			loader:    &cuFakeLoader{found: true}, wantStatus: http.StatusUnprocessableEntity, wantCalled: true,
			wantField: "username", wantMessage: "username already exist",
		},
		{name: "not logged in", authed: false, auth: nil, body: `{"user":{}}`, loader: &cuFakeLoader{}, wantStatus: http.StatusUnauthorized},
		{name: "malformed body", authed: true, auth: cuStrPtr("Token t"), body: `{bad`, loader: &cuFakeLoader{found: true}, wantStatus: http.StatusBadRequest},
		{name: "missing header", authed: true, auth: nil, body: `{"user":{}}`, loader: &cuFakeLoader{found: true}, wantStatus: http.StatusBadRequest},
		{
			name: "updater internal error", authed: true, auth: cuStrPtr("Token t"), body: `{"user":{"bio":"x"}}`,
			updErr: errors.New("boom"), loader: &cuFakeLoader{found: true}, wantStatus: http.StatusInternalServerError, wantCalled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u *core.User
			if tt.authed {
				u = cuTestUser()
			}
			up := &cuFakeUpdater{err: tt.updErr}
			h, err := NewCurrentUserHandler(up, tt.loader, cuResolverFor(u))
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			h.Register(mux)
			req := httptest.NewRequest(http.MethodPut, "/user", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.auth != nil {
				req.Header.Set("Authorization", *tt.auth)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if up.called != tt.wantCalled {
				t.Fatalf("updater called = %v, want %v", up.called, tt.wantCalled)
			}
			if tt.wantStatus != http.StatusOK && tt.loader.called {
				t.Errorf("read model should not be loaded on failure")
			}
			switch tt.wantStatus {
			case http.StatusOK:
				if up.cmd.TargetUser() != u {
					t.Errorf("wrong target user")
				}
				p := up.cmd.Param()
				if p.Email() != "newemail@example.com" || p.Username() != "newusernamee" || p.Bio() != "updated" {
					t.Errorf("param not decoded correctly")
				}
				if !tt.loader.called || tt.loader.gotID != "u1" {
					t.Errorf("profile not re-read by id")
				}
				got := cuDecodeUser(t, rec.Body.String())
				if got["token"] != "tok123" {
					t.Errorf("token = %v", got["token"])
				}
				if got["email"] != updated.Email || got["username"] != updated.Username || got["bio"] != updated.Bio {
					t.Errorf("response user %v", got)
				}
			case http.StatusUnprocessableEntity:
				var body struct {
					Errors map[string][]string `json:"errors"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				msgs := body.Errors[tt.wantField]
				if len(msgs) == 0 || msgs[0] != tt.wantMessage {
					t.Errorf("errors = %v", body.Errors)
				}
			}
		})
	}
}

func cuStrPtr(s string) *string { return &s }