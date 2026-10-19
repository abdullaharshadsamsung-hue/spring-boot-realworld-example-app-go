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

type artQuerierCall struct {
	slug string
	user *core.User
}

type artFakeQuerier struct {
	data  map[string]app.ArticleData
	err   error
	calls []artQuerierCall
}

func (f *artFakeQuerier) FindBySlug(_ context.Context, slug string, user *core.User) (app.ArticleData, bool, error) {
	f.calls = append(f.calls, artQuerierCall{slug, user})
	if f.err != nil {
		return app.ArticleData{}, false, f.err
	}
	d, ok := f.data[slug]
	return d, ok, nil
}

type artFakeRepo struct {
	articles  map[string]*core.Article
	findErr   error
	removeErr error
	removed   []*core.Article
}

func (f *artFakeRepo) FindBySlug(_ context.Context, slug string) (*core.Article, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	a, ok := f.articles[slug]
	if !ok {
		return nil, core.ErrNotFound
	}
	return a, nil
}

func (f *artFakeRepo) Remove(_ context.Context, a *core.Article) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, a)
	return nil
}

type artFakeUpdater struct {
	calls int
	err   error
}

func (f *artFakeUpdater) UpdateArticle(_ context.Context, a *core.Article, p app.UpdateArticleParam) (*core.Article, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	a.Update(p.Title(), p.Description(), p.Body())
	return a, nil
}

var (
	artAuthor   = &core.User{ID: "u1", Username: "author"}
	artStranger = &core.User{ID: "u2", Username: "other"}
	artT0       = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
)

func artArticleData(slug, body string) app.ArticleData {
	return app.NewArticleData("a1", slug, "T", "D", body, false, 0, artT0, artT0, nil,
		&app.ProfileData{ID: "u1", Username: "author"})
}

func artNewHandler(t *testing.T, q *artFakeQuerier, r *artFakeRepo, u *artFakeUpdater) *ArticleHandler {
	t.Helper()
	h, err := NewArticleHandler(q, r, u, core.CanWriteArticle)
	if err != nil {
		t.Fatalf("NewArticleHandler: %v", err)
	}
	return h
}

func artMakeReq(method, slug, body string, user *core.User) *http.Request {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, "/articles/"+slug, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, "/articles/"+slug, nil)
	}
	req.SetPathValue("slug", slug)
	if user != nil {
		req = req.WithContext(auth.WithUser(req.Context(), user))
	}
	return req
}

func TestNewArticleHandler(t *testing.T) {
	q, r, u := &artFakeQuerier{}, &artFakeRepo{}, &artFakeUpdater{}
	tests := []struct {
		name string
		q    ArticleQuerier
		r    ArticleRepository
		u    ArticleUpdater
		a    ArticleWriteAuthorizer
		want error
	}{
		{"ok", q, r, u, core.CanWriteArticle, nil},
		{"nil querier", nil, r, u, core.CanWriteArticle, ErrNilArticleQuerier},
		{"nil repo", q, nil, u, core.CanWriteArticle, ErrNilArticleRepository},
		{"nil updater", q, r, nil, core.CanWriteArticle, ErrNilArticleUpdater},
		{"nil authz", q, r, u, nil, ErrNilArticleAuthorizer},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, err := NewArticleHandler(tc.q, tc.r, tc.u, tc.a)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
			if tc.want == nil && h == nil {
				t.Fatal("nil handler")
			}
		})
	}
}

func TestGetArticle(t *testing.T) {
	tests := []struct {
		name       string
		slug       string
		user       *core.User
		qErr       error
		wantStatus int
		wantErr    bool
	}{
		{"anonymous existing", "test-new-article", nil, nil, 200, false},
		{"authenticated existing", "test-new-article", artStranger, nil, 200, false},
		{"not found", "not-exists", nil, nil, 404, false},
		{"not found error", "x", nil, core.ErrNotFound, 404, false},
		{"internal error", "x", nil, errors.New("db"), 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := &artFakeQuerier{data: map[string]app.ArticleData{"test-new-article": artArticleData("test-new-article", "Body")}, err: tc.qErr}
			h := artNewHandler(t, q, &artFakeRepo{}, &artFakeUpdater{})
			w := httptest.NewRecorder()
			err := h.GetArticle(w, artMakeReq(http.MethodGet, tc.slug, "", tc.user))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if w.Code != tc.wantStatus {
				t.Fatalf("status=%d want %d", w.Code, tc.wantStatus)
			}
			if len(q.calls) != 1 || q.calls[0].slug != tc.slug || q.calls[0].user != tc.user {
				t.Fatalf("unexpected query calls %+v", q.calls)
			}
			if tc.wantStatus != 200 {
				return
			}
			var top map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &top); err != nil {
				t.Fatal(err)
			}
			if len(top) != 1 || top["article"] == nil {
				t.Fatalf("body must have single 'article' key: %s", w.Body.String())
			}
			var a map[string]any
			_ = json.Unmarshal(top["article"], &a)
			if a["slug"] != "test-new-article" || a["body"] != "Body" || a["createdAt"] != "2020-01-01T00:00:00.000Z" {
				t.Fatalf("bad article %v", a)
			}
		})
	}
}

func TestUpdateArticle(t *testing.T) {
	validBody := `{"article":{"title":"new title","body":"new body","description":"new description"}}`
	tests := []struct {
		name        string
		slug        string
		user        *core.User
		body        string
		findErr     error
		wantStatus  int
		wantErr     bool
		wantUpdates int
	}{
		{"author updates", "old-title", artAuthor, validBody, nil, 200, false, 1},
		{"non-author forbidden", "old-title", artStranger, validBody, nil, 403, false, 0},
		{"not found", "missing", artAuthor, validBody, nil, 404, false, 0},
		{"not found before authz", "missing", artStranger, validBody, nil, 404, false, 0},
		{"unauthenticated", "old-title", nil, validBody, nil, 401, false, 0},
		{"malformed body", "old-title", artAuthor, `{bad`, nil, 400, false, 0},
		{"missing root", "old-title", artAuthor, `{"title":"x"}`, nil, 400, false, 0},
		{"repo error", "old-title", artAuthor, validBody, errors.New("db"), 0, true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			art := &core.Article{ID: "a1", UserID: artAuthor.ID, Slug: "old-title", Title: "old title"}
			repo := &artFakeRepo{articles: map[string]*core.Article{"old-title": art}, findErr: tc.findErr}
			q := &artFakeQuerier{data: map[string]app.ArticleData{"new-title": artArticleData("new-title", "new body")}}
			u := &artFakeUpdater{}
			h := artNewHandler(t, q, repo, u)
			w := httptest.NewRecorder()
			err := h.UpdateArticle(w, artMakeReq(http.MethodPut, tc.slug, tc.body, tc.user))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if w.Code != tc.wantStatus {
				t.Fatalf("status=%d want %d body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			if u.calls != tc.wantUpdates {
				t.Fatalf("updates=%d want %d", u.calls, tc.wantUpdates)
			}
			if tc.wantStatus != 200 {
				return
			}
			if len(q.calls) != 1 || q.calls[0].slug != "new-title" || q.calls[0].user != artAuthor {
				t.Fatalf("re-query wrong: %+v", q.calls)
			}
			var resp struct {
				Article map[string]any `json:"article"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.Article["slug"] != "new-title" {
				t.Fatalf("slug=%v", resp.Article["slug"])
			}
		})
	}
}

func TestDeleteArticle(t *testing.T) {
	tests := []struct {
		name        string
		slug        string
		user        *core.User
		findErr     error
		removeErr   error
		wantStatus  int
		wantErr     bool
		wantRemoved int
	}{
		{"author deletes", "title", artAuthor, nil, nil, 204, false, 1},
		{"non-author forbidden", "title", artStranger, nil, nil, 403, false, 0},
		{"not found", "missing", artAuthor, nil, nil, 404, false, 0},
		{"not found before authz", "missing", artStranger, nil, nil, 404, false, 0},
		{"unauthenticated", "title", nil, nil, nil, 401, false, 0},
		{"find error", "title", artAuthor, errors.New("db"), nil, 0, true, 0},
		{"remove error", "title", artAuthor, nil, errors.New("db"), 0, true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			art := &core.Article{ID: "a1", UserID: artAuthor.ID, Slug: "title", Title: "title"}
			repo := &artFakeRepo{articles: map[string]*core.Article{"title": art}, findErr: tc.findErr, removeErr: tc.removeErr}
			h := artNewHandler(t, &artFakeQuerier{}, repo, &artFakeUpdater{})
			w := httptest.NewRecorder()
			err := h.DeleteArticle(w, artMakeReq(http.MethodDelete, tc.slug, "", tc.user))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if w.Code != tc.wantStatus {
				t.Fatalf("status=%d want %d", w.Code, tc.wantStatus)
			}
			if len(repo.removed) != tc.wantRemoved {
				t.Fatalf("removed=%d want %d", len(repo.removed), tc.wantRemoved)
			}
			if tc.wantStatus == 204 {
				if w.Body.Len() != 0 {
					t.Fatalf("expected empty body, got %q", w.Body.String())
				}
				if repo.removed[0] != art {
					t.Fatal("wrong article removed")
				}
			}
		})
	}
}