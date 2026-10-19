package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"migrated-app/internal/core"
)

type memRow struct {
	id, body, userID, articleID string
	createdAt                   time.Time
}

type memStore struct {
	mu      sync.Mutex
	rows    map[string]memRow
	failErr error
}

type memConnector struct{ s *memStore }

func (c memConnector) Connect(context.Context) (driver.Conn, error) { return &memConn{s: c.s}, nil }
func (c memConnector) Driver() driver.Driver                       { return memDriver{} }

type memDriver struct{}

func (memDriver) Open(string) (driver.Conn, error) { return nil, errors.New("not supported") }

type memConn struct{ s *memStore }

func (c *memConn) Prepare(q string) (driver.Stmt, error) { return &memStmt{s: c.s, q: q}, nil }
func (c *memConn) Close() error                          { return nil }
func (c *memConn) Begin() (driver.Tx, error)             { return nil, errors.New("no tx") }

type memStmt struct {
	s *memStore
	q string
}

func (st *memStmt) Close() error  { return nil }
func (st *memStmt) NumInput() int { return -1 }

func (st *memStmt) Exec(args []driver.Value) (driver.Result, error) {
	s := st.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failErr != nil {
		return nil, s.failErr
	}
	q := strings.TrimSpace(st.q)
	switch {
	case strings.HasPrefix(q, "INSERT"):
		ts, _ := args[4].(time.Time)
		s.rows[args[0].(string)] = memRow{
			id: args[0].(string), body: args[1].(string), userID: args[2].(string),
			articleID: args[3].(string), createdAt: ts,
		}
		return driver.RowsAffected(1), nil
	case strings.HasPrefix(q, "DELETE"):
		delete(s.rows, args[0].(string))
		return driver.RowsAffected(1), nil
	}
	return nil, errors.New("unsupported exec")
}

func (st *memStmt) Query(args []driver.Value) (driver.Rows, error) {
	s := st.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failErr != nil {
		return nil, s.failErr
	}
	r := &memRows{}
	if row, ok := s.rows[args[0].(string)]; ok && row.articleID == args[1].(string) {
		r.data = [][]driver.Value{{row.id, row.body, row.userID, row.articleID, row.createdAt}}
	}
	return r, nil
}

type memRows struct {
	data [][]driver.Value
	i    int
}

func (r *memRows) Columns() []string {
	return []string{"id", "body", "user_id", "article_id", "created_at"}
}
func (r *memRows) Close() error { return nil }
func (r *memRows) Next(dest []driver.Value) error {
	if r.i >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.i])
	r.i++
	return nil
}

func openMemDB(t *testing.T) (*sql.DB, *memStore) {
	t.Helper()
	s := &memStore{rows: map[string]memRow{}}
	db := sql.OpenDB(memConnector{s: s})
	t.Cleanup(func() { db.Close() })
	return db, s
}

func TestCommentRepoX_SaveFindRemove(t *testing.T) {
	ctx := context.Background()
	db, s := openMemDB(t)
	repo := NewCommentRepo(db)
	ts := time.Date(2020, 1, 2, 3, 4, 5, 123456789, time.FixedZone("X", 3600))
	c := core.RestoreComment("c1", "body", "u1", "a1", ts)

	if err := repo.Save(ctx, c); err != nil {
		t.Fatalf("save: %v", err)
	}
	row := s.rows["c1"]
	if row.createdAt.Location() != time.UTC || !row.createdAt.Equal(ts.Truncate(time.Millisecond)) {
		t.Fatalf("bad stored time %v", row.createdAt)
	}

	got, found, err := repo.FindByID(ctx, "a1", "c1")
	if err != nil || !found || got == nil {
		t.Fatalf("find: found=%v err=%v", found, err)
	}
	if got.ID() != "c1" || got.Body() != "body" || got.UserID() != "u1" || got.ArticleID() != "a1" {
		t.Fatalf("mismatch: %v %v %v %v", got.ID(), got.Body(), got.UserID(), got.ArticleID())
	}
	if !got.CreatedAt().Equal(ts.Truncate(time.Millisecond)) {
		t.Fatalf("created at %v", got.CreatedAt())
	}

	got, found, err = repo.FindByID(ctx, "other", "c1")
	if err != nil || found || got != nil {
		t.Fatalf("expected not found for other article")
	}

	if err := repo.Remove(ctx, c); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := repo.Remove(ctx, c); err != nil {
		t.Fatalf("remove missing: %v", err)
	}
	if _, found, _ := repo.FindByID(ctx, "a1", "c1"); found {
		t.Fatal("expected removed")
	}
}