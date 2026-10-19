package core

import (
	"testing"
	"time"
)

func TestCanWriteArticle(t *testing.T) {
	tests := []struct {
		name string
		u    *User
		a    *Article
		want bool
	}{
		{"author matches", &User{ID: "u1"}, &Article{UserID: "u1"}, true},
		{"different user", &User{ID: "u2"}, &Article{UserID: "u1"}, false},
		{"case sensitive", &User{ID: "abc"}, &Article{UserID: "ABC"}, false},
		{"article user id empty", &User{ID: "u1"}, &Article{UserID: ""}, false},
		{"nil user", nil, &Article{UserID: "u1"}, false},
		{"nil article", &User{ID: "u1"}, nil, false},
		{"both nil", nil, nil, false},
		{"empty user id vs non-empty", &User{ID: ""}, &Article{UserID: "u1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var uCopy User
			var aCopy Article
			if tt.u != nil {
				uCopy = *tt.u
			}
			if tt.a != nil {
				aCopy = *tt.a
			}
			got := CanWriteArticle(tt.u, tt.a)
			if got != tt.want {
				t.Fatalf("CanWriteArticle = %v, want %v", got, tt.want)
			}
			if again := CanWriteArticle(tt.u, tt.a); again != got {
				t.Fatalf("not deterministic")
			}
			if tt.u != nil && *tt.u != uCopy {
				t.Fatalf("user mutated")
			}
			if tt.a != nil && (tt.a.UserID != aCopy.UserID || tt.a.ID != aCopy.ID) {
				t.Fatalf("article mutated")
			}
		})
	}
}

func TestCanWriteComment(t *testing.T) {
	cmt := func(userID string) *Comment {
		return RestoreComment("c1", "body", userID, "a1", time.Unix(0, 0))
	}
	tests := []struct {
		name string
		u    *User
		a    *Article
		c    *Comment
		want bool
	}{
		{"article author, other comment author", &User{ID: "u1"}, &Article{UserID: "u1"}, cmt("u2"), true},
		{"comment author, other article author", &User{ID: "u2"}, &Article{UserID: "u1"}, cmt("u2"), true},
		{"authored both", &User{ID: "u1"}, &Article{UserID: "u1"}, cmt("u1"), true},
		{"authored neither", &User{ID: "u3"}, &Article{UserID: "u1"}, cmt("u2"), false},
		{"article author, nil comment", &User{ID: "u1"}, &Article{UserID: "u1"}, nil, true},
		{"not article author, nil comment", &User{ID: "u2"}, &Article{UserID: "u1"}, nil, false},
		{"case sensitive article", &User{ID: "abc"}, &Article{UserID: "ABC"}, cmt("x"), false},
		{"case sensitive comment", &User{ID: "abc"}, &Article{UserID: "x"}, cmt("ABC"), false},
		{"nil user", nil, &Article{UserID: "u1"}, cmt("u1"), false},
		{"nil article", &User{ID: "u1"}, nil, cmt("u1"), false},
		{"all nil", nil, nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanWriteComment(tt.u, tt.a, tt.c)
			if got != tt.want {
				t.Fatalf("CanWriteComment = %v, want %v", got, tt.want)
			}
			if again := CanWriteComment(tt.u, tt.a, tt.c); again != got {
				t.Fatalf("not deterministic")
			}
			if tt.c != nil && (tt.c.ID() != "c1" || tt.c.Body() != "body" || tt.c.ArticleID() != "a1") {
				t.Fatalf("comment mutated")
			}
		})
	}
}