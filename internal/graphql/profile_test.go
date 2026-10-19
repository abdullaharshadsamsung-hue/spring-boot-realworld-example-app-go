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

type findCall struct {
	username string
	current  *core.User
}

type fakeFinder struct {
	profiles map[string]app.ProfileData
	follows  map[string]bool // key: username followed by current user
	err      error
	calls    []findCall
}

func (f *fakeFinder) FindByUsername(ctx context.Context, username string, current *core.User) (app.ProfileData, bool, error) {
	f.calls = append(f.calls, findCall{username, current})
	if f.err != nil {
		return app.ProfileData{}, false, f.err
	}
	p, ok := f.profiles[username]
	if !ok {
		return app.ProfileData{}, false, nil
	}
	if current != nil {
		p.Following = f.follows[username]
	}
	return p, true, nil
}

func newFinder() *fakeFinder {
	return &fakeFinder{
		profiles: map[string]app.ProfileData{
			"alice": app.NewProfileData("id-a", "alice", "bio a", "img a", false),
			"bob":   app.NewProfileData("id-b", "bob", "bio b", "img b", false),
		},
		follows: map[string]bool{"alice": true},
	}
}

var me = &core.User{ID: "me", Username: "me"}

func TestNewProfileDatafetcher(t *testing.T) {
	if _, err := NewProfileDatafetcher(nil); !errors.Is(err, ErrNilProfileFinder) {
		t.Fatalf("want ErrNilProfileFinder, got %v", err)
	}
	d, err := NewProfileDatafetcher(newFinder())
	if err != nil || d == nil {
		t.Fatalf("unexpected: %v %v", d, err)
	}
}

func TestQueryProfile(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name     string
		user     *core.User
		username string
		svcErr   error
		want     *Profile
		wantErr  error
	}{
		{"anonymous", nil, "alice", nil, &Profile{"alice", "bio a", "img a", false}, nil},
		{"auth following", me, "alice", nil, &Profile{"alice", "bio a", "img a", true}, nil},
		{"auth not following", me, "bob", nil, &Profile{"bob", "bio b", "img b", false}, nil},
		{"not found", me, "nobody", nil, nil, httpapi.ErrResourceNotFound},
		{"service error", nil, "alice", boom, nil, boom},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFinder()
			f.err = tc.svcErr
			d, _ := NewProfileDatafetcher(f)
			ctx := auth.WithUser(context.Background(), tc.user)
			got, err := d.QueryProfile(ctx, tc.username)
			if len(f.calls) != 1 || f.calls[0].username != tc.username || f.calls[0].current != tc.user {
				t.Fatalf("bad calls: %+v", f.calls)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || got != nil {
					t.Fatalf("want err %v, got %v %v", tc.wantErr, got, err)
				}
				return
			}
			if err != nil || got == nil || got.Profile == nil {
				t.Fatalf("unexpected: %v %v", got, err)
			}
			if *got.Profile != *tc.want {
				t.Fatalf("got %+v want %+v", *got.Profile, *tc.want)
			}
		})
	}
}

func TestUserProfile(t *testing.T) {
	tests := []struct {
		name    string
		user    *core.User
		ctxUser *core.User
		want    *Profile
		wantErr error
	}{
		{"found anonymous", &core.User{Username: "alice"}, nil, &Profile{"alice", "bio a", "img a", false}, nil},
		{"found following", &core.User{Username: "alice"}, me, &Profile{"alice", "bio a", "img a", true}, nil},
		{"not found", &core.User{Username: "ghost"}, nil, nil, httpapi.ErrResourceNotFound},
		{"nil user", nil, nil, nil, ErrMissingLocalContext},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFinder()
			d, _ := NewProfileDatafetcher(f)
			got, err := d.UserProfile(auth.WithUser(context.Background(), tc.ctxUser), tc.user)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || got != nil {
					t.Fatalf("want %v got %v %v", tc.wantErr, got, err)
				}
				return
			}
			if err != nil || *got != *tc.want {
				t.Fatalf("got %+v %v", got, err)
			}
			if f.calls[0].current != tc.ctxUser {
				t.Fatalf("current user not passed")
			}
		})
	}
}

func TestArticleAuthor(t *testing.T) {
	alice := app.NewProfileData("id-a", "alice", "", "", false)
	ghost := app.NewProfileData("id-g", "ghost", "", "", false)
	local := map[string]app.ArticleData{
		"s1":       {Slug: "s1", Author: &alice},
		"s-ghost":  {Slug: "s-ghost", Author: &ghost},
		"s-noauth": {Slug: "s-noauth"},
	}
	tests := []struct {
		name    string
		slug    string
		local   map[string]app.ArticleData
		ctxUser *core.User
		want    *Profile
		wantErr error
	}{
		{"found", "s1", local, me, &Profile{"alice", "bio a", "img a", true}, nil},
		{"author not found", "s-ghost", local, nil, nil, httpapi.ErrResourceNotFound},
		{"slug missing", "nope", local, nil, nil, ErrMissingLocalContext},
		{"nil author", "s-noauth", local, nil, nil, ErrMissingLocalContext},
		{"nil map", "s1", nil, nil, nil, ErrMissingLocalContext},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFinder()
			d, _ := NewProfileDatafetcher(f)
			got, err := d.ArticleAuthor(auth.WithUser(context.Background(), tc.ctxUser), tc.slug, tc.local)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || got != nil {
					t.Fatalf("want %v got %v %v", tc.wantErr, got, err)
				}
				return
			}
			if err != nil || *got != *tc.want {
				t.Fatalf("got %+v %v", got, err)
			}
		})
	}
}

func TestCommentAuthor(t *testing.T) {
	m := map[string]string{"c1": "bob", "c-ghost": "ghost"}
	lookup := AuthorLookup(func(k string) (string, bool) { u, ok := m[k]; return u, ok })
	tests := []struct {
		name    string
		id      string
		lookup  AuthorLookup
		ctxUser *core.User
		want    *Profile
		wantErr error
	}{
		{"found", "c1", lookup, me, &Profile{"bob", "bio b", "img b", false}, nil},
		{"found anonymous", "c1", lookup, nil, &Profile{"bob", "bio b", "img b", false}, nil},
		{"author not found", "c-ghost", lookup, nil, nil, httpapi.ErrResourceNotFound},
		{"id missing", "zzz", lookup, nil, nil, ErrMissingLocalContext},
		{"nil lookup", "c1", nil, nil, nil, ErrMissingLocalContext},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFinder()
			d, _ := NewProfileDatafetcher(f)
			got, err := d.CommentAuthor(auth.WithUser(context.Background(), tc.ctxUser), tc.id, tc.lookup)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || got != nil {
					t.Fatalf("want %v got %v %v", tc.wantErr, got, err)
				}
				return
			}
			if err != nil || *got != *tc.want {
				t.Fatalf("got %+v %v", got, err)
			}
			if f.calls[0].current != tc.ctxUser {
				t.Fatalf("current user not passed")
			}
		})
	}
}