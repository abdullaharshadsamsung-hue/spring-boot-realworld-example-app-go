package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
)

type articlesFakeCommander struct {
	calls   int
	param   app.NewArticleParam
	creator *core.User
	article *core.Article
	err     error
}

func (f *articlesFakeCommander) CreateArticle(_ context.Context, p app.NewArticleParam, c *core.User) (*core.Article, error) {
	f.calls++
	f.param = p
	f.creator = c
	if f.err != nil {
		return nil, f.err
	}
	return f.article, nil
}

type articlesFakeQuerier struct {
	byIDCalls  int
	byIDID     string
	byIDUser   *core.User
	data       app.ArticleData
	found      bool
	findErr    error
	recentArgs []string
	recentPage app.Page
	recentUser *core.User
	recentCall int
	feedUser   *core.User
	feedPage   app.Page
	feedCall   int
	feedErr    error
	list       app.ArticleDataList
}

func (f *articlesFakeQuerier) FindByID(_ context.Context, id string, u *core.User) (app.ArticleData, bool, error) {
	f.byIDCalls++
	f.byIDID = id
	f.byIDUser = u
	return f.data, f.found, f.findErr
}

func (f *articlesFakeQuerier) FindRecentArticles(_ context.Context, tag, author, fav string, p app.Page, u *core.User) (app.ArticleDataList, error) {
	f.recentCall++
	f.recentArgs = []string{tag, author, fav}
	f.recentPage = p
	f.recentUser = u
	return f.list, nil
}

func (f *articlesFakeQuerier) FindUserFeed(_ context.Context, u *core.User, p app.Page) (app.ArticleDataList, error) {
	f.feedCall++
	f.feedUser = u
	f.feedPage = p
	return f.list, f.feedErr
}

type articlesFieldMapErr struct{ m map[string][]string }

func (e *articlesFieldMapErr) Error() string                    { return "validation failed" }
func (e *articlesFieldMapErr) FieldErrors() map[string][]string { return e.m }

func newArticlesMux(t *testing.T, c *articlesFakeCommander, q *articlesFakeQuerier) *http.ServeMux {
	t.Helper()
	h, err := NewArticlesHandler(c, q, nil)
	if err != nil {
		t.Fatalf("NewArticlesHandler: %v", err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func articlesWithUser(r *http.Request, u *core.User) *http.Request {
	if u == nil {
		return r
	}
	return r.WithContext(auth.WithUser(r.Context(), u))
}

func TestNewArticlesHandler(t *testing.T) {
	tests := []struct {
		name string
		c    ArticlesCommander
		q    ArticlesQuerier
		want error
	}{
		{"nil commander", nil, &articlesFakeQuerier{}, ErrNilArticlesCommander},
		{"nil querier", &articlesFakeCommander{}, nil, ErrNilArticlesQuerier},
		{"ok", &articlesFakeCommander{}, &articlesFakeQuerier{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewArticlesHandler(tt.c, tt.q, nil)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want %v", err, tt.want)
			}
			if tt.want == nil && h == nil {
				t.Fatal("nil handler")
			}
		})
	}
}

func TestCreateArticle(t *testing.T) {
	user := &core.User{ID: "u1", Username: "johnjacob", Email: "j@x.com"}
	validBody := `{"article":{"title":"How to train your dragon","description":"Ever wonder how?","body":"You have to believe","tagList":["reactjs","angularjs","dragons"]}}`

	tests := []struct {
		name       string
		user       *core.User
		body       string
		cmdErr     error
		found      bool
		wantStatus int
		wantCalls  int
		check      func(t *testing.T, resp map[string]any, c *articlesFakeCommander, q *articlesFakeQuerier)
	}{
		{
			name: "success", user: user, body: validBody, found: true,
			wantStatus: http.StatusOK, wantCalls: 1,
			check: func(t *testing.T, resp map[string]any, c *articlesFakeCommander, q *articlesFakeQuerier) {
				if c.param.Title != "How to train your dragon" || c.param.Body != "You have to believe" ||
					c.param.Description != "Ever wonder how?" {
					t.Errorf("unexpected param %+v", c.param)
				}
				if !reflect.DeepEqual(c.param.TagList, []string{"reactjs", "angularjs", "dragons"}) {
					t.Errorf("tags=%v", c.param.TagList)
				}
				if c.creator != user {
					t.Errorf("creator mismatch")
				}
				if q.byIDCalls != 1 || q.byIDID != "a1" || q.byIDUser != user {
					t.Errorf("read model not re-read for current user: %+v", q)
				}
				art, ok := resp["article"].(map[string]any)
				if !ok {
					t.Fatalf("missing article key: %v", resp)
				}
				if art["favorited"] != false {
					t.Errorf("favorited=%v", art["favorited"])
				}
				if art["favoritesCount"] != float64(0) {
					t.Errorf("favoritesCount=%v", art["favoritesCount"])
				}
				if author, ok := art["author"].(map[string]any); ok {
					if id, present := author["id"]; present && id != nil {
						t.Errorf("author id exposed: %v", id)
					}
				}
			},
		},
		{
			name: "empty body field", user: user,
			body:       `{"article":{"title":"t","description":"d","body":"","tagList":[]}}`,
			cmdErr:     &articlesFieldMapErr{m: map[string][]string{"body": {"can't be empty"}}},
			wantStatus: http.StatusUnprocessableEntity, wantCalls: 1,
			check: func(t *testing.T, resp map[string]any, _ *articlesFakeCommander, q *articlesFakeQuerier) {
				errs, _ := resp["errors"].(map[string]any)
				b, _ := errs["body"].([]any)
				if len(b) == 0 || b[0] != "can't be empty" {
					t.Errorf("errors=%v", resp)
				}
				if q.byIDCalls != 0 {
					t.Error("should not read model")
				}
			},
		},
		{
			name: "duplicated title", user: user, body: validBody,
			cmdErr:     &articlesFieldMapErr{m: map[string][]string{"title": {"article name exists"}}},
			wantStatus: http.StatusUnprocessableEntity, wantCalls: 1,
		},
		{name: "malformed json", user: user, body: `{bad`, wantStatus: http.StatusBadRequest},
		{name: "unauthenticated", body: validBody, wantStatus: http.StatusUnauthorized},
		{name: "disappeared", user: user, body: validBody, found: false, wantStatus: http.StatusInternalServerError, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &articlesFakeCommander{article: &core.Article{ID: "a1"}, err: tt.cmdErr}
			q := &articlesFakeQuerier{found: tt.found, data: app.ArticleData{ID: "a1"}}
			mux := newArticlesMux(t, c, q)
			req := articlesWithUser(httptest.NewRequest(http.MethodPost, "/articles", strings.NewReader(tt.body)), tt.user)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status=%d want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if c.calls != tt.wantCalls {
				t.Fatalf("command calls=%d want %d", c.calls, tt.wantCalls)
			}
			if tt.check != nil {
				var resp map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode: %v", err)
				}
				tt.check(t, resp, c, q)
			}
		})
	}
}

func TestGetFeed(t *testing.T) {
	user := &core.User{ID: "u1", Username: "me"}
	tests := []struct {
		name       string
		user       *core.User
		query      string
		feedErr    error
		wantStatus int
		wantPage   app.Page
		wantCalls  int
	}{
		{"defaults", user, "", nil, http.StatusOK, app.NewPage(0, 20), 1},
		{"custom paging", user, "?offset=5&limit=7", nil, http.StatusOK, app.NewPage(5, 7), 1},
		{"no auth", nil, "", nil, http.StatusUnauthorized, app.Page{}, 0},
		{"bad offset", user, "?offset=x", nil, http.StatusBadRequest, app.Page{}, 0},
		{"feed requires user", user, "", app.ErrFeedRequiresUser, http.StatusUnauthorized, app.NewPage(0, 20), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &articlesFakeQuerier{list: app.NewArticleDataList([]app.ArticleData{}, 0), feedErr: tt.feedErr}
			mux := newArticlesMux(t, &articlesFakeCommander{}, q)
			req := articlesWithUser(httptest.NewRequest(http.MethodGet, "/articles/feed"+tt.query, nil), tt.user)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status=%d want %d", rec.Code, tt.wantStatus)
			}
			if q.feedCall != tt.wantCalls {
				t.Fatalf("calls=%d", q.feedCall)
			}
			if tt.wantCalls == 1 {
				if q.feedUser != user || !reflect.DeepEqual(q.feedPage, tt.wantPage) {
					t.Errorf("user=%v page=%+v", q.feedUser, q.feedPage)
				}
			}
		})
	}
}

func TestGetArticles(t *testing.T) {
	user := &core.User{ID: "u1"}
	tests := []struct {
		name       string
		user       *core.User
		query      string
		wantStatus int
		wantArgs   []string
		wantPage   app.Page
	}{
		{"anonymous defaults", nil, "", http.StatusOK, []string{"", "", ""}, app.NewPage(0, 20)},
		{"filters", user, "?tag=java&author=bob&favorited=al&offset=3&limit=4", http.StatusOK, []string{"java", "bob", "al"}, app.NewPage(3, 4)},
		{"bad limit", nil, "?limit=abc", http.StatusBadRequest, nil, app.Page{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &articlesFakeQuerier{list: app.NewArticleDataList([]app.ArticleData{}, 0)}
			mux := newArticlesMux(t, &articlesFakeCommander{}, q)
			req := articlesWithUser(httptest.NewRequest(http.MethodGet, "/articles"+tt.query, nil), tt.user)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status=%d want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantArgs == nil {
				if q.recentCall != 0 {
					t.Fatal("query should not be called")
				}
				return
			}
			if q.recentCall != 1 || !reflect.DeepEqual(q.recentArgs, tt.wantArgs) ||
				!reflect.DeepEqual(q.recentPage, tt.wantPage) || q.recentUser != tt.user {
				t.Errorf("args=%v page=%+v user=%v", q.recentArgs, q.recentPage, q.recentUser)
			}
		})
	}
}