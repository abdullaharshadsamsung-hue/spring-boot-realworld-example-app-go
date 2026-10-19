package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"migrated-app/internal/core"
)

type fakeUserRepo struct {
	emails      map[string]bool
	usernames   map[string]bool
	emailErr    error
	usernameErr error
	saveErr     error
	saved       []*core.User
	emailCalls  int
	userCalls   int
}

func (f *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*core.User, error) {
	f.emailCalls++
	if f.emailErr != nil {
		return nil, f.emailErr
	}
	if f.emails[email] {
		return &core.User{}, nil
	}
	return nil, core.ErrNotFound
}

func (f *fakeUserRepo) FindByUsername(ctx context.Context, username string) (*core.User, error) {
	f.userCalls++
	if f.usernameErr != nil {
		return nil, f.usernameErr
	}
	if f.usernames[username] {
		return &core.User{}, nil
	}
	return nil, core.ErrNotFound
}

func (f *fakeUserRepo) Save(ctx context.Context, u *core.User) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, u)
	return nil
}

type fakeHasher struct {
	err   error
	calls int
}

func (h *fakeHasher) Hash(p string) (string, error) {
	h.calls++
	if h.err != nil {
		return "", h.err
	}
	return "hashed:" + p, nil
}

func TestDecodeRegisterParam(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    RegisterParam
		wantErr bool
	}{
		{"wrapped", `{"user":{"email":"a@b.com","username":"u","password":"p"}}`, NewRegisterParam("a@b.com", "u", "p"), false},
		{"missing root", `{"email":"a@b.com"}`, RegisterParam{}, false},
		{"invalid json", `{`, RegisterParam{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeRegisterParam(strings.NewReader(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestRegisterParamValidate(t *testing.T) {
	tests := []struct {
		name  string
		p     RegisterParam
		repo  *fakeUserRepo
		want  map[string][]string
	}{
		{"valid", NewRegisterParam("a@b.com", "u", "p"), &fakeUserRepo{}, nil},
		{"empty email", NewRegisterParam("", "u", "p"), &fakeUserRepo{}, map[string][]string{"email": {msgCantBeEmpty}}},
		{"whitespace email", NewRegisterParam("  ", "u", "p"), &fakeUserRepo{}, map[string][]string{"email": {msgCantBeEmpty, msgShouldBeEmail}}},
		{"malformed email", NewRegisterParam("not-an-email", "u", "p"), &fakeUserRepo{}, map[string][]string{"email": {msgShouldBeEmail}}},
		{"duplicated email", NewRegisterParam("a@b.com", "u", "p"), &fakeUserRepo{emails: map[string]bool{"a@b.com": true}}, map[string][]string{"email": {msgDuplicatedEmail}}},
		{"empty username", NewRegisterParam("a@b.com", "", "p"), &fakeUserRepo{}, map[string][]string{"username": {msgCantBeEmpty}}},
		{"whitespace username", NewRegisterParam("a@b.com", "   ", "p"), &fakeUserRepo{}, map[string][]string{"username": {msgCantBeEmpty}}},
		{"duplicated username", NewRegisterParam("a@b.com", "u", "p"), &fakeUserRepo{usernames: map[string]bool{"u": true}}, map[string][]string{"username": {msgDuplicatedUsernam}}},
		{"empty password", NewRegisterParam("a@b.com", "u", ""), &fakeUserRepo{}, map[string][]string{"password": {msgCantBeEmpty}}},
		{"whitespace password", NewRegisterParam("a@b.com", "u", " \t"), &fakeUserRepo{}, map[string][]string{"password": {msgCantBeEmpty}}},
		{"all empty", RegisterParam{}, &fakeUserRepo{}, map[string][]string{"email": {msgCantBeEmpty}, "username": {msgCantBeEmpty}, "password": {msgCantBeEmpty}}},
		{"multiple", NewRegisterParam("bad", "u", ""), &fakeUserRepo{usernames: map[string]bool{"u": true}}, map[string][]string{"email": {msgShouldBeEmail}, "username": {msgDuplicatedUsernam}, "password": {msgCantBeEmpty}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate(context.Background(), tt.repo)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("unexpected err %v", err)
				}
				return
			}
			var verr *RegisterValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("want validation error, got %v", err)
			}
			if !reflect.DeepEqual(verr.Fields, tt.want) {
				t.Fatalf("got %v want %v", verr.Fields, tt.want)
			}
			if !strings.HasPrefix(verr.Error(), "validation failed: ") {
				t.Fatalf("bad message %q", verr.Error())
			}
		})
	}
}

func TestRegisterParamValidateErrors(t *testing.T) {
	boom := errors.New("db down")
	p := NewRegisterParam("a@b.com", "u", "p")
	if err := p.Validate(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil finder")
	}
	for _, repo := range []*fakeUserRepo{{emailErr: boom}, {usernameErr: boom}} {
		err := p.Validate(context.Background(), repo)
		if !errors.Is(err, boom) {
			t.Fatalf("want wrapped boom, got %v", err)
		}
		var verr *RegisterValidationError
		if errors.As(err, &verr) {
			t.Fatal("lookup failure must not be validation error")
		}
	}
}

func TestRegisterValidationErrorMessage(t *testing.T) {
	e := &RegisterValidationError{Fields: map[string][]string{"username": {"x"}, "email": {"a", "b"}}}
	want := "validation failed: email: a, b; username: x"
	if e.Error() != want {
		t.Fatalf("got %q", e.Error())
	}
}

func TestNewUserService(t *testing.T) {
	if _, err := NewUserService(nil, &fakeHasher{}, "img"); err == nil {
		t.Fatal("expected nil repo error")
	}
	if _, err := NewUserService(&fakeUserRepo{}, nil, "img"); err == nil {
		t.Fatal("expected nil hasher error")
	}
	if s, err := NewUserService(&fakeUserRepo{}, &fakeHasher{}, "img"); err != nil || s == nil {
		t.Fatalf("unexpected %v", err)
	}
}

func TestUserServiceCreateUser(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name        string
		p           RegisterParam
		repo        *fakeUserRepo
		hasher      *fakeHasher
		wantValid   bool
		wantErrIs   error
		wantSuccess bool
	}{
		{"success", NewRegisterParam("a@b.com", "u", "p"), &fakeUserRepo{}, &fakeHasher{}, false, nil, true},
		{"blank fields", RegisterParam{}, &fakeUserRepo{}, &fakeHasher{}, true, nil, false},
		{"malformed email", NewRegisterParam("bad", "u", "p"), &fakeUserRepo{}, &fakeHasher{}, true, nil, false},
		{"dup email", NewRegisterParam("a@b.com", "u", "p"), &fakeUserRepo{emails: map[string]bool{"a@b.com": true}}, &fakeHasher{}, true, nil, false},
		{"dup username", NewRegisterParam("a@b.com", "u", "p"), &fakeUserRepo{usernames: map[string]bool{"u": true}}, &fakeHasher{}, true, nil, false},
		{"hash error", NewRegisterParam("a@b.com", "u", "p"), &fakeUserRepo{}, &fakeHasher{err: boom}, false, boom, false},
		{"save error", NewRegisterParam("a@b.com", "u", "p"), &fakeUserRepo{saveErr: boom}, &fakeHasher{}, false, boom, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewUserService(tt.repo, tt.hasher, "default.png")
			if err != nil {
				t.Fatal(err)
			}
			u, err := s.CreateUser(context.Background(), tt.p)
			if tt.wantSuccess {
				if err != nil || u == nil {
					t.Fatalf("unexpected %v", err)
				}
				if len(tt.repo.saved) != 1 || tt.repo.saved[0] != u {
					t.Fatal("user not saved")
				}
				if tt.repo.emailCalls == 0 || tt.repo.userCalls == 0 {
					t.Fatal("uniqueness not checked")
				}
				return
			}
			if u != nil {
				t.Fatal("expected nil user")
			}
			if len(tt.repo.saved) != 0 {
				t.Fatal("user must not be created")
			}
			var verr *RegisterValidationError
			if tt.wantValid {
				if !errors.As(err, &verr) {
					t.Fatalf("want validation error, got %v", err)
				}
				if tt.hasher.calls != 0 {
					t.Fatal("hasher should not run")
				}
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Fatalf("want %v got %v", tt.wantErrIs, err)
			}
		})
	}
}