package graphql

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"migrated-app/internal/app"
	"migrated-app/internal/core"
	"migrated-app/internal/httpapi"
)

type umFakeUsers struct {
	user   *core.User
	err    error
	calls  int
	gotArg string
}

func (f *umFakeUsers) FindByEmail(_ context.Context, email string) (*core.User, error) {
	f.calls++
	f.gotArg = email
	return f.user, f.err
}

type fakeMatcher struct {
	result  bool
	calls   int
	raw     string
	encoded string
}

func (f *fakeMatcher) Matches(raw, encoded string) bool {
	f.calls++
	f.raw, f.encoded = raw, encoded
	return f.result
}

type fakeService struct {
	createUser  *core.User
	createErr   error
	createCalls int
	gotEmail    string
	gotUsername string
	gotPassword string

	updateErr   error
	updateCalls int
	gotCmd      *app.UpdateUserCommand
}

func (f *fakeService) CreateUser(_ context.Context, email, username, password string) (*core.User, error) {
	f.createCalls++
	f.gotEmail, f.gotUsername, f.gotPassword = email, username, password
	return f.createUser, f.createErr
}

func (f *fakeService) UpdateUser(_ context.Context, cmd *app.UpdateUserCommand) error {
	f.updateCalls++
	f.gotCmd = cmd
	return f.updateErr
}

func TestNewUserMutation(t *testing.T) {
	tests := []struct {
		name    string
		users   UserMutationUsers
		matcher PasswordMatcher
		svc     UserMutationService
		wantErr error
	}{
		{"nil users", nil, &fakeMatcher{}, &fakeService{}, ErrNilUserMutationUsers},
		{"nil matcher", &umFakeUsers{}, nil, &fakeService{}, ErrNilUserMutationMatcher},
		{"nil service", &umFakeUsers{}, &fakeMatcher{}, nil, ErrNilUserMutationService},
		{"ok", &umFakeUsers{}, &fakeMatcher{}, &fakeService{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewUserMutation(tt.users, tt.matcher, tt.svc)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && m == nil {
				t.Fatal("expected mutation")
			}
			if tt.wantErr != nil && m != nil {
				t.Fatal("expected nil mutation")
			}
		})
	}
}

func TestCreateUser(t *testing.T) {
	created := &core.User{ID: "u1", Email: "a@b.c", Username: "alice"}
	input := CreateUserInput{Email: "a@b.c", Username: "alice", Password: "secret"}
	boom := errors.New("db down")

	tests := []struct {
		name        string
		svc         *fakeService
		wantPayload bool
		wantData    *Error
		wantErr     error
	}{
		{
			name:        "success",
			svc:         &fakeService{createUser: created},
			wantPayload: true,
		},
		{
			name: "validation errors as data",
			svc: &fakeService{createErr: errors.Join(
				&app.FieldError{Field: "username", Message: "duplicated username"},
				&app.FieldError{Field: "email", Message: "duplicated email"},
				&app.FieldError{Field: "email", Message: "should be an email"},
			)},
			wantData: &Error{Message: "BAD_REQUEST", Errors: []ErrorItem{
				{Key: "email", Value: []string{"duplicated email", "should be an email"}},
				{Key: "username", Value: []string{"duplicated username"}},
			}},
		},
		{
			name: "single field error as data",
			svc:  &fakeService{createErr: &app.FieldError{Field: "password", Message: "can't be empty"}},
			wantData: &Error{Message: "BAD_REQUEST", Errors: []ErrorItem{
				{Key: "password", Value: []string{"can't be empty"}},
			}},
		},
		{
			name:    "unexpected error",
			svc:     &fakeService{createErr: boom},
			wantErr: boom,
		},
		{
			name: "mixed errors surface as error",
			svc: &fakeService{createErr: errors.Join(
				&app.FieldError{Field: "email", Message: "duplicated email"}, boom)},
			wantErr: boom,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &fakeMatcher{}
			m, err := NewUserMutation(&umFakeUsers{}, matcher, tt.svc)
			if err != nil {
				t.Fatal(err)
			}
			res, err := m.CreateUser(context.Background(), input)
			if tt.svc.createCalls != 1 {
				t.Fatalf("CreateUser calls = %d, want 1", tt.svc.createCalls)
			}
			if tt.svc.gotEmail != "a@b.c" || tt.svc.gotUsername != "alice" || tt.svc.gotPassword != "secret" {
				t.Fatalf("bad args: %q %q %q", tt.svc.gotEmail, tt.svc.gotUsername, tt.svc.gotPassword)
			}
			if matcher.calls != 0 {
				t.Fatal("matcher must not be used")
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || res != nil {
					t.Fatalf("got (%v, %v), want error %v", res, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if tt.wantPayload {
				if res.Error != nil || res.Payload == nil || res.Payload.LocalContext != created {
					t.Fatalf("bad result %+v", res)
				}
				if res.TypeName() != UserPayloadType {
					t.Fatalf("TypeName = %s", res.TypeName())
				}
			}
			if tt.wantData != nil {
				if res.Payload != nil || res.Error == nil {
					t.Fatalf("bad result %+v", res)
				}
				if !reflect.DeepEqual(*res.Error, *tt.wantData) {
					t.Fatalf("data = %+v, want %+v", *res.Error, *tt.wantData)
				}
				if res.TypeName() != "Error" {
					t.Fatalf("TypeName = %s", res.TypeName())
				}
			}
		})
	}
}

func TestLogin(t *testing.T) {
	stored := &core.User{ID: "u1", Email: "a@b.c", Password: "hashed"}
	boom := errors.New("db down")
	tests := []struct {
		name             string
		users            *umFakeUsers
		match            bool
		wantMatcherCalls int
		wantUser         *core.User
		wantInvalidAuth  bool
		wantErr          error
	}{
		{"success", &umFakeUsers{user: stored}, true, 1, stored, false, nil},
		{"unknown email", &umFakeUsers{err: core.ErrNotFound}, true, 0, nil, true, nil},
		{"wrong password", &umFakeUsers{user: stored}, false, 1, nil, true, nil},
		{"nil user", &umFakeUsers{}, true, 0, nil, true, nil},
		{"repo failure", &umFakeUsers{err: boom}, true, 0, nil, false, boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &fakeMatcher{result: tt.match}
			svc := &fakeService{}
			m, _ := NewUserMutation(tt.users, matcher, svc)
			res, err := m.Login(context.Background(), "a@b.c", "raw")
			if tt.users.calls != 1 || tt.users.gotArg != "a@b.c" {
				t.Fatalf("lookup calls=%d arg=%q", tt.users.calls, tt.users.gotArg)
			}
			if matcher.calls != tt.wantMatcherCalls {
				t.Fatalf("matcher calls = %d, want %d", matcher.calls, tt.wantMatcherCalls)
			}
			if matcher.calls > 0 && (matcher.raw != "raw" || matcher.encoded != "hashed") {
				t.Fatalf("matcher args = (%q,%q)", matcher.raw, matcher.encoded)
			}
			if svc.createCalls != 0 || svc.updateCalls != 0 {
				t.Fatal("service must not be called")
			}
			switch {
			case tt.wantInvalidAuth:
				if !errors.Is(err, httpapi.ErrInvalidAuthentication) || res != nil {
					t.Fatalf("got (%v,%v)", res, err)
				}
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) || errors.Is(err, httpapi.ErrInvalidAuthentication) || res != nil {
					t.Fatalf("got (%v,%v)", res, err)
				}
			default:
				if err != nil || res == nil || res.LocalContext != tt.wantUser {
					t.Fatalf("got (%v,%v)", res, err)
				}
				if stored.Password != "hashed" || stored.Email != "a@b.c" {
					t.Fatal("user mutated")
				}
			}
		})
	}
}

func TestUpdateUserAnonymous(t *testing.T) {
	svc := &fakeService{}
	m, _ := NewUserMutation(&umFakeUsers{}, &fakeMatcher{}, svc)
	email := "x@y.z"
	res, err := m.UpdateUser(context.Background(), UpdateUserInput{Email: &email})
	if res != nil || err != nil {
		t.Fatalf("got (%v,%v), want (nil,nil)", res, err)
	}
	if svc.updateCalls != 0 {
		t.Fatal("service must not be called")
	}
}

func TestFieldErrorsToViolations(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name   string
		err    error
		wantOK bool
		want   map[string][]string
	}{
		{"single", &app.FieldError{Field: "email", Message: "email already exist"}, true,
			map[string][]string{"email": {"email already exist"}}},
		{"joined", errors.Join(
			&app.FieldError{Field: "email", Message: "email already exist"},
			&app.FieldError{Field: "username", Message: "username already exist"}), true,
			map[string][]string{"email": {"email already exist"}, "username": {"username already exist"}}},
		{"plain error", boom, false, nil},
		{"mixed", errors.Join(&app.FieldError{Field: "email", Message: "x"}, boom), false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cve, ok := fieldErrorsToViolations(tt.err, "updateUser.command")
			if ok != tt.wantOK {
				t.Fatalf("ok = %v", ok)
			}
			if !ok {
				if cve != nil {
					t.Fatal("expected nil cve")
				}
				return
			}
			if !reflect.DeepEqual(cve.FieldErrors(), tt.want) {
				t.Fatalf("got %v, want %v", cve.FieldErrors(), tt.want)
			}
			for _, v := range cve.Violations {
				if len(v.PropertyPath) < len("updateUser.command.") || v.PropertyPath[:19] != "updateUser.command." {
					t.Fatalf("bad path %q", v.PropertyPath)
				}
			}
			res := NewCustomizeExceptionHandler(nil).HandleError(cve, nil)
			if len(res.Errors) != 1 || res.Errors[0].Extensions["errorType"] != string(ErrorTypeBadRequest) {
				t.Fatalf("bad handler result %+v", res)
			}
		})
	}
}