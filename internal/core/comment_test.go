package core

import (
	"regexp"
	"testing"
	"time"
)

func TestCoreCmtNewAssignsFields(t *testing.T) {
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	before := time.Now()
	c, err := NewComment("Nice article", "user-1", "article-1")
	after := time.Now()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil comment")
	}
	if c.Body() != "Nice article" || c.UserID() != "user-1" || c.ArticleID() != "article-1" {
		t.Errorf("fields mismatch: %q %q %q", c.Body(), c.UserID(), c.ArticleID())
	}
	if !uuidRe.MatchString(c.ID()) {
		t.Errorf("ID %q is not a canonical v4 UUID", c.ID())
	}
	if c.CreatedAt().Before(before) || c.CreatedAt().After(after) {
		t.Errorf("CreatedAt %v not within [%v, %v]", c.CreatedAt(), before, after)
	}
}

func TestCoreCmtNewUniqueIDs(t *testing.T) {
	a, err := NewComment("b", "u", "a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewComment("b", "u", "a")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID() == b.ID() {
		t.Fatal("expected distinct ids")
	}
	if a.Equal(b) {
		t.Fatal("comments with different ids should not be equal")
	}
}

func TestCoreCmtRestore(t *testing.T) {
	ts := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	c := RestoreComment("id-1", "body", "u1", "a1", ts)
	if c.ID() != "id-1" || c.Body() != "body" || c.UserID() != "u1" || c.ArticleID() != "a1" {
		t.Errorf("fields mismatch")
	}
	if !c.CreatedAt().Equal(ts) {
		t.Errorf("CreatedAt = %v, want %v", c.CreatedAt(), ts)
	}
}

func TestCoreCmtEqualityAndHashKey(t *testing.T) {
	t1 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	x1 := RestoreComment("x", "b1", "u1", "a1", t1)
	x2 := RestoreComment("x", "b2", "u2", "a2", t1.Add(time.Hour))
	y := RestoreComment("y", "b1", "u1", "a1", t1)
	var n *Comment

	if !x1.Equal(x2) || !x2.Equal(x1) {
		t.Error("same id should be equal")
	}
	if x1.HashKey() != x2.HashKey() {
		t.Error("equal comments should share hash key")
	}
	if x1.Equal(y) {
		t.Error("different ids should not be equal")
	}
	if x1.Equal(n) || n.Equal(x1) {
		t.Error("nil vs non-nil should not be equal")
	}
	if !n.Equal(n) {
		t.Error("two nils should be equal")
	}
	if n.HashKey() != "" {
		t.Error("nil HashKey should be empty")
	}
	m := map[string]*Comment{}
	m[x1.HashKey()] = x1
	m[x2.HashKey()] = x2
	if len(m) != 1 {
		t.Errorf("expected 1 map entry, got %d", len(m))
	}
}