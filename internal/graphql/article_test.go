package graphql

import (
	"context"
	"errors"
	"testing"
	"time"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
	"migrated-app/internal/httpapi"
)

type fakeArticles struct {
	byID        map[string]app.ArticleData
	bySlug      map[string]app.ArticleData
	err         error
	pager       *app.CursorPager[app.ArticleNode]
	gotUser     *core.User
	gotPage     app.CursorPageParameter
	gotTag      string
	gotAuthor   string
	gotFav      string
	gotID       string
	feedCalled  bool
	recentCalls int
}

func (f *fakeArticles) FindByID(ctx context.Context, id string, u *core.User) (app.ArticleData, bool, error) {
	f.gotID, f.gotUser = id, u
	if f.err != nil {
		return app.ArticleData{}, false, f.err
	}
	d, ok := f.byID[id]
	return d, ok, nil
}

func (f *fakeArticles) FindBySlug(ctx context.Context, s string, u *core.User) (app.ArticleData, bool, error) {
	f.gotUser = u
	if f.err != nil {
		return app.ArticleData{}, false, f.err
	}
	d, ok := f.bySlug[s]
	return d, ok, nil
}

func (f *fakeArticles) FindRecentArticlesWithCursor(ctx context.Context, tag, author, fav string, p app.CursorPageParameter, u *core.User) (*app.CursorPager[app.ArticleNode], error) {
	f.recentCalls++
	f.gotTag, f.gotAuthor, f.gotFav, f.gotPage, f.gotUser = tag, author, fav, p, u
	if f.err != nil {
		return nil, f.err
	}
	return f.pager, nil
}

func (f *fakeArticles) FindUserFeedWithCursor(ctx context.Context, u *core.User, p app.CursorPageParameter) (*app.CursorPager[app.ArticleNode], error) {
	f.feedCalled = true
	f.gotUser, f.gotPage = u, p
	if f.err != nil {
		return nil, f.err
	}
	return f.pager, nil
}

type fakeUsers struct {
	users  map[string]*core.User
	err    error
	called bool
}

func (f *fakeUsers) FindByUsername(ctx context.Context, name string) (*core.User, bool, error) {
	f.called = true
	if f.err != nil {
		return nil, false, f.err
	}
	u, ok := f.users[name]
	return u, ok, nil
}

func ip(i int) *int { return &i }

var (
	t1 = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 = time.Date(2020, 1, 2, 3, 4, 5, 6000000, time.UTC)
)

func sampleData() []app.ArticleData {
	return []app.ArticleData{
		app.NewArticleData("id1", "slug-1", "T1", "D1", "B1", true, 3, t1, t2, []string{"go"}, nil),
		app.NewArticleData("id2", "slug-2", "T2", "D2", "B2", false, 0, t1, t1, nil, nil),
	}
}

func samplePager(dir app.Direction, extra bool) *app.CursorPager[app.ArticleNode] {
	var nodes []app.ArticleNode
	for _, d := range sampleData() {
		nodes = append(nodes, app.ArticleNode{ArticleData: d})
	}
	return app.NewCursorPager(nodes, dir, extra)
}

func newDF(t *testing.T, a *fakeArticles, u *fakeUsers) *ArticleDatafetcher {
	t.Helper()
	d, err := NewArticleDatafetcher(a, u)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func checkConnection(t *testing.T, res *ArticlesConnectionResult) {
	t.Helper()
	data := sampleData()
	if len(res.Data.Edges) != 2 {
		t.Fatalf("edges=%d", len(res.Data.Edges))
	}
	for i, e := range res.Data.Edges {
		if e.Cursor != data[i].Cursor().String() {
			t.Errorf("cursor %q", e.Cursor)
		}
		if e.Node.Slug != data[i].Slug {
			t.Errorf("order mismatch")
		}
		if _, ok := res.LocalContext[data[i].Slug]; !ok {
			t.Errorf("local context missing %s", data[i].Slug)
		}
	}
	n := res.Data.Edges[0].Node
	if n.Body != "B1" || n.Title != "T1" || n.Description != "D1" || !n.Favorited || n.FavoritesCount != 3 || len(n.TagList) != 1 {
		t.Errorf("bad node %+v", n)
	}
	if n.CreatedAt != "2020-01-01T00:00:00.000Z" || n.UpdatedAt != "2020-01-02T03:04:05.006Z" {
		t.Errorf("bad times %s %s", n.CreatedAt, n.UpdatedAt)
	}
	pi := res.Data.PageInfo
	if pi.StartCursor == nil || *pi.StartCursor != data[0].Cursor().String() || pi.EndCursor == nil || *pi.EndCursor != data[1].Cursor().String() {
		t.Errorf("bad page info cursors")
	}
}

func TestNewArticleDatafetcher(t *testing.T) {
	if _, err := NewArticleDatafetcher(nil, &fakeUsers{}); !errors.Is(err, ErrNilArticleQuery) {
		t.Error(err)
	}
	if _, err := NewArticleDatafetcher(&fakeArticles{}, nil); !errors.Is(err, ErrNilUserByUsernameFinder) {
		t.Error(err)
	}
}

func TestPageFor(t *testing.T) {
	cases := []struct {
		name    string
		args    CursorArgs
		wantErr error
		dir     app.Direction
		limit   int
		cursor  *time.Time
	}{
		{"none", CursorArgs{}, ErrFirstOrLastRequired, 0, 0, nil},
		{"first", CursorArgs{First: ip(5)}, nil, app.DirectionNext, 5, nil},
		{"first after", CursorArgs{First: ip(5), After: "1577836800000"}, nil, app.DirectionNext, 5, &t1},
		{"last before", CursorArgs{Last: ip(7), Before: "1577836800000"}, nil, app.DirectionPrev, 7, &t1},
		{"both first wins", CursorArgs{First: ip(3), Last: ip(9), Before: "bad"}, nil, app.DirectionNext, 3, nil},
		{"bad after", CursorArgs{First: ip(3), After: "x"}, app.ErrInvalidCursor, 0, 0, nil},
		{"bad before", CursorArgs{Last: ip(3), Before: "x"}, app.ErrInvalidCursor, 0, 0, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := pageFor(c.args)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := app.NewCursorPageParameter(c.cursor, c.limit, c.dir)
			if !p.Equal(want) {
				t.Errorf("got %v want %v", p, want)
			}
		})
	}
	if ErrFirstOrLastRequired.Error() != "first 和 last 必须只存在一个" {
		t.Error("message changed")
	}
}

func TestFeed(t *testing.T) {
	user := &core.User{ID: "u1", Username: "me"}
	cases := []struct {
		name    string
		ctx     context.Context
		args    CursorArgs
		wantDir app.Direction
		wantErr error
	}{
		{"first auth", auth.WithUser(context.Background(), user), CursorArgs{First: ip(2)}, app.DirectionNext, nil},
		{"last anon", context.Background(), CursorArgs{Last: ip(2)}, app.DirectionPrev, nil},
		{"none", context.Background(), CursorArgs{}, 0, ErrFirstOrLastRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fa := &fakeArticles{pager: samplePager(c.wantDir, true)}
			res, err := newDF(t, fa, &fakeUsers{}).Feed(c.ctx, c.args)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) || fa.feedCalled {
					t.Fatalf("err=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if fa.gotPage.Direction() != c.wantDir {
				t.Error("direction")
			}
			cu, _ := auth.UserFrom(c.ctx)
			if fa.gotUser != cu {
				t.Error("user mismatch")
			}
			checkConnection(t, res)
			if c.wantDir == app.DirectionNext && !res.Data.PageInfo.HasNextPage {
				t.Error("hasNext")
			}
			if c.wantDir == app.DirectionPrev && !res.Data.PageInfo.HasPreviousPage {
				t.Error("hasPrev")
			}
		})
	}
}

func TestFeedEmptyAndError(t *testing.T) {
	fa := &fakeArticles{pager: app.NewCursorPager([]app.ArticleNode{}, app.DirectionNext, false)}
	res, err := newDF(t, fa, &fakeUsers{}).Feed(context.Background(), CursorArgs{First: ip(1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Data.Edges) != 0 || res.Data.PageInfo.StartCursor != nil || res.Data.PageInfo.EndCursor != nil || res.Data.PageInfo.HasNextPage {
		t.Errorf("bad empty %+v", res.Data)
	}
	fa = &fakeArticles{err: app.ErrFeedRequiresUser}
	if _, err := newDF(t, fa, &fakeUsers{}).Feed(context.Background(), CursorArgs{First: ip(1)}); !errors.Is(err, app.ErrFeedRequiresUser) {
		t.Error(err)
	}
}

func TestProfileFeed(t *testing.T) {
	target := &core.User{ID: "t1", Username: "bob"}
	viewer := &core.User{ID: "v1"}
	dbErr := errors.New("db")
	cases := []struct {
		name       string
		profile    *Profile
		args       CursorArgs
		users      *fakeUsers
		wantDir    app.Direction
		wantErr    error
		lookupDone bool
	}{
		{"first", &Profile{Username: "bob"}, CursorArgs{First: ip(2)}, &fakeUsers{users: map[string]*core.User{"bob": target}}, app.DirectionNext, nil, true},
		{"last", &Profile{Username: "bob"}, CursorArgs{Last: ip(2), Before: "1577836800000"}, &fakeUsers{users: map[string]*core.User{"bob": target}}, app.DirectionPrev, nil, true},
		{"not found", &Profile{Username: "x"}, CursorArgs{First: ip(2)}, &fakeUsers{}, 0, httpapi.ErrResourceNotFound, true},
		{"lookup err", &Profile{Username: "x"}, CursorArgs{First: ip(2)}, &fakeUsers{err: dbErr}, 0, dbErr, true},
		{"no args", &Profile{Username: "bob"}, CursorArgs{}, &fakeUsers{}, 0, ErrFirstOrLastRequired, false},
		{"nil profile", nil, CursorArgs{First: ip(1)}, &fakeUsers{}, 0, ErrMissingLocalContext, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fa := &fakeArticles{pager: samplePager(c.wantDir, false)}
			ctx := auth.WithUser(context.Background(), viewer)
			res, err := newDF(t, fa, c.users).ProfileFeed(ctx, c.profile, c.args)
			if c.users.called != c.lookupDone {
				t.Errorf("lookup called=%v", c.users.called)
			}
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if fa.gotUser != target {
				t.Error("must use profile user")
			}
			if fa.gotPage.Direction() != c.wantDir {
				t.Error("dir")
			}
			checkConnection(t, res)
		})
	}
}

func TestRecentResolvers(t *testing.T) {
	viewer := &core.User{ID: "v"}
	type call func(d *ArticleDatafetcher, ctx context.Context, a CursorArgs) (*ArticlesConnectionResult, error)
	p := &Profile{Username: "bob"}
	cases := []struct {
		name                string
		fn                  call
		args                CursorArgs
		ctx                 context.Context
		tag, author, fav    string
		dir                 app.Direction
		wantErr             error
	}{
		{"favorites first", func(d *ArticleDatafetcher, c context.Context, a CursorArgs) (*ArticlesConnectionResult, error) {
			return d.ProfileFavorites(c, p, a)
		}, CursorArgs{First: ip(2)}, auth.WithUser(context.Background(), viewer), "", "", "bob", app.DirectionNext, nil},
		{"favorites last", func(d *ArticleDatafetcher, c context.Context, a CursorArgs) (*ArticlesConnectionResult, error) {
			return d.ProfileFavorites(c, p, a)
		}, CursorArgs{Last: ip(2)}, context.Background(), "", "", "bob", app.DirectionPrev, nil},
		{"favorites none", func(d *ArticleDatafetcher, c context.Context, a CursorArgs) (*ArticlesConnectionResult, error) {
			return d.ProfileFavorites(c, p, a)
		}, CursorArgs{}, context.Background(), "", "", "", 0, ErrFirstOrLastRequired},
		{"favorites nil profile", func(d *ArticleDatafetcher, c context.Context, a CursorArgs) (*ArticlesConnectionResult, error) {
			return d.ProfileFavorites(c, nil, a)
		}, CursorArgs{First: ip(1)}, context.Background(), "", "", "", 0, ErrMissingLocalContext},
		{"articles first", func(d *ArticleDatafetcher, c context.Context, a CursorArgs) (*ArticlesConnectionResult, error) {
			return d.ProfileArticles(c, p, a)
		}, CursorArgs{First: ip(2)}, auth.WithUser(context.Background(), viewer), "", "bob", "", app.DirectionNext, nil},
		{"articles last", func(d *ArticleDatafetcher, c context.Context, a CursorArgs) (*ArticlesConnectionResult, error) {
			return d.ProfileArticles(c, p, a)
		}, CursorArgs{Last: ip(2)}, context.Background(), "", "bob", "", app.DirectionPrev, nil},
		{"articles none", func(d *ArticleDatafetcher, c context.Context, a CursorArgs) (*ArticlesConnectionResult, error) {
			return d.ProfileArticles(c, p, a)
		}, CursorArgs{}, context.Background(), "", "", "", 0, ErrFirstOrLastRequired},
		{"articles nil profile", func(d *ArticleDatafetcher, c context.Context, a CursorArgs) (*ArticlesConnectionResult, error) {
			return d.ProfileArticles(c, nil, a)
		}, CursorArgs{First: ip(1)}, context.Background(), "", "", "", 0, ErrMissingLocalContext},
		{"query filters", func(d *ArticleDatafetcher, c context.Context, a CursorArgs) (*ArticlesConnectionResult, error) {
			return d.Articles(c, a, ArticleFilter{AuthoredBy: "a", FavoritedBy: "f", WithTag: "t"})
		}, CursorArgs{First: ip(2)}, auth.WithUser(context.Background(), viewer), "t", "a", "f", app.DirectionNext, nil},
		{"query no filters last", func(d *ArticleDatafetcher, c context.Context, a CursorArgs) (*ArticlesConnectionResult, error) {
			return d.Articles(c, a, ArticleFilter{})
		}, CursorArgs{Last: ip(2)}, context.Background(), "", "", "", app.DirectionPrev, nil},
		{"query none", func(d *ArticleDatafetcher, c context.Context, a CursorArgs) (*ArticlesConnectionResult, error) {
			return d.Articles(c, a, ArticleFilter{})
		}, CursorArgs{}, context.Background(), "", "", "", 0, ErrFirstOrLastRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fa := &fakeArticles{pager: samplePager(c.dir, false)}
			res, err := c.fn(newDF(t, fa, &fakeUsers{}), c.ctx, c.args)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) || fa.recentCalls != 0 {
					t.Fatalf("err=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if fa.gotTag != c.tag || fa.gotAuthor != c.author || fa.gotFav != c.fav {
				t.Errorf("filters %q %q %q", fa.gotTag, fa.gotAuthor, fa.gotFav)
			}
			if fa.gotPage.Direction() != c.dir {
				t.Error("dir")
			}
			cu, _ := auth.UserFrom(c.ctx)
			if fa.gotUser != cu {
				t.Error("viewer")
			}
			checkConnection(t, res)
		})
	}
}

func TestRecentServiceError(t *testing.T) {
	e := errors.New("boom")
	fa := &fakeArticles{err: e}
	if _, err := newDF(t, fa, &fakeUsers{}).Articles(context.Background(), CursorArgs{First: ip(1)}, ArticleFilter{}); !errors.Is(err, e) {
		t.Error(err)
	}
}

func TestSingleArticleResolvers(t *testing.T) {
	d := sampleData()[0]
	viewer := &core.User{ID: "v"}
	boom := errors.New("boom")
	cases := []struct {
		name    string
		call    func(df *ArticleDatafetcher, ctx context.Context) (*ArticleResult, error)
		err     error
		wantErr error
	}{
		{"payload found", func(df *ArticleDatafetcher, c context.Context) (*ArticleResult, error) {
			return df.ArticlePayloadArticle(c, &core.Article{ID: "id1"})
		}, nil, nil},
		{"payload missing", func(df *ArticleDatafetcher, c context.Context) (*ArticleResult, error) {
			return df.ArticlePayloadArticle(c, &core.Article{ID: "nope"})
		}, nil, httpapi.ErrResourceNotFound},
		{"payload nil", func(df *ArticleDatafetcher, c context.Context) (*ArticleResult, error) {
			return df.ArticlePayloadArticle(c, nil)
		}, nil, ErrMissingLocalContext},
		{"comment found", func(df *ArticleDatafetcher, c context.Context) (*ArticleResult, error) {
			return df.CommentArticle(c, "id1")
		}, nil, nil},
		{"comment missing", func(df *ArticleDatafetcher, c context.Context) (*ArticleResult, error) {
			return df.CommentArticle(c, "nope")
		}, nil, httpapi.ErrResourceNotFound},
		{"comment err", func(df *ArticleDatafetcher, c context.Context) (*ArticleResult, error) {
			return df.CommentArticle(c, "id1")
		}, boom, boom},
		{"slug found", func(df *ArticleDatafetcher, c context.Context) (*ArticleResult, error) {
			return df.ArticleBySlug(c, "slug-1")
		}, nil, nil},
		{"slug missing", func(df *ArticleDatafetcher, c context.Context) (*ArticleResult, error) {
			return df.ArticleBySlug(c, "nope")
		}, nil, httpapi.ErrResourceNotFound},
		{"slug err", func(df *ArticleDatafetcher, c context.Context) (*ArticleResult, error) {
			return df.ArticleBySlug(c, "slug-1")
		}, boom, boom},
	}
	for _, c := range cases {
		for _, anon := range []bool{false, true} {
			t.Run(c.name, func(t *testing.T) {
				fa := &fakeArticles{byID: map[string]app.ArticleData{"id1": d}, bySlug: map[string]app.ArticleData{"slug-1": d}, err: c.err}
				ctx := context.Background()
				if !anon {
					ctx = auth.WithUser(ctx, viewer)
				}
				res, err := c.call(newDF(t, fa, &fakeUsers{}), ctx)
				if c.wantErr != nil {
					if !errors.Is(err, c.wantErr) {
						t.Fatalf("err=%v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if anon && fa.gotUser != nil || !anon && fa.gotUser != viewer {
					t.Error("viewer")
				}
				if res.Data.Slug != "slug-1" || res.Data.CreatedAt != "2020-01-01T00:00:00.000Z" {
					t.Errorf("bad %+v", res.Data)
				}
				if lc, ok := res.LocalContext["slug-1"]; !ok || lc.ID != "id1" || len(res.LocalContext) != 1 {
					t.Error("local context")
				}
			})
		}
	}
}

func TestIsoUTC(t *testing.T) {
	loc := time.FixedZone("X", 3600)
	tt := time.Date(2020, 1, 1, 1, 0, 0, 0, loc)
	var nilT *time.Time
	cases := []struct {
		in   any
		want string
	}{
		{tt, "2020-01-01T00:00:00.000Z"},
		{&tt, "2020-01-01T00:00:00.000Z"},
		{nilT, ""},
		{int64(1577836800000), "2020-01-01T00:00:00.000Z"},
		{"x", ""},
	}
	for _, c := range cases {
		if got := isoUTC(c.in); got != c.want {
			t.Errorf("%v: got %q", c.in, got)
		}
	}
}