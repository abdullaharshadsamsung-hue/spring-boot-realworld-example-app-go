package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFollowRelationZeroValue(t *testing.T) {
	var r FollowRelation
	if r.UserID != "" || r.TargetID != "" {
		t.Fatalf("zero value not empty: %+v", r)
	}
	if !r.Equal(FollowRelation{}) {
		t.Fatalf("zero value should equal FollowRelation{}")
	}
}

func TestNewFollowRelation(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		targetID string
	}{
		{"normal", "u1", "u2"},
		{"empty values", "", ""},
		{"empty user", "", "u2"},
		{"empty target", "u1", ""},
		{"self follow", "u1", "u1"},
		{"whitespace preserved", " ", "  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewFollowRelation(tt.userID, tt.targetID)
			if r.UserID != tt.userID {
				t.Errorf("UserID = %q, want %q", r.UserID, tt.userID)
			}
			if r.TargetID != tt.targetID {
				t.Errorf("TargetID = %q, want %q", r.TargetID, tt.targetID)
			}
		})
	}
}

func TestFollowRelationArgumentOrder(t *testing.T) {
	r := NewFollowRelation("follower", "target")
	if r.UserID != "follower" || r.TargetID != "target" {
		t.Fatalf("argument order not preserved: %+v", r)
	}
}

func TestFollowRelationSetters(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*FollowRelation)
		wantUser   string
		wantTarget string
	}{
		{"set user", func(r *FollowRelation) { r.UserID = "a" }, "a", "t0"},
		{"set target", func(r *FollowRelation) { r.TargetID = "b" }, "u0", "b"},
		{"set both last wins", func(r *FollowRelation) {
			r.UserID = "x"
			r.UserID = "y"
			r.TargetID = "z"
		}, "y", "z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewFollowRelation("u0", "t0")
			tt.mutate(&r)
			if r.UserID != tt.wantUser || r.TargetID != tt.wantTarget {
				t.Errorf("got %+v, want (%q,%q)", r, tt.wantUser, tt.wantTarget)
			}
		})
	}
}

func TestFollowRelationEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b FollowRelation
		want bool
	}{
		{"same values", NewFollowRelation("a", "b"), NewFollowRelation("a", "b"), true},
		{"different user", NewFollowRelation("a", "b"), NewFollowRelation("c", "b"), false},
		{"different target", NewFollowRelation("a", "b"), NewFollowRelation("a", "c"), false},
		{"swapped", NewFollowRelation("a", "b"), NewFollowRelation("b", "a"), false},
		{"both empty", FollowRelation{}, FollowRelation{}, true},
		{"empty vs set", FollowRelation{}, NewFollowRelation("a", "b"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("a.Equal(b) = %v, want %v", got, tt.want)
			}
			if got := tt.b.Equal(tt.a); got != tt.want {
				t.Errorf("symmetry: b.Equal(a) = %v, want %v", got, tt.want)
			}
			if got := tt.a == tt.b; got != tt.want {
				t.Errorf("== = %v, want %v", got, tt.want)
			}
			if !tt.a.Equal(tt.a) {
				t.Errorf("not reflexive")
			}
		})
	}
}

func TestFollowRelationEqualTransitive(t *testing.T) {
	a := NewFollowRelation("x", "y")
	b := NewFollowRelation("x", "y")
	c := NewFollowRelation("x", "y")
	if !(a.Equal(b) && b.Equal(c) && a.Equal(c)) {
		t.Fatal("equality not transitive")
	}
}

func TestFollowRelationDifferentTypeAndNil(t *testing.T) {
	var a interface{} = NewFollowRelation("a", "b")
	others := []interface{}{nil, "a", ArticleFavorite{ArticleID: "a", UserID: "b"}, &FollowRelation{UserID: "a", TargetID: "b"}}
	for _, o := range others {
		if a == o {
			t.Errorf("FollowRelation should not equal %#v", o)
		}
	}
}

func TestFollowRelationMapKey(t *testing.T) {
	m := map[FollowRelation]int{}
	m[NewFollowRelation("a", "b")] = 1
	m[NewFollowRelation("a", "b")]++
	m[NewFollowRelation("b", "a")] = 10
	if len(m) != 2 {
		t.Fatalf("len = %d, want 2", len(m))
	}
	if m[NewFollowRelation("a", "b")] != 2 {
		t.Errorf("equal keys should hash identically")
	}
}

func TestFollowRelationString(t *testing.T) {
	tests := []struct {
		name string
		r    FollowRelation
		want string
	}{
		{"set", NewFollowRelation("u1", "u2"), "FollowRelation(userId=u1, targetId=u2)"},
		{"empty", FollowRelation{}, "FollowRelation(userId=, targetId=)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
	s := NewFollowRelation("u1", "u2").String()
	if !strings.Contains(s, "u1") || !strings.Contains(s, "u2") {
		t.Errorf("String() missing fields: %q", s)
	}
}

func TestFollowRelationJSON(t *testing.T) {
	r := NewFollowRelation("u1", "u2")
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"userId":"u1","targetId":"u2"}` {
		t.Errorf("json = %s", b)
	}
	var back FollowRelation
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Equal(r) {
		t.Errorf("roundtrip mismatch: %+v", back)
	}
}