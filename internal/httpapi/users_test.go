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

const defaultImg = "https://static.productionready.io/images/smiley-cyrus.jpg"

type fakeRegistrar struct {
	calls int
	user  *core.User
	err   error
}

func (f *fakeRegistrar) CreateUser(ctx context.Context, email, username, password string) (*core.User, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.user != nil {
		return f.user, nil
	}
	return &core.User{ID: "u1", Email: email, Username: username, Password: "hashed", Image: defaultImg}, nil
}

type usersFakeFinder struct {
	user *core.User
	err  error
}

func (f *usersFakeFinder) FindByEmail(ctx context.Context, email string) (*core.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.user == nil || f.user.Email != email {
		return nil, core.ErrNotFound
	}
	return f.user, nil
}

type fakeLoader struct {
	data  map[string]app.UserData
	err   error
	calls int
}

func (f *fakeLoader) FindByID(ctx context.Context, id string) (app.UserData, bool, error) {
	f.calls++
	if f.err != nil {
		return app.UserData{}, false, f.err
	}
	d, ok := f.data[id]
	return d, ok, nil
}

type fakeMatcher struct{}

func (fakeMatcher) Matches(raw, encoded string) bool { return "hash:"+raw == encoded }

type fakeTokens struct {
	calls int
	err   error
}

func (f *fakeTokens) ToToken(u *core.User) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return "123", nil
}

func TestNewUsersHandler(t *testing.T) {
	r, f, l, m, tk := &fakeRegistrar{}, &usersFakeFinder{}, &fakeLoader{}, fakeMatcher{}, &fakeTokens{}
	tests := []struct {
		name string
		fn   func() (*UsersHandler, error)
		want error
	}{
		{"ok", func() (*UsersHandler, error) { return NewUsersHandler(r, f, l, m, tk) }, nil},
		{"nil registrar", func() (*UsersHandler, error) { return NewUsersHandler(nil, f, l, m, tk) }, ErrNilUserRegistrar},
		{"nil finder", func() (*UsersHandler, error) { return NewUsersHandler(r, nil, l, m, tk) }, ErrNilUserEmailFinder},
		{"nil loader", func() (*UsersHandler, error) { return NewUsersHandler(r, f, nil, m, tk) }, ErrNilUserDataLoader},
		{"nil matcher", func() (*UsersHandler, error) { return NewUsersHandler(r, f, l, nil, tk) }, ErrNilPasswordMatcher},
		{"nil tokens", func() (*UsersHandler, error) { return NewUsersHandler(r, f, l, m, nil) }, ErrNilTokenIssuer},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, err := tc.fn()
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.want == nil && h == nil {
				t.Fatal("nil handler")
			}
		})
	}
}

type respBody struct {
	User struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Bio      string `json:"bio"`
		Image    string `json:"image"`
		Token    string `json:"token"`
	} `json:"user"`
	Errors  map[string][]string `json:"errors"`
	Message string              `json:"message"`
}

func serve(t *testing.T, h *UsersHandler, path, body string) (*httptest.ResponseRecorder, respBody) {
	t.Helper()
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var b respBody
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &b)
	}
	return rec, b
}

func TestCreateUser(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		regErr     error
		loaderErr  error
		missing    bool
		tokenErr   error
		wantStatus int
		wantField  string
		wantMsg    string
		wantTokens int
	}{
		{name: "success", body: `{"user":{"email":"john@jacob.com","username":"johnjacob","password":"johnnyjacob"}}`, wantStatus: 201, wantTokens: 1},
		{name: "blank username", body: `{"user":{"email":"john@jacob.com","username":"","password":"p"}}`,
			regErr: errors.Join(&app.FieldError{Field: "username", Message: "can't be empty"}), wantStatus: 422, wantField: "username", wantMsg: "can't be empty"},
		{name: "invalid email", body: `{"user":{"email":"johnxjacob.com","username":"johnjacob","password":"p"}}`,
			regErr: errors.Join(&app.FieldError{Field: "email", Message: "should be an email"}), wantStatus: 422, wantField: "email", wantMsg: "should be an email"},
		{name: "duplicated username", body: `{"user":{"email":"john@jacob.com","username":"johnjacob","password":"p"}}`,
			regErr: &app.FieldError{Field: "username", Message: "duplicated username"}, wantStatus: 422, wantField: "username", wantMsg: "duplicated username"},
		{name: "duplicated email", body: `{"user":{"email":"john@jacob.com","username":"johnjacob2","password":"p"}}`,
			regErr: errors.Join(&app.FieldError{Field: "email", Message: "duplicated email"}), wantStatus: 422, wantField: "email", wantMsg: "duplicated email"},
		{name: "malformed json", body: `{bad`, wantStatus: 400},
		{name: "registrar internal error", body: `{"user":{"email":"a@b.c","username":"u","password":"p"}}`, regErr: errors.New("db"), wantStatus: 500},
		{name: "loader error", body: `{"user":{"email":"a@b.c","username":"u","password":"p"}}`, loaderErr: errors.New("db"), wantStatus: 500},
		{name: "read model missing", body: `{"user":{"email":"a@b.c","username":"u","password":"p"}}`, missing: true, wantStatus: 500},
		{name: "token error", body: `{"user":{"email":"a@b.c","username":"u","password":"p"}}`, tokenErr: errors.New("sign"), wantStatus: 500, wantTokens: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := &fakeRegistrar{err: tc.regErr}
			loader := &fakeLoader{err: tc.loaderErr, data: map[string]app.UserData{}}
			if !tc.missing {
				loader.data["u1"] = app.NewUserData("u1", "john@jacob.com", "johnjacob", "", defaultImg)
			}
			tk := &fakeTokens{err: tc.tokenErr}
			h, err := NewUsersHandler(reg, &usersFakeFinder{}, loader, fakeMatcher{}, tk)
			if err != nil {
				t.Fatal(err)
			}
			rec, b := serve(t, h, "/users", tc.body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tk.calls != tc.wantTokens {
				t.Errorf("token calls = %d, want %d", tk.calls, tc.wantTokens)
			}
			if tc.wantStatus == 201 {
				if reg.calls != 1 {
					t.Errorf("registrar calls = %d", reg.calls)
				}
				u := b.User
				if u.Email != "john@jacob.com" || u.Username != "johnjacob" || u.Bio != "" || u.Image != defaultImg || u.Token != "123" {
					t.Errorf("unexpected user %+v", u)
				}
			}
			if tc.wantField != "" {
				msgs := b.Errors[tc.wantField]
				if len(msgs) == 0 || msgs[0] != tc.wantMsg {
					t.Errorf("errors[%s] = %v, want %q", tc.wantField, msgs, tc.wantMsg)
				}
			}
		})
	}
}

func TestLogin(t *testing.T) {
	stored := &core.User{ID: "u2", Email: "john@jacob.com", Username: "johnjacob2", Password: "hash:123", Image: defaultImg}
	tests := []struct {
		name       string
		body       string
		finderErr  error
		wantStatus int
		wantMsg    string
		wantErrs   map[string]string
		wantTokens int
	}{
		{name: "success", body: `{"user":{"email":"john@jacob.com","password":"123"}}`, wantStatus: 200, wantTokens: 1},
		{name: "wrong password", body: `{"user":{"email":"john@jacob.com","password":"123123"}}`, wantStatus: 422, wantMsg: "invalid email or password"},
		{name: "unknown email", body: `{"user":{"email":"nobody@x.com","password":"123"}}`, wantStatus: 422, wantMsg: "invalid email or password"},
		{name: "blank email", body: `{"user":{"email":"","password":"123"}}`, wantStatus: 422, wantErrs: map[string]string{"email": "can't be empty"}},
		{name: "malformed email", body: `{"user":{"email":"johnxjacob.com","password":"123"}}`, wantStatus: 422, wantErrs: map[string]string{"email": "should be an email"}},
		{name: "blank password", body: `{"user":{"email":"john@jacob.com","password":"  "}}`, wantStatus: 422, wantErrs: map[string]string{"password": "can't be empty"}},
		{name: "missing root", body: `{}`, wantStatus: 422, wantErrs: map[string]string{"email": "can't be empty", "password": "can't be empty"}},
		{name: "finder internal error", body: `{"user":{"email":"john@jacob.com","password":"123"}}`, finderErr: errors.New("db"), wantStatus: 500},
		{name: "malformed json", body: `not json`, wantStatus: 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loader := &fakeLoader{data: map[string]app.UserData{"u2": app.NewUserData("u2", "john@jacob.com", "johnjacob2", "", defaultImg)}}
			tk := &fakeTokens{}
			h, err := NewUsersHandler(&fakeRegistrar{}, &usersFakeFinder{user: stored, err: tc.finderErr}, loader, fakeMatcher{}, tk)
			if err != nil {
				t.Fatal(err)
			}
			rec, b := serve(t, h, "/users/login", tc.body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tk.calls != tc.wantTokens {
				t.Errorf("token calls = %d, want %d", tk.calls, tc.wantTokens)
			}
			if tc.wantStatus == 200 {
				u := b.User
				if u.Email != "john@jacob.com" || u.Username != "johnjacob2" || u.Bio != "" || u.Image != defaultImg || u.Token != "123" {
					t.Errorf("unexpected user %+v", u)
				}
			}
			if tc.wantMsg != "" && b.Message != tc.wantMsg {
				t.Errorf("message = %q, want %q", b.Message, tc.wantMsg)
			}
			for f, m := range tc.wantErrs {
				if msgs := b.Errors[f]; len(msgs) == 0 || msgs[0] != m {
					t.Errorf("errors[%s] = %v, want %q", f, msgs, m)
				}
			}
		})
	}
}

func TestLoginFailuresIndistinguishable(t *testing.T) {
	stored := &core.User{ID: "u2", Email: "john@jacob.com", Password: "hash:123"}
	h, _ := NewUsersHandler(&fakeRegistrar{}, &usersFakeFinder{user: stored}, &fakeLoader{}, fakeMatcher{}, &fakeTokens{})
	r1, _ := serve(t, h, "/users/login", `{"user":{"email":"john@jacob.com","password":"bad"}}`)
	r2, _ := serve(t, h, "/users/login", `{"user":{"email":"other@jacob.com","password":"bad"}}`)
	if r1.Code != r2.Code || r1.Body.String() != r2.Body.String() {
		t.Errorf("responses differ: %d %q vs %d %q", r1.Code, r1.Body.String(), r2.Code, r2.Body.String())
	}
}

func TestCollectAppFieldErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want map[string][]string
	}{
		{"nil", nil, map[string][]string{}},
		{"plain", errors.New("x"), map[string][]string{}},
		{"single", &app.FieldError{Field: "email", Message: "m"}, map[string][]string{"email": {"m"}}},
		{"joined", errors.Join(&app.FieldError{Field: "email", Message: "a"}, &app.FieldError{Field: "email", Message: "b"}, &app.FieldError{Field: "username", Message: "c"}),
			map[string][]string{"email": {"a", "b"}, "username": {"c"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := collectAppFieldErrors(tc.err)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if strings.Join(got[k], "|") != strings.Join(v, "|") {
					t.Errorf("%s: got %v want %v", k, got[k], v)
				}
			}
		})
	}
}