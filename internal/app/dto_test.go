package app

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func fixtureProfile() *ProfileData {
	p := NewProfileData("uid", "user", "bio", "img", false)
	return &p
}

func articleDataFixture(seed string, now time.Time) ArticleData {
	return NewArticleData(seed+"id", "title-"+seed, "title "+seed, "desc "+seed, "body "+seed,
		false, 0, now, now, []string{}, fixtureProfile())
}

func TestNewArticleData(t *testing.T) {
	now := time.Date(2023, 1, 2, 3, 4, 5, 6000000, time.UTC)
	later := now.Add(time.Hour)
	tests := []struct {
		name    string
		got     ArticleData
		id      string
		slug    string
		title   string
		desc    string
		body    string
		fav     bool
		count   int
		created time.Time
		updated time.Time
		tags    []string
	}{
		{"fixture x", articleDataFixture("x", now), "xid", "title-x", "title x", "desc x", "body x", false, 0, now, now, []string{}},
		{"from article", NewArticleData("aid", "s", "t", "d", "b", false, 0, now, later, []string{"joda"}, fixtureProfile()),
			"aid", "s", "t", "d", "b", false, 0, now, later, []string{"joda"}},
		{"favorited", NewArticleData("i", "s", "t", "d", "b", true, 7, now, later, nil, fixtureProfile()),
			"i", "s", "t", "d", "b", true, 7, now, later, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.got
			if a.ID != tc.id || a.Slug != tc.slug || a.Title != tc.title || a.Description != tc.desc || a.Body != tc.body {
				t.Fatalf("string fields mismatch: %+v", a)
			}
			if a.Favorited != tc.fav || a.FavoritesCount != tc.count {
				t.Fatalf("fav mismatch: %+v", a)
			}
			if !a.CreatedAt.Equal(tc.created) || !a.UpdatedAt.Equal(tc.updated) {
				t.Fatalf("time mismatch")
			}
			if len(a.TagList) != len(tc.tags) {
				t.Fatalf("tags %v want %v", a.TagList, tc.tags)
			}
			for i := range tc.tags {
				if a.TagList[i] != tc.tags[i] {
					t.Fatalf("tags %v want %v", a.TagList, tc.tags)
				}
			}
			if a.Author == nil || !a.Author.Equal(*fixtureProfile()) || a.Author.Following {
				t.Fatalf("author mismatch: %+v", a.Author)
			}
		})
	}
}

func TestArticleDataZeroValueAndSetters(t *testing.T) {
	var a ArticleData
	if a.ID != "" || a.Favorited || a.FavoritesCount != 0 || a.TagList != nil || a.Author != nil || !a.CreatedAt.IsZero() {
		t.Fatalf("zero value not empty: %+v", a)
	}
	now := time.Now()
	a.ID = "id"
	a.Slug = "slug"
	a.FavoritesCount = 3
	a.UpdatedAt = now
	a.Author = fixtureProfile()
	if a.ID != "id" || a.Slug != "slug" || a.FavoritesCount != 3 || !a.UpdatedAt.Equal(now) || a.Author.Username != "user" {
		t.Fatalf("set fields not retained: %+v", a)
	}
}

func TestArticleDataCursor(t *testing.T) {
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2023, 11, 14, 22, 13, 20, 123456789, time.UTC)
	tests := []struct {
		name    string
		updated time.Time
	}{
		{"set", updated},
		{"zero", time.Time{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewArticleData("i", "s", "t", "d", "b", false, 0, created, tc.updated, nil, nil)
			c := a.Cursor()
			if !c.Data().Equal(tc.updated) {
				t.Fatalf("cursor data %v want %v", c.Data(), tc.updated)
			}
			if c.String() != strconv.FormatInt(tc.updated.UnixMilli(), 10) {
				t.Fatalf("cursor string %s", c.String())
			}
			if c.String() == strconv.FormatInt(created.UnixMilli(), 10) {
				t.Fatalf("cursor uses createdAt")
			}
		})
	}
}

func TestArticleDataMarshalJSON(t *testing.T) {
	ts := time.Date(2023, 1, 2, 3, 4, 5, 6000000, time.FixedZone("x", 3600))
	tests := []struct {
		name     string
		a        ArticleData
		wantTags int
		author   bool
	}{
		{"full", NewArticleData("i", "s", "t", "d", "b", true, 2, ts, ts, []string{"a"}, fixtureProfile()), 1, true},
		{"nil tags nil author", NewArticleData("i", "s", "t", "d", "b", false, 0, ts, ts, nil, nil), 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.a)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			for _, k := range []string{"id", "slug", "title", "description", "body", "favorited", "favoritesCount", "createdAt", "updatedAt", "tagList", "author"} {
				if _, ok := m[k]; !ok {
					t.Errorf("missing key %s", k)
				}
			}
			if _, ok := m["profileData"]; ok {
				t.Error("unexpected profileData key")
			}
			tags, ok := m["tagList"].([]any)
			if !ok || len(tags) != tc.wantTags {
				t.Errorf("tagList %v", m["tagList"])
			}
			if m["createdAt"] != "2023-01-02T02:04:05.006Z" {
				t.Errorf("createdAt %v", m["createdAt"])
			}
			if tc.author {
				au := m["author"].(map[string]any)
				if au["username"] != "user" || au["following"] != false {
					t.Errorf("author %v", au)
				}
				if _, ok := au["id"]; ok {
					t.Error("author id leaked")
				}
			} else if m["author"] != nil {
				t.Errorf("author %v", m["author"])
			}
		})
	}
}

func TestArticleDataEqual(t *testing.T) {
	now := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	base := articleDataFixture("x", now)
	mod := func(f func(*ArticleData)) ArticleData {
		c := articleDataFixture("x", now)
		f(&c)
		return c
	}
	tests := []struct {
		name  string
		other ArticleData
		want  bool
	}{
		{"identical", articleDataFixture("x", now), true},
		{"same instant other zone", mod(func(a *ArticleData) { a.UpdatedAt = now.In(time.FixedZone("z", 7200)) }), true},
		{"nil vs empty tags", mod(func(a *ArticleData) { a.TagList = nil }), true},
		{"diff id", mod(func(a *ArticleData) { a.ID = "y" }), false},
		{"diff count", mod(func(a *ArticleData) { a.FavoritesCount = 1 }), false},
		{"diff fav", mod(func(a *ArticleData) { a.Favorited = true }), false},
		{"diff updated", mod(func(a *ArticleData) { a.UpdatedAt = now.Add(time.Second) }), false},
		{"diff tags", mod(func(a *ArticleData) { a.TagList = []string{"a"} }), false},
		{"nil author", mod(func(a *ArticleData) { a.Author = nil }), false},
		{"diff author", mod(func(a *ArticleData) { p := *a.Author; p.Following = true; a.Author = &p }), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := base.Equal(tc.other); got != tc.want {
				t.Fatalf("Equal=%v want %v", got, tc.want)
			}
			if got := tc.other.Equal(base); got != tc.want {
				t.Fatalf("symmetric Equal=%v want %v", got, tc.want)
			}
		})
	}
	var a, b ArticleData
	if !a.Equal(b) {
		t.Fatal("zero values should be equal")
	}
}