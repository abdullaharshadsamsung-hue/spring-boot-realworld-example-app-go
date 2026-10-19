package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"migrated-app/internal/core"
)

// ---------------------------------------------------------------------------
// In-memory fake SQL driver understanding exactly the statements of
// article_repo.go. Supports transactions via snapshot/restore.
// ---------------------------------------------------------------------------

type artFakeArticleRow struct {
	id, slug, title, description, body, userID string
	createdAt, updatedAt                       time.Time
}

type artFakeLink struct{ articleID, tagID string }

type artFakeState struct {
	articles map[string]artFakeArticleRow
	order    []string
	tags     []core.Tag
	links    []artFakeLink
}

func (s *artFakeState) clone() *artFakeState {
	c := &artFakeState{articles: map[string]artFakeArticleRow{}}
	for k, v := range s.articles {
		c.articles[k] = v
	}
	c.order = append([]string(nil), s.order...)
	c.tags = append([]core.Tag(nil), s.tags...)
	c.links = append([]artFakeLink(nil), s.links...)
	return c
}

type artFakeDB struct {
	mu       sync.Mutex
	state    *artFakeState
	snapshot *artFakeState
	failOn   string
	beginErr error
}

func newArtFakeDB() *artFakeDB {
	return &artFakeDB{state: &artFakeState{articles: map[string]artFakeArticleRow{}}}
}

type artFakeConnector struct{ db *artFakeDB }

func (c artFakeConnector) Connect(context.Context) (driver.Conn, error) {
	return &artFakeConn{db: c.db}, nil
}
func (c artFakeConnector) Driver() driver.Driver { return artFakeDriver{} }

type artFakeDriver struct{}

func (artFakeDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type artFakeConn struct{ db *artFakeDB }

func (c *artFakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unsupported")
}
func (c *artFakeConn) Close() error { return nil }
func (c *artFakeConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *artFakeConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	c.db.mu.Lock()
	defer c.db.mu.Unlock()
	if c.db.beginErr != nil {
		return nil, c.db.beginErr
	}
	c.db.snapshot = c.db.state.clone()
	return &artFakeTx{db: c.db}, nil
}

type artFakeTx struct{ db *artFakeDB }

func (t *artFakeTx) Commit() error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	t.db.snapshot = nil
	return nil
}

func (t *artFakeTx) Rollback() error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	if t.db.snapshot != nil {
		t.db.state = t.db.snapshot
		t.db.snapshot = nil
	}
	return nil
}

func artArgStr(args []driver.NamedValue, i int) string {
	s, _ := args[i].Value.(string)
	return s
}

func artArgTime(args []driver.NamedValue, i int) time.Time {
	t, _ := args[i].Value.(time.Time)
	return t
}

func (c *artFakeConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	d := c.db
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failOn != "" && strings.Contains(query, d.failOn) {
		return nil, errors.New("injected exec failure")
	}
	s := d.state
	switch {
	case query == insertTagSQL:
		for _, t := range s.tags {
			if t.Name == artArgStr(args, 1) {
				return nil, errors.New("duplicate tag name")
			}
		}
		s.tags = append(s.tags, core.Tag{ID: artArgStr(args, 0), Name: artArgStr(args, 1)})
	case query == insertArticleTagSQL:
		s.links = append(s.links, artFakeLink{artArgStr(args, 0), artArgStr(args, 1)})
	case query == insertArticleSQL:
		id := artArgStr(args, 0)
		if _, ok := s.articles[id]; ok {
			return nil, errors.New("duplicate article id")
		}
		for _, a := range s.articles {
			if a.slug == artArgStr(args, 1) {
				return nil, errors.New("duplicate slug")
			}
		}
		s.articles[id] = artFakeArticleRow{
			id: id, slug: artArgStr(args, 1), title: artArgStr(args, 2), description: artArgStr(args, 3),
			body: artArgStr(args, 4), userID: artArgStr(args, 5), createdAt: artArgTime(args, 6), updatedAt: artArgTime(args, 7),
		}
		s.order = append(s.order, id)
	case query == deleteArticleSQL:
		delete(s.articles, artArgStr(args, 0))
	case strings.HasPrefix(query, "UPDATE articles SET "):
		rest := strings.TrimPrefix(query, "UPDATE articles SET ")
		idx := strings.Index(rest, " WHERE id = $")
		if idx < 0 {
			return nil, errors.New("bad update")
		}
		id := artArgStr(args, len(args)-1)
		a, ok := s.articles[id]
		if !ok {
			return driver.RowsAffected(0), nil
		}
		for _, part := range strings.Split(rest[:idx], ", ") {
			kv := strings.SplitN(part, " = $", 2)
			n, err := strconv.Atoi(kv[1])
			if err != nil {
				return nil, err
			}
			v := artArgStr(args, n-1)
			switch kv[0] {
			case "title":
				a.title = v
			case "slug":
				a.slug = v
			case "description":
				a.description = v
			case "body":
				a.body = v
			default:
				return nil, errors.New("unknown column " + kv[0])
			}
		}
		s.articles[id] = a
	default:
		return nil, errors.New("unexpected exec: " + query)
	}
	return driver.RowsAffected(1), nil
}

func (c *artFakeConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	d := c.db
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failOn != "" && strings.Contains(query, d.failOn) {
		return nil, errors.New("injected query failure")
	}
	s := d.state
	switch query {
	case findTagByNameSQL:
		r := &artFakeRows{cols: []string{"id", "name"}}
		for _, t := range s.tags {
			if t.Name == artArgStr(args, 0) {
				r.data = append(r.data, []driver.Value{t.ID, t.Name})
				break
			}
		}
		return r, nil
	case findArticleByIDSQL, findArticleBySlugSQL:
		r := &artFakeRows{cols: make([]string, 10)}
		key := artArgStr(args, 0)
		for _, id := range s.order {
			a, ok := s.articles[id]
			if !ok {
				continue
			}
			if (query == findArticleByIDSQL && a.id != key) || (query == findArticleBySlugSQL && a.slug != key) {
				continue
			}
			base := []driver.Value{a.id, a.slug, a.title, a.description, a.body, a.userID, a.createdAt, a.updatedAt}
			matched := false
			for _, l := range s.links {
				if l.articleID != a.id {
					continue
				}
				var tid, tname driver.Value
				for _, t := range s.tags {
					if t.ID == l.tagID {
						tid, tname = t.ID, t.Name
					}
				}
				row := append(append([]driver.Value{}, base...), tid, tname)
				r.data = append(r.data, row)
				matched = true
			}
			if !matched {
				r.data = append(r.data, append(append([]driver.Value{}, base...), nil, nil))
			}
		}
		return r, nil
	}
	return nil, errors.New("unexpected query: " + query)
}

type artFakeRows struct {
	cols []string
	data [][]driver.Value
	i    int
}

func (r *artFakeRows) Columns() []string { return r.cols }
func (r *artFakeRows) Close() error      { return nil }
func (r *artFakeRows) Next(dest []driver.Value) error {
	if r.i >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.i])
	r.i++
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newArtTestRepo(t *testing.T) (*ArticleRepo, *artFakeDB) {
	t.Helper()
	f := newArtFakeDB()
	db := sql.OpenDB(artFakeConnector{db: f})
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return NewArticleRepository(db), f
}

var artFixedTime = time.Date(2024, 1, 2, 3, 4, 5, 6_000_000, time.UTC)

func newArtTestArticle(t *testing.T, title string, tags []string) *core.Article {
	t.Helper()
	a, err := core.NewArticle(title, "desc", "body", tags, "user-aisensiy", artFixedTime)
	if err != nil {
		t.Fatalf("NewArticle: %v", err)
	}
	return a
}

func artTagNames(tags []core.Tag) map[string]bool {
	m := map[string]bool{}
	for _, tg := range tags {
		m[tg.Name] = true
	}
	return m
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestArtFakeStore(t *testing.T) {
	t.Run("SaveNewArticleFindByID", artCaseSaveNewArticleFindByID)
	t.Run("SaveNoTagsReturnsEmptyTags", artCaseSaveNoTagsReturnsEmptyTags)
	t.Run("SaveReusesExistingTag", artCaseSaveReusesExistingTag)
	t.Run("SaveTransactionRollback", artCaseSaveTransactionRollback)
	t.Run("SaveExistingUpdates", artCaseSaveExistingUpdates)
	t.Run("SaveExistingAllEmptyIsNoop", artCaseSaveExistingAllEmptyIsNoop)
	t.Run("FindNotFound", artCaseFindNotFound)
	t.Run("Remove", artCaseRemove)
	t.Run("NilArticle", artCaseNilArticle)
	t.Run("StorageErrors", artCaseStorageErrors)
	t.Run("SaveUpdateFails", artCaseSaveUpdateFails)
}

func artCaseSaveNewArticleFindByID(t *testing.T) {
	repo, f := newArtTestRepo(t)
	ctx := context.Background()
	a := newArtTestArticle(t, "test", []string{"java", "spring"})

	if err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.ID != a.ID {
		t.Fatalf("got id %q want %q", got.ID, a.ID)
	}
	checks := []struct{ name, got, want string }{
		{"slug", got.Slug, a.Slug},
		{"title", got.Title, "test"},
		{"description", got.Description, "desc"},
		{"body", got.Body, "body"},
		{"userID", got.UserID, "user-aisensiy"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if !got.CreatedAt.Equal(artFixedTime) || !got.UpdatedAt.Equal(artFixedTime) {
		t.Errorf("times = %v/%v, want %v", got.CreatedAt, got.UpdatedAt, artFixedTime)
	}
	names := artTagNames(got.Tags)
	if len(got.Tags) != 2 || !names["java"] || !names["spring"] {
		t.Errorf("tags = %v, want java and spring", got.Tags)
	}
	if len(f.state.tags) != 2 || len(f.state.links) != 2 || len(f.state.articles) != 1 {
		t.Errorf("rows: tags=%d links=%d articles=%d", len(f.state.tags), len(f.state.links), len(f.state.articles))
	}
}

func artCaseSaveNoTagsReturnsEmptyTags(t *testing.T) {
	repo, _ := newArtTestRepo(t)
	ctx := context.Background()
	a := newArtTestArticle(t, "no tags", nil)
	if err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Tags == nil || len(got.Tags) != 0 {
		t.Errorf("tags = %#v, want empty non-nil", got.Tags)
	}
}

func artCaseSaveReusesExistingTag(t *testing.T) {
	repo, f := newArtTestRepo(t)
	ctx := context.Background()
	f.state.tags = append(f.state.tags, core.Tag{ID: "existing-java", Name: "java"})

	a := newArtTestArticle(t, "reuse", []string{"java"})
	if err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(f.state.tags) != 1 {
		t.Fatalf("tags duplicated: %v", f.state.tags)
	}
	if len(f.state.links) != 1 || f.state.links[0].tagID != "existing-java" {
		t.Fatalf("links = %v, want link to existing-java", f.state.links)
	}
	got, err := repo.FindByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if len(got.Tags) != 1 || got.Tags[0].ID != "existing-java" {
		t.Errorf("tags = %v", got.Tags)
	}
}

func artCaseSaveTransactionRollback(t *testing.T) {
	repo, f := newArtTestRepo(t)
	ctx := context.Background()
	first := newArtTestArticle(t, "test", []string{"java", "spring"})
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	second := newArtTestArticle(t, "test", []string{"java", "spring", "other"})
	if err := repo.Save(ctx, second); err == nil {
		t.Fatal("expected error for duplicate slug")
	}
	for _, tg := range f.state.tags {
		if tg.Name == "other" {
			t.Errorf("tag 'other' should have been rolled back")
		}
	}
	if len(f.state.links) != 2 {
		t.Errorf("links = %d, want 2 (no partial rows)", len(f.state.links))
	}
	if _, err := repo.FindByID(ctx, second.ID); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("second article should not exist, err=%v", err)
	}
}

func artCaseSaveExistingUpdates(t *testing.T) {
	repo, f := newArtTestRepo(t)
	ctx := context.Background()
	a := newArtTestArticle(t, "test", []string{"java"})
	if err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save: %v", err)
	}
	upd := &core.Article{ID: a.ID, Title: "new test 2", Slug: "new-test-2"}
	upd.Tags = append(upd.Tags, core.Tag{ID: "x", Name: "ignored"})
	if err := repo.Save(ctx, upd); err != nil {
		t.Fatalf("Save update: %v", err)
	}
	got, err := repo.FindBySlug(ctx, "new-test-2")
	if err != nil {
		t.Fatalf("FindBySlug: %v", err)
	}
	if got.Title != "new test 2" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Body != "body" {
		t.Errorf("body = %q, want unchanged 'body'", got.Body)
	}
	if got.Description != "desc" {
		t.Errorf("description = %q, want unchanged", got.Description)
	}
	if !got.UpdatedAt.Equal(artFixedTime) {
		t.Errorf("updated_at changed: %v", got.UpdatedAt)
	}
	if len(f.state.tags) != 1 || len(f.state.links) != 1 {
		t.Errorf("update touched tags: tags=%v links=%v", f.state.tags, f.state.links)
	}
}

func artCaseSaveExistingAllEmptyIsNoop(t *testing.T) {
	repo, _ := newArtTestRepo(t)
	ctx := context.Background()
	a := newArtTestArticle(t, "keep", nil)
	if err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save: %v", err)
	}
	empty := &core.Article{ID: a.ID}
	if err := repo.Save(ctx, empty); err != nil {
		t.Fatalf("Save empty: %v", err)
	}
	got, err := repo.FindByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Title != "keep" || got.Slug != a.Slug || got.Body != "body" {
		t.Errorf("article changed: %+v", got)
	}
}

func artCaseFindNotFound(t *testing.T) {
	repo, _ := newArtTestRepo(t)
	ctx := context.Background()
	tests := []struct {
		name string
		find func() (*core.Article, error)
	}{
		{"FindByID", func() (*core.Article, error) { return repo.FindByID(ctx, "missing") }},
		{"FindBySlug", func() (*core.Article, error) { return repo.FindBySlug(ctx, "missing") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := tt.find()
			if !errors.Is(err, core.ErrNotFound) {
				t.Fatalf("err = %v, want ErrNotFound", err)
			}
			if a != nil {
				t.Fatalf("article = %+v, want nil", a)
			}
		})
	}
}

func artCaseRemove(t *testing.T) {
	repo, _ := newArtTestRepo(t)
	ctx := context.Background()
	a := newArtTestArticle(t, "to remove", []string{"x"})
	if err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Remove(ctx, a); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := repo.FindByID(ctx, a.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func artCaseNilArticle(t *testing.T) {
	repo, _ := newArtTestRepo(t)
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
	}{
		{"Save", func() error { return repo.Save(ctx, nil) }},
		{"Remove", func() error { return repo.Remove(ctx, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, errNilArticle) {
				t.Fatalf("err = %v, want errNilArticle", err)
			}
		})
	}
}

func artCaseStorageErrors(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		failOn   string
		beginErr error
		call     func(r *ArticleRepo, a *core.Article) error
	}{
		{"save begin fails", "", errors.New("begin boom"), func(r *ArticleRepo, a *core.Article) error { return r.Save(ctx, a) }},
		{"save find fails", "FROM articles a", nil, func(r *ArticleRepo, a *core.Article) error { return r.Save(ctx, a) }},
		{"save tag lookup fails", "FROM tags WHERE", nil, func(r *ArticleRepo, a *core.Article) error { return r.Save(ctx, a) }},
		{"save tag insert fails", "INSERT INTO tags", nil, func(r *ArticleRepo, a *core.Article) error { return r.Save(ctx, a) }},
		{"save link fails", "INSERT INTO article_tags", nil, func(r *ArticleRepo, a *core.Article) error { return r.Save(ctx, a) }},
		{"save article insert fails", "INSERT INTO articles", nil, func(r *ArticleRepo, a *core.Article) error { return r.Save(ctx, a) }},
		{"find by id fails", "FROM articles a", nil, func(r *ArticleRepo, a *core.Article) error { _, err := r.FindByID(ctx, a.ID); return err }},
		{"find by slug fails", "FROM articles a", nil, func(r *ArticleRepo, a *core.Article) error { _, err := r.FindBySlug(ctx, a.Slug); return err }},
		{"remove fails", "DELETE FROM articles", nil, func(r *ArticleRepo, a *core.Article) error { return r.Remove(ctx, a) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, f := newArtTestRepo(t)
			f.failOn = tt.failOn
			f.beginErr = tt.beginErr
			a := newArtTestArticle(t, "err", []string{"java"})
			err := tt.call(repo, a)
			if err == nil {
				t.Fatal("expected error")
			}
			if errors.Is(err, core.ErrNotFound) {
				t.Fatalf("storage error should not be ErrNotFound: %v", err)
			}
			if len(f.state.tags) != 0 || len(f.state.links) != 0 || len(f.state.articles) != 0 {
				t.Errorf("partial rows remain: %+v", f.state)
			}
		})
	}
}

func artCaseSaveUpdateFails(t *testing.T) {
	repo, f := newArtTestRepo(t)
	ctx := context.Background()
	a := newArtTestArticle(t, "orig", nil)
	if err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.failOn = "UPDATE articles"
	a.Title = "changed"
	if err := repo.Save(ctx, a); err == nil {
		t.Fatal("expected update error")
	}
	f.failOn = ""
	got, err := repo.FindByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Title != "orig" {
		t.Errorf("title = %q, want orig", got.Title)
	}
}