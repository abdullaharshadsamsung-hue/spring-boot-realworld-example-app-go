package app

import (
	"encoding/json"
	"testing"
	"time"
)

func sampleArticle(id string) ArticleData {
	ts := time.Date(2023, 1, 2, 3, 4, 5, 6000000, time.UTC)
	p := NewProfileData("u1", "jake", "bio", "img", false)
	return NewArticleData(id, "slug-"+id, "title "+id, "desc", "body", false, 1, ts, ts, []string{"a"}, &p)
}

func TestNewArticleDataList(t *testing.T) {
	two := []ArticleData{sampleArticle("1"), sampleArticle("2")}
	one := []ArticleData{sampleArticle("1")}
	tests := []struct {
		name     string
		articles []ArticleData
		count    int
		wantLen  int
		wantNil  bool
	}{
		{"two articles count 10", two, 10, 2, false},
		{"empty list count 0", []ArticleData{}, 0, 0, false},
		{"count differs from size", one, 50, 1, false},
		{"nil list", nil, 3, 0, true},
		{"negative count", one, -5, 1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := NewArticleDataList(tc.articles, tc.count)
			if l.Count != tc.count {
				t.Errorf("Count = %d, want %d", l.Count, tc.count)
			}
			if len(l.Articles) != tc.wantLen {
				t.Errorf("len = %d, want %d", len(l.Articles), tc.wantLen)
			}
			if (l.Articles == nil) != tc.wantNil {
				t.Errorf("nil = %v, want %v", l.Articles == nil, tc.wantNil)
			}
			if len(tc.articles) > 0 && &l.Articles[0] != &tc.articles[0] {
				t.Errorf("slice was copied; identity not preserved")
			}
		})
	}
}

func TestArticleDataListNoDefensiveCopy(t *testing.T) {
	src := []ArticleData{sampleArticle("1")}
	l := NewArticleDataList(src, 1)
	src[0].Title = "changed"
	if l.Articles[0].Title != "changed" {
		t.Errorf("expected shared backing array")
	}
}

func TestArticleDataListMarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		list      ArticleDataList
		wantCount float64
		wantIDs   []string
	}{
		{"two articles", NewArticleDataList([]ArticleData{sampleArticle("1"), sampleArticle("2")}, 2), 2, []string{"1", "2"}},
		{"empty", NewArticleDataList([]ArticleData{}, 0), 0, []string{}},
		{"nil becomes empty array", NewArticleDataList(nil, 0), 0, []string{}},
		{"count independent", NewArticleDataList([]ArticleData{sampleArticle("x")}, 50), 50, []string{"x"}},
		{"negative count", NewArticleDataList(nil, -1), -1, []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.list)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(m) != 2 {
				t.Errorf("keys = %v, want exactly articles/articlesCount", m)
			}
			for _, bad := range []string{"articleDatas", "count", "Articles", "Count"} {
				if _, ok := m[bad]; ok {
					t.Errorf("unexpected key %q", bad)
				}
			}
			if string(m["articlesCount"]) != jsonNum(tc.wantCount) {
				t.Errorf("articlesCount = %s, want integer %v", m["articlesCount"], tc.wantCount)
			}
			if string(m["articles"]) == "null" {
				t.Fatalf("articles serialized as null")
			}
			var arts []map[string]any
			if err := json.Unmarshal(m["articles"], &arts); err != nil {
				t.Fatalf("articles not array: %v", err)
			}
			if len(arts) != len(tc.wantIDs) {
				t.Fatalf("len = %d, want %d", len(arts), len(tc.wantIDs))
			}
			for i, id := range tc.wantIDs {
				if arts[i]["id"] != id {
					t.Errorf("articles[%d].id = %v, want %s", i, arts[i]["id"], id)
				}
				if _, ok := arts[i]["author"]; !ok {
					t.Errorf("article not serialized via ArticleData.MarshalJSON")
				}
			}
		})
	}
}

func TestArticleDataListMarshalEmptyExact(t *testing.T) {
	b, err := json.Marshal(NewArticleDataList(nil, 0))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"articles":[],"articlesCount":0}` {
		t.Errorf("got %s", b)
	}
}

func jsonNum(f float64) string {
	b, _ := json.Marshal(int(f))
	return string(b)
}