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

	"migrated-app/internal/core"
)

// ---- fake in-memory SQL driver (mocks the DB behind userQuerier) ----

type fakeUserRow struct {
	id                                     string
	username, email, password, bio, image *string
}

type fakeState struct {
	mu        sync.Mutex
	users     map[string]*fakeUserRow
	follows   [][2]string
	execs     []string
	queryErr  error
	execErr   error
	execCount int
}

func newFakeState() *fakeState { return &fakeState{users: map[string]*fakeUserRow{}} }

type fakeConnector struct{ st *fakeState }

func (c *fakeConnector) Connect(context.Context) (driver.Conn, error) { return &fakeConn{st: c.st}, nil }
func (c *fakeConnector) Driver() driver.Driver                       { return fakeDrv{} }

type fakeDrv struct{}

func (fakeDrv) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type fakeConn struct{ st *fakeState }

func (c *fakeConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("prepare unsupported") }
func (c *fakeConn) Close() error                        { return nil }
func (c *fakeConn) Begin() (driver.Tx, error)           { return nil, errors.New("tx unsupported") }

func toPtr(v driver.Value) *string {
	if v == nil {
		return nil
	}
	s := v.(string)
	return &s
}

func ptrVal(p *string) driver.Value {
	if p == nil {
		return nil
	}
	return *p
}

func (c *fakeConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	st := c.st
	st.mu.Lock()
	defer st.mu.Unlock()
	st.execs = append(st.execs, query)
	st.execCount++
	if st.execErr != nil {
		return nil, st.execErr
	}
	switch {
	case query == insertUserSQL:
		id := args[0].Value.(string)
		if _, ok := st.users[id]; ok {
			return nil, errors.New("duplicate key")
		}
		st.users[id] = &fakeUserRow{id: id, username: toPtr(args[1].Value), email: toPtr(args[2].Value),
			password: toPtr(args[3].Value), bio: toPtr(args[4].Value), image: toPtr(args[5].Value)}
	case strings.HasPrefix(query, "UPDATE users SET "):
		body := strings.TrimPrefix(query, "UPDATE users SET ")
		parts := strings.SplitN(body, " WHERE id = $", 2)
		idx, _ := strconv.Atoi(parts[1])
		id := args[idx-1].Value.(string)
		row, ok := st.users[id]
		if !ok {
			return driver.RowsAffected(0), nil
		}
		for _, set := range strings.Split(parts[0], ", ") {
			kv := strings.SplitN(set, " = $", 2)
			n, _ := strconv.Atoi(kv[1])
			v := toPtr(args[n-1].Value)
			switch kv[0] {
			case "username":
				row.username = v
			case "email":
				row.email = v
			case "password":
				row.password = v
			case "bio":
				row.bio = v
			case "image":
				row.image = v
			}
		}
	case query == insertFollowSQL:
		st.follows = append(st.follows, [2]string{args[0].Value.(string), args[1].Value.(string)})
	case query == deleteFollowSQL:
		u, t := args[0].Value.(string), args[1].Value.(string)
		kept := st.follows[:0]
		for _, f := range st.follows {
			if f[0] != u || f[1] != t {
				kept = append(kept, f)
			}
		}
		st.follows = kept
	default:
		return nil, errors.New("unexpected exec: " + query)
	}
	return driver.RowsAffected(1), nil
}

func (c *fakeConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	st := c.st
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.queryErr != nil {
		return nil, st.queryErr
	}
	userCols := []string{"id", "username", "email", "password", "bio", "image"}
	arg := args[0].Value.(string)
	var match func(r *fakeUserRow) bool
	switch query {
	case findUserByIDSQL:
		match = func(r *fakeUserRow) bool { return r.id == arg }
	case findUserByUsernameSQL:
		match = func(r *fakeUserRow) bool { return r.username != nil && *r.username == arg }
	case findUserByEmailSQL:
		match = func(r *fakeUserRow) bool { return r.email != nil && *r.email == arg }
	case findFollowSQL:
		t := args[1].Value.(string)
		var out [][]driver.Value
		for _, f := range st.follows {
			if f[0] == arg && f[1] == t {
				out = append(out, []driver.Value{f[0], f[1]})
			}
		}
		return &fakeRows{cols: []string{"user_id", "follow_id"}, data: out}, nil
	default:
		return nil, errors.New("unexpected query: " + query)
	}
	var out [][]driver.Value
	for _, r := range st.users {
		if match(r) {
			out = append(out, []driver.Value{r.id, ptrVal(r.username), ptrVal(r.email),
				ptrVal(r.password), ptrVal(r.bio), ptrVal(r.image)})
		}
	}
	return &fakeRows{cols: userCols, data: out}, nil
}

type fakeRows struct {
	cols []string
	data [][]driver.Value
	i    int
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.i >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.i])
	r.i++
	return nil
}

func newTestRepo(t *testing.T) (*UserRepo, *fakeState) {
	t.Helper()
	st := newFakeState()
	db := sql.OpenDB(&fakeConnector{st: st})
	t.Cleanup(func() { db.Close() })
	return NewUserRepository(db), st
}

func sameUser(a, b *core.User) bool {
	return a != nil && b != nil && *a == *b
}

func mustUser(t *testing.T, email, username, password, bio, image string) *core.User {
	t.Helper()
	u, err := core.NewUser(email, username, password, bio, image)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// ---- tests ----

func TestSaveAndFind(t *testing.T) {
	ctx := context.Background()
	repo, st := newTestRepo(t)
	u := mustUser(t, "aisensiy@163.com", "aisensiy", "123", "", "default")
	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("save: %v", err)
	}
	if st.users[u.ID].bio != nil {
		t.Errorf("empty bio should be stored as NULL")
	}
	finders := []struct {
		name string
		fn   func() (*core.User, error)
	}{
		{"byID", func() (*core.User, error) { return repo.FindByID(ctx, u.ID) }},
		{"byUsername", func() (*core.User, error) { return repo.FindByUsername(ctx, "aisensiy") }},
		{"byEmail", func() (*core.User, error) { return repo.FindByEmail(ctx, "aisensiy@163.com") }},
	}
	for _, f := range finders {
		t.Run(f.name, func(t *testing.T) {
			got, err := f.fn()
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if !sameUser(got, u) {
				t.Errorf("got %+v want %+v", got, u)
			}
		})
	}
}

func TestFindNotFound(t *testing.T) {
	ctx := context.Background()
	repo, _ := newTestRepo(t)
	if err := repo.Save(ctx, mustUser(t, "a@b.c", "a", "p", "", "")); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		fn   func() (*core.User, error)
	}{
		{"id", func() (*core.User, error) { return repo.FindByID(ctx, "nope") }},
		{"username", func() (*core.User, error) { return repo.FindByUsername(ctx, "nope") }},
		{"email", func() (*core.User, error) { return repo.FindByEmail(ctx, "nope@x") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn()
			if !errors.Is(err, core.ErrNotFound) {
				t.Errorf("want ErrNotFound, got %v", err)
			}
			if got != nil {
				t.Errorf("want nil user")
			}
		})
	}
}

func TestSaveUpdates(t *testing.T) {
	ctx := context.Background()
	repo, st := newTestRepo(t)
	u := mustUser(t, "aisensiy@163.com", "aisensiy", "123", "", "default")
	if err := repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}

	u.Update("newemail@email.com", "", "", "", "")
	if err := repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByUsername(ctx, u.Username)
	if err != nil || got.Email != "newemail@email.com" {
		t.Fatalf("email update: got %+v err %v", got, err)
	}

	u.Update("", "newUsername", "", "", "")
	if err := repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}
	got, err = repo.FindByEmail(ctx, u.Email)
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "newUsername" || got.Image != u.Image || got.Image != "default" {
		t.Errorf("unexpected user %+v", got)
	}
	if len(st.users) != 1 {
		t.Errorf("want 1 row, got %d", len(st.users))
	}
}

func TestSaveUpdateAllEmptyIsNoop(t *testing.T) {
	ctx := context.Background()
	repo, st := newTestRepo(t)
	st.users["x"] = &fakeUserRow{id: "x"}
	before := st.execCount
	if err := repo.Save(ctx, &core.User{ID: "x"}); err != nil {
		t.Fatal(err)
	}
	if st.execCount != before {
		t.Errorf("expected no exec, got %d", st.execCount-before)
	}
}

func TestNullColumnsMapToEmpty(t *testing.T) {
	repo, st := newTestRepo(t)
	n := "n"
	st.users["x"] = &fakeUserRow{id: "x", username: &n}
	got, err := repo.FindByID(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if got.Bio != "" || got.Image != "" || got.Email != "" || got.Username != "n" {
		t.Errorf("unexpected %+v", got)
	}
}

func TestSaveErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		setup   func(st *fakeState)
		user    *core.User
		wantErr error
	}{
		{"nil user", nil, nil, core.ErrNilUser},
		{"query error", func(st *fakeState) { st.queryErr = boom }, &core.User{ID: "1", Username: "u"}, boom},
		{"insert error", func(st *fakeState) { st.execErr = boom }, &core.User{ID: "1", Username: "u"}, boom},
		{"update error", func(st *fakeState) {
			st.users["1"] = &fakeUserRow{id: "1"}
			st.execErr = boom
		}, &core.User{ID: "1", Username: "u"}, boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, st := newTestRepo(t)
			if tt.setup != nil {
				tt.setup(st)
			}
			err := repo.Save(context.Background(), tt.user)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("want %v got %v", tt.wantErr, err)
			}
			if errors.Is(err, core.ErrNotFound) {
				t.Errorf("must not be ErrNotFound")
			}
		})
	}
}

func TestFindDBError(t *testing.T) {
	repo, st := newTestRepo(t)
	boom := errors.New("boom")
	st.queryErr = boom
	if _, err := repo.FindByID(context.Background(), "x"); !errors.Is(err, boom) || errors.Is(err, core.ErrNotFound) {
		t.Errorf("got %v", err)
	}
	if _, err := repo.FindRelation(context.Background(), "a", "b"); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

func TestRelations(t *testing.T) {
	ctx := context.Background()
	repo, st := newTestRepo(t)
	u := mustUser(t, "a@a", "a", "p", "", "")
	o := mustUser(t, "b@b", "b", "p", "", "")
	for _, x := range []*core.User{u, o} {
		if err := repo.Save(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	rel := core.NewFollowRelation(u.ID, o.ID)

	if err := repo.SaveRelation(ctx, rel); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindRelation(ctx, u.ID, o.ID)
	if err != nil || !got.Equal(rel) {
		t.Fatalf("find: %v %v", got, err)
	}
	// idempotent
	if err := repo.SaveRelation(ctx, rel); err != nil {
		t.Fatal(err)
	}
	if len(st.follows) != 1 {
		t.Errorf("want 1 follow, got %d", len(st.follows))
	}
	// directional
	if _, err := repo.FindRelation(ctx, o.ID, u.ID); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("reverse should be not found, got %v", err)
	}
	// remove
	if err := repo.RemoveRelation(ctx, rel); err != nil {
		t.Fatal(err)
	}
	if r, err := repo.FindRelation(ctx, u.ID, o.ID); !errors.Is(err, core.ErrNotFound) || r != nil {
		t.Errorf("after remove: %v %v", r, err)
	}
	// remove non-existent
	if err := repo.RemoveRelation(ctx, rel); err != nil {
		t.Errorf("remove missing: %v", err)
	}
}

func TestRelationErrors(t *testing.T) {
	boom := errors.New("boom")
	rel := core.NewFollowRelation("a", "b")
	tests := []struct {
		name    string
		setup   func(st *fakeState)
		call    func(r *UserRepo) error
		wantErr error
	}{
		{"save nil", nil, func(r *UserRepo) error { return r.SaveRelation(context.Background(), nil) }, core.ErrNilFollowRelation},
		{"remove nil", nil, func(r *UserRepo) error { return r.RemoveRelation(context.Background(), nil) }, core.ErrNilFollowRelation},
		{"save query err", func(st *fakeState) { st.queryErr = boom }, func(r *UserRepo) error { return r.SaveRelation(context.Background(), rel) }, boom},
		{"save exec err", func(st *fakeState) { st.execErr = boom }, func(r *UserRepo) error { return r.SaveRelation(context.Background(), rel) }, boom},
		{"remove exec err", func(st *fakeState) { st.execErr = boom }, func(r *UserRepo) error { return r.RemoveRelation(context.Background(), rel) }, boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, st := newTestRepo(t)
			if tt.setup != nil {
				tt.setup(st)
			}
			if err := tt.call(repo); !errors.Is(err, tt.wantErr) {
				t.Errorf("want %v got %v", tt.wantErr, err)
			}
		})
	}
}

func TestNullIfEmpty(t *testing.T) {
	tests := []struct {
		in   string
		want sql.NullString
	}{
		{"", sql.NullString{}},
		{"x", sql.NullString{String: "x", Valid: true}},
	}
	for _, tt := range tests {
		if got := nullIfEmpty(tt.in); got != tt.want {
			t.Errorf("nullIfEmpty(%q)=%v want %v", tt.in, got, tt.want)
		}
	}
}