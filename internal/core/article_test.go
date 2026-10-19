package core

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewTag(t *testing.T) {
	tests := []struct {
		name    string
		tagName string
	}{
		{"simple", "java"},
		{"empty", ""},
		{"whitespace preserved", "  Go Lang  "},
		{"case preserved", "JaVa"},
		{"unicode", "日本語"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tag := NewTag(tc.tagName)
			if tag == nil {
				t.Fatal("NewTag returned nil")
			}
			if tag.Name != tc.tagName {
				t.Errorf("Name = %q, want %q", tag.Name, tc.tagName)
			}
			if tag.ID == "" {
				t.Error("ID is empty")
			}
			if len(tag.ID) != 36 {
				t.Errorf("ID length = %d, want 36", len(tag.ID))
			}
			if !uuidV4Re.MatchString(tag.ID) {
				t.Errorf("ID %q is not a canonical v4 UUID", tag.ID)
			}
		})
	}
}

func TestNewTagSameNameDifferentIDsButEqual(t *testing.T) {
	a, b := NewTag("java"), NewTag("java")
	if a.ID == b.ID {
		t.Errorf("expected different IDs, both %q", a.ID)
	}
	if !a.Equal(b) || !b.Equal(a) {
		t.Error("tags with same name should be equal")
	}
	if a.Key() != b.Key() {
		t.Error("equal tags should have equal keys")
	}
}

func TestNewTagUniqueIDs(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := NewTag("x").ID
		if seen[id] {
			t.Fatalf("duplicate ID %q", id)
		}
		seen[id] = true
	}
}

func TestNewRandomUUIDFormat(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := newRandomUUID()
		if !uuidV4Re.MatchString(id) {
			t.Fatalf("invalid UUID %q", id)
		}
	}
}

func TestZeroValueTag(t *testing.T) {
	var tag Tag
	if tag.ID != "" || tag.Name != "" {
		t.Errorf("zero Tag = %+v, want empty fields", tag)
	}
	if got := tag.String(); got != "Tag(id=, name=)" {
		t.Errorf("String() = %q", got)
	}
}

func TestTagJSONDecodeAndEncode(t *testing.T) {
	var tag Tag
	if err := json.Unmarshal([]byte(`{"id":"abc","name":"go"}`), &tag); err != nil {
		t.Fatal(err)
	}
	if tag.ID != "abc" || tag.Name != "go" {
		t.Errorf("decoded = %+v", tag)
	}
	out, err := json.Marshal(tag)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"id":"abc","name":"go"}` {
		t.Errorf("encoded = %s", out)
	}
}

func TestTagSetGet(t *testing.T) {
	tests := []struct {
		name string
		id   string
		tn   string
	}{
		{"values", "id-1", "java"},
		{"empty", "", ""},
		{"whitespace", " ", " "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tag := NewTag("orig")
			tag.ID = tc.id
			tag.Name = tc.tn
			if tag.ID != tc.id || tag.Name != tc.tn {
				t.Errorf("got %+v", tag)
			}
		})
	}
}

func TestTagEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b *Tag
		want bool
	}{
		{"same name diff ids", &Tag{ID: "1", Name: "java"}, &Tag{ID: "2", Name: "java"}, true},
		{"diff names same id", &Tag{ID: "1", Name: "java"}, &Tag{ID: "1", Name: "go"}, false},
		{"case differs", &Tag{ID: "1", Name: "Java"}, &Tag{ID: "1", Name: "java"}, false},
		{"both empty names", &Tag{ID: "1"}, &Tag{ID: "2"}, true},
		{"nil other", &Tag{Name: "java"}, nil, false},
		{"nil receiver", nil, &Tag{Name: "java"}, false},
		{"both nil", nil, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Equal(tc.b); got != tc.want {
				t.Errorf("a.Equal(b) = %v, want %v", got, tc.want)
			}
			if got := tc.b.Equal(tc.a); got != tc.want {
				t.Errorf("symmetry: b.Equal(a) = %v, want %v", got, tc.want)
			}
			if tc.want && tc.a.Key() != tc.b.Key() {
				t.Errorf("equal tags have different keys %q vs %q", tc.a.Key(), tc.b.Key())
			}
		})
	}
}

func TestTagEqualReflexiveTransitive(t *testing.T) {
	a := &Tag{ID: "1", Name: "x"}
	b := &Tag{ID: "2", Name: "x"}
	c := &Tag{ID: "3", Name: "x"}
	if !a.Equal(a) {
		t.Error("not reflexive")
	}
	if !(a.Equal(b) && b.Equal(c) && a.Equal(c)) {
		t.Error("not transitive")
	}
}

func TestTagKey(t *testing.T) {
	tests := []struct {
		name string
		tag  *Tag
		want string
	}{
		{"name only", &Tag{ID: "abc", Name: "java"}, "java"},
		{"empty", &Tag{}, ""},
		{"nil", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tag.Key(); got != tc.want {
				t.Errorf("Key() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTagSetDeduplicatesByName(t *testing.T) {
	tags := []*Tag{NewTag("java"), NewTag("go"), NewTag("java"), NewTag("Java")}
	set := make(map[string]*Tag)
	for _, tg := range tags {
		set[tg.Key()] = tg
	}
	if len(set) != 3 {
		t.Errorf("set size = %d, want 3", len(set))
	}
}

func TestTagString(t *testing.T) {
	tests := []struct {
		name string
		tag  *Tag
		want string
	}{
		{"populated", &Tag{ID: "id-1", Name: "java"}, "Tag(id=id-1, name=java)"},
		{"empty", &Tag{}, "Tag(id=, name=)"},
		{"nil", nil, "Tag(<nil>)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tag.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTagStringContainsGeneratedID(t *testing.T) {
	tag := NewTag("java")
	s := tag.String()
	if !strings.Contains(s, tag.ID) || !strings.Contains(s, "java") {
		t.Errorf("String() = %q missing id or name", s)
	}
}

func TestIsEmptyHelper(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", true},
		{" ", false},
		{"a", false},
	}
	for _, tc := range tests {
		if got := isEmpty(tc.in); got != tc.want {
			t.Errorf("isEmpty(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}