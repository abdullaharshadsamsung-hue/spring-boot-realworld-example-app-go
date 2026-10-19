package app

import (
	"context"
	"errors"
	"testing"

	"migrated-app/internal/core"
)

type fakeSlugFinder struct {
	existing map[string]bool
	err      error
	calls    []string
	users    []*core.User
}

func (f *fakeSlugFinder) FindBySlug(ctx context.Context, slug string, user *core.User) (ArticleData, bool, error) {
	f.calls = append(f.calls, slug)
	f.users = append(f.users, user)
	if f.err != nil {
		return ArticleData{}, false, f.err
	}
	if f.existing[slug] {
		return ArticleData{ID: "id-" + slug, Slug: slug}, true, nil
	}
	return ArticleData{}, false, nil
}

func TestNewDuplicatedArticleValidator(t *testing.T) {
	v, err := NewDuplicatedArticleValidator(nil)
	if !errors.Is(err, ErrNilArticleBySlugFinder) || v != nil {
		t.Fatalf("want ErrNilArticleBySlugFinder, got %v %v", v, err)
	}
	v, err = NewDuplicatedArticleValidator(&fakeSlugFinder{})
	if err != nil || v == nil {
		t.Fatalf("unexpected: %v %v", v, err)
	}
}

func TestDuplicatedArticleValidator_IsValid(t *testing.T) {
	dbErr := errors.New("db down")
	tests := []struct {
		name      string
		existing  map[string]bool
		findErr   error
		title     string
		want      bool
		wantErr   error
		wantCalls int
		wantSlug  string
	}{
		{"no match", map[string]bool{}, nil, "How to train your dragon", true, nil, 1, "how-to-train-your-dragon"},
		{"exact duplicate", map[string]bool{"how-to-train-your-dragon": true}, nil, "How to train your dragon", false, nil, 1, "how-to-train-your-dragon"},
		{"normalized duplicate", map[string]bool{"how-to-train-your-dragon": true}, nil, "HOW to, train   your dragon.", false, nil, 1, "how-to-train-your-dragon"},
		{"blank title skips lookup", map[string]bool{"": true}, nil, "", true, nil, 0, ""},
		{"whitespace title skips lookup", nil, nil, "   ", true, nil, 0, ""},
		{"lookup error propagates", nil, dbErr, "anything", false, dbErr, 1, "anything"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeSlugFinder{existing: tc.existing, err: tc.findErr}
			v, _ := NewDuplicatedArticleValidator(f)
			got, err := v.IsValid(context.Background(), tc.title)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want err %v, got %v", tc.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
			if len(f.calls) != tc.wantCalls {
				t.Fatalf("calls %d want %d", len(f.calls), tc.wantCalls)
			}
			if tc.wantCalls > 0 {
				if f.calls[0] != tc.wantSlug {
					t.Errorf("slug %q want %q", f.calls[0], tc.wantSlug)
				}
				if f.calls[0] != core.ToSlug(tc.title) {
					t.Errorf("slug differs from core.ToSlug")
				}
				if f.users[0] != nil {
					t.Errorf("lookup must be anonymous")
				}
			}
		})
	}
}