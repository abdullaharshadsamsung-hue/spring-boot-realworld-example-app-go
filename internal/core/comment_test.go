package core

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var errInvalidComment = errors.New("invalid comment")

type memCommentRepo struct {
	mu   sync.Mutex
	rows map[string]Comment
}

func newMemCommentRepo() *memCommentRepo {
	return &memCommentRepo{rows: map[string]Comment{}}
}

var _ CommentRepository = (*memCommentRepo)(nil)

func (r *memCommentRepo) Save(_ context.Context, c *Comment) error {
	if c == nil || c.ID() == "" || c.ArticleID() == "" || c.UserID() == "" {
		return errInvalidComment
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[c.ID()] = *c
	return nil
}

func (r *memCommentRepo) FindByID(_ context.Context, articleID, id string) (*Comment, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.rows[id]
	if !ok || c.ArticleID() != articleID {
		return nil, false, nil
	}
	cp := c
	return &cp, true, nil
}

func (r *memCommentRepo) Remove(_ context.Context, c *Comment) error {
	if c == nil {
		return errInvalidComment
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, c.ID())
	return nil
}

func TestNewComment(t *testing.T) {
	before := time.Now().UTC().Add(-time.Millisecond)
	c := NewComment("body", "u1", "a1")
	after := time.Now().UTC().Add(time.Millisecond)
	if !uuidV4Re.MatchString(c.ID()) {
		t.Fatalf("bad id %q", c.ID())
	}
	if c.Body() != "body" || c.UserID() != "u1" || c.ArticleID() != "a1" {
		t.Fatalf("fields mismatch: %+v", c)
	}
	ca := c.CreatedAt()
	if ca.Location() != time.UTC {
		t.Errorf("expected UTC")
	}
	if ca.Nanosecond()%int(time.Millisecond) != 0 {
		t.Errorf("not ms truncated: %v", ca)
	}
	if ca.Before(before) || ca.After(after) {
		t.Errorf("createdAt %v out of range", ca)
	}
	if NewComment("b", "u", "a").ID() == c.ID() {
		t.Errorf("ids should be unique")
	}
}

func TestNewCommentIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := newCommentID()
		if !uuidV4Re.MatchString(id) {
			t.Fatalf("bad id %q", id)
		}
		if seen[id] {
			t.Fatalf("dup id %q", id)
		}
		seen[id] = true
	}
}

func TestRestoreComment(t *testing.T) {
	ts := time.Date(2020, 1, 2, 3, 4, 5, 123456789, time.FixedZone("x", 3600))
	tests := []struct {
		name, id, body, user, article string
	}{
		{"normal", "id1", "hello", "u", "a"},
		{"empty", "", "", "", ""},
		{"unicode", "id2", "héllo\n\t世界", "u2", "a2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := RestoreComment(tt.id, tt.body, tt.user, tt.article, ts)
			if c.ID() != tt.id || c.Body() != tt.body || c.UserID() != tt.user || c.ArticleID() != tt.article || !c.CreatedAt().Equal(ts) || c.CreatedAt().Location() != ts.Location() {
				t.Fatalf("mismatch")
			}
		})
	}
}

func TestCommentEqualAndHashKey(t *testing.T) {
	now := time.Now()
	a := RestoreComment("x", "b1", "u1", "a1", now)
	b := RestoreComment("x", "b2", "u2", "a2", now.Add(time.Hour))
	c := RestoreComment("y", "b1", "u1", "a1", now)
	var n1, n2 *Comment
	tests := []struct {
		name string
		l, r *Comment
		want bool
	}{
		{"same id diff fields", a, b, true},
		{"diff id", a, c, false},
		{"self", a, a, true},
		{"both nil", n1, n2, true},
		{"nil left", nil, a, false},
		{"nil right", a, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.l.Equal(tt.r); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
			if tt.want && tt.l.HashKey() != tt.r.HashKey() {
				t.Fatalf("hash inconsistent")
			}
		})
	}
	if n1.HashKey() != "" || a.HashKey() != "x" {
		t.Fatalf("hashkey wrong")
	}
}

func TestRepoSave(t *testing.T) {
	ctx := context.Background()
	long := strings.Repeat("ab ", 10000)
	tests := []struct {
		name    string
		c       *Comment
		wantErr bool
	}{
		{"valid", NewComment("body", "u", "a"), false},
		{"arbitrary body", NewComment("  <script>'\"; DROP TABLE\n😀 "+long, "u", "a"), false},
		{"empty body", NewComment("", "u", "a"), false},
		{"nil", nil, true},
		{"missing article", NewComment("b", "u", ""), true},
		{"missing user", NewComment("b", "", "a"), true},
		{"missing id", RestoreComment("", "b", "u", "a", time.Now()), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newMemCommentRepo()
			err := r.Save(ctx, tt.c)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if len(r.rows) != 0 {
					t.Fatal("partial record stored")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, found, err := r.FindByID(ctx, tt.c.ArticleID(), tt.c.ID())
			if err != nil || !found || got == nil {
				t.Fatalf("not found: %v", err)
			}
			if !got.Equal(tt.c) || got.Body() != tt.c.Body() || got.UserID() != tt.c.UserID() ||
				got.ArticleID() != tt.c.ArticleID() || !got.CreatedAt().Equal(tt.c.CreatedAt()) {
				t.Fatal("round trip mismatch")
			}
		})
	}
}

func TestRepoFindByID(t *testing.T) {
	ctx := context.Background()
	r := newMemCommentRepo()
	saved := NewComment("b", "u", "a1")
	removed := NewComment("r", "u", "a1")
	for _, c := range []*Comment{saved, removed} {
		if err := r.Save(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Remove(ctx, removed); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, article, id string
		want              bool
	}{
		{"existing", "a1", saved.ID(), true},
		{"other article", "a2", saved.ID(), false},
		{"missing id", "a1", "nope", false},
		{"removed", "a1", removed.ID(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(r.rows)
			got, found, err := r.FindByID(ctx, tt.article, tt.id)
			if err != nil {
				t.Fatal(err)
			}
			if found != tt.want {
				t.Fatalf("found=%v", found)
			}
			if !found && got != nil {
				t.Fatal("expected nil comment")
			}
			if found && (got.ID() != tt.id || got.ArticleID() != tt.article) {
				t.Fatal("key mismatch")
			}
			if len(r.rows) != before {
				t.Fatal("find mutated store")
			}
		})
	}
}

func TestRepoRemove(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		saveTarg bool
	}{
		{"existing", true},
		{"not stored", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newMemCommentRepo()
			target := NewComment("t", "u", "a")
			sibling := NewComment("s", "u", "a")
			other := NewComment("o", "u", "b")
			for _, c := range []*Comment{sibling, other} {
				_ = r.Save(ctx, c)
			}
			if tt.saveTarg {
				_ = r.Save(ctx, target)
			}
			if err := r.Remove(ctx, target); err != nil {
				t.Fatal(err)
			}
			if _, f, _ := r.FindByID(ctx, "a", target.ID()); f {
				t.Fatal("target still present")
			}
			for _, c := range []*Comment{sibling, other} {
				if _, f, _ := r.FindByID(ctx, c.ArticleID(), c.ID()); !f {
					t.Fatalf("collateral delete of %s", c.Body())
				}
			}
		})
	}
}