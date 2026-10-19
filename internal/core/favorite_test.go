package core

import (
	"encoding/json"
	"testing"
)

func TestZeroValueArticleFavorite(t *testing.T) {
	var a, b ArticleFavorite
	if a.GetArticleID() != "" || a.GetUserID() != "" {
		t.Fatalf("zero value should have empty fields, got %+v", a)
	}
	if !a.Equal(b) || a != b {
		t.Fatal("two zero values should be equal")
	}
	if a.Hash() != b.Hash() {
		t.Fatal("two zero values should have equal hashes")
	}
}

func TestNewArticleFavorite(t *testing.T) {
	tests := []struct {
		name, articleID, userID string
	}{
		{"normal", "a1", "u1"},
		{"empty strings", "", ""},
		{"whitespace not trimmed", "  a1 ", " u1  "},
		{"empty article", "", "u1"},
		{"empty user", "a1", ""},
		{"unicode", "文章", "用户"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := NewArticleFavorite(tc.articleID, tc.userID)
			for i := 0; i < 2; i++ {
				if got := f.GetArticleID(); got != tc.articleID {
					t.Errorf("GetArticleID() = %q, want %q", got, tc.articleID)
				}
				if got := f.GetUserID(); got != tc.userID {
					t.Errorf("GetUserID() = %q, want %q", got, tc.userID)
				}
			}
			if f.ArticleID != tc.articleID || f.UserID != tc.userID {
				t.Errorf("fields mismatch: %+v", f)
			}
		})
	}
}

func TestNewArticleFavoriteEmptyEqualsZero(t *testing.T) {
	if !NewArticleFavorite("", "").Equal(ArticleFavorite{}) {
		t.Fatal("NewArticleFavorite(\"\", \"\") should equal zero value")
	}
}

func TestEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b ArticleFavorite
		want bool
	}{
		{"same values", NewArticleFavorite("a1", "u1"), NewArticleFavorite("a1", "u1"), true},
		{"different user", NewArticleFavorite("a1", "u1"), NewArticleFavorite("a1", "u2"), false},
		{"different article", NewArticleFavorite("a1", "u1"), NewArticleFavorite("a2", "u1"), false},
		{"swapped", NewArticleFavorite("a1", "u1"), NewArticleFavorite("u1", "a1"), false},
		{"both empty", ArticleFavorite{}, ArticleFavorite{}, true},
		{"one empty", ArticleFavorite{}, NewArticleFavorite("a1", "u1"), false},
		{"concat ambiguity", NewArticleFavorite("ab", "c"), NewArticleFavorite("a", "bc"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Equal(tc.b); got != tc.want {
				t.Errorf("a.Equal(b) = %v, want %v", got, tc.want)
			}
			if got := tc.b.Equal(tc.a); got != tc.want {
				t.Errorf("symmetry: b.Equal(a) = %v, want %v", got, tc.want)
			}
			if !tc.a.Equal(tc.a) || !tc.b.Equal(tc.b) {
				t.Error("reflexivity violated")
			}
			if (tc.a == tc.b) != tc.want {
				t.Error("== disagrees with Equal")
			}
		})
	}
}

func TestEqualNonArticleFavorite(t *testing.T) {
	var x interface{} = NewArticleFavorite("a1", "u1")
	others := []interface{}{nil, "a1", 42, struct{ ArticleID, UserID string }{"a1", "u1"}}
	for _, o := range others {
		if x == o {
			t.Errorf("ArticleFavorite should not equal %#v", o)
		}
	}
}

func TestEqualTransitive(t *testing.T) {
	a := NewArticleFavorite("a", "u")
	b := NewArticleFavorite("a", "u")
	c := NewArticleFavorite("a", "u")
	if !(a.Equal(b) && b.Equal(c) && a.Equal(c)) {
		t.Fatal("transitivity violated")
	}
}

func TestHash(t *testing.T) {
	tests := []struct {
		name string
		a, b ArticleFavorite
	}{
		{"normal", NewArticleFavorite("a1", "u1"), NewArticleFavorite("a1", "u1")},
		{"empty", ArticleFavorite{}, NewArticleFavorite("", "")},
		{"unicode", NewArticleFavorite("é", "ü"), NewArticleFavorite("é", "ü")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h1 := tc.a.Hash()
			if h1 != tc.a.Hash() {
				t.Error("hash not stable across calls")
			}
			if h1 != tc.b.Hash() {
				t.Error("equal values have different hashes")
			}
		})
	}
}

func TestHashDistinguishes(t *testing.T) {
	pairs := [][2]ArticleFavorite{
		{NewArticleFavorite("ab", "c"), NewArticleFavorite("a", "bc")},
		{NewArticleFavorite("a1", "u1"), NewArticleFavorite("u1", "a1")},
	}
	for _, p := range pairs {
		if p[0].Hash() == p[1].Hash() {
			t.Errorf("unexpected hash collision for %+v and %+v", p[0], p[1])
		}
	}
}

func TestMapKeyAndSetUsage(t *testing.T) {
	set := map[ArticleFavorite]struct{}{}
	set[NewArticleFavorite("a1", "u1")] = struct{}{}
	set[NewArticleFavorite("a1", "u1")] = struct{}{}
	set[NewArticleFavorite("a1", "u2")] = struct{}{}
	set[ArticleFavorite{}] = struct{}{}
	set[ArticleFavorite{}] = struct{}{}
	if len(set) != 3 {
		t.Fatalf("set size = %d, want 3", len(set))
	}
	if _, ok := set[NewArticleFavorite("a1", "u1")]; !ok {
		t.Fatal("lookup by equal value failed")
	}

	hset := map[uint64]int{}
	hset[NewArticleFavorite("x", "y").Hash()]++
	hset[NewArticleFavorite("x", "y").Hash()]++
	if len(hset) != 1 {
		t.Fatal("equal values should hash to same bucket")
	}
}

func TestJSON(t *testing.T) {
	tests := []struct {
		name string
		in   ArticleFavorite
		want string
	}{
		{"normal", NewArticleFavorite("a1", "u1"), `{"articleId":"a1","userId":"u1"}`},
		{"empty", ArticleFavorite{}, `{"articleId":"","userId":""}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tc.want {
				t.Errorf("got %s, want %s", b, tc.want)
			}
			var out ArticleFavorite
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatal(err)
			}
			if !out.Equal(tc.in) {
				t.Errorf("round trip mismatch: %+v vs %+v", out, tc.in)
			}
		})
	}
}