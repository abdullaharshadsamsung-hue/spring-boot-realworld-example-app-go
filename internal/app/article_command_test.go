package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"migrated-app/internal/core"
)

type fakeArticleWriter struct {
	saved []*core.Article
	err   error
}

func (f *fakeArticleWriter) Save(_ context.Context, a *core.Article) error {
	f.saved = append(f.saved, a)
	return f.err
}

type fakeValidator struct {
	err    error
	calls  int
	params []NewArticleParam
}

func (f *fakeValidator) Validate(_ context.Context, p NewArticleParam) error {
	f.calls++
	f.params = append(f.params, p)
	return f.err
}

func TestCreateArticle(t *testing.T) {
	fixed := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	validationErr := errors.New("can't be empty")
	repoErr := errors.New("db down")
	creator := &core.User{ID: "user-1", Username: "jake"}

	tests := []struct {
		name         string
		param        NewArticleParam
		creator      *core.User
		validatorErr error
		repoErr      error
		wantErr      error
		wantAnyErr   bool
		wantSaves    int
		wantTags     []string
	}{
		{
			name:      "valid param",
			param:     NewArticleParam{Title: "How To Train", Description: "desc", Body: "body", TagList: []string{"java", "go", "java"}},
			creator:   creator,
			wantSaves: 1,
			wantTags:  []string{"java", "go"},
		},
		{
			name:      "empty tag list",
			param:     NewArticleParam{Title: "Title", Description: "d", Body: "b", TagList: []string{}},
			creator:   creator,
			wantSaves: 1,
			wantTags:  []string{},
		},
		{
			name:         "validation failure",
			param:        NewArticleParam{Title: "", Description: "d", Body: "b"},
			creator:      creator,
			validatorErr: validationErr,
			wantErr:      validationErr,
			wantSaves:    0,
		},
		{
			name:      "repository error propagates",
			param:     NewArticleParam{Title: "T", Description: "d", Body: "b"},
			creator:   creator,
			repoErr:   repoErr,
			wantErr:   repoErr,
			wantSaves: 1,
		},
		{
			name:       "nil creator",
			param:      NewArticleParam{Title: "T", Description: "d", Body: "b"},
			creator:    nil,
			wantAnyErr: true,
			wantSaves:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeArticleWriter{err: tc.repoErr}
			val := &fakeValidator{err: tc.validatorErr}
			svc := NewArticleCommandService(repo, val)
			svc.now = func() time.Time { return fixed }

			got, err := svc.CreateArticle(context.Background(), tc.param, tc.creator)

			if len(repo.saved) != tc.wantSaves {
				t.Fatalf("saves = %d, want %d", len(repo.saved), tc.wantSaves)
			}
			if tc.wantErr != nil || tc.wantAnyErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("expected nil article")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if val.calls != 1 {
				t.Fatalf("validator calls = %d", val.calls)
			}
			if repo.saved[0] != got {
				t.Fatalf("saved instance differs from returned")
			}
			if got.Title != tc.param.Title || got.Description != tc.param.Description || got.Body != tc.param.Body {
				t.Fatalf("fields mismatch: %+v", got)
			}
			if got.UserID != tc.creator.ID {
				t.Fatalf("UserID = %q", got.UserID)
			}
			if got.ID == "" {
				t.Fatalf("missing ID")
			}
			if got.Slug != core.ToSlug(tc.param.Title) {
				t.Fatalf("slug = %q", got.Slug)
			}
			if !got.CreatedAt.Equal(fixed) {
				t.Fatalf("CreatedAt = %v", got.CreatedAt)
			}
			if len(got.Tags) != len(tc.wantTags) {
				t.Fatalf("tags = %v, want %v", got.Tags, tc.wantTags)
			}
			for i, n := range tc.wantTags {
				if got.Tags[i].Name != n {
					t.Fatalf("tag[%d] = %q, want %q", i, got.Tags[i].Name, n)
				}
			}
		})
	}
}

func TestUpdateArticle(t *testing.T) {
	repoErr := errors.New("db down")
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	base := func() *core.Article {
		return &core.Article{
			ID: "a1", UserID: "u1", Slug: "old-title", Title: "Old Title",
			Description: "old desc", Body: "old body",
			Tags:      []core.Tag{{ID: "t1", Name: "go"}},
			CreatedAt: old, UpdatedAt: old,
		}
	}

	tests := []struct {
		name        string
		article     *core.Article
		param       UpdateArticleParam
		repoErr     error
		wantErr     bool
		wantSaves   int
		wantTitle   string
		wantSlug    string
		wantDesc    string
		wantBody    string
		wantUpdated bool
	}{
		{
			name: "all fields", article: base(),
			param:     NewUpdateArticleParam("New Title", "new body", "new desc"),
			wantSaves: 1, wantTitle: "New Title", wantSlug: "new-title",
			wantDesc: "new desc", wantBody: "new body", wantUpdated: true,
		},
		{
			name: "only body", article: base(),
			param:     NewUpdateArticleParam("", "new body", ""),
			wantSaves: 1, wantTitle: "Old Title", wantSlug: "old-title",
			wantDesc: "old desc", wantBody: "new body", wantUpdated: true,
		},
		{
			name: "all empty", article: base(),
			param:     NewUpdateArticleParam("", "", ""),
			wantSaves: 1, wantTitle: "Old Title", wantSlug: "old-title",
			wantDesc: "old desc", wantBody: "old body",
		},
		{
			name: "repo error", article: base(),
			param:   NewUpdateArticleParam("X", "", ""),
			repoErr: repoErr, wantErr: true, wantSaves: 1,
		},
		{
			name: "nil article", article: nil,
			param: NewUpdateArticleParam("X", "", ""), wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeArticleWriter{err: tc.repoErr}
			svc := NewArticleCommandService(repo, &fakeValidator{})
			got, err := svc.UpdateArticle(context.Background(), tc.article, tc.param)
			if len(repo.saved) != tc.wantSaves {
				t.Fatalf("saves = %d, want %d", len(repo.saved), tc.wantSaves)
			}
			if tc.wantErr {
				if err == nil || got != nil {
					t.Fatalf("expected error, got %v %v", got, err)
				}
				if tc.repoErr != nil && !errors.Is(err, tc.repoErr) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.article || repo.saved[0] != tc.article {
				t.Fatalf("expected same instance")
			}
			if got.Title != tc.wantTitle || got.Slug != tc.wantSlug || got.Description != tc.wantDesc || got.Body != tc.wantBody {
				t.Fatalf("unexpected article: %+v", got)
			}
			if tc.wantUpdated == got.UpdatedAt.Equal(old) {
				t.Fatalf("UpdatedAt refresh mismatch: %v", got.UpdatedAt)
			}
			if got.UserID != "u1" || len(got.Tags) != 1 || got.Tags[0].Name != "go" {
				t.Fatalf("tags/author changed: %+v", got)
			}
		})
	}
}

func TestUpdateArticleParamJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    UpdateArticleParam
		wantErr error
		anyErr  bool
	}{
		{"full", `{"article":{"title":"t","body":"b","description":"d"}}`, NewUpdateArticleParam("t", "b", "d"), nil, false},
		{"partial", `{"article":{"body":"b"}}`, NewUpdateArticleParam("", "b", ""), nil, false},
		{"missing root", `{"title":"t"}`, UpdateArticleParam{}, ErrMissingArticleRoot, true},
		{"null root", `{"article":null}`, UpdateArticleParam{}, ErrMissingArticleRoot, true},
		{"invalid json", `{`, UpdateArticleParam{}, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var p UpdateArticleParam
			err := json.Unmarshal([]byte(tc.input), &p)
			if tc.anyErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if p != tc.want {
				t.Fatalf("got %+v want %+v", p, tc.want)
			}
		})
	}

	orig := NewUpdateArticleParam("t", "b", "d")
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var back UpdateArticleParam
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back != orig || back.Title() != "t" || back.Body() != "b" || back.Description() != "d" {
		t.Fatalf("round trip mismatch: %s -> %+v", data, back)
	}
}