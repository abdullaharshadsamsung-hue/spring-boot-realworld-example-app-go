package core

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// memFavoriteRepo is an in-memory reference implementation of
// ArticleFavoriteRepository used to validate the contract.
type memFavoriteRepo struct {
	mu   sync.Mutex
	data map[ArticleFavorite]struct{}
	fail error
}

func newMemFavoriteRepo() *memFavoriteRepo {
	return &memFavoriteRepo{data: map[ArticleFavorite]struct{}{}}
}

func (r *memFavoriteRepo) Save(_ context.Context, f ArticleFavorite) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return r.fail
	}
	r.data[f] = struct{}{}
	return nil
}

func (r *memFavoriteRepo) Find(_ context.Context, articleID, userID string) (ArticleFavorite, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return ArticleFavorite{}, false, r.fail
	}
	k := NewArticleFavorite(articleID, userID)
	if _, ok := r.data[k]; ok {
		return k, true, nil
	}
	return ArticleFavorite{}, false, nil
}

func (r *memFavoriteRepo) Remove(_ context.Context, f ArticleFavorite) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return r.fail
	}
	delete(r.data, NewArticleFavorite(f.ArticleID, f.UserID))
	return nil
}

func (r *memFavoriteRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.data)
}

var _ ArticleFavoriteRepository = (*memFavoriteRepo)(nil)

type findCheck struct {
	articleID, userID string
	want              bool
}

func TestArticleFavoriteRepositoryContract(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		saves     []ArticleFavorite
		removes   []ArticleFavorite
		checks    []findCheck
		wantCount int
	}{
		{
			name:      "save new pair then find",
			saves:     []ArticleFavorite{NewArticleFavorite("a1", "u1")},
			checks:    []findCheck{{"a1", "u1", true}},
			wantCount: 1,
		},
		{
			name:      "save existing pair is idempotent",
			saves:     []ArticleFavorite{NewArticleFavorite("a1", "u1"), NewArticleFavorite("a1", "u1")},
			checks:    []findCheck{{"a1", "u1", true}},
			wantCount: 1,
		},
		{
			name:  "same article different users stored independently",
			saves: []ArticleFavorite{NewArticleFavorite("a1", "u1"), NewArticleFavorite("a1", "u2")},
			checks: []findCheck{
				{"a1", "u1", true}, {"a1", "u2", true}, {"a1", "u3", false}, {"a2", "u1", false},
			},
			wantCount: 2,
		},
		{
			name:      "find never saved",
			checks:    []findCheck{{"a1", "u1", false}},
			wantCount: 0,
		},
		{
			name:      "arguments not interchangeable",
			saves:     []ArticleFavorite{NewArticleFavorite("x", "y")},
			checks:    []findCheck{{"y", "x", false}, {"x", "y", true}},
			wantCount: 1,
		},
		{
			name:      "remove existing",
			saves:     []ArticleFavorite{NewArticleFavorite("a1", "u1")},
			removes:   []ArticleFavorite{NewArticleFavorite("a1", "u1")},
			checks:    []findCheck{{"a1", "u1", false}},
			wantCount: 0,
		},
		{
			name:      "remove non-existing is no-op",
			saves:     []ArticleFavorite{NewArticleFavorite("a1", "u1")},
			removes:   []ArticleFavorite{NewArticleFavorite("a9", "u9"), NewArticleFavorite("u1", "a1")},
			checks:    []findCheck{{"a1", "u1", true}},
			wantCount: 1,
		},
		{
			name:      "remove only matching pair",
			saves:     []ArticleFavorite{NewArticleFavorite("a1", "u1"), NewArticleFavorite("a1", "u2")},
			removes:   []ArticleFavorite{NewArticleFavorite("a1", "u1")},
			checks:    []findCheck{{"a1", "u1", false}, {"a1", "u2", true}},
			wantCount: 1,
		},
		{
			name:      "remove from empty repo",
			removes:   []ArticleFavorite{NewArticleFavorite("a1", "u1")},
			checks:    []findCheck{{"a1", "u1", false}},
			wantCount: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var repo ArticleFavoriteRepository = newMemFavoriteRepo()
			for _, f := range tt.saves {
				if err := repo.Save(ctx, f); err != nil {
					t.Fatalf("Save(%v): %v", f, err)
				}
			}
			for _, f := range tt.removes {
				if err := repo.Remove(ctx, f); err != nil {
					t.Fatalf("Remove(%v): %v", f, err)
				}
			}
			for _, c := range tt.checks {
				got, ok, err := repo.Find(ctx, c.articleID, c.userID)
				if err != nil {
					t.Fatalf("Find: %v", err)
				}
				if ok != c.want {
					t.Errorf("Find(%q,%q) ok=%v want %v", c.articleID, c.userID, ok, c.want)
				}
				if ok && (got.GetArticleID() != c.articleID || got.GetUserID() != c.userID) {
					t.Errorf("Find returned %+v", got)
				}
				if !ok && got != (ArticleFavorite{}) {
					t.Errorf("absent find should return zero value, got %+v", got)
				}
			}
			// Find is read-only.
			if n := repo.(*memFavoriteRepo).count(); n != tt.wantCount {
				t.Errorf("count=%d want %d", n, tt.wantCount)
			}
		})
	}
}

func TestArticleFavoriteRepositoryStorageErrors(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("db down")
	repo := newMemFavoriteRepo()
	repo.fail = boom
	f := NewArticleFavorite("a", "u")
	tests := []struct {
		name string
		call func() error
	}{
		{"save", func() error { return repo.Save(ctx, f) }},
		{"find", func() error { _, _, err := repo.Find(ctx, "a", "u"); return err }},
		{"remove", func() error { return repo.Remove(ctx, f) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, boom) {
				t.Errorf("err=%v want %v", err, boom)
			}
		})
	}
}

func TestArticleFavoriteRepositoryConcurrent(t *testing.T) {
	ctx := context.Background()
	repo := newMemFavoriteRepo()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = repo.Save(ctx, NewArticleFavorite("a", "u"))
			_, _, _ = repo.Find(ctx, "a", "u")
		}()
	}
	wg.Wait()
	if repo.count() != 1 {
		t.Errorf("count=%d want 1", repo.count())
	}
}