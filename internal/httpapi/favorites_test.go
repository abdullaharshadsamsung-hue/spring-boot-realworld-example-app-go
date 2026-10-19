package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
)

type fakeFavRepo struct {
	saved    []core.ArticleFavorite
	removed  []core.ArticleFavorite
	existing map[core.ArticleFavorite]bool
	saveErr  error
	findErr  error
	calls    []string
}

func (f *fakeFavRepo) Save(_ context.Context, fav core.ArticleFavorite) error {
	f.calls = append(f.calls, "save")
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, fav)
	return nil
}

func (f *fakeFavRepo) Find(_ context.Context, articleID, userID string) (core.ArticleFavorite, bool, error) {
	if f.findErr != nil {
		return core.ArticleFavorite{}, false, f.findErr
	}
	k := core.NewArticleFavorite(articleID, userID)
	if f.existing[k] {
		return k, true, nil
	}
	return core.ArticleFavorite{}, false, nil
}

func (f *fakeFavRepo) Remove(_ context.Context, fav core.ArticleFavorite) error {
	f.removed = append(f.removed, fav)
	return nil
}

type fakeFinder struct {
	articles map[string]*core.Article
}

func (f *fakeFinder) FindBySlug(_ context.Context, slug string) (*core.Article, error) {
	if a, ok := f.articles[slug]; ok {
		return a, nil
	}
	return nil, core.ErrNotFound
}

type fakeQuerier struct {
	data    map[string]app.ArticleData
	viewer  *core.User
	repo    *fakeFavRepo
	missing bool
}

func (q *fakeQuerier) FindBySlug(_ context.Context, slug string, user *core.User) (app.ArticleData, bool, error) {
	q.viewer = user
	if q.repo != nil {
		q.repo.calls = append(q.repo.calls, "query")
	}
	if q.missing {
		return app.ArticleData{}, false, nil
	}
	d, ok := q.data[slug]
	return d, ok, nil
}

func TestNewFavoritesHandlerNilDeps(t *testing.T) {
	repo, finder, q := &fakeFavRepo{}, &fakeFinder{}, &fakeQuerier{}
	tests := []struct {
		name string
		fn   func() (*FavoritesHandler, error)
		want error
	}{
		{"nil repo", func() (*FavoritesHandler, error) { return NewFavoritesHandler(nil, finder, q) }, ErrNilFavoriteRepository},
		{"nil finder", func() (*FavoritesHandler, error) { return NewFavoritesHandler(repo, nil, q) }, ErrNilFavoriteArticleFinder},
		{"nil querier", func() (*FavoritesHandler, error) { return NewFavoritesHandler(repo, finder, nil) }, ErrNilFavoriteArticleQuerier},
		{"ok", func() (*FavoritesHandler, error) { return NewFavoritesHandler(repo, finder, q) }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := tt.fn()
			if !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want %v", err, tt.want)
			}
			if tt.want == nil && h == nil {
				t.Fatal("nil handler")
			}
		})
	}
}

func TestFavoritesEndpoints(t *testing.T) {
	user := &core.User{ID: "u1", Username: "me"}
	article := &core.Article{ID: "a1", UserID: "other", Slug: "title", Title: "title"}
	now := time.Now()

	tests := []struct {
		name        string
		method      string
		slug        string
		authed      bool
		existingFav bool
		missingRead bool
		wantStatus  int
		wantSaved   int
		wantRemoved int
	}{
		{"favorite ok", http.MethodPost, "title", true, false, false, http.StatusOK, 1, 0},
		{"favorite not found", http.MethodPost, "nope", true, false, false, http.StatusNotFound, 0, 0},
		{"favorite unauthenticated", http.MethodPost, "title", false, false, false, http.StatusUnauthorized, 0, 0},
		{"favorite vanished after save", http.MethodPost, "title", true, false, true, http.StatusInternalServerError, 1, 0},
		{"unfavorite existing", http.MethodDelete, "title", true, true, false, http.StatusOK, 0, 1},
		{"unfavorite none", http.MethodDelete, "title", true, false, false, http.StatusOK, 0, 0},
		{"unfavorite not found", http.MethodDelete, "nope", true, true, false, http.StatusNotFound, 0, 0},
		{"unfavorite unauthenticated", http.MethodDelete, "title", false, true, false, http.StatusUnauthorized, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeFavRepo{existing: map[core.ArticleFavorite]bool{}}
			if tt.existingFav {
				repo.existing[core.NewArticleFavorite("a1", "u1")] = true
			}
			favorited := tt.method == http.MethodPost
			count := 0
			if favorited {
				count = 1
			}
			q := &fakeQuerier{repo: repo, missing: tt.missingRead, data: map[string]app.ArticleData{
				"title": app.NewArticleData("a1", "title", "title", "d", "b", favorited, count, now, now, nil,
					&app.ProfileData{ID: "other", Username: "other"}),
			}}
			h, err := NewFavoritesHandler(repo, &fakeFinder{articles: map[string]*core.Article{"title": article}}, q)
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			h.Register(mux)

			req := httptest.NewRequest(tt.method, "/articles/"+tt.slug+"/favorite", nil)
			if tt.authed {
				req = req.WithContext(auth.WithUser(req.Context(), user))
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if tt.wantStatus == http.StatusUnauthorized {
				if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
					t.Fatalf("status=%d want 401/403", rec.Code)
				}
			} else if rec.Code != tt.wantStatus {
				t.Fatalf("status=%d want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if len(repo.saved) != tt.wantSaved {
				t.Fatalf("saved=%d want %d", len(repo.saved), tt.wantSaved)
			}
			if tt.wantSaved == 1 && repo.saved[0] != core.NewArticleFavorite("a1", "u1") {
				t.Fatalf("saved=%+v", repo.saved[0])
			}
			if len(repo.removed) != tt.wantRemoved {
				t.Fatalf("removed=%d want %d", len(repo.removed), tt.wantRemoved)
			}
			if tt.wantRemoved == 1 && repo.removed[0] != core.NewArticleFavorite("a1", "u1") {
				t.Fatalf("removed=%+v", repo.removed[0])
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			if tt.method == http.MethodPost && (len(repo.calls) < 2 || repo.calls[0] != "save" || repo.calls[1] != "query") {
				t.Fatalf("order=%v", repo.calls)
			}
			if q.viewer == nil || q.viewer.ID != "u1" {
				t.Fatalf("viewer=%v", q.viewer)
			}
			var body struct {
				Article struct {
					ID             string `json:"id"`
					Favorited      bool   `json:"favorited"`
					FavoritesCount int    `json:"favoritesCount"`
				} `json:"article"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Article.ID != "a1" || body.Article.Favorited != favorited || body.Article.FavoritesCount != count {
				t.Fatalf("body=%+v", body)
			}
		})
	}
}

func TestFavoriteRepoErrors(t *testing.T) {
	user := &core.User{ID: "u1"}
	finder := &fakeFinder{articles: map[string]*core.Article{"s": {ID: "a1", Slug: "s"}}}
	tests := []struct {
		name   string
		method string
		repo   *fakeFavRepo
	}{
		{"save error", http.MethodPost, &fakeFavRepo{saveErr: errors.New("db")}},
		{"find error", http.MethodDelete, &fakeFavRepo{findErr: errors.New("db")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := NewFavoritesHandler(tt.repo, finder, &fakeQuerier{})
			mux := http.NewServeMux()
			h.Register(mux)
			req := httptest.NewRequest(tt.method, "/articles/s/favorite", nil)
			req = req.WithContext(auth.WithUser(req.Context(), user))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d", rec.Code)
			}
		})
	}
}