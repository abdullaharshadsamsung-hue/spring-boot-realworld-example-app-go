package graphql

import (
	"context"
	"errors"
	"testing"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
	"migrated-app/internal/httpapi"
)

type cmFakeArticles struct {
	article *core.Article
	err     error
	calls   int
	gotSlug string
}

func (f *cmFakeArticles) FindBySlug(ctx context.Context, slug string) (*core.Article, error) {
	f.calls++
	f.gotSlug = slug
	return f.article, f.err
}

type cmFakeComments struct {
	saved      []*core.Comment
	saveErr    error
	found      *core.Comment
	findErr    error
	findCalls  int
	gotArticle string
	gotID      string
	removed    []*core.Comment
	removeErr  error
}

func (f *cmFakeComments) Save(ctx context.Context, c *core.Comment) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, c)
	return nil
}

func (f *cmFakeComments) FindByID(ctx context.Context, articleID, id string) (*core.Comment, error) {
	f.findCalls++
	f.gotArticle = articleID
	f.gotID = id
	return f.found, f.findErr
}

func (f *cmFakeComments) Remove(ctx context.Context, c *core.Comment) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, c)
	return nil
}

type cmFakeQuery struct {
	notFound bool
	err      error
	gotID    string
	gotUser  *core.User
	calls    int
}

func (f *cmFakeQuery) FindByID(ctx context.Context, id string, user *core.User) (app.CommentData, bool, error) {
	f.calls++
	f.gotID = id
	f.gotUser = user
	if f.err != nil {
		return app.CommentData{}, false, f.err
	}
	if f.notFound {
		return app.CommentData{}, false, nil
	}
	return app.CommentData{ID: id, Body: "read-body", Author: &app.ProfileData{ID: user.ID}}, true, nil
}

func TestNewCommentMutation(t *testing.T) {
	a, c, q := &cmFakeArticles{}, &cmFakeComments{}, &cmFakeQuery{}
	tests := []struct {
		name    string
		a       CommentMutationArticles
		c       CommentMutationStore
		q       CommentMutationQuery
		wantErr error
	}{
		{"ok", a, c, q, nil},
		{"nil articles", nil, c, q, ErrNilCommentMutationArticles},
		{"nil store", a, nil, q, ErrNilCommentMutationStore},
		{"nil query", a, c, nil, ErrNilCommentMutationQuery},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewCommentMutation(tt.a, tt.c, tt.q)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && m == nil {
				t.Fatal("nil mutation")
			}
			if tt.wantErr != nil && m != nil {
				t.Fatal("expected nil mutation")
			}
		})
	}
}

func TestAddComment(t *testing.T) {
	user := &core.User{ID: "u1", Username: "alice"}
	article := &core.Article{ID: "a1", UserID: "u2", Slug: "hello"}
	dbErr := errors.New("db down")

	tests := []struct {
		name       string
		user       *core.User
		articles   *cmFakeArticles
		comments   *cmFakeComments
		query      *cmFakeQuery
		wantErr    error
		wantAnyErr bool
		wantSaved  int
		wantLookup int
	}{
		{"success", user, &cmFakeArticles{article: article}, &cmFakeComments{}, &cmFakeQuery{}, nil, false, 1, 1},
		{"unauthenticated", nil, &cmFakeArticles{article: article}, &cmFakeComments{}, &cmFakeQuery{}, core.ErrUnauthenticated, true, 0, 0},
		{"article not found sentinel", user, &cmFakeArticles{err: core.ErrNotFound}, &cmFakeComments{}, &cmFakeQuery{}, httpapi.ErrResourceNotFound, true, 0, 1},
		{"article nil", user, &cmFakeArticles{}, &cmFakeComments{}, &cmFakeQuery{}, httpapi.ErrResourceNotFound, true, 0, 1},
		{"article lookup error", user, &cmFakeArticles{err: dbErr}, &cmFakeComments{}, &cmFakeQuery{}, dbErr, true, 0, 1},
		{"save error", user, &cmFakeArticles{article: article}, &cmFakeComments{saveErr: dbErr}, &cmFakeQuery{}, dbErr, true, 0, 1},
		{"read model missing", user, &cmFakeArticles{article: article}, &cmFakeComments{}, &cmFakeQuery{notFound: true}, httpapi.ErrResourceNotFound, true, 1, 1},
		{"read model error", user, &cmFakeArticles{article: article}, &cmFakeComments{}, &cmFakeQuery{err: dbErr}, dbErr, true, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewCommentMutation(tt.articles, tt.comments, tt.query)
			if err != nil {
				t.Fatal(err)
			}
			ctx := auth.WithUser(context.Background(), tt.user)
			res, err := m.AddComment(ctx, "hello", "nice post")
			if tt.wantAnyErr {
				if err == nil || !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if res != nil {
					t.Fatal("expected nil result")
				}
			} else if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if len(tt.comments.saved) != tt.wantSaved {
				t.Fatalf("saved = %d, want %d", len(tt.comments.saved), tt.wantSaved)
			}
			if tt.articles.calls != tt.wantLookup {
				t.Fatalf("article lookups = %d, want %d", tt.articles.calls, tt.wantLookup)
			}
			if tt.wantSaved == 1 {
				c := tt.comments.saved[0]
				if c.UserID() != user.ID || c.ArticleID() != article.ID || c.Body() != "nice post" || c.ID() == "" {
					t.Fatalf("bad saved comment: %+v", c)
				}
				if tt.query.gotID != c.ID() || tt.query.gotUser != user {
					t.Fatalf("read-model lookup id=%q user=%v", tt.query.gotID, tt.query.gotUser)
				}
				if tt.articles.gotSlug != "hello" {
					t.Fatalf("slug = %q", tt.articles.gotSlug)
				}
			}
			if !tt.wantAnyErr {
				if res == nil || res.LocalContext == nil {
					t.Fatal("missing local context")
				}
				if res.LocalContext.ID != tt.comments.saved[0].ID() {
					t.Fatalf("local context id = %q", res.LocalContext.ID)
				}
				if res.LocalContext.Author == nil || res.LocalContext.Author.ID != user.ID {
					t.Fatal("author should be current user")
				}
			}
		})
	}
}

func TestDeleteComment(t *testing.T) {
	articleAuthor := &core.User{ID: "author"}
	commentAuthor := &core.User{ID: "commenter"}
	stranger := &core.User{ID: "stranger"}
	article := &core.Article{ID: "a1", UserID: "author", Slug: "hello"}
	comment := core.RestoreComment("c1", "body", "commenter", "a1", article.CreatedAt)
	dbErr := errors.New("db down")

	tests := []struct {
		name        string
		user        *core.User
		articles    *cmFakeArticles
		comments    *cmFakeComments
		wantErr     error
		wantRemoved bool
		wantFind    int
	}{
		{"comment author", commentAuthor, &cmFakeArticles{article: article}, &cmFakeComments{found: comment}, nil, true, 1},
		{"article author", articleAuthor, &cmFakeArticles{article: article}, &cmFakeComments{found: comment}, nil, true, 1},
		{"unauthenticated", nil, &cmFakeArticles{article: article}, &cmFakeComments{found: comment}, core.ErrUnauthenticated, false, 0},
		{"article not found", commentAuthor, &cmFakeArticles{err: core.ErrNotFound}, &cmFakeComments{found: comment}, httpapi.ErrResourceNotFound, false, 0},
		{"article nil", commentAuthor, &cmFakeArticles{}, &cmFakeComments{found: comment}, httpapi.ErrResourceNotFound, false, 0},
		{"article lookup error", commentAuthor, &cmFakeArticles{err: dbErr}, &cmFakeComments{found: comment}, dbErr, false, 0},
		{"comment not found", commentAuthor, &cmFakeArticles{article: article}, &cmFakeComments{findErr: core.ErrNotFound}, httpapi.ErrResourceNotFound, false, 1},
		{"comment nil (other article)", commentAuthor, &cmFakeArticles{article: article}, &cmFakeComments{}, httpapi.ErrResourceNotFound, false, 1},
		{"comment lookup error", commentAuthor, &cmFakeArticles{article: article}, &cmFakeComments{findErr: dbErr}, dbErr, false, 1},
		{"stranger forbidden", stranger, &cmFakeArticles{article: article}, &cmFakeComments{found: comment}, httpapi.ErrNoAuthorization, false, 1},
		{"remove error", commentAuthor, &cmFakeArticles{article: article}, &cmFakeComments{found: comment, removeErr: dbErr}, dbErr, false, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewCommentMutation(tt.articles, tt.comments, &cmFakeQuery{})
			if err != nil {
				t.Fatal(err)
			}
			ctx := auth.WithUser(context.Background(), tt.user)
			res, err := m.DeleteComment(ctx, "hello", "c1")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if res != nil {
					t.Fatal("expected nil result")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if res == nil || !res.Success {
					t.Fatal("expected success=true")
				}
			}
			if got := len(tt.comments.removed) == 1; got != tt.wantRemoved {
				t.Fatalf("removed = %v, want %v", got, tt.wantRemoved)
			}
			if tt.user == nil && tt.articles.calls != 0 {
				t.Fatal("article lookup before auth")
			}
			if tt.comments.findCalls != tt.wantFind {
				t.Fatalf("find calls = %d, want %d", tt.comments.findCalls, tt.wantFind)
			}
			if tt.wantFind == 1 && (tt.comments.gotArticle != article.ID || tt.comments.gotID != "c1") {
				t.Fatalf("lookup scoped to %q/%q", tt.comments.gotArticle, tt.comments.gotID)
			}
		})
	}
}