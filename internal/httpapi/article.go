package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"migrated-app/internal/app"
	"migrated-app/internal/core"
)

// ArticleQuerier is the read-side dependency of ArticleHandler.
// *app.ArticleQueryService satisfies it.
type ArticleQuerier interface {
	// FindBySlug loads the article read model as seen by user (nil = anonymous).
	FindBySlug(ctx context.Context, slug string, user *core.User) (app.ArticleData, bool, error)
}

// ArticleRepository is the write-side aggregate store used by ArticleHandler.
type ArticleRepository interface {
	// FindBySlug returns the article with slug. A nil article or an error
	// matching core.ErrNotFound means it does not exist.
	FindBySlug(ctx context.Context, slug string) (*core.Article, error)
	// Remove deletes article.
	Remove(ctx context.Context, article *core.Article) error
}

// ArticleUpdater applies an UpdateArticleParam to an article and persists it.
// *app.ArticleCommandService is expected to satisfy it.
type ArticleUpdater interface {
	// UpdateArticle changes title/description/body (blank values are ignored)
	// and returns the updated aggregate, whose slug may have changed.
	UpdateArticle(ctx context.Context, article *core.Article, param app.UpdateArticleParam) (*core.Article, error)
}

// ArticleWriteAuthorizer reports whether user may modify article. It replaces
// the static AuthorizationService.canWriteArticle.
type ArticleWriteAuthorizer func(user *core.User, article *core.Article) bool

// Constructor errors for NewArticleHandler.
var (
	ErrNilArticleQuerier    = errors.New("httpapi: article query service is nil")
	ErrNilArticleRepository = errors.New("httpapi: article repository is nil")
	ErrNilArticleUpdater    = errors.New("httpapi: article command service is nil")
	ErrNilArticleAuthorizer = errors.New("httpapi: article write authorizer is nil")
)

// ArticleHandler serves the single-article resource /articles/{slug}. It
// replaces the source's ArticleApi @RestController.
type ArticleHandler struct {
	queries  ArticleQuerier
	repo     ArticleRepository
	commands ArticleUpdater
	canWrite ArticleWriteAuthorizer
}

// NewArticleHandler creates an ArticleHandler. All dependencies are required.
func NewArticleHandler(queries ArticleQuerier, repo ArticleRepository, commands ArticleUpdater, canWrite ArticleWriteAuthorizer) (*ArticleHandler, error) {
	switch {
	case queries == nil:
		return nil, ErrNilArticleQuerier
	case repo == nil:
		return nil, ErrNilArticleRepository
	case commands == nil:
		return nil, ErrNilArticleUpdater
	case canWrite == nil:
		return nil, ErrNilArticleAuthorizer
	}
	return &ArticleHandler{queries: queries, repo: repo, commands: commands, canWrite: canWrite}, nil
}

// Register mounts the handler's routes on mux:
//
//	GET    /articles/{slug} → GetArticle    (auth optional)
//	PUT    /articles/{slug} → UpdateArticle (auth required)
//	DELETE /articles/{slug} → DeleteArticle (auth required)
func (h *ArticleHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /articles/{slug}", ErrorHandlerFunc(h.GetArticle))
	mux.Handle("PUT /articles/{slug}", requireUser(h.UpdateArticle))
	mux.Handle("DELETE /articles/{slug}", requireUser(h.DeleteArticle))
}

type articleResponse struct {
	Article app.ArticleData `json:"article"`
}

// writeSpringError writes Spring Boot's default error body for status.
func writeSpringError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	writeErrorJSON(w, status, ErrorBody{
		Timestamp: now().UTC().Format(timestampLayout),
		Status:    status,
		Error:     http.StatusText(status),
		Message:   msg,
		Path:      r.URL.Path,
	})
}

// loadArticle fetches the aggregate by slug. found is false for unknown slugs.
func (h *ArticleHandler) loadArticle(ctx context.Context, slug string) (*core.Article, bool, error) {
	article, err := h.repo.FindBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if article == nil {
		return nil, false, nil
	}
	return article, true, nil
}

// GetArticle writes 200 {"article": ArticleData} or 404.
func (h *ArticleHandler) GetArticle(w http.ResponseWriter, r *http.Request) error {
	slug := r.PathValue("slug")
	data, found, err := h.queries.FindBySlug(r.Context(), slug, currentUser(r))
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeSpringError(w, r, http.StatusNotFound, ErrResourceNotFound.Error())
			return nil
		}
		return fmt.Errorf("get article %q: %w", slug, err)
	}
	if !found {
		writeSpringError(w, r, http.StatusNotFound, ErrResourceNotFound.Error())
		return nil
	}
	writeErrorJSON(w, http.StatusOK, articleResponse{Article: data})
	return nil
}

// UpdateArticle decodes {"article":{...}}, updates the article if the current
// user is its author and writes 200 {"article": ArticleData} re-read by the
// (possibly new) slug. 400 malformed body, 404 unknown slug (checked before
// authorization), 403 non-author, 422 validation failures.
func (h *ArticleHandler) UpdateArticle(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	user := currentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}
	slug := r.PathValue("slug")

	var param app.UpdateArticleParam
	if err := json.NewDecoder(r.Body).Decode(&param); err != nil {
		msg := "malformed request body"
		if errors.Is(err, app.ErrMissingArticleRoot) {
			msg = app.ErrMissingArticleRoot.Error()
		}
		writeSpringError(w, r, http.StatusBadRequest, msg)
		return nil
	}

	article, found, err := h.loadArticle(ctx, slug)
	if err != nil {
		return fmt.Errorf("update article %q: %w", slug, err)
	}
	if !found {
		writeSpringError(w, r, http.StatusNotFound, ErrResourceNotFound.Error())
		return nil
	}
	if !h.canWrite(user, article) {
		writeSpringError(w, r, http.StatusForbidden, ErrNoAuthorization.Error())
		return nil
	}

	updated, err := h.commands.UpdateArticle(ctx, article, param)
	if err != nil {
		if conv := toHTTPValidationError(err); conv != err {
			return conv
		}
		return fmt.Errorf("update article %q: %w", slug, err)
	}

	data, found, err := h.queries.FindBySlug(ctx, updated.Slug, user)
	if err != nil {
		return fmt.Errorf("update article %q: load read model: %w", slug, err)
	}
	if !found {
		// Source: Optional.get() → NoSuchElementException → 500.
		return fmt.Errorf("update article: article %q disappeared after update", updated.Slug)
	}
	writeErrorJSON(w, http.StatusOK, articleResponse{Article: data})
	return nil
}

// DeleteArticle removes the article if the current user is its author and
// writes 204 with no body. 404 unknown slug (checked first), 403 non-author.
func (h *ArticleHandler) DeleteArticle(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	user := currentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}
	slug := r.PathValue("slug")

	article, found, err := h.loadArticle(ctx, slug)
	if err != nil {
		return fmt.Errorf("delete article %q: %w", slug, err)
	}
	if !found {
		writeSpringError(w, r, http.StatusNotFound, ErrResourceNotFound.Error())
		return nil
	}
	if !h.canWrite(user, article) {
		writeSpringError(w, r, http.StatusForbidden, ErrNoAuthorization.Error())
		return nil
	}
	if err := h.repo.Remove(ctx, article); err != nil {
		return fmt.Errorf("delete article %q: %w", slug, err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
