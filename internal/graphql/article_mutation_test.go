package graphql

import (
	"context"
	"errors"
	"testing"

	"migrated-app/internal/app"
	"migrated-app/internal/core"
	"migrated-app/internal/httpapi"
)

type amCommands struct {
	createCalls int
	updateCalls int
}

func (c *amCommands) CreateArticle(ctx context.Context, p app.NewArticleParam, u *core.User) (*core.Article, error) {
	c.createCalls++
	return nil, nil
}

func (c *amCommands) UpdateArticle(ctx context.Context, a *core.Article, p app.UpdateArticleParam) (*core.Article, error) {
	c.updateCalls++
	return nil, nil
}

type amFavorites struct {
	core.ArticleFavoriteRepository
}

type amArticles struct {
	findCalls   int
	removeCalls int
	article     *core.Article
	findErr     error
}

func (a *amArticles) FindBySlug(ctx context.Context, slug string) (*core.Article, error) {
	a.findCalls++
	return a.article, a.findErr
}

func (a *amArticles) Remove(ctx context.Context, art *core.Article) error {
	a.removeCalls++
	return nil
}

func amNew(t *testing.T, c *amCommands, a *amArticles) *ArticleMutation {
	t.Helper()
	m, err := NewArticleMutation(c, &amFavorites{}, a)
	if err != nil || m == nil {
		t.Fatalf("NewArticleMutation: %v", err)
	}
	return m
}

func TestNewArticleMutationErrors(t *testing.T) {
	if _, err := NewArticleMutation(nil, &amFavorites{}, &amArticles{}); !errors.Is(err, ErrNilArticleMutationCommands) {
		t.Fatalf("got %v", err)
	}
	if _, err := NewArticleMutation(&amCommands{}, nil, &amArticles{}); !errors.Is(err, ErrNilArticleMutationFavorites) {
		t.Fatalf("got %v", err)
	}
	if _, err := NewArticleMutation(&amCommands{}, &amFavorites{}, nil); !errors.Is(err, ErrNilArticleMutationArticles) {
		t.Fatalf("got %v", err)
	}
}

func TestDeref(t *testing.T) {
	if deref(nil) != "" {
		t.Fatal("nil should give empty")
	}
	s := "x"
	if deref(&s) != "x" {
		t.Fatal("want x")
	}
}

func TestCreateArticleUnauthenticated(t *testing.T) {
	c := &amCommands{}
	m := amNew(t, c, &amArticles{})
	res, err := m.CreateArticle(context.Background(), CreateArticleInput{Title: "t"})
	if err == nil || res != nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if c.createCalls != 0 {
		t.Fatal("command should not be called")
	}
}

func TestUpdateArticleNotFoundBeforeAuth(t *testing.T) {
	c := &amCommands{}
	a := &amArticles{}
	m := amNew(t, c, a)
	_, err := m.UpdateArticle(context.Background(), "slug", UpdateArticleInput{})
	if !errors.Is(err, httpapi.ErrResourceNotFound) {
		t.Fatalf("got %v", err)
	}
	if a.findCalls != 1 || c.updateCalls != 0 {
		t.Fatalf("find=%d update=%d", a.findCalls, c.updateCalls)
	}
}

func TestDeleteAndFavoriteUnauthenticated(t *testing.T) {
	a := &amArticles{}
	m := amNew(t, &amCommands{}, a)
	if res, err := m.DeleteArticle(context.Background(), "s"); err == nil || res != nil {
		t.Fatalf("delete res=%v err=%v", res, err)
	}
	if res, err := m.FavoriteArticle(context.Background(), "s"); err == nil || res != nil {
		t.Fatalf("favorite res=%v err=%v", res, err)
	}
	if res, err := m.UnfavoriteArticle(context.Background(), "s"); err == nil || res != nil {
		t.Fatalf("unfavorite res=%v err=%v", res, err)
	}
	if a.findCalls != 0 || a.removeCalls != 0 {
		t.Fatalf("find=%d remove=%d", a.findCalls, a.removeCalls)
	}
}

func TestMutationFieldNames(t *testing.T) {
	if MutationFieldCreateArticle != "createArticle" || MutationFieldDeleteArticle != "deleteArticle" ||
		MutationFieldUpdateArticle != "updateArticle" || MutationFieldFavoriteArticle != "favoriteArticle" ||
		MutationFieldUnfavoriteArticle != "unfavoriteArticle" {
		t.Fatal("unexpected field names")
	}
}