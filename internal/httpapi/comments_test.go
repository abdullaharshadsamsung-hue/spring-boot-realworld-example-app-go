package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
)

// ---- mocks ----

type fakeArticles struct {
	articles map[string]*core.Article
	calls    []string
}

func (f *fakeArticles) FindBySlug(ctx context.Context, slug string) (*core.Article, error) {
	f.calls = append(f.calls, slug)
	if a, ok := f.articles[slug]; ok {
		return a, nil
	}
	return nil, core.ErrNotFound
}

type findCall struct{ articleID, id string }

type fakeComments struct {
	saved     []*core.Comment
	removed   []*core.Comment
	byKey     map[findCall]*core.Comment
	findCalls []findCall
}

func (f *fakeComments) Save(ctx context.Context, c *core.Comment) error {
	f.saved = append(f.saved, c)
	return nil
}

func (f *fakeComments) FindByID(ctx context.Context, articleID, id string) (*core.Comment, error) {
	k := findCall{articleID, id}
	f.findCalls = append(f.findCalls, k)
	if c, ok := f.byKey[k]; ok {
		return c, nil
	}
	return nil, core.ErrNotFound
}

func (f *fakeComments) Remove(ctx context.Context, c *core.Comment) error {
	f.removed = append(f.removed, c)
	return nil
}

type fakeQueries struct {
	byID        func(id string) (app.CommentData, bool)
	byArticle   map[string][]app.CommentData
	gotIDUser   *core.User
	gotID       string
	gotListUser *core.User
	listCalled  bool
}

func (f *fakeQueries) FindByID(ctx context.Context, id string, user *core.User) (app.CommentData, bool, error) {
	f.gotID = id
	f.gotIDUser = user
	if f.byID == nil {
		return app.CommentData{}, false, nil
	}
	d, ok := f.byID(id)
	return d, ok, nil
}

func (f *fakeQueries) FindByArticleID(ctx context.Context, articleID string, user *core.User) ([]app.CommentData, error) {
	f.listCalled = true
	f.gotListUser = user
	return f.byArticle[articleID], nil
}

// ---- helpers ----

func withUser(r *http.Request, u *core.User) *http.Request {
	if u == nil {
		return r
	}
	return r.WithContext(auth.WithUser(r.Context(), u))
}

func newTestMux(t *testing.T, a *fakeArticles, c *fakeComments, q *fakeQueries) *http.ServeMux {
	t.Helper()
	h, err := NewCommentsHandler(a, c, q)
	if err != nil {
		t.Fatalf("NewCommentsHandler: %v", err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

var (
	author  = &core.User{ID: "u1", Username: "john", Email: "john@x.com"}
	other   = &core.User{ID: "u2", Username: "other", Email: "o@x.com"}
	article = &core.Article{ID: "a1", UserID: "u1", Slug: "title", Title: "title"}
)

// ---- tests ----

func TestNewCommentsHandlerNilDeps(t *testing.T) {
	a, c, q := &fakeArticles{}, &fakeComments{}, &fakeQueries{}
	tests := []struct {
		name string
		a    CommentArticleFinder
		c    CommentStore
		q    CommentQuerier
		want error
	}{
		{"nil articles", nil, c, q, ErrNilCommentArticleFinder},
		{"nil comments", a, nil, q, ErrNilCommentStore},
		{"nil queries", a, c, nil, ErrNilCommentQuerier},
		{"ok", a, c, q, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewCommentsHandler(tt.a, tt.c, tt.q)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if tt.want == nil && h == nil {
				t.Fatal("nil handler")
			}
		})
	}
}

func TestCreateComment(t *testing.T) {
	tests := []struct {
		name       string
		slug       string
		user       *core.User
		body       string
		wantStatus int
		wantSaved  bool
		check      func(t *testing.T, body []byte)
	}{
		{
			name: "success", slug: "title", user: author,
			body:       `{"comment":{"body":"comment content"}}`,
			wantStatus: http.StatusCreated, wantSaved: true,
			check: func(t *testing.T, b []byte) {
				var resp struct {
					Comment struct {
						ID   string `json:"id"`
						Body string `json:"body"`
					} `json:"comment"`
				}
				if err := json.Unmarshal(b, &resp); err != nil {
					t.Fatalf("decode: %v (%s)", err, b)
				}
				if resp.Comment.Body != "comment" {
					t.Errorf("comment.body = %q, want %q", resp.Comment.Body, "comment")
				}
				if resp.Comment.ID == "" {
					t.Error("missing comment.id")
				}
			},
		},
		{
			name: "empty body", slug: "title", user: author,
			body:       `{"comment":{"body":""}}`,
			wantStatus: http.StatusUnprocessableEntity,
			check: func(t *testing.T, b []byte) {
				var resp struct {
					Errors map[string][]string `json:"errors"`
				}
				if err := json.Unmarshal(b, &resp); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if got := resp.Errors["body"]; len(got) == 0 || got[0] != "can't be empty" {
					t.Errorf("errors.body = %v", got)
				}
			},
		},
		{
			name: "missing body field", slug: "title", user: author,
			body:       `{"comment":{}}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "blank body on unknown article still 422", slug: "nope", user: author,
			body:       `{"comment":{"body":"   "}}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "article not found", slug: "nope", user: author,
			body:       `{"comment":{"body":"hi"}}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "unauthenticated", slug: "title", user: nil,
			body:       `{"comment":{"body":"hi"}}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "malformed json", slug: "title", user: author,
			body:       `{bad`,
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &fakeArticles{articles: map[string]*core.Article{"title": article}}
			c := &fakeComments{}
			q := &fakeQueries{}
			q.byID = func(id string) (app.CommentData, bool) {
				now := time.Now()
				return app.NewCommentData(id, "comment", article.ID, now, now,
					&app.ProfileData{ID: author.ID}), true
			}
			mux := newTestMux(t, a, c, q)

			req := httptest.NewRequest(http.MethodPost, "/articles/"+tt.slug+"/comments", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = withUser(req, tt.user)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantSaved {
				if len(c.saved) != 1 {
					t.Fatalf("saved %d comments, want 1", len(c.saved))
				}
				s := c.saved[0]
				if s.Body() != "comment content" || s.UserID() != author.ID || s.ArticleID() != article.ID {
					t.Errorf("saved comment = %+v", s)
				}
				if q.gotID != s.ID() || q.gotIDUser != author {
					t.Errorf("query FindByID called with id=%q user=%v", q.gotID, q.gotIDUser)
				}
			} else if len(c.saved) != 0 {
				t.Errorf("unexpected save: %d", len(c.saved))
			}
			if tt.check != nil {
				tt.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestGetComments(t *testing.T) {
	now := time.Now()
	cd := app.NewCommentData("c1", "hello", article.ID, now, now, &app.ProfileData{ID: author.ID})
	tests := []struct {
		name       string
		slug       string
		user       *core.User
		list       []app.CommentData
		wantStatus int
		wantIDs    []string
	}{
		{"anonymous one comment", "title", nil, []app.CommentData{cd}, http.StatusOK, []string{"c1"}},
		{"authenticated", "title", other, []app.CommentData{cd}, http.StatusOK, []string{"c1"}},
		{"nil list renders empty array", "title", nil, nil, http.StatusOK, []string{}},
		{"slug not found", "nope", nil, nil, http.StatusNotFound, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &fakeArticles{articles: map[string]*core.Article{"title": article}}
			c := &fakeComments{}
			q := &fakeQueries{byArticle: map[string][]app.CommentData{article.ID: tt.list}}
			mux := newTestMux(t, a, c, q)

			req := withUser(httptest.NewRequest(http.MethodGet, "/articles/"+tt.slug+"/comments", nil), tt.user)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				if q.listCalled {
					t.Error("query should not be called")
				}
				return
			}
			if q.gotListUser != tt.user {
				t.Errorf("user passed = %v, want %v", q.gotListUser, tt.user)
			}
			var resp struct {
				Comments []struct {
					ID string `json:"id"`
				} `json:"comments"`
			}
			raw := map[string]json.RawMessage{}
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if string(raw["comments"]) == "null" || raw["comments"] == nil {
				t.Fatalf("comments must be array, got %s", rec.Body.String())
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &resp)
			if len(resp.Comments) != len(tt.wantIDs) {
				t.Fatalf("got %d comments, want %d", len(resp.Comments), len(tt.wantIDs))
			}
			for i, id := range tt.wantIDs {
				if resp.Comments[i].ID != id {
					t.Errorf("comments[%d].id = %q, want %q", i, resp.Comments[i].ID, id)
				}
			}
			if len(c.saved)+len(c.removed) != 0 {
				t.Error("GET must be read-only")
			}
		})
	}
}

func TestDeleteComment(t *testing.T) {
	byAuthor := core.RestoreComment("c1", "x", author.ID, article.ID, time.Now())
	byOther := core.RestoreComment("c2", "y", other.ID, article.ID, time.Now())
	third := &core.User{ID: "u3"}
	tests := []struct {
		name        string
		slug, id    string
		user        *core.User
		wantStatus  int
		wantRemoved string
	}{
		{"comment+article author deletes", "title", "c1", author, http.StatusNoContent, "c1"},
		{"article author deletes others comment", "title", "c2", author, http.StatusNoContent, "c2"},
		{"comment author deletes", "title", "c2", other, http.StatusNoContent, "c2"},
		{"unrelated user forbidden", "title", "c1", third, http.StatusForbidden, ""},
		{"article not found", "nope", "c1", author, http.StatusNotFound, ""},
		{"comment not found", "title", "missing", author, http.StatusNotFound, ""},
		{"unauthenticated", "title", "c1", nil, http.StatusUnauthorized, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &fakeArticles{articles: map[string]*core.Article{"title": article}}
			c := &fakeComments{byKey: map[findCall]*core.Comment{
				{article.ID, "c1"}: byAuthor,
				{article.ID, "c2"}: byOther,
			}}
			mux := newTestMux(t, a, c, &fakeQueries{})

			req := withUser(httptest.NewRequest(http.MethodDelete, "/articles/"+tt.slug+"/comments/"+tt.id, nil), tt.user)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus == http.StatusNoContent && rec.Body.Len() != 0 {
				t.Errorf("expected empty body, got %q", rec.Body.String())
			}
			if tt.wantRemoved == "" {
				if len(c.removed) != 0 {
					t.Errorf("unexpected removal")
				}
			} else {
				if len(c.removed) != 1 || c.removed[0].ID() != tt.wantRemoved {
					t.Errorf("removed = %v, want %s", c.removed, tt.wantRemoved)
				}
			}
			for _, fc := range c.findCalls {
				if fc.articleID != article.ID {
					t.Errorf("comment lookup not scoped to article: %+v", fc)
				}
			}
		})
	}
}

func TestCanWriteComment(t *testing.T) {
	cm := core.RestoreComment("c", "b", "u2", "a1", time.Now())
	tests := []struct {
		name string
		u    *core.User
		a    *core.Article
		c    *core.Comment
		want bool
	}{
		{"article author", author, article, cm, true},
		{"comment author", other, article, cm, true},
		{"neither", &core.User{ID: "u3"}, article, cm, false},
		{"nil user", nil, article, cm, false},
		{"nil article", author, nil, cm, false},
		{"nil comment", author, article, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canWriteComment(tt.u, tt.a, tt.c); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}