package core

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"
)

var articleUUIDV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var errArticleFakeNotFound = errors.New("not found")

// fakeArticleRepo is an in-memory ArticleRepository used to validate the port contract.
type fakeArticleRepo struct {
	mu       sync.Mutex
	articles map[string]Article
	tags     map[string]Tag // by name
	links    map[string]map[string]struct{}
}

func newFakeArticleRepo() *fakeArticleRepo {
	return &fakeArticleRepo{
		articles: map[string]Article{},
		tags:     map[string]Tag{},
		links:    map[string]map[string]struct{}{},
	}
}

var _ ArticleRepository = (*fakeArticleRepo)(nil)

func (r *fakeArticleRepo) Save(_ context.Context, a *Article) error {
	if a == nil {
		return errors.New("nil article")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *a
	cp.Tags = nil
	r.articles[a.ID] = cp
	if r.links[a.ID] == nil {
		r.links[a.ID] = map[string]struct{}{}
	}
	for _, t := range a.Tags {
		if _, ok := r.tags[t.Name]; !ok {
			r.tags[t.Name] = t
		}
		r.links[a.ID][t.Name] = struct{}{}
	}
	return nil
}

func (r *fakeArticleRepo) load(a Article) *Article {
	out := a
	out.Tags = nil
	for name := range r.links[a.ID] {
		out.Tags = append(out.Tags, r.tags[name])
	}
	return &out
}

func (r *fakeArticleRepo) FindByID(_ context.Context, id string) (*Article, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.articles[id]
	if !ok {
		return nil, errArticleFakeNotFound
	}
	return r.load(a), nil
}

func (r *fakeArticleRepo) FindBySlug(_ context.Context, slug string) (*Article, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.articles {
		if a.Slug == slug {
			return r.load(a), nil
		}
	}
	return nil, errArticleFakeNotFound
}

func (r *fakeArticleRepo) Remove(_ context.Context, a *Article) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.articles, a.ID)
	delete(r.links, a.ID)
	return nil
}

func mustArticle(t *testing.T, title string, tags []string) *Article {
	t.Helper()
	a, err := NewArticle(title, "desc", "body", tags, "user-1", time.Time{})
	if err != nil {
		t.Fatalf("NewArticle: %v", err)
	}
	return a
}

func tagNames(tags []Tag) map[string]bool {
	m := map[string]bool{}
	for _, t := range tags {
		m[t.Name] = true
	}
	return m
}

func TestToSlug(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Hello World", "hello-world"},
		{"Tom & Jerry", "tom-jerry"},
		{"it's", "it-s"},
		{"a, b. c?", "a-b-c-"},
		{"say \"hi\"", "say-hi-"},
		{"a|b", "a-b"},
		{"multiple   spaces", "multiple-spaces"},
		{"", ""},
		{"UPPER", "upper"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := ToSlug(tc.in); got != tc.want {
				t.Errorf("ToSlug(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNewTag(t *testing.T) {
	a, err := NewTag("go")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewTag("go")
	c, _ := NewTag("java")
	if !articleUUIDV4Re.MatchString(a.ID) {
		t.Errorf("bad id %q", a.ID)
	}
	if a.ID == b.ID {
		t.Error("ids should differ")
	}
	tests := []struct {
		name string
		x, y Tag
		want bool
	}{
		{"same name", a, b, true},
		{"diff name", a, c, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.x.Equal(tc.y) != tc.want {
				t.Errorf("Equal want %v", tc.want)
			}
		})
	}
	if a.Key() != "go" || a.String() != "go" {
		t.Error("Key/String should be name")
	}
}

func TestNewArticle(t *testing.T) {
	fixed := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		tags     []string
		created  time.Time
		wantTags []string
	}{
		{"dedupe tags", []string{"a", "b", "a"}, fixed, []string{"a", "b"}},
		{"no tags", nil, fixed, nil},
		{"zero time uses now", []string{"x"}, time.Time{}, []string{"x"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := time.Now()
			a, err := NewArticle("Hello World", "d", "b", tc.tags, "u1", tc.created)
			if err != nil {
				t.Fatal(err)
			}
			if !articleUUIDV4Re.MatchString(a.ID) {
				t.Errorf("bad id %q", a.ID)
			}
			if a.Slug != "hello-world" || a.Title != "Hello World" || a.Description != "d" || a.Body != "b" || a.UserID != "u1" {
				t.Errorf("unexpected fields %+v", a)
			}
			if len(a.Tags) != len(tc.wantTags) {
				t.Fatalf("tags %v", a.Tags)
			}
			for i, n := range tc.wantTags {
				if a.Tags[i].Name != n {
					t.Errorf("tag %d=%q want %q", i, a.Tags[i].Name, n)
				}
			}
			if !a.CreatedAt.Equal(a.UpdatedAt) {
				t.Error("created != updated")
			}
			if tc.created.IsZero() {
				if a.CreatedAt.Before(before) {
					t.Error("created should be now")
				}
			} else if !a.CreatedAt.Equal(tc.created) {
				t.Error("created mismatch")
			}
		})
	}
	a1 := mustArticle(t, "x", nil)
	a2 := mustArticle(t, "x", nil)
	if a1.ID == a2.ID {
		t.Error("ids must be unique")
	}
}

func TestArticleUpdate(t *testing.T) {
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name                string
		title, desc, body   string
		wantTitle, wantSlug string
		wantDesc, wantBody  string
		wantUpdatedChanged  bool
	}{
		{"none", "", "", "", "Old Title", "old-title", "od", "ob", false},
		{"title", "New Title", "", "", "New Title", "new-title", "od", "ob", true},
		{"desc", "", "nd", "", "Old Title", "old-title", "nd", "ob", true},
		{"body", "", "", "nb", "Old Title", "old-title", "od", "nb", true},
		{"all", "A B", "nd", "nb", "A B", "a-b", "nd", "nb", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := NewArticle("Old Title", "od", "ob", nil, "u", old)
			a.Update(tc.title, tc.desc, tc.body)
			if a.Title != tc.wantTitle || a.Slug != tc.wantSlug || a.Description != tc.wantDesc || a.Body != tc.wantBody {
				t.Errorf("got %+v", a)
			}
			if changed := !a.UpdatedAt.Equal(old); changed != tc.wantUpdatedChanged {
				t.Errorf("updated changed=%v", changed)
			}
			if !a.CreatedAt.Equal(old) {
				t.Error("created must not change")
			}
		})
	}
	var nilA *Article
	nilA.Update("x", "y", "z") // must not panic
}

func TestArticleEqual(t *testing.T) {
	a := &Article{ID: "1", Title: "a"}
	b := &Article{ID: "1", Title: "b"}
	c := &Article{ID: "2"}
	var n *Article
	tests := []struct {
		name string
		x, y *Article
		want bool
	}{
		{"same id", a, b, true},
		{"diff id", a, c, false},
		{"nil vs nonnil", n, a, false},
		{"nonnil vs nil", a, nil, false},
		{"both nil", n, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.x.Equal(tc.y) != tc.want {
				t.Errorf("want %v", tc.want)
			}
		})
	}
}

func TestRepositorySaveAndFind(t *testing.T) {
	ctx := context.Background()
	repo := newFakeArticleRepo()
	a := mustArticle(t, "First Post", []string{"go", "test"})
	if err := repo.Save(ctx, a); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		find func() (*Article, error)
	}{
		{"by id", func() (*Article, error) { return repo.FindByID(ctx, a.ID) }},
		{"by slug", func() (*Article, error) { return repo.FindBySlug(ctx, "first-post") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.find()
			if err != nil || got == nil {
				t.Fatalf("err=%v got=%v", err, got)
			}
			if got.ID != a.ID || got.Slug != a.Slug || got.Title != a.Title || got.Description != a.Description ||
				got.Body != a.Body || got.UserID != a.UserID {
				t.Errorf("round-trip mismatch %+v", got)
			}
			n := tagNames(got.Tags)
			if len(n) != 2 || !n["go"] || !n["test"] {
				t.Errorf("tags %v", got.Tags)
			}
		})
	}

	missing := []struct {
		name string
		find func() (*Article, error)
	}{
		{"unknown id", func() (*Article, error) { return repo.FindByID(ctx, "nope") }},
		{"unknown slug", func() (*Article, error) { return repo.FindBySlug(ctx, "nope") }},
	}
	for _, tc := range missing {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.find()
			if !errors.Is(err, errArticleFakeNotFound) || got != nil {
				t.Errorf("want not found, got %v %v", got, err)
			}
		})
	}
}

func TestRepositorySaveUpdatesExisting(t *testing.T) {
	ctx := context.Background()
	repo := newFakeArticleRepo()
	a := mustArticle(t, "Old Title", nil)
	_ = repo.Save(ctx, a)
	a.Update("New Title", "nd", "nb")
	_ = repo.Save(ctx, a)
	_ = repo.Save(ctx, a)

	if len(repo.articles) != 1 {
		t.Fatalf("duplicate articles: %d", len(repo.articles))
	}
	got, err := repo.FindByID(ctx, a.ID)
	if err != nil || got.Title != "New Title" || got.Description != "nd" || got.Body != "nb" || got.Slug != "new-title" {
		t.Errorf("got %+v err %v", got, err)
	}
	if s, err := repo.FindBySlug(ctx, "new-title"); err != nil || s.ID != a.ID {
		t.Errorf("new slug lookup failed")
	}
	if _, err := repo.FindBySlug(ctx, "old-title"); !errors.Is(err, errArticleFakeNotFound) {
		t.Errorf("old slug should be missing")
	}
}

func TestRepositoryReusesTags(t *testing.T) {
	ctx := context.Background()
	repo := newFakeArticleRepo()
	a := mustArticle(t, "A", []string{"shared", "one"})
	b := mustArticle(t, "B", []string{"shared", "two"})
	_ = repo.Save(ctx, a)
	_ = repo.Save(ctx, b)
	if len(repo.tags) != 3 {
		t.Errorf("tags duplicated: %d", len(repo.tags))
	}
	got, _ := repo.FindByID(ctx, b.ID)
	n := tagNames(got.Tags)
	if !n["shared"] || !n["two"] || n["one"] {
		t.Errorf("tags %v", got.Tags)
	}
}

func TestRepositoryRemove(t *testing.T) {
	ctx := context.Background()
	repo := newFakeArticleRepo()
	a := mustArticle(t, "Gone", nil)
	keep := mustArticle(t, "Keep", nil)
	_ = repo.Save(ctx, a)
	_ = repo.Save(ctx, keep)
	if err := repo.Remove(ctx, a); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.FindByID(ctx, a.ID); !errors.Is(err, errArticleFakeNotFound) || got != nil {
		t.Error("removed article found by id")
	}
	if _, err := repo.FindBySlug(ctx, "gone"); !errors.Is(err, errArticleFakeNotFound) {
		t.Error("removed article found by slug")
	}
	if got, err := repo.FindByID(ctx, keep.ID); err != nil || got.ID != keep.ID {
		t.Error("other article should remain")
	}
}

func TestCanWriteArticleWithArticle(t *testing.T) {
	a := mustArticle(t, "T", nil)
	tests := []struct {
		name string
		u    *User
		want bool
	}{
		{"author", &User{ID: "user-1"}, true},
		{"other", &User{ID: "x"}, false},
		{"nil", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if CanWriteArticle(tc.u, a) != tc.want {
				t.Errorf("want %v", tc.want)
			}
		})
	}
}