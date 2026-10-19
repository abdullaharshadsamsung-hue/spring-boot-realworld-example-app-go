package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"migrated-app/internal/app"
	"migrated-app/internal/core"
)

// FavoriteArticleFinder is the write-side article lookup used by
// FavoritesHandler. core.ArticleRepository satisfies it.
type FavoriteArticleFinder interface {
	// FindBySlug returns the article or an error matching core.ErrNotFound.
	FindBySlug(ctx context.Context, slug string) (*core.Article, error)
}

// FavoriteArticleQuerier is the read-side dependency of FavoritesHandler.
// *app.ArticleQueryService satisfies it.
type FavoriteArticleQuerier interface {
	// FindBySlug returns the article read model as seen by user. The bool is
	// false when no such article exists.
	FindBySlug(ctx context.Context, slug string, user *core.User) (app.ArticleData, bool, error)
}

// Compile-time checks that the real collaborators satisfy the ports.
var (
	_ FavoriteArticleFinder  = (core.ArticleRepository)(nil)
	_ FavoriteArticleQuerier = (*app.ArticleQueryService)(nil)
)

// ErrNilFavoriteRepository is returned by NewFavoritesHandler when no favorite repository is supplied.
var ErrNilFavoriteRepository = errors.New("httpapi: article favorite repository is nil")

// ErrNilFavoriteArticleFinder is returned by NewFavoritesHandler when no article repository is supplied.
var ErrNilFavoriteArticleFinder = errors.New("httpapi: article repository is nil")

// ErrNilFavoriteArticleQuerier is returned by NewFavoritesHandler when no article query service is supplied.
var ErrNilFavoriteArticleQuerier = errors.New("httpapi: article query service is nil")

// FavoritesHandler serves the /articles/{slug}/favorite endpoints. It
// replaces the source's ArticleFavoriteApi @RestController.
type FavoritesHandler struct {
	favorites core.ArticleFavoriteRepository
	articles  FavoriteArticleFinder
	queries   FavoriteArticleQuerier
}

// NewFavoritesHandler creates a FavoritesHandler.
func NewFavoritesHandler(favorites core.ArticleFavoriteRepository, articles FavoriteArticleFinder, queries FavoriteArticleQuerier) (*FavoritesHandler, error) {
	if favorites == nil {
		return nil, ErrNilFavoriteRepository
	}
	if articles == nil {
		return nil, ErrNilFavoriteArticleFinder
	}
	if queries == nil {
		return nil, ErrNilFavoriteArticleQuerier
	}
	return &FavoritesHandler{favorites: favorites, articles: articles, queries: queries}, nil
}

// Register mounts the handler's routes on mux:
//
//	POST   /articles/{slug}/favorite → FavoriteArticle   (auth required)
//	DELETE /articles/{slug}/favorite → UnfavoriteArticle (auth required)
//
// The Authenticate middleware must wrap mux so the current user is on the
// request context. Anonymous requests get 401 with an empty body.
func (h *FavoritesHandler) Register(mux *http.ServeMux) {
	mux.Handle("POST /articles/{slug}/favorite", requireUser(h.FavoriteArticle))
	mux.Handle("DELETE /articles/{slug}/favorite", requireUser(h.UnfavoriteArticle))
}

// articleResponse is the wire shape {"article": {...}}.
type favoriteArticleResponse struct {
	Article app.ArticleData `json:"article"`
}

// FavoriteArticle marks {slug} as favorited by the current user and writes
// 200 {"article":...}; 404 if the article does not exist. Saving is
// idempotent (delegated to the repository). The request body is ignored.
func (h *FavoritesHandler) FavoriteArticle(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	user := currentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}
	slug := r.PathValue("slug")
	article, err := h.articles.FindBySlug(ctx, slug)
	if err != nil {
		return notFoundOr(err, "favorite article: find article")
	}
	if err := h.favorites.Save(ctx, core.NewArticleFavorite(article.ID, user.ID)); err != nil {
		return fmt.Errorf("favorite article: save favorite: %w", err)
	}
	return h.writeArticle(w, r, slug, user)
}

// UnfavoriteArticle removes the current user's favorite on {slug} if one
// exists and writes 200 {"article":...}; 404 if the article does not exist.
// A missing favorite is silently ignored.
func (h *FavoritesHandler) UnfavoriteArticle(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	user := currentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}
	slug := r.PathValue("slug")
	article, err := h.articles.FindBySlug(ctx, slug)
	if err != nil {
		return notFoundOr(err, "unfavorite article: find article")
	}
	favorite, found, err := h.favorites.Find(ctx, article.ID, user.ID)
	if err != nil {
		return fmt.Errorf("unfavorite article: find favorite: %w", err)
	}
	if found {
		if err := h.favorites.Remove(ctx, favorite); err != nil {
			return fmt.Errorf("unfavorite article: remove favorite: %w", err)
		}
	}
	return h.writeArticle(w, r, slug, user)
}

// writeArticle loads the article read model from the viewer's perspective
// and writes 200 {"article":...}.
//
// MIGRATION_NOTE: the source called Optional.get() here, which throws
// NoSuchElementException (500) if the article vanished between the write and
// the read. That behaviour is preserved: a missing article yields a plain
// error, which the error handler maps to 500.
func (h *FavoritesHandler) writeArticle(w http.ResponseWriter, r *http.Request, slug string, viewer *core.User) error {
	data, found, err := h.queries.FindBySlug(r.Context(), slug, viewer)
	if err != nil {
		return fmt.Errorf("article %q: %w", slug, err)
	}
	if !found {
		return fmt.Errorf("article %q disappeared after favorite update", slug)
	}
	writeErrorJSON(w, http.StatusOK, favoriteArticleResponse{Article: data})
	return nil
}
