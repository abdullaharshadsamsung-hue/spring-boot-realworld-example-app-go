package graphql

import (
	"context"
	"errors"
	"testing"
	"time"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
)

type fakeCommentQuery struct {
	called    bool
	articleID string
	user      *core.User
	page      app.CursorPageParameter
	pager     *app.CursorPager[app.CommentNode]
	err       error
}

func (f *fakeCommentQuery) FindByArticleIDWithCursor(_ context.Context, articleID string, user *core.User, page app.CursorPageParameter) (*app.CursorPager[app.CommentNode], error) {
	f.called = true
	f.articleID = articleID
	f.user = user
	f.page = page
	if f.err != nil {
		return nil, f.err
	}
	if f.pager == nil {
		return app.NewCursorPager([]app.CommentNode{}, page.Direction(), false), nil
	}
	return f.pager, nil
}

func commentIntPtr(i int) *int { return &i }

var (
	commentT1 = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	commentT2 = time.Date(2020, 1, 2, 3, 4, 5, 6000000, time.FixedZone("X", 3600))
)

func TestNewCommentDatafetcher(t *testing.T) {
	if _, err := NewCommentDatafetcher(nil); !errors.Is(err, ErrNilCommentQuery) {
		t.Fatalf("want ErrNilCommentQuery, got %v", err)
	}
	d, err := NewCommentDatafetcher(&fakeCommentQuery{})
	if err != nil || d == nil {
		t.Fatalf("unexpected: %v %v", d, err)
	}
}

func TestCommentPayloadComment(t *testing.T) {
	tests := []struct {
		name    string
		in      *app.CommentData
		wantErr bool
		wantTS  string
	}{
		{"nil local context", nil, true, ""},
		{"utc", &app.CommentData{ID: "c1", Body: "b", CreatedAt: commentT1, UpdatedAt: commentT2}, false, "2020-01-01T00:00:00.000Z"},
		{"non-utc zone", &app.CommentData{ID: "c2", Body: "x", CreatedAt: commentT2, UpdatedAt: commentT1}, false, "2020-01-02T02:04:05.006Z"},
	}
	d, _ := NewCommentDatafetcher(&fakeCommentQuery{})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := d.CommentPayloadComment(context.Background(), tc.in)
			if tc.wantErr {
				if !errors.Is(err, ErrMissingLocalContext) {
					t.Fatalf("want ErrMissingLocalContext, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.Data.ID != tc.in.ID || res.Data.Body != tc.in.Body {
				t.Errorf("bad data %+v", res.Data)
			}
			if res.Data.CreatedAt != tc.wantTS || res.Data.UpdatedAt != tc.wantTS {
				t.Errorf("timestamps %q %q want %q", res.Data.CreatedAt, res.Data.UpdatedAt, tc.wantTS)
			}
			if len(res.LocalContext) != 1 {
				t.Fatalf("local ctx size %d", len(res.LocalContext))
			}
			if got, ok := res.LocalContext[tc.in.ID]; !ok || !got.Equal(*tc.in) {
				t.Errorf("local ctx mismatch %+v", got)
			}
		})
	}
}

func TestArticleCommentsArgs(t *testing.T) {
	afterMs := "1577836800000"
	cur := time.UnixMilli(1577836800000).UTC()
	local := map[string]app.ArticleData{"slug": {ID: "a1", Slug: "slug"}}
	user := &core.User{ID: "u1", Username: "bob"}

	tests := []struct {
		name       string
		args       CursorArgs
		article    *Article
		local      map[string]app.ArticleData
		user       *core.User
		wantErr    bool
		wantCalled bool
		wantLimit  int
		wantDir    app.Direction
		wantCursor *time.Time
	}{
		{name: "both nil", args: CursorArgs{}, article: &Article{Slug: "slug"}, local: local, wantErr: true},
		{name: "first only", args: CursorArgs{First: commentIntPtr(5)}, article: &Article{Slug: "slug"}, local: local, user: user, wantCalled: true, wantLimit: 5, wantDir: app.DirectionNext},
		{name: "first with after", args: CursorArgs{First: commentIntPtr(3), After: afterMs}, article: &Article{Slug: "slug"}, local: local, wantCalled: true, wantLimit: 3, wantDir: app.DirectionNext, wantCursor: &cur},
		{name: "first wins over last", args: CursorArgs{First: commentIntPtr(2), After: afterMs, Last: commentIntPtr(9), Before: "1"}, article: &Article{Slug: "slug"}, local: local, wantCalled: true, wantLimit: 2, wantDir: app.DirectionNext, wantCursor: &cur},
		{name: "last with before", args: CursorArgs{Last: commentIntPtr(4), Before: afterMs}, article: &Article{Slug: "slug"}, local: local, wantCalled: true, wantLimit: 4, wantDir: app.DirectionPrev, wantCursor: &cur},
		{name: "nil article", args: CursorArgs{First: commentIntPtr(1)}, article: nil, local: local, wantErr: true},
		{name: "slug missing", args: CursorArgs{First: commentIntPtr(1)}, article: &Article{Slug: "other"}, local: local, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeCommentQuery{}
			d, _ := NewCommentDatafetcher(q)
			ctx := auth.WithUser(context.Background(), tc.user)
			res, err := d.ArticleComments(ctx, tc.article, tc.local, tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if q.called {
					t.Error("query should not be called")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res == nil || !q.called {
				t.Fatal("expected query call")
			}
			if q.articleID != "a1" {
				t.Errorf("articleID %q", q.articleID)
			}
			if q.user != tc.user {
				t.Errorf("user %v want %v", q.user, tc.user)
			}
			if q.page.Limit() != tc.wantLimit || q.page.Direction() != tc.wantDir {
				t.Errorf("page %v", q.page)
			}
			c := q.page.Cursor()
			if (c == nil) != (tc.wantCursor == nil) || (c != nil && !c.Equal(*tc.wantCursor)) {
				t.Errorf("cursor %v want %v", c, tc.wantCursor)
			}
		})
	}
}

func TestArticleCommentsResults(t *testing.T) {
	local := map[string]app.ArticleData{"slug": {ID: "a1", Slug: "slug"}}
	c1 := app.CommentData{ID: "c1", Body: "one", CreatedAt: commentT1, UpdatedAt: commentT2, Author: &app.ProfileData{Username: "al"}}
	c2 := app.CommentData{ID: "c2", Body: "two", CreatedAt: commentT2, UpdatedAt: commentT2}

	tests := []struct {
		name     string
		pager    *app.CursorPager[app.CommentNode]
		qerr     error
		wantErr  bool
		wantIDs  []string
		wantPrev bool
		wantNext bool
	}{
		{name: "page next", pager: app.NewCursorPager([]app.CommentNode{{CommentData: c1}, {CommentData: c2}}, app.DirectionNext, true), wantIDs: []string{"c1", "c2"}, wantNext: true},
		{name: "page prev", pager: app.NewCursorPager([]app.CommentNode{{CommentData: c2}}, app.DirectionPrev, true), wantIDs: []string{"c2"}, wantPrev: true},
		{name: "empty", pager: app.NewCursorPager([]app.CommentNode{}, app.DirectionNext, false), wantIDs: nil},
		{name: "service error", qerr: errors.New("boom"), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeCommentQuery{pager: tc.pager, err: tc.qerr}
			d, _ := NewCommentDatafetcher(q)
			res, err := d.ArticleComments(context.Background(), &Article{Slug: "slug"}, local, CursorArgs{First: commentIntPtr(10)})
			if tc.wantErr {
				if !errors.Is(err, tc.qerr) {
					t.Fatalf("want wrapped err, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if q.user != nil {
				t.Error("anonymous user should be nil")
			}
			edges := res.Data.Edges
			if edges == nil || len(edges) != len(tc.wantIDs) {
				t.Fatalf("edges %v", edges)
			}
			if len(res.LocalContext) != len(tc.wantIDs) {
				t.Errorf("local ctx size %d", len(res.LocalContext))
			}
			nodes := tc.pager.Data()
			for i, e := range edges {
				n := nodes[i]
				if e.Node.ID != tc.wantIDs[i] || e.Node.Body != n.Body {
					t.Errorf("edge %d %+v", i, e.Node)
				}
				if e.Cursor != n.Cursor() {
					t.Errorf("cursor %q", e.Cursor)
				}
				want := n.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
				if e.Node.CreatedAt != want || e.Node.UpdatedAt != want {
					t.Errorf("ts %q %q want %q", e.Node.CreatedAt, e.Node.UpdatedAt, want)
				}
				if lc, ok := res.LocalContext[n.ID]; !ok || !lc.Equal(n.CommentData) {
					t.Errorf("local ctx missing %s", n.ID)
				}
			}
			pi := res.Data.PageInfo
			if pi.HasPreviousPage != tc.wantPrev || pi.HasNextPage != tc.wantNext {
				t.Errorf("pageinfo flags %+v", pi)
			}
			if len(tc.wantIDs) == 0 {
				if pi.StartCursor != nil || pi.EndCursor != nil {
					t.Error("expected nil cursors")
				}
			} else {
				if pi.StartCursor == nil || *pi.StartCursor != edges[0].Cursor {
					t.Error("bad start cursor")
				}
				if pi.EndCursor == nil || *pi.EndCursor != edges[len(edges)-1].Cursor {
					t.Error("bad end cursor")
				}
			}
		})
	}
}

func TestCommentAuthorLookup(t *testing.T) {
	local := map[string]app.CommentData{
		"c1": {ID: "c1", Author: &app.ProfileData{Username: "al"}},
		"c2": {ID: "c2"},
	}
	lookup := CommentAuthorLookup(local)
	tests := []struct {
		id     string
		want   string
		wantOK bool
	}{
		{"c1", "al", true},
		{"c2", "", false},
		{"missing", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			got, ok := lookup(tc.id)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("got %q %v", got, ok)
			}
		})
	}
}