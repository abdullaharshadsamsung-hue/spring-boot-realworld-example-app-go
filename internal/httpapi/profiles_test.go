package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
)

type fakeProfileQuerier struct {
	profiles map[string]app.ProfileData
	follows  map[[2]string]bool
	err      error
	viewers  []*core.User
}

func (f *fakeProfileQuerier) FindByUsername(ctx context.Context, username string, current *core.User) (app.ProfileData, bool, error) {
	f.viewers = append(f.viewers, current)
	if f.err != nil {
		return app.ProfileData{}, false, f.err
	}
	p, ok := f.profiles[username]
	if !ok {
		return app.ProfileData{}, false, nil
	}
	if current != nil {
		p.Following = f.follows[[2]string{current.ID, p.ID}]
	}
	return p, true, nil
}

type fakeRepo struct {
	users     map[string]*core.User
	relations map[[2]string]bool
	findErr   error
	saveErr   error
	saved     []*core.FollowRelation
	removed   []*core.FollowRelation
}

func (f *fakeRepo) FindByUsername(ctx context.Context, username string) (*core.User, error) {
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
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, r)
	f.relations[[2]string{r.UserID, r.TargetID}] = true
	return nil
}

func (f *fakeRepo) FindRelation(ctx context.Context, userID, targetID string) (*core.FollowRelation, error) {
	if !f.relations[[2]string{userID, targetID}] {
		return nil, core.ErrNotFound
	}
	return core.NewFollowRelation(userID, targetID), nil
}

func (f *fakeRepo) RemoveRelation(ctx context.Context, r *core.FollowRelation) error {
	f.removed = append(f.removed, r)
	delete(f.relations, [2]string{r.UserID, r.TargetID})
	return nil
}

func TestNewProfilesHandler(t *testing.T) {
	tests := []struct {
		name string
		q    ProfileQuerier
		r    FollowRepository
		want error
	}{
		{"nil querier", nil, &fakeRepo{}, ErrNilProfileQuerier},
		{"nil repo", &fakeProfileQuerier{}, nil, ErrNilFollowRepository},
		{"ok", &fakeProfileQuerier{}, &fakeRepo{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewProfilesHandler(tt.q, tt.r)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want %v", err, tt.want)
			}
			if tt.want == nil && h == nil {
				t.Fatal("nil handler")
			}
		})
	}
}

func TestProfilesHandler(t *testing.T) {
	viewer := &core.User{ID: "viewer-id", Username: "viewer"}
	target := &core.User{ID: "target-id", Username: "username"}

	type tc struct {
		name          string
		method        string
		path          string
		user          *core.User
		following     bool
		queryErr      error
		findErr       error
		saveErr       error
		wantStatus    int
		wantFollowing *bool
		wantSaved     int
		wantRemoved   int
	}
	tr, fa := true, false
	tests := []tc{
		{name: "get anonymous", method: "GET", path: "/profiles/username", wantStatus: 200, wantFollowing: &fa},
		{name: "get auth following", method: "GET", path: "/profiles/username", user: viewer, following: true, wantStatus: 200, wantFollowing: &tr},
		{name: "get auth not following", method: "GET", path: "/profiles/username", user: viewer, wantStatus: 200, wantFollowing: &fa},
		{name: "get missing", method: "GET", path: "/profiles/nobody", wantStatus: 404},
		{name: "get query error", method: "GET", path: "/profiles/username", queryErr: errors.New("db"), wantStatus: 500},
		{name: "follow ok", method: "POST", path: "/profiles/username/follow", user: viewer, wantStatus: 200, wantFollowing: &tr, wantSaved: 1},
		{name: "follow missing", method: "POST", path: "/profiles/nobody/follow", user: viewer, wantStatus: 404},
		{name: "follow unauth", method: "POST", path: "/profiles/username/follow", wantStatus: 401},
		{name: "follow repo error", method: "POST", path: "/profiles/username/follow", user: viewer, findErr: errors.New("db"), wantStatus: 500},
		{name: "follow save error", method: "POST", path: "/profiles/username/follow", user: viewer, saveErr: errors.New("db"), wantStatus: 500},
		{name: "unfollow ok", method: "DELETE", path: "/profiles/username/follow", user: viewer, following: true, wantStatus: 200, wantFollowing: &fa, wantRemoved: 1},
		{name: "unfollow no relation", method: "DELETE", path: "/profiles/username/follow", user: viewer, wantStatus: 404},
		{name: "unfollow missing target", method: "DELETE", path: "/profiles/nobody/follow", user: viewer, wantStatus: 404},
		{name: "unfollow unauth", method: "DELETE", path: "/profiles/username/follow", following: true, wantStatus: 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rels := map[[2]string]bool{}
			if tt.following {
				rels[[2]string{viewer.ID, target.ID}] = true
			}
			repo := &fakeRepo{users: map[string]*core.User{"username": target}, relations: rels, findErr: tt.findErr, saveErr: tt.saveErr}
			q := &fakeProfileQuerier{
				profiles: map[string]app.ProfileData{"username": app.NewProfileData(target.ID, "username", "", "", false)},
				follows:  rels,
				err:      tt.queryErr,
			}
			h, err := NewProfilesHandler(q, repo)
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			h.Register(mux)
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.user != nil {
				req = req.WithContext(auth.WithUser(req.Context(), tt.user))
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status=%d want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if len(repo.saved) != tt.wantSaved {
				t.Fatalf("saved=%d want %d", len(repo.saved), tt.wantSaved)
			}
			if tt.wantSaved > 0 && !repo.saved[0].Equal(core.NewFollowRelation(viewer.ID, target.ID)) {
				t.Fatalf("saved wrong relation %v", repo.saved[0])
			}
			if len(repo.removed) != tt.wantRemoved {
				t.Fatalf("removed=%d want %d", len(repo.removed), tt.wantRemoved)
			}
			if tt.wantRemoved > 0 && !repo.removed[0].Equal(core.NewFollowRelation(viewer.ID, target.ID)) {
				t.Fatalf("removed wrong relation %v", repo.removed[0])
			}
			if tt.wantStatus == 401 && rec.Body.Len() != 0 {
				t.Fatalf("expected empty 401 body, got %q", rec.Body.String())
			}
			if tt.wantStatus != 200 {
				return
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 1 {
				t.Fatalf("expected single top-level key, got %v", body)
			}
			var p map[string]any
			if err := json.Unmarshal(body["profile"], &p); err != nil {
				t.Fatal(err)
			}
			for _, k := range []string{"username", "bio", "image", "following"} {
				if _, ok := p[k]; !ok {
					t.Fatalf("missing key %q in %v", k, p)
				}
			}
			if _, ok := p["id"]; ok {
				t.Fatal("id must not be serialized")
			}
			if p["username"] != "username" {
				t.Fatalf("username=%v", p["username"])
			}
			if tt.wantFollowing != nil && p["following"] != *tt.wantFollowing {
				t.Fatalf("following=%v want %v", p["following"], *tt.wantFollowing)
			}
			if tt.user != nil {
				last := q.viewers[len(q.viewers)-1]
				if last == nil || last.ID != tt.user.ID {
					t.Fatalf("viewer not passed to querier")
				}
			}
		})
	}
}

func TestGetProfileIsReadOnly(t *testing.T) {
	repo := &fakeRepo{users: map[string]*core.User{}, relations: map[[2]string]bool{}}
	q := &fakeProfileQuerier{profiles: map[string]app.ProfileData{"u": {ID: "1", Username: "u"}}}
	h, _ := NewProfilesHandler(q, repo)
	mux := http.NewServeMux()
	h.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/profiles/u", nil))
	if rec.Code != 200 || len(repo.saved) != 0 || len(repo.removed) != 0 {
		t.Fatalf("unexpected side effects or status %d", rec.Code)
	}
}

func TestNotFoundOr(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want404 bool
	}{
		{"not found", core.ErrNotFound, true},
		{"other", errors.New("x"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := notFoundOr(tt.err, "op")
			if errors.Is(got, ErrResourceNotFound) != tt.want404 {
				t.Fatalf("got %v", got)
			}
			if !tt.want404 && !errors.Is(got, tt.err) {
				t.Fatal("not wrapped")
			}
		})
	}
}