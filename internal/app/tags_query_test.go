package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeTagReadService struct {
	tags  []string
	err   error
	calls int
	ctx   context.Context
}

func (f *fakeTagReadService) All(ctx context.Context) ([]string, error) {
	f.calls++
	f.ctx = ctx
	return f.tags, f.err
}

// inMemoryTagStore simulates persisting articles whose tags are stored once by name.
type inMemoryTagStore struct {
	order []string
	seen  map[string]bool
}

func newInMemoryTagStore() *inMemoryTagStore {
	return &inMemoryTagStore{seen: map[string]bool{}}
}

func (s *inMemoryTagStore) saveArticle(title, description, body string, tagList []string, authorID string) {
	for _, t := range tagList {
		if !s.seen[t] {
			s.seen[t] = true
			s.order = append(s.order, t)
		}
	}
}

func (s *inMemoryTagStore) All(ctx context.Context) ([]string, error) {
	out := make([]string, len(s.order))
	copy(out, s.order)
	return out, nil
}

func TestNewTagsQueryService(t *testing.T) {
	tests := []struct {
		name    string
		svc     TagReadService
		wantErr error
	}{
		{"nil service", nil, ErrNilTagReadService},
		{"valid service", &fakeTagReadService{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTagsQueryService(tt.svc)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && got != nil {
				t.Fatalf("expected nil service on error")
			}
			if tt.wantErr == nil && got == nil {
				t.Fatalf("expected non-nil service")
			}
		})
	}
}

func TestTagsQueryService_AllTags(t *testing.T) {
	dbErr := errors.New("db down")
	tests := []struct {
		name    string
		tags    []string
		err     error
		want    []string
		wantErr error
	}{
		{"nil from store becomes empty", nil, nil, []string{}, nil},
		{"empty from store", []string{}, nil, []string{}, nil},
		{"single tag", []string{"java"}, nil, []string{"java"}, nil},
		{"pass-through order unchanged", []string{"zeta", "alpha", "mid"}, nil, []string{"zeta", "alpha", "mid"}, nil},
		{"no dedup or filtering", []string{"a", "a", ""}, nil, []string{"a", "a", ""}, nil},
		{"error propagated", []string{"x"}, dbErr, nil, dbErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeTagReadService{tags: tt.tags, err: tt.err}
			svc, err := NewTagsQueryService(fake)
			if err != nil {
				t.Fatal(err)
			}
			type ctxKey struct{}
			ctx := context.WithValue(context.Background(), ctxKey{}, "v")
			got, err := svc.AllTags(ctx)
			if fake.calls != 1 {
				t.Fatalf("calls = %d, want 1", fake.calls)
			}
			if fake.ctx != ctx {
				t.Fatalf("context not forwarded")
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want wrapping %v", err, tt.wantErr)
				}
				if got != nil {
					t.Fatalf("got = %v, want nil", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got == nil {
				t.Fatalf("got nil slice")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}