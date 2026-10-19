package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"migrated-app/internal/core"
)

type fakeUserRepo struct {
	byEmail       map[string]*core.User
	byUsername    map[string]*core.User
	findEmailErr  error
	findUserErr   error
	saveErr       error
	saved         []*core.User
	emailLookups  []string
	usernameLooks []string
}

func newFakeRepo() *fakeUserRepo {
	return &fakeUserRepo{byEmail: map[string]*core.User{}, byUsername: map[string]*core.User{}}
}

func (f *fakeUserRepo) Save(ctx context.Context, u *core.User) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, u)
	return nil
}
func (f *fakeUserRepo) FindByID(ctx context.Context, id string) (*core.User, error) {
	return nil, core.ErrNotFound
}
func (f *fakeUserRepo) FindByUsername(ctx context.Context, username string) (*core.User, error) {
	f.usernameLooks = append(f.usernameLooks, username)
	if f.findUserErr != nil {
		return nil, f.findUserErr
	}
	if u, ok := f.byUsername[username]; ok {
		return u, nil
	}
	return nil, core.ErrNotFound
}
func (f *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*core.User, error) {
	f.emailLookups = append(f.emailLookups, email)
	if f.findEmailErr != nil {
		return nil, f.findEmailErr
	}
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, core.ErrNotFound
}
func (f *fakeUserRepo) SaveRelation(ctx context.Context, r *core.FollowRelation) error { return nil }
func (f *fakeUserRepo) FindRelation(ctx context.Context, userID, targetID string) (*core.FollowRelation, error) {
	return nil, core.ErrNotFound
}
func (f *fakeUserRepo) RemoveRelation(ctx context.Context, r *core.FollowRelation) error { return nil }

type fakeEncoder struct {
	calls []string
	err   error
}

func (e *fakeEncoder) Encode(raw string) (string, error) {
	e.calls = append(e.calls, raw)
	if e.err != nil {
		return "", e.err
	}
	return "hashed:" + raw, nil
}

func paramPtr(p UpdateUserParam) *UpdateUserParam { return &p }

func collectFieldErrors(err error) []FieldError {
	var out []FieldError
	var walk func(error)
	walk = func(e error) {
		if e == nil {
			return
		}
		if fe, ok := e.(*FieldError); ok {
			out = append(out, *fe)
			return
		}
		if m, ok := e.(interface{ Unwrap() []error }); ok {
			for _, c := range m.Unwrap() {
				walk(c)
			}
			return
		}
		walk(errors.Unwrap(e))
	}
	walk(err)
	return out
}

func hasFieldError(errs []FieldError, field, msg string) bool {
	for _, e := range errs {
		if e.Field == field && e.Message == msg {
			return true
		}
	}
	return false
}

func newSvc(t *testing.T, repo *fakeUserRepo, enc *fakeEncoder) *UserService {
	t.Helper()
	s, err := NewUserService(repo, "default.png", enc)
	if err != nil {
		t.Fatalf("NewUserService: %v", err)
	}
	return s
}

func TestNewUserService(t *testing.T) {
	tests := []struct {
		name    string
		repo    core.UserRepository
		enc     UserPasswordEncoder
		wantErr error
	}{
		{"ok", newFakeRepo(), &fakeEncoder{}, nil},
		{"nil repo", nil, &fakeEncoder{}, ErrNilUserServiceRepository},
		{"nil encoder", newFakeRepo(), nil, ErrNilUserServiceEncoder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewUserService(tt.repo, "img", tt.enc)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && s == nil {
				t.Fatal("nil service")
			}
		})
	}
}

func TestCreateUserSuccess(t *testing.T) {
	repo := newFakeRepo()
	enc := &fakeEncoder{}
	s := newSvc(t, repo, enc)
	u, err := s.CreateUser(context.Background(), "a@b.com", "alice", "secret")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Email != "a@b.com" || u.Username != "alice" {
		t.Errorf("unexpected user %v", u)
	}
	if u.Password != "hashed:secret" {
		t.Errorf("password = %q", u.Password)
	}
	if u.Bio != "" || u.Image != "default.png" {
		t.Errorf("bio/image = %q/%q", u.Bio, u.Image)
	}
	if u.ID == "" {
		t.Error("empty ID")
	}
	if len(repo.saved) != 1 || repo.saved[0] != u {
		t.Errorf("saved = %v", repo.saved)
	}
	if len(enc.calls) != 1 || enc.calls[0] != "secret" {
		t.Errorf("encoder calls = %v", enc.calls)
	}
}

func TestCreateUserValidation(t *testing.T) {
	other := &core.User{ID: "other", Email: "taken@b.com", Username: "taken"}
	tests := []struct {
		name               string
		email, user, pass  string
		wantField, wantMsg string
	}{
		{"empty email", "", "alice", "pw", "email", "can't be empty"},
		{"blank email", "   ", "alice", "pw", "email", "can't be empty"},
		{"invalid email", "not-an-email", "alice", "pw", "email", "should be an email"},
		{"duplicated email", "taken@b.com", "alice", "pw", "email", "duplicated email"},
		{"empty username", "a@b.com", "", "pw", "username", "can't be empty"},
		{"duplicated username", "a@b.com", "taken", "pw", "username", "duplicated username"},
		{"empty password", "a@b.com", "alice", "", "password", "can't be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.byEmail[other.Email] = other
			repo.byUsername[other.Username] = other
			enc := &fakeEncoder{}
			s := newSvc(t, repo, enc)
			u, err := s.CreateUser(context.Background(), tt.email, tt.user, tt.pass)
			if err == nil || u != nil {
				t.Fatalf("expected error, got user %v", u)
			}
			fes := collectFieldErrors(err)
			if !hasFieldError(fes, tt.wantField, tt.wantMsg) {
				t.Errorf("field errors %v missing %s %s", fes, tt.wantField, tt.wantMsg)
			}
			if len(repo.saved) != 0 {
				t.Error("user saved despite validation failure")
			}
			if len(enc.calls) != 0 {
				t.Error("encoder called despite validation failure")
			}
		})
	}
}

func TestCreateUserMultipleViolations(t *testing.T) {
	repo := newFakeRepo()
	s := newSvc(t, repo, &fakeEncoder{})
	_, err := s.CreateUser(context.Background(), "", "", "")
	fes := collectFieldErrors(err)
	if len(fes) != 3 {
		t.Fatalf("got %d field errors: %v", len(fes), fes)
	}
	if len(repo.saved) != 0 {
		t.Error("saved")
	}
}

func TestCreateUserDependencyErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name  string
		setup func(*fakeUserRepo, *fakeEncoder)
	}{
		{"email lookup", func(r *fakeUserRepo, e *fakeEncoder) { r.findEmailErr = boom }},
		{"username lookup", func(r *fakeUserRepo, e *fakeEncoder) { r.findUserErr = boom }},
		{"encode", func(r *fakeUserRepo, e *fakeEncoder) { e.err = boom }},
		{"save", func(r *fakeUserRepo, e *fakeEncoder) { r.saveErr = boom }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			enc := &fakeEncoder{}
			tt.setup(repo, enc)
			s := newSvc(t, repo, enc)
			u, err := s.CreateUser(context.Background(), "a@b.com", "alice", "pw")
			if !errors.Is(err, boom) || u != nil {
				t.Fatalf("err = %v, user = %v", err, u)
			}
		})
	}
}

func TestUpdateUser(t *testing.T) {
	type want struct {
		ok     bool
		fields [][2]string
	}
	tests := []struct {
		name  string
		param *UpdateUserParam
		setup func(r *fakeUserRepo, target *core.User)
		want  want
	}{
		{
			name:  "unused email and username",
			param: paramPtr(NewUpdateUserParam("new@b.com", "newpw", "newname", "bio", "img")),
			want:  want{ok: true},
		},
		{
			name:  "owned by target",
			param: paramPtr(NewUpdateUserParam("t@b.com", "", "target", "bio", "")),
			setup: func(r *fakeUserRepo, tg *core.User) {
				r.byEmail[tg.Email] = &core.User{ID: tg.ID, Email: tg.Email}
				r.byUsername[tg.Username] = &core.User{ID: tg.ID, Username: tg.Username}
			},
			want: want{ok: true},
		},
		{
			name:  "email conflict",
			param: paramPtr(NewUpdateUserParam("x@b.com", "", "newname", "", "")),
			setup: func(r *fakeUserRepo, tg *core.User) { r.byEmail["x@b.com"] = &core.User{ID: "x"} },
			want:  want{fields: [][2]string{{"email", "email already exist"}}},
		},
		{
			name:  "username conflict",
			param: paramPtr(NewUpdateUserParam("new@b.com", "", "x", "", "")),
			setup: func(r *fakeUserRepo, tg *core.User) { r.byUsername["x"] = &core.User{ID: "x"} },
			want:  want{fields: [][2]string{{"username", "username already exist"}}},
		},
		{
			name:  "both conflict",
			param: paramPtr(NewUpdateUserParam("x@b.com", "", "y", "", "")),
			setup: func(r *fakeUserRepo, tg *core.User) {
				r.byEmail["x@b.com"] = &core.User{ID: "x"}
				r.byUsername["y"] = &core.User{ID: "y"}
			},
			want: want{fields: [][2]string{{"email", "email already exist"}, {"username", "username already exist"}}},
		},
		{
			name:  "invalid email format",
			param: paramPtr(NewUpdateUserParam("bad", "", "", "", "")),
			want:  want{fields: [][2]string{{"email", "should be an email"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			enc := &fakeEncoder{}
			s := newSvc(t, repo, enc)
			target := &core.User{ID: "t1", Email: "t@b.com", Username: "target", Password: "oldhash", Bio: "oldbio", Image: "oldimg"}
			orig := *target
			if tt.setup != nil {
				tt.setup(repo, target)
			}
			err := s.UpdateUser(context.Background(), NewUpdateUserCommand(target, tt.param))
			if tt.want.ok {
				if err != nil {
					t.Fatalf("UpdateUser: %v", err)
				}
				if len(repo.saved) != 1 || repo.saved[0] != target {
					t.Fatalf("saved = %v", repo.saved)
				}
				p := tt.param
				check := func(name, got, newV, old string) {
					w := old
					if newV != "" {
						w = newV
					}
					if got != w {
						t.Errorf("%s = %q, want %q", name, got, w)
					}
				}
				check("email", target.Email, p.Email(), orig.Email)
				check("username", target.Username, p.Username(), orig.Username)
				check("bio", target.Bio, p.Bio(), orig.Bio)
				check("image", target.Image, p.Image(), orig.Image)
				if p.Password() != "" {
					if target.Password != "hashed:"+p.Password() {
						t.Errorf("password = %q", target.Password)
					}
				} else if target.Password != orig.Password {
					t.Errorf("password changed to %q", target.Password)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			fes := collectFieldErrors(err)
			if len(fes) != len(tt.want.fields) {
				t.Errorf("field errors = %v, want %v", fes, tt.want.fields)
			}
			for _, f := range tt.want.fields {
				if !hasFieldError(fes, f[0], f[1]) {
					t.Errorf("missing %v in %v", f, fes)
				}
			}
			if len(repo.saved) != 0 {
				t.Error("saved despite failure")
			}
			if *target != orig {
				t.Errorf("target modified: %v", target)
			}
		})
	}
}

func TestUpdateUserNilCommand(t *testing.T) {
	u := &core.User{ID: "1"}
	p := paramPtr(NewUpdateUserParam("", "", "", "", ""))
	tests := []struct {
		name string
		cmd  *UpdateUserCommand
	}{
		{"nil cmd", nil},
		{"nil target", NewUpdateUserCommand(nil, p)},
		{"nil param", NewUpdateUserCommand(u, nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSvc(t, newFakeRepo(), &fakeEncoder{})
			if err := s.UpdateUser(context.Background(), tt.cmd); !errors.Is(err, ErrNilUpdateUserCommand) {
				t.Errorf("UpdateUser err = %v", err)
			}
			if err := s.ValidateUpdateUser(context.Background(), tt.cmd); !errors.Is(err, ErrNilUpdateUserCommand) {
				t.Errorf("ValidateUpdateUser err = %v", err)
			}
		})
	}
}

func TestUpdateUserDependencyErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name  string
		setup func(*fakeUserRepo, *fakeEncoder)
	}{
		{"email lookup", func(r *fakeUserRepo, e *fakeEncoder) { r.findEmailErr = boom }},
		{"username lookup", func(r *fakeUserRepo, e *fakeEncoder) { r.findUserErr = boom }},
		{"encode", func(r *fakeUserRepo, e *fakeEncoder) { e.err = boom }},
		{"save", func(r *fakeUserRepo, e *fakeEncoder) { r.saveErr = boom }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			enc := &fakeEncoder{}
			tt.setup(repo, enc)
			s := newSvc(t, repo, enc)
			cmd := NewUpdateUserCommand(&core.User{ID: "1"}, paramPtr(NewUpdateUserParam("a@b.com", "pw", "n", "", "")))
			if err := s.UpdateUser(context.Background(), cmd); !errors.Is(err, boom) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestValidateUpdateUserAlwaysQueriesBoth(t *testing.T) {
	repo := newFakeRepo()
	repo.byEmail["x@b.com"] = &core.User{ID: "x"}
	s := newSvc(t, repo, &fakeEncoder{})
	cmd := NewUpdateUserCommand(&core.User{ID: "1"}, paramPtr(NewUpdateUserParam("x@b.com", "", "name", "", "")))
	err := s.ValidateUpdateUser(context.Background(), cmd)
	if !hasFieldError(collectFieldErrors(err), "email", "email already exist") {
		t.Errorf("err = %v", err)
	}
	if len(repo.emailLookups) != 1 || repo.emailLookups[0] != "x@b.com" {
		t.Errorf("email lookups = %v", repo.emailLookups)
	}
	if len(repo.usernameLooks) != 1 || repo.usernameLooks[0] != "name" {
		t.Errorf("username lookups = %v", repo.usernameLooks)
	}
	if len(repo.saved) != 0 {
		t.Error("validate must not save")
	}
}

func TestUpdateUserParamValidate(t *testing.T) {
	tests := []struct {
		email string
		ok    bool
	}{
		{"", true}, {"a@b.com", true}, {"bad", false}, {"a@@b", false}, {"a b@c", false},
	}
	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			err := NewUpdateUserParam(tt.email, "", "", "", "").Validate()
			if (err == nil) != tt.ok {
				t.Errorf("Validate(%q) = %v", tt.email, err)
			}
			if err != nil && err.Error() != "email should be an email" {
				t.Errorf("message = %q", err.Error())
			}
		})
	}
}

func TestUpdateUserParamJSON(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
		missing bool
		want    *UpdateUserParam
	}{
		{"full", `{"user":{"email":"a@b.com","password":"p","username":"u","bio":"b","image":"i"}}`, false, false, paramPtr(NewUpdateUserParam("a@b.com", "p", "u", "b", "i"))},
		{"null body", `{"user":null}`, false, false, paramPtr(NewUpdateUserParam("", "", "", "", ""))},
		{"missing root", `{"email":"a@b.com"}`, true, true, nil},
		{"invalid json", `{`, true, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p UpdateUserParam
			err := json.Unmarshal([]byte(tt.in), &p)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.missing && !errors.Is(err, ErrMissingUserRoot) {
					t.Errorf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p != *tt.want {
				t.Errorf("got %+v want %+v", p, *tt.want)
			}
			b, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			var back UpdateUserParam
			if err := json.Unmarshal(b, &back); err != nil || back != p {
				t.Errorf("round trip: %v %+v", err, back)
			}
		})
	}
}

func TestUpdateUserParamBuilder(t *testing.T) {
	b := NewUpdateUserParamBuilder().Email("e@x.com").Password("p").Username("u").Bio("b").Image("i")
	p := b.Build()
	if *p != NewUpdateUserParam("e@x.com", "p", "u", "b", "i") {
		t.Errorf("got %+v", *p)
	}
	b.Email("other@x.com")
	if p.Email() != "e@x.com" {
		t.Error("Build must return a copy")
	}
	u := &core.User{ID: "1"}
	cmd := NewUpdateUserCommand(u, p)
	if cmd.TargetUser() != u || cmd.Param() != p {
		t.Error("command accessors")
	}
}