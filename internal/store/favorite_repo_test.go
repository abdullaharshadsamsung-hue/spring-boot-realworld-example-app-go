package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"migrated-app/internal/core"
)

// ---- in-memory fake SQL driver (mock of the DB) ----

type favKey struct{ article, user string }

type fakeFavStore struct {
	mu       sync.Mutex
	rows     map[favKey]bool
	execErr  error
	queryErr error
	queries  []string
}

var (
	fakeStoresMu sync.Mutex
	fakeStores   = map[string]*fakeFavStore{}
)

func init() {
	sql.Register("favfake", fakeFavDriver{})
}

type fakeFavDriver struct{}

func (fakeFavDriver) Open(name string) (driver.Conn, error) {
	fakeStoresMu.Lock()
	defer fakeStoresMu.Unlock()
	s, ok := fakeStores[name]
	if !ok {
		return nil, fmt.Errorf("no fake store %q", name)
	}
	return &fakeFavConn{s: s}, nil
}

type fakeFavConn struct{ s *fakeFavStore }

func (c *fakeFavConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("prepare unsupported") }
func (c *fakeFavConn) Close() error                         { return nil }
func (c *fakeFavConn) Begin() (driver.Tx, error)            { return nil, errors.New("tx unsupported") }

func argStrings(args []driver.NamedValue) (string, string, error) {
	if len(args) != 2 {
		return "", "", fmt.Errorf("want 2 args, got %d", len(args))
	}
	a, ok1 := args[0].Value.(string)
	u, ok2 := args[1].Value.(string)
	if !ok1 || !ok2 {
		return "", "", errors.New("non-string args")
	}
	return a, u, nil
}

func (c *fakeFavConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	c.s.queries = append(c.s.queries, query)
	if c.s.execErr != nil {
		return nil, c.s.execErr
	}
	a, u, err := argStrings(args)
	if err != nil {
		return nil, err
	}
	k := favKey{a, u}
	switch {
	case strings.HasPrefix(strings.TrimSpace(query), "INSERT INTO article_favorites"):
		if c.s.rows[k] {
			if strings.Contains(query, "ON CONFLICT") {
				return driver.RowsAffected(0), nil
			}
			return nil, errors.New("duplicate key")
		}
		c.s.rows[k] = true
		return driver.RowsAffected(1), nil
	case strings.HasPrefix(strings.TrimSpace(query), "DELETE FROM article_favorites"):
		if c.s.rows[k] {
			delete(c.s.rows, k)
			return driver.RowsAffected(1), nil
		}
		return driver.RowsAffected(0), nil
	}
	return nil, fmt.Errorf("unexpected exec: %s", query)
}

func (c *fakeFavConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	c.s.queries = append(c.s.queries, query)
	if c.s.queryErr != nil {
		return nil, c.s.queryErr
	}
	if !strings.HasPrefix(strings.TrimSpace(query), "SELECT article_id, user_id") {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
	a, u, err := argStrings(args)
	if err != nil {
		return nil, err
	}
	r := &fakeFavRows{}
	if c.s.rows[favKey{a, u}] {
		r.data = [][]driver.Value{{a, u}}
	}
	return r, nil
}

type fakeFavRows struct {
	data [][]driver.Value
	i    int
}

func (r *fakeFavRows) Columns() []string { return []string{"article_id", "user_id"} }
func (r *fakeFavRows) Close() error      { return nil }
func (r *fakeFavRows) Next(dest []driver.Value) error {
	if r.i >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.i])
	r.i++
	return nil
}

func newFakeRepo(t *testing.T) (*FavoriteRepo, *fakeFavStore) {
	t.Helper()
	s := &fakeFavStore{rows: map[favKey]bool{}}
	name := t.Name()
	fakeStoresMu.Lock()
	fakeStores[name] = s
	fakeStoresMu.Unlock()
	db, err := sql.Open("favfake", name)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		fakeStoresMu.Lock()
		delete(fakeStores, name)
		fakeStoresMu.Unlock()
	})
	return NewFavoriteRepo(db), s
}

func (s *fakeFavStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rows)
}

// ---- tests ----

func TestFavoriteRepo_Save(t *testing.T) {
	tests := []struct {
		name      string
		preexist  bool
		saves     int
		wantCount int
	}{
		{"new favorite inserted", false, 1, 1},
		{"duplicate save is idempotent", true, 1, 1},
		{"repeated saves keep one row", false, 3, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, s := newFakeRepo(t)
			ctx := context.Background()
			fav := core.NewArticleFavorite("123", "456")
			if tc.preexist {
				s.rows[favKey{"123", "456"}] = true
			}
			for i := 0; i < tc.saves; i++ {
				if err := repo.Save(ctx, fav); err != nil {
					t.Fatalf("Save: %v", err)
				}
			}
			if got := s.count(); got != tc.wantCount {
				t.Fatalf("rows = %d, want %d", got, tc.wantCount)
			}
			got, found, err := repo.Find(ctx, "123", "456")
			if err != nil || !found {
				t.Fatalf("Find after save: found=%v err=%v", found, err)
			}
			if got != fav {
				t.Fatalf("Find = %+v, want %+v", got, fav)
			}
		})
	}
}

func TestFavoriteRepoFake_Find(t *testing.T) {
	tests := []struct {
		name      string
		stored    []favKey
		article   string
		user      string
		wantFound bool
	}{
		{"exists", []favKey{{"a1", "u1"}}, "a1", "u1", true},
		{"absent empty store", nil, "a1", "u1", false},
		{"different user", []favKey{{"a1", "u1"}}, "a1", "u2", false},
		{"different article", []favKey{{"a1", "u1"}}, "a2", "u1", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, s := newFakeRepo(t)
			for _, k := range tc.stored {
				s.rows[k] = true
			}
			before := s.count()
			got, found, err := repo.Find(context.Background(), tc.article, tc.user)
			if err != nil {
				t.Fatalf("Find err: %v", err)
			}
			if found != tc.wantFound {
				t.Fatalf("found = %v, want %v", found, tc.wantFound)
			}
			if tc.wantFound {
				want := core.NewArticleFavorite(tc.article, tc.user)
				if got != want {
					t.Fatalf("got %+v, want %+v", got, want)
				}
			} else if got != (core.ArticleFavorite{}) {
				t.Fatalf("expected zero favorite, got %+v", got)
			}
			if s.count() != before {
				t.Fatalf("Find mutated store")
			}
		})
	}
}