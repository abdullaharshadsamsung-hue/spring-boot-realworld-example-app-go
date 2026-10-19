package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewCommentData(t *testing.T) {
	created := time.Date(2023, 1, 2, 3, 4, 5, 123000000, time.UTC)
	updated := created.Add(time.Hour)
	p := NewProfileData("u1", "jake", "bio", "img", true)

	tests := []struct {
		name    string
		got     CommentData
		id      string
		body    string
		art     string
		created time.Time
		updated time.Time
		author  *ProfileData
	}{
		{"zero value", CommentData{}, "", "", "", time.Time{}, time.Time{}, nil},
		{"all args", NewCommentData("c1", "hi", "a1", created, updated, &p), "c1", "hi", "a1", created, updated, &p},
		{"zero args", NewCommentData("", "", "", time.Time{}, time.Time{}, nil), "", "", "", time.Time{}, time.Time{}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.got
			if c.ID != tc.id || c.Body != tc.body || c.ArticleID != tc.art {
				t.Errorf("string fields mismatch: %+v", c)
			}
			if !c.CreatedAt.Equal(tc.created) || !c.UpdatedAt.Equal(tc.updated) {
				t.Errorf("time fields mismatch: %+v", c)
			}
			if c.Author != tc.author {
				t.Errorf("author = %p, want %p", c.Author, tc.author)
			}
		})
	}
}

func TestCommentDataSetters(t *testing.T) {
	base := NewCommentData("c1", "b", "a1", time.Unix(1, 0), time.Unix(2, 0), &ProfileData{Username: "x"})
	p2 := ProfileData{Username: "y"}
	tests := []struct {
		name  string
		set   func(*CommentData)
		check func(CommentData) bool
	}{
		{"id", func(c *CommentData) { c.ID = "c2" }, func(c CommentData) bool { return c.ID == "c2" }},
		{"body", func(c *CommentData) { c.Body = "" }, func(c CommentData) bool { return c.Body == "" }},
		{"articleId", func(c *CommentData) { c.ArticleID = "a2" }, func(c CommentData) bool { return c.ArticleID == "a2" }},
		{"createdAt", func(c *CommentData) { c.CreatedAt = time.Unix(9, 0) }, func(c CommentData) bool { return c.CreatedAt.Equal(time.Unix(9, 0)) }},
		{"updatedAt", func(c *CommentData) { c.UpdatedAt = time.Unix(8, 0) }, func(c CommentData) bool { return c.UpdatedAt.Equal(time.Unix(8, 0)) }},
		{"author", func(c *CommentData) { c.Author = &p2 }, func(c CommentData) bool { return c.Author == &p2 }},
		{"author nil", func(c *CommentData) { c.Author = nil }, func(c CommentData) bool { return c.Author == nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.set(&c)
			if !tc.check(c) {
				t.Fatalf("set did not take effect: %+v", c)
			}
			// restore field via check of others: reapply set to copy of base and compare remaining fields
			others := base
			tc.set(&others)
			if tc.name != "id" && c.ID != base.ID ||
				tc.name != "body" && c.Body != base.Body ||
				tc.name != "articleId" && c.ArticleID != base.ArticleID ||
				tc.name != "createdAt" && !c.CreatedAt.Equal(base.CreatedAt) ||
				tc.name != "updatedAt" && !c.UpdatedAt.Equal(base.UpdatedAt) ||
				!strings.HasPrefix(tc.name, "author") && c.Author != base.Author {
				t.Errorf("other fields changed: %+v", c)
			}
		})
	}
}

func TestCommentDataCursor(t *testing.T) {
	created := time.Date(2023, 11, 14, 22, 13, 20, 123456789, time.UTC)
	tests := []struct {
		name    string
		c       CommentData
		wantStr string
	}{
		{"set", NewCommentData("c", "b", "a", created, created.Add(48*time.Hour), nil), "1700000000123"},
		{"zero", CommentData{}, "-62135596800000"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cur := tc.c.Cursor()
			if !cur.Data().Equal(tc.c.CreatedAt) {
				t.Errorf("Data = %v, want %v", cur.Data(), tc.c.CreatedAt)
			}
			if cur.String() != tc.wantStr {
				t.Errorf("String = %q, want %q", cur.String(), tc.wantStr)
			}
		})
	}

	t.Run("reflects current createdAt", func(t *testing.T) {
		c := NewCommentData("c", "b", "a", created, created, nil)
		first := c.Cursor()
		c.CreatedAt = created.Add(time.Second)
		second := c.Cursor()
		if !first.Data().Equal(created) || !second.Data().Equal(c.CreatedAt) {
			t.Errorf("cursors: %v %v", first.Data(), second.Data())
		}
		c.UpdatedAt = time.Unix(0, 0)
		c.ID = "other"
		if !c.Cursor().Data().Equal(c.CreatedAt) {
			t.Error("cursor depends on other fields")
		}
	})
}

func TestCommentDataEqual(t *testing.T) {
	ts := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	p := NewProfileData("u", "n", "b", "i", false)
	pCopy := p
	base := NewCommentData("c", "b", "a", ts, ts, &p)

	tests := []struct {
		name string
		mod  func(*CommentData)
		want bool
	}{
		{"identical", func(c *CommentData) {}, true},
		{"author copy", func(c *CommentData) { c.Author = &pCopy }, true},
		{"same instant other zone", func(c *CommentData) { c.CreatedAt = ts.In(time.FixedZone("X", 3600)) }, true},
		{"id", func(c *CommentData) { c.ID = "x" }, false},
		{"body", func(c *CommentData) { c.Body = "x" }, false},
		{"articleId", func(c *CommentData) { c.ArticleID = "x" }, false},
		{"createdAt", func(c *CommentData) { c.CreatedAt = ts.Add(time.Millisecond) }, false},
		{"updatedAt", func(c *CommentData) { c.UpdatedAt = ts.Add(time.Millisecond) }, false},
		{"author nil", func(c *CommentData) { c.Author = nil }, false},
		{"author differs", func(c *CommentData) { q := p; q.Following = true; c.Author = &q }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := base
			tc.mod(&o)
			if got := base.Equal(o); got != tc.want {
				t.Errorf("Equal = %v, want %v", got, tc.want)
			}
			if got := o.Equal(base); got != tc.want {
				t.Errorf("symmetric Equal = %v, want %v", got, tc.want)
			}
		})
	}
	if !base.Equal(base) || !(CommentData{}).Equal(CommentData{}) {
		t.Error("not reflexive")
	}
}

func TestCommentDataMarshalJSON(t *testing.T) {
	created := time.Date(2023, 1, 2, 3, 4, 5, 123456789, time.FixedZone("X", 7200))
	updated := time.Date(2023, 2, 3, 4, 5, 6, 0, time.UTC)
	p := NewProfileData("secret-id", "jake", "bio", "img", true)

	tests := []struct {
		name       string
		c          CommentData
		wantAuthor bool
	}{
		{"populated", NewCommentData("c1", "hello", "art-1", created, updated, &p), true},
		{"nil author", NewCommentData("c1", "hello", "art-1", created, updated, nil), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.c)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), "art-1") || strings.Contains(string(b), "secret-id") {
				t.Errorf("leaked internal ids: %s", b)
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			for _, k := range []string{"id", "body", "createdAt", "updatedAt", "author", "cursor"} {
				if _, ok := m[k]; !ok {
					t.Errorf("missing key %q", k)
				}
			}
			for _, k := range []string{"articleId", "profileData"} {
				if _, ok := m[k]; ok {
					t.Errorf("unexpected key %q", k)
				}
			}
			var out struct {
				ID        string `json:"id"`
				Body      string `json:"body"`
				CreatedAt string `json:"createdAt"`
				UpdatedAt string `json:"updatedAt"`
				Author    *struct {
					Username  string `json:"username"`
					Bio       string `json:"bio"`
					Image     string `json:"image"`
					Following bool   `json:"following"`
				} `json:"author"`
				Cursor struct {
					Data string `json:"data"`
				} `json:"cursor"`
			}
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatal(err)
			}
			if out.ID != "c1" || out.Body != "hello" {
				t.Errorf("bad id/body: %+v", out)
			}
			if out.CreatedAt != "2023-01-02T01:04:05.123Z" || out.UpdatedAt != "2023-02-03T04:05:06.000Z" {
				t.Errorf("bad timestamps: %s %s", out.CreatedAt, out.UpdatedAt)
			}
			if out.Cursor.Data != out.CreatedAt {
				t.Errorf("cursor = %q", out.Cursor.Data)
			}
			if tc.wantAuthor {
				if out.Author == nil || out.Author.Username != "jake" || out.Author.Bio != "bio" || out.Author.Image != "img" || !out.Author.Following {
					t.Errorf("bad author: %+v", out.Author)
				}
			} else if string(m["author"]) != "null" {
				t.Errorf("author = %s, want null", m["author"])
			}
		})
	}
}

func TestCommentDataUnmarshalJSON(t *testing.T) {
	input := `{"id":"c1","body":"hi","articleId":"art-1","author":{"username":"jake","bio":"b","image":"i","following":true}}`
	tests := []struct {
		name string
		in   string
	}{{"with author and articleId", input}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c CommentData
			if err := json.Unmarshal([]byte(tc.in), &c); err != nil {
				t.Fatal(err)
			}
			if c.ID != "c1" || c.Body != "hi" {
				t.Errorf("got %+v", c)
			}
			if c.Author == nil || c.Author.Username != "jake" || !c.Author.Following {
				t.Errorf("author = %+v", c.Author)
			}
		})
	}
}