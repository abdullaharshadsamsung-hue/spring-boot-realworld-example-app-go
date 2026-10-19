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

type fakeRepo struct {
	users     map[string]*core.User
	relations map[string]*core.FollowRelation
	findErr   error
	saveErr   error
	relErr    error
	removeErr error
	calls     []string
	saved     []*core.FollowRelation
	removed   []*core.FollowRelation
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{users: map[string]*core.User{}, relations: map[string]*core.FollowRelation{}}
}

func key(a, b string) string { return a + "->" + b }

func (f *fakeRepo) FindByUsername(ctx context.Context, username string) (*core.User, error) {
	f.calls = append(f.calls, "FindByUsername")
	if f.findErr != nil {
		return nil, f.findErr
	}
	u, ok := f.users[username]
	if !ok {
		return nil, core.ErrNotFound
	}
	return u, nil
}

func (f *fakeRepo) SaveRelation(ctx context.Context, r *core.FollowRelation) error {
	f.calls = append(f.calls, "SaveRelation")
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, r)
	f.relations[key(r.UserID, r.TargetID)] = r
	return nil
}

func (f *fakeRepo) FindRelation(ctx context.Context, userID, targetID string) (*core.FollowRelation, error) {
	f.calls = append(f.calls, "FindRelation")
	if f.relErr != nil {
		return nil, f.relErr
	}
	r, ok := f.relations[key(userID, targetID)]
	if !ok {
		return nil, core.ErrNotFound
	}
	return r, nil
}

func (f *fakeRepo) RemoveRelation(ctx context.Context, r *core.FollowRelation) error {
	f.calls = append(f.calls, "RemoveRelation")
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, r)
	delete(f.relations, key(r.UserID, r.TargetID))
	return nil
}

// fakeProfiles computes following from repo state at call time.
type fakeProfiles struct {
	repo     *fakeRepo
	err      error
	notFound bool
	current  *core.User
	called   int
}

func (p *fakeProfiles) FindByUsername(ctx context.Context, username string, current *core.User) (app.ProfileData, bool, error) {
	p.called++
	p.current = current
	if p.err != nil {
		return app.ProfileData{}, false, p.err
	}
	u, ok := p.repo.users[username]
	if !ok || p.notFound {
		return app.ProfileData{}, false, nil
	}
	following := false
	if current != nil {
		_, following = p.repo.relations[key(current.ID, u.ID)]
	}
	return app.NewProfileData(u.ID, u.Username, u.Bio, u.Image, following), true, nil
}

var (
	relMe     = &core.User{ID: "u1", Username: "me", Email: "me@x"}
	relTarget = &core.User{ID: "u2", Username: "bob", Bio: "bio", Image: "img"}
)

func setup() (*fakeRepo, *fakeProfiles, *RelationMutation) {
	r := newFakeRepo()
	tCopy := *relTarget
	r.users["bob"] = &tCopy
	r.users["me"] = relMe
	p := &fakeProfiles{repo: r}
	m, err := NewRelationMutation(r, p)
	if err != nil {
		panic(err)
	}
	return r, p, m
}

func TestNewRelationMutation(t *testing.T) {
	r := newFakeRepo()
	p := &fakeProfiles{repo: r}
	tests := []struct {
		name    string
		repo    RelationRepository
		prof    ProfileFinder
		wantErr error
	}{
		{"nil repo", nil, p, ErrNilRelationRepository},
		{"nil profiles", r, nil, ErrNilProfileFinder},
		{"ok", r, p, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewRelationMutation(tc.repo, tc.prof)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && m == nil {
				t.Fatal("nil mutation")
			}
			if tc.wantErr != nil && m != nil {
				t.Fatal("expected nil mutation")
			}
		})
	}
}

func TestFollowUser(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name        string
		auth        bool
		username    string
		prep        func(*fakeRepo, *fakeProfiles)
		wantErr     error
		wantCalls   []string
		wantSaved   int
		wantProfile *Profile
	}{
		{"follows new", true, "bob", nil, nil, []string{"FindByUsername", "SaveRelation"}, 1,
			&Profile{Username: "bob", Bio: "bio", Image: "img", Following: true}},
		{"already following", true, "bob", func(r *fakeRepo, _ *fakeProfiles) {
			r.relations[key("u1", "u2")] = core.NewFollowRelation("u1", "u2")
		}, nil, []string{"FindByUsername", "SaveRelation"}, 1,
			&Profile{Username: "bob", Bio: "bio", Image: "img", Following: true}},
		{"unauthenticated", false, "bob", nil, core.ErrUnauthenticated, nil, 0, nil},
		{"target missing", true, "ghost", nil, httpapi.ErrResourceNotFound, []string{"FindByUsername"}, 0, nil},
		{"find error", true, "bob", func(r *fakeRepo, _ *fakeProfiles) { r.findErr = boom }, boom, []string{"FindByUsername"}, 0, nil},
		{"save error", true, "bob", func(r *fakeRepo, _ *fakeProfiles) { r.saveErr = boom }, boom, []string{"FindByUsername", "SaveRelation"}, 0, nil},
		{"profile error", true, "bob", func(_ *fakeRepo, p *fakeProfiles) { p.err = boom }, boom, []string{"FindByUsername", "SaveRelation"}, 1, nil},
		{"profile vanished", true, "bob", func(_ *fakeRepo, p *fakeProfiles) { p.notFound = true }, httpapi.ErrResourceNotFound, []string{"FindByUsername", "SaveRelation"}, 1, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, p, m := setup()
			if tc.prep != nil {
				tc.prep(r, p)
			}
			ctx := context.Background()
			if tc.auth {
				ctx = auth.WithUser(ctx, relMe)
			}
			got, err := m.FollowUser(ctx, tc.username)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v want %v", err, tc.wantErr)
			}
			if !equalCalls(r.calls, tc.wantCalls) {
				t.Fatalf("calls=%v want %v", r.calls, tc.wantCalls)
			}
			if len(r.saved) != tc.wantSaved {
				t.Fatalf("saved=%d want %d", len(r.saved), tc.wantSaved)
			}
			for _, s := range r.saved {
				if s.UserID != "u1" || s.TargetID != "u2" {
					t.Fatalf("bad relation %v", s)
				}
			}
			if tc.wantProfile == nil {
				if got != nil {
					t.Fatalf("expected nil payload, got %+v", got)
				}
				return
			}
			if got == nil || got.Profile == nil || *got.Profile != *tc.wantProfile {
				t.Fatalf("got %+v want %+v", got, tc.wantProfile)
			}
			if p.current != relMe {
				t.Fatal("profile not computed for current user")
			}
			if *r.users["bob"] != *relTarget {
				t.Fatal("target modified")
			}
		})
	}
}

func TestUnfollowUser(t *testing.T) {
	boom := errors.New("boom")
	follow := func(r *fakeRepo, _ *fakeProfiles) {
		r.relations[key("u1", "u2")] = core.NewFollowRelation("u1", "u2")
	}
	tests := []struct {
		name        string
		auth        bool
		username    string
		prep        func(*fakeRepo, *fakeProfiles)
		wantErr     error
		wantCalls   []string
		wantRemoved int
		wantProfile *Profile
	}{
		{"unfollows", true, "bob", follow, nil, []string{"FindByUsername", "FindRelation", "RemoveRelation"}, 1,
			&Profile{Username: "bob", Bio: "bio", Image: "img", Following: false}},
		{"not following", true, "bob", nil, httpapi.ErrResourceNotFound, []string{"FindByUsername", "FindRelation"}, 0, nil},
		{"other user's relation not removable", true, "bob", func(r *fakeRepo, _ *fakeProfiles) {
			r.relations[key("u3", "u2")] = core.NewFollowRelation("u3", "u2")
		}, httpapi.ErrResourceNotFound, []string{"FindByUsername", "FindRelation"}, 0, nil},
		{"unauthenticated", false, "bob", follow, core.ErrUnauthenticated, nil, 0, nil},
		{"target missing", true, "ghost", nil, httpapi.ErrResourceNotFound, []string{"FindByUsername"}, 0, nil},
		{"find relation error", true, "bob", func(r *fakeRepo, _ *fakeProfiles) { r.relErr = boom }, boom, []string{"FindByUsername", "FindRelation"}, 0, nil},
		{"remove error", true, "bob", func(r *fakeRepo, p *fakeProfiles) { follow(r, p); r.removeErr = boom }, boom, []string{"FindByUsername", "FindRelation", "RemoveRelation"}, 0, nil},
		{"profile vanished", true, "bob", func(r *fakeRepo, p *fakeProfiles) { follow(r, p); p.notFound = true }, httpapi.ErrResourceNotFound, []string{"FindByUsername", "FindRelation", "RemoveRelation"}, 1, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, p, m := setup()
			if tc.prep != nil {
				tc.prep(r, p)
			}
			ctx := context.Background()
			if tc.auth {
				ctx = auth.WithUser(ctx, relMe)
			}
			got, err := m.UnfollowUser(ctx, tc.username)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v want %v", err, tc.wantErr)
			}
			if !equalCalls(r.calls, tc.wantCalls) {
				t.Fatalf("calls=%v want %v", r.calls, tc.wantCalls)
			}
			if len(r.removed) != tc.wantRemoved {
				t.Fatalf("removed=%d want %d", len(r.removed), tc.wantRemoved)
			}
			for _, rm := range r.removed {
				if rm.UserID != "u1" || rm.TargetID != "u2" {
					t.Fatalf("bad relation %v", rm)
				}
			}
			if tc.wantProfile == nil {
				if got != nil {
					t.Fatalf("expected nil payload")
				}
				return
			}
			if got == nil || got.Profile == nil || *got.Profile != *tc.wantProfile {
				t.Fatalf("got %+v want %+v", got, tc.wantProfile)
			}
			if *r.users["bob"] != *relTarget {
				t.Fatal("target modified")
			}
		})
	}
}

func TestNotFoundOrWrap(t *testing.T) {
	boom := errors.New("boom")
	if err := notFoundOrWrap(core.ErrNotFound, "op"); err != httpapi.ErrResourceNotFound {
		t.Fatalf("got %v", err)
	}
	err := notFoundOrWrap(boom, "op")
	if !errors.Is(err, boom) || errors.Is(err, httpapi.ErrResourceNotFound) {
		t.Fatalf("got %v", err)
	}
}

func equalCalls(a, b []string) bool {
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