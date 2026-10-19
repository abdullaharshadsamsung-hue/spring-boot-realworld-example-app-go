package app

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"migrated-app/internal/core"
)

// ---- fakes ----

type fakeArticles struct {
	data        []ArticleData // newest first
	favoritedBy map[string]map[string]struct{}
	usernames   map[string]string // username -> user id
	pageEmpty   bool
	err         error
	calls       int
}

func (f *fakeArticles) clone(a ArticleData) ArticleData {
	if a.Author != nil {
		p := *a.Author
		a.Author = &p
	}
	a.TagList = append([]string(nil), a.TagList...)
	return a
}

func (f *fakeArticles) FindByID(ctx context.Context, id string) (ArticleData, bool, error) {
	f.calls++
	if f.err != nil {
		return ArticleData{}, false, f.err
	}
	for _, a := range f.data {
		if a.ID == id {
			return f.clone(a), true, nil
		}
	}
	return ArticleData{}, false, nil
}

func (f *fakeArticles) FindBySlug(ctx context.Context, slug string) (ArticleData, bool, error) {
	f.calls++
	if f.err != nil {
		return ArticleData{}, false, f.err
	}
	for _, a := range f.data {
		if a.Slug == slug {
			return f.clone(a), true, nil
		}
	}
	return ArticleData{}, false, nil
}

func (f *fakeArticles) match(a ArticleData, tag, author, favoritedBy string) bool {
	if tag != "" {
		found := false
		for _, t := range a.TagList {
			if t == tag {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if author != "" && (a.Author == nil || a.Author.Username != author) {
		return false
	}
	if favoritedBy != "" {
		if _, ok := f.favoritedBy[favoritedBy][a.ID]; !ok {
			return false
		}
	}
	return true
}

func (f *fakeArticles) filtered(tag, author, favoritedBy string) []ArticleData {
	var out []ArticleData
	for _, a := range f.data {
		if f.match(a, tag, author, favoritedBy) {
			out = append(out, a)
		}
	}
	return out
}

func (f *fakeArticles) QueryArticles(ctx context.Context, tag, author, favoritedBy string, page Page) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.pageEmpty {
		return nil, nil
	}
	var ids []string
	for _, a := range f.filtered(tag, author, favoritedBy) {
		ids = append(ids, a.ID)
	}
	return ids, nil
}

func (f *fakeArticles) CountArticle(ctx context.Context, tag, author, favoritedBy string) (int, error) {
	return len(f.filtered(tag, author, favoritedBy)), nil
}

func (f *fakeArticles) FindArticles(ctx context.Context, ids []string) ([]ArticleData, error) {
	var out []ArticleData
	for _, id := range ids {
		for _, a := range f.data {
			if a.ID == id {
				out = append(out, f.clone(a))
			}
		}
	}
	return out, nil
}

func (f *fakeArticles) ofAuthors(authorIDs []string) []ArticleData {
	set := map[string]bool{}
	for _, id := range authorIDs {
		set[id] = true
	}
	var out []ArticleData
	for _, a := range f.data {
		if a.Author != nil && set[a.Author.ID] {
			out = append(out, f.clone(a))
		}
	}
	return out
}

func (f *fakeArticles) FindArticlesOfAuthors(ctx context.Context, authorIDs []string, page Page) ([]ArticleData, error) {
	return f.ofAuthors(authorIDs), nil
}

func (f *fakeArticles) CountFeedSize(ctx context.Context, authorIDs []string) (int, error) {
	return len(f.ofAuthors(authorIDs)), nil
}

func cursorSlice(in []ArticleData, page CursorPageParameter) []ArticleData {
	var out []ArticleData
	c := page.Cursor()
	for _, a := range in {
		if c != nil {
			if page.IsNext() && !a.UpdatedAt.Before(*c) {
				continue
			}
			if !page.IsNext() && !a.UpdatedAt.After(*c) {
				continue
			}
		}
		out = append(out, a)
	}
	if !page.IsNext() {
		sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	}
	if len(out) > page.QueryLimit() {
		out = out[:page.QueryLimit()]
	}
	return out
}

func (f *fakeArticles) FindArticlesWithCursor(ctx context.Context, tag, author, favoritedBy string, page CursorPageParameter) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	var ids []string
	for _, a := range cursorSlice(f.filtered(tag, author, favoritedBy), page) {
		ids = append(ids, a.ID)
	}
	return ids, nil
}

func (f *fakeArticles) FindArticlesOfAuthorsWithCursor(ctx context.Context, authorIDs []string, page CursorPageParameter) ([]ArticleData, error) {
	return cursorSlice(f.ofAuthors(authorIDs), page), nil
}

type fakeRelations struct {
	follows map[string]map[string]struct{}
	err     error
	calls   int
}

func (r *fakeRelations) IsUserFollowing(ctx context.Context, userID, other string) (bool, error) {
	r.calls++
	_, ok := r.follows[userID][other]
	return ok, r.err
}

func (r *fakeRelations) FollowingAuthors(ctx context.Context, userID string, authorIDs []string) (map[string]struct{}, error) {
	r.calls++
	out := map[string]struct{}{}
	for _, id := range authorIDs {
		if _, ok := r.follows[userID][id]; ok {
			out[id] = struct{}{}
		}
	}
	return out, r.err
}

func (r *fakeRelations) FollowedUsers(ctx context.Context, userID string) ([]string, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	var out []string
	for id := range r.follows[userID] {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

type fakeFavorites struct {
	favs  map[string]map[string]struct{} // userID -> articleIDs
	calls int
	err   error
}

func (f *fakeFavorites) count(articleID string) int {
	n := 0
	for _, s := range f.favs {
		if _, ok := s[articleID]; ok {
			n++
		}
	}
	return n
}

func (f *fakeFavorites) IsUserFavorite(ctx context.Context, userID, articleID string) (bool, error) {
	f.calls++
	_, ok := f.favs[userID][articleID]
	return ok, f.err
}

func (f *fakeFavorites) ArticleFavoriteCount(ctx context.Context, articleID string) (int, error) {
	f.calls++
	return f.count(articleID), f.err
}

func (f *fakeFavorites) ArticlesFavoriteCount(ctx context.Context, ids []string) (map[string]int, error) {
	f.calls++
	out := map[string]int{}
	for _, id := range ids {
		if n := f.count(id); n > 0 { // omit zeros on purpose
			out[id] = n
		}
	}
	return out, f.err
}

func (f *fakeFavorites) UserFavorites(ctx context.Context, ids []string, user *core.User) (map[string]struct{}, error) {
	f.calls++
	out := map[string]struct{}{}
	for _, id := range ids {
		if _, ok := f.favs[user.ID][id]; ok {
			out[id] = struct{}{}
		}
	}
	return out, f.err
}

// ---- fixtures ----

var (
	base   = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	author = &core.User{ID: "u-author", Username: "author"}
	other  = &core.User{ID: "u-other", Username: "other"}
)

type env struct {
	a   *fakeArticles
	r   *fakeRelations
	f   *fakeFavorites
	svc *ArticleQueryService
}

func newEnv() *env {
	prof := func(u *core.User) *ProfileData { p := NewProfileData(u.ID, u.Username, "", "", false); return &p }
	older := NewArticleData("a1", "old", "Old", "d", "b", false, 0, base, base, []string{"java"}, prof(author))
	newer := NewArticleData("a2", "new", "New", "d", "b", false, 0, base.Add(time.Hour), base.Add(time.Hour), []string{"spring"}, prof(other))
	e := &env{
		a: &fakeArticles{data: []ArticleData{newer, older}, favoritedBy: map[string]map[string]struct{}{}},
		r: &fakeRelations{follows: map[string]map[string]struct{}{}},
		f: &fakeFavorites{favs: map[string]map[string]struct{}{}},
	}
	e.svc = NewArticleQueryService(e.a, e.r, e.f)
	return e
}

func (e *env) favorite(u *core.User, articleID string) {
	if e.f.favs[u.ID] == nil {
		e.f.favs[u.ID] = map[string]struct{}{}
	}
	e.f.favs[u.ID][articleID] = struct{}{}
	if e.a.favoritedBy[u.Username] == nil {
		e.a.favoritedBy[u.Username] = map[string]struct{}{}
	}
	e.a.favoritedBy[u.Username][articleID] = struct{}{}
}

func (e *env) follow(u *core.User, target string) {
	if e.r.follows[u.ID] == nil {
		e.r.follows[u.ID] = map[string]struct{}{}
	}
	e.r.follows[u.ID][target] = struct{}{}
}

// ---- tests ----

func TestFindByID(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		user      *core.User
		setup     func(*env)
		wantOK    bool
		wantCount int
		wantFav   bool
		wantFollow bool
		noEnrich  bool
	}{
		{name: "viewer is author no favorites", id: "a1", user: author, wantOK: true},
		{name: "favorited by viewer", id: "a1", user: other, setup: func(e *env) { e.favorite(other, "a1") }, wantOK: true, wantCount: 1, wantFav: true},
		{name: "viewer follows author", id: "a1", user: other, setup: func(e *env) { e.follow(other, author.ID) }, wantOK: true, wantFollow: true},
		{name: "not found", id: "missing", user: author, wantOK: false},
		{name: "nil user", id: "a1", user: nil, setup: func(e *env) { e.favorite(other, "a1") }, wantOK: true, noEnrich: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv()
			if tc.setup != nil {
				tc.setup(e)
			}
			got, ok, err := e.svc.FindByID(context.Background(), tc.id, tc.user)
			if err != nil {
				t.Fatal(err)
			}
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if tc.noEnrich {
				if e.f.calls != 0 || e.r.calls != 0 {
					t.Fatalf("enrichment called without viewer")
				}
			}
			if got.FavoritesCount != tc.wantCount || got.Favorited != tc.wantFav || got.Author.Following != tc.wantFollow {
				t.Fatalf("got count=%d fav=%v follow=%v", got.FavoritesCount, got.Favorited, got.Author.Following)
			}
			if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
				t.Fatal("timestamps zero")
			}
			if len(got.TagList) != 1 || got.TagList[0] != "java" {
				t.Fatalf("tags %v", got.TagList)
			}
		})
	}
}

func TestFindByIDErrors(t *testing.T) {
	boom := errors.New("boom")
	e := newEnv()
	e.a.err = boom
	if _, _, err := e.svc.FindByID(context.Background(), "a1", nil); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	e = newEnv()
	e.f.err = boom
	if _, ok, err := e.svc.FindByID(context.Background(), "a1", author); !errors.Is(err, boom) || ok {
		t.Fatalf("got %v %v", ok, err)
	}
}

func TestFindBySlug(t *testing.T) {
	tests := []struct {
		name   string
		slug   string
		user   *core.User
		wantOK bool
	}{
		{"not found", "nope", other, false},
		{"found with viewer", "old", other, true},
		{"found without viewer", "old", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv()
			e.favorite(other, "a1")
			e.follow(other, author.ID)
			got, ok, err := e.svc.FindBySlug(context.Background(), tc.slug, tc.user)
			if err != nil || ok != tc.wantOK {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
			if !ok {
				return
			}
			if got.ID != "a1" {
				t.Fatalf("id %s", got.ID)
			}
			want := tc.user != nil
			if got.Favorited != want || got.Author.Following != want {
				t.Fatalf("fav=%v follow=%v", got.Favorited, got.Author.Following)
			}
			if want && got.FavoritesCount != 1 {
				t.Fatalf("count %d", got.FavoritesCount)
			}
		})
	}
}

func TestFindRecentArticles(t *testing.T) {
	var pg Page
	tests := []struct {
		name                  string
		tag, auth, favBy      string
		user                  *core.User
		setup                 func(*env)
		wantCount, wantLen    int
		wantFirst             string
		wantFav, wantFollow   bool
		wantFavCount          int
	}{
		{name: "default page", wantCount: 2, wantLen: 2, wantFirst: "a2"},
		{name: "beyond data", setup: func(e *env) { e.a.pageEmpty = true }, wantCount: 2, wantLen: 0},
		{name: "by author", auth: "author", wantCount: 1, wantLen: 1, wantFirst: "a1"},
		{name: "favorited by viewer", favBy: "other", user: other, setup: func(e *env) { e.favorite(other, "a1") },
			wantCount: 1, wantLen: 1, wantFirst: "a1", wantFav: true, wantFavCount: 1},
		{name: "by tag", tag: "spring", wantCount: 1, wantLen: 1, wantFirst: "a2"},
		{name: "no tag", tag: "notag", wantCount: 0, wantLen: 0},
		{name: "viewer follows", auth: "author", user: other, setup: func(e *env) { e.follow(other, author.ID) },
			wantCount: 1, wantLen: 1, wantFirst: "a1", wantFollow: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv()
			if tc.setup != nil {
				tc.setup(e)
			}
			got, err := e.svc.FindRecentArticles(context.Background(), tc.tag, tc.auth, tc.favBy, pg, tc.user)
			if err != nil {
				t.Fatal(err)
			}
			if got.Count != tc.wantCount || len(got.Articles) != tc.wantLen {
				t.Fatalf("count=%d len=%d", got.Count, len(got.Articles))
			}
			if tc.wantLen == 0 {
				return
			}
			a := got.Articles[0]
			if a.ID != tc.wantFirst || a.Favorited != tc.wantFav || a.Author.Following != tc.wantFollow || a.FavoritesCount != tc.wantFavCount {
				t.Fatalf("got %+v following=%v", a, a.Author.Following)
			}
		})
	}
}

func TestFindRecentArticlesWithCursor(t *testing.T) {
	ctx := context.Background()
	e := newEnv()

	next := NewCursorPageParameter(nil, 20, DirectionNext)
	p, err := e.svc.FindRecentArticlesWithCursor(ctx, "", "", "", next, nil)
	if err != nil || len(p.Data()) != 2 || p.Data()[0].ID != "a2" || p.HasNext() {
		t.Fatalf("next: %v %+v", err, p)
	}
	end, ok := p.EndCursor()
	if !ok {
		t.Fatal("no end cursor")
	}
	ct, err := ParseDateTimeCursor(end)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := e.svc.FindRecentArticlesWithCursor(ctx, "", "", "", NewCursorPageParameter(ct, 20, DirectionNext), nil)
	if err != nil || len(p2.Data()) != 0 || p2.HasNext() {
		t.Fatalf("after end: %v", err)
	}
	if _, ok := p2.StartCursor(); ok {
		t.Fatal("start cursor should be absent")
	}

	prev, err := e.svc.FindRecentArticlesWithCursor(ctx, "", "", "", NewCursorPageParameter(nil, 20, DirectionPrev), nil)
	if err != nil || len(prev.Data()) != 2 || prev.Data()[0].ID != "a2" {
		t.Fatalf("prev: %v", err)
	}

	// more than limit
	e = newEnv()
	for i := 0; i < 5; i++ {
		ts := base.Add(time.Duration(10+i) * time.Hour)
		e.a.data = append([]ArticleData{NewArticleData(string(rune('m'+i)), "s", "t", "d", "b", false, 0, ts, ts, nil, nil)}, e.a.data...)
	}
	for _, dir := range []Direction{DirectionNext, DirectionPrev} {
		pp, err := e.svc.FindRecentArticlesWithCursor(ctx, "", "", "", NewCursorPageParameter(nil, 3, dir), nil)
		if err != nil || len(pp.Data()) != 3 {
			t.Fatalf("limit %v: %v", dir, err)
		}
		if dir == DirectionNext && !pp.HasNext() || dir == DirectionPrev && !pp.HasPrevious() {
			t.Fatalf("hasExtra false for %v", dir)
		}
	}

	e = newEnv()
	e.a.err = errors.New("x")
	if _, err := e.svc.FindRecentArticlesWithCursor(ctx, "", "", "", next, nil); err == nil {
		t.Fatal("want error")
	}
}

func TestFindUserFeed(t *testing.T) {
	var pg Page
	tests := []struct {
		name      string
		user      *core.User
		follow    bool
		wantErr   error
		wantCount int
	}{
		{"nil user", nil, false, ErrFeedRequiresUser, 0},
		{"follows nobody", other, false, nil, 0},
		{"follows author", other, true, nil, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv()
			if tc.follow {
				e.follow(other, author.ID)
			}
			got, err := e.svc.FindUserFeed(context.Background(), tc.user, pg)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err %v", err)
			}
			if err != nil {
				return
			}
			if got.Count != tc.wantCount || len(got.Articles) != tc.wantCount {
				t.Fatalf("count=%d len=%d", got.Count, len(got.Articles))
			}
			for _, a := range got.Articles {
				if !a.Author.Following {
					t.Fatal("following not set")
				}
			}
		})
	}
}

func TestFindUserFeedWithCursor(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	if _, err := e.svc.FindUserFeedWithCursor(ctx, nil, NewCursorPageParameter(nil, 2, DirectionNext)); !errors.Is(err, ErrFeedRequiresUser) {
		t.Fatalf("err %v", err)
	}
	p, err := e.svc.FindUserFeedWithCursor(ctx, other, NewCursorPageParameter(nil, 2, DirectionNext))
	if err != nil || len(p.Data()) != 0 || p.HasNext() || p.HasPrevious() {
		t.Fatalf("empty feed: %v", err)
	}

	prof := NewProfileData(author.ID, author.Username, "", "", false)
	for i := 0; i < 4; i++ {
		ts := base.Add(time.Duration(10+i) * time.Hour)
		pr := prof
		e.a.data = append([]ArticleData{NewArticleData(string(rune('m'+i)), "s", "t", "d", "b", false, 0, ts, ts, nil, &pr)}, e.a.data...)
	}
	e.follow(other, author.ID)
	for _, dir := range []Direction{DirectionNext, DirectionPrev} {
		p, err := e.svc.FindUserFeedWithCursor(ctx, other, NewCursorPageParameter(nil, 2, dir))
		if err != nil || len(p.Data()) != 2 {
			t.Fatalf("%v: %v", dir, err)
		}
		if dir == DirectionNext && !p.HasNext() || dir == DirectionPrev && !p.HasPrevious() {
			t.Fatalf("hasExtra false %v", dir)
		}
		for _, n := range p.Data() {
			if !n.Author.Following {
				t.Fatal("following false")
			}
		}
		if dir == DirectionPrev && !p.Data()[0].UpdatedAt.After(p.Data()[1].UpdatedAt) {
			t.Fatal("prev page not newest first")
		}
	}
}

func TestArticleNodeCursor(t *testing.T) {
	ts := time.UnixMilli(1700000000123)
	n := ArticleNode{ArticleData: ArticleData{UpdatedAt: ts}}
	if n.Cursor() != "1700000000123" {
		t.Fatalf("cursor %s", n.Cursor())
	}
}