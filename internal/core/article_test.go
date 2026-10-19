package core

import (
	"regexp"
	"testing"
	"time"
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewArticleSlug(t *testing.T) {
	tests := []struct {
		title, want string
	}{
		{"a new   title", "a-new-title"},
		{"a new title 2", "a-new-title-2"},
		{"A NEW TITLE", "a-new-title"},
		{"中文：标题", "中文-标题"},
		{"what?the.hell,w", "what-the-hell-w"},
	}
	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			a := NewArticle(tc.title, "desc", "body", []string{"java"}, "123", time.Time{})
			if a.Slug != tc.want {
				t.Errorf("slug = %q, want %q", a.Slug, tc.want)
			}
		})
	}
}

func TestToSlug(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Hello & World", "hello-world"},
		{"a|b", "a-b"},
		{"it’s “quoted”", "it-s-“quoted-"},
		{"tab\tnew\nline\x0Bvt", "tab-new-line-vt"},
		{"", ""},
		{"Simple", "simple"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := ToSlug(tc.in)
			if got != tc.want {
				t.Errorf("ToSlug(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if again := ToSlug(tc.in); again != got {
				t.Errorf("not deterministic: %q vs %q", got, again)
			}
		})
	}
}

func TestNewArticleFields(t *testing.T) {
	created := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	a := NewArticle("Title", "desc", "body", []string{"java", "java", "spring"}, "u1", created)

	if a.Title != "Title" || a.Description != "desc" || a.Body != "body" || a.UserID != "u1" {
		t.Errorf("unexpected fields: %+v", a)
	}
	if !uuidRe.MatchString(a.ID) {
		t.Errorf("id %q is not a v4 UUID", a.ID)
	}
	if !a.CreatedAt.Equal(created) || !a.UpdatedAt.Equal(created) {
		t.Errorf("timestamps = %v/%v, want %v", a.CreatedAt, a.UpdatedAt, created)
	}
	if len(a.Tags) != 2 {
		t.Fatalf("tags len = %d, want 2", len(a.Tags))
	}
	names := map[string]bool{}
	for _, tg := range a.Tags {
		names[tg.Name] = true
		if !uuidRe.MatchString(tg.ID) {
			t.Errorf("tag id %q invalid", tg.ID)
		}
	}
	if !names["java"] || !names["spring"] {
		t.Errorf("tags = %v", a.Tags)
	}
}

func TestNewArticleDefaultsAndEdgeCases(t *testing.T) {
	before := time.Now()
	a := NewArticle("", "", "", nil, "u", time.Time{})
	after := time.Now()
	if a.CreatedAt.Before(before) || a.CreatedAt.After(after) {
		t.Errorf("createdAt %v not within [%v,%v]", a.CreatedAt, before, after)
	}
	if !a.UpdatedAt.Equal(a.CreatedAt) {
		t.Errorf("updatedAt != createdAt")
	}
	// Go analogue of null title/tagList: no panic, empty values.
	if a.Slug != "" {
		t.Errorf("slug = %q", a.Slug)
	}
	if a.Tags == nil || len(a.Tags) != 0 {
		t.Errorf("tags = %#v, want empty non-nil", a.Tags)
	}
}

func TestArticleEquality(t *testing.T) {
	a := NewArticle("t", "d", "b", []string{"x"}, "u", time.Time{})
	b := NewArticle("t", "d", "b", []string{"x"}, "u", time.Time{})
	sameID := &Article{ID: a.ID, Title: "other"}
	var nilA *Article

	tests := []struct {
		name string
		x, y *Article
		want bool
	}{
		{"identical inputs differ", a, b, false},
		{"same id equal", a, sameID, true},
		{"self", a, a, true},
		{"nil vs nil", nilA, nil, true},
		{"nil vs non-nil", nilA, a, false},
		{"non-nil vs nil", a, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.x.Equal(tc.y); got != tc.want {
				t.Errorf("Equal = %v, want %v", got, tc.want)
			}
		})
	}
	if a.ID == b.ID {
		t.Errorf("ids should differ")
	}
}

func TestArticleUpdate(t *testing.T) {
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name                    string
		title, desc, body       string
		wantTitle, wantSlug     string
		wantDesc, wantBody      string
		wantUpdated             bool
	}{
		{"title", "New Title?", "", "", "New Title?", "new-title-", "desc", "body", true},
		{"description", "", "nd", "", "Old Title", "old-title", "nd", "body", true},
		{"body", "", "", "nb", "Old Title", "old-title", "desc", "nb", true},
		{"all", "X Y", "nd", "nb", "X Y", "x-y", "nd", "nb", true},
		{"none", "", "", "", "Old Title", "old-title", "desc", "body", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewArticle("Old Title", "desc", "body", []string{"a", "b"}, "u", created)
			id, tags := a.ID, append([]Tag(nil), a.Tags...)
			before := time.Now()
			a.Update(tc.title, tc.desc, tc.body)

			if a.Title != tc.wantTitle || a.Slug != tc.wantSlug || a.Description != tc.wantDesc || a.Body != tc.wantBody {
				t.Errorf("got %+v", a)
			}
			if a.Slug != ToSlug(a.Title) {
				t.Errorf("slug out of sync with title")
			}
			if tc.wantUpdated {
				if a.UpdatedAt.Before(before) {
					t.Errorf("updatedAt not refreshed: %v", a.UpdatedAt)
				}
			} else if !a.UpdatedAt.Equal(created) {
				t.Errorf("updatedAt changed: %v", a.UpdatedAt)
			}
			if a.ID != id || a.UserID != "u" || !a.CreatedAt.Equal(created) {
				t.Errorf("immutable fields changed")
			}
			if len(a.Tags) != len(tags) {
				t.Fatalf("tags changed")
			}
			for i := range tags {
				if a.Tags[i] != tags[i] {
					t.Errorf("tag %d changed", i)
				}
			}
		})
	}
}

func TestTag(t *testing.T) {
	a := NewTag("go")
	b := NewTag("go")
	c := NewTag("rust")
	tests := []struct {
		name string
		x, y Tag
		want bool
	}{
		{"same name diff id", a, b, true},
		{"diff name", a, c, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.x.Equal(tc.y); got != tc.want {
				t.Errorf("Equal = %v, want %v", got, tc.want)
			}
		})
	}
	if a.ID == b.ID {
		t.Errorf("tag ids should differ")
	}
	if a.Key() != "go" || a.String() != "go" {
		t.Errorf("Key/String = %q/%q", a.Key(), a.String())
	}
}

func TestNewRandomUUIDForCore(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := newRandomUUIDForCore()
		if !uuidRe.MatchString(id) {
			t.Fatalf("invalid uuid %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate uuid %q", id)
		}
		seen[id] = true
	}
}