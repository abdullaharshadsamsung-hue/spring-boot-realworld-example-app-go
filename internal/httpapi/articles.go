package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"migrated-app/internal/app"
	"migrated-app/internal/core"
)

// ArticlesCommander is the write-side dependency of ArticlesHandler.
// *app.ArticleCommandService satisfies it.
type ArticlesCommander interface {
	// CreateArticle validates param and persists a new article owned by creator.
	CreateArticle(ctx context.Context, param app.NewArticleParam, creator *core.User) (*core.Article, error)
}

// ArticlesQuerier is the read-side dependency of ArticlesHandler.
// *app.ArticleQueryService satisfies it.
type ArticlesQuerier interface {
	// FindByID loads the article read model as seen by user (nil = anonymous).
	FindByID(ctx context.Context, id string, user *core.User) (app.ArticleData, bool, error)
	// FindRecentArticles lists articles matching the optional filters.
	FindRecentArticles(ctx context.Context, tag, author, favoritedBy string, page app.Page, currentUser *core.User) (app.ArticleDataList, error)
	// FindUserFeed lists articles written by authors user follows.
	FindUserFeed(ctx context.Context, user *core.User, page app.Page) (app.ArticleDataList, error)
}

// ErrNilArticlesCommander is returned by NewArticlesHandler when no command service is supplied.
var ErrNilArticlesCommander = errors.New("httpapi: article command service is nil")

// ErrNilArticlesQuerier is returned by NewArticlesHandler when no query service is supplied.
var ErrNilArticlesQuerier = errors.New("httpapi: article query service is nil")

// ArticlesHandler serves the /articles collection resource. It replaces the
// source's ArticlesApi @RestController.
type ArticlesHandler struct {
	commands ArticlesCommander
	queries  ArticlesQuerier
	slugs    app.ArticleSlugChecker
}

// NewArticlesHandler creates an ArticlesHandler. slugs is optional: when
// non-nil, the handler performs the @Valid NewArticleParam checks itself
// (blank fields + duplicated slug) before calling the command service, which
// mirrors Spring validating the request body in the controller.
func NewArticlesHandler(commands ArticlesCommander, queries ArticlesQuerier, slugs app.ArticleSlugChecker) (*ArticlesHandler, error) {
	if commands == nil {
		return nil, ErrNilArticlesCommander
	}
	if queries == nil {
		return nil, ErrNilArticlesQuerier
	}
	return &ArticlesHandler{commands: commands, queries: queries, slugs: slugs}, nil
}

// Register mounts the handler's routes on mux:
//
//	POST /articles      → CreateArticle (auth required)
//	GET  /articles/feed → GetFeed       (auth required)
//	GET  /articles      → GetArticles   (auth optional)
func (h *ArticlesHandler) Register(mux *http.ServeMux) {
	mux.Handle("POST /articles", requireUser(h.CreateArticle))
	mux.Handle("GET /articles/feed", requireUser(h.GetFeed))
	mux.Handle("GET /articles", ErrorHandlerFunc(h.GetArticles))
}

type articlesCreateResponse struct {
	Article app.ArticleData `json:"article"`
}

// fieldMapErrorer matches app-layer validation errors whose FieldErrors
// method returns a plain map (e.g. *app.NewArticleValidationError).
type fieldMapErrorer interface {
	error
	FieldErrors() map[string][]string
}

// toHTTPValidationError converts app-layer validation errors into an
// *InvalidRequestError so WriteError renders 422 {"errors":{...}}.
func toHTTPValidationError(err error) error {
	var fe FieldErrorer
	if errors.As(err, &fe) {
		return err
	}
	var fm fieldMapErrorer
	if errors.As(err, &fm) {
		errs := FieldErrors{}
		for field, msgs := range fm.FieldErrors() {
			for _, m := range msgs {
				errs.Add(field, m)
			}
		}
		return NewInvalidRequestError(errs)
	}
	return err
}

// writeArticlesBadRequest writes Spring's default 400 error JSON.
func writeArticlesBadRequest(w http.ResponseWriter, r *http.Request, msg string) {
	writeErrorJSON(w, http.StatusBadRequest, ErrorBody{
		Timestamp: now().UTC().Format(timestampLayout),
		Status:    http.StatusBadRequest,
		Error:     http.StatusText(http.StatusBadRequest),
		Message:   msg,
		Path:      r.URL.Path,
	})
}

// parsePageParams reads offset (default 0) and limit (default 20). The bool is
// false when a value is not an int (Spring's MethodArgumentTypeMismatch → 400).
func parsePageParams(r *http.Request) (int, int, string, bool) {
	q := r.URL.Query()
	offset, limit := 0, 20
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, 0, "offset must be an integer", false
		}
		offset = n
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, 0, "limit must be an integer", false
		}
		limit = n
	}
	return offset, limit, "", true
}

// CreateArticle decodes {"article":{...}}, creates the article for the
// current user and writes 200 {"article": ArticleData}. 400 for malformed
// JSON or a missing root, 422 for validation failures.
func (h *ArticlesHandler) CreateArticle(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	user := currentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}

	var param app.NewArticleParam
	if err := json.NewDecoder(r.Body).Decode(&param); err != nil {
		msg := "malformed request body"
		if errors.Is(err, app.ErrMissingArticleRoot) {
			msg = app.ErrMissingArticleRoot.Error()
		}
		writeArticlesBadRequest(w, r, msg)
		return nil
	}

	if h.slugs != nil {
		if err := app.ValidateNewArticleParam(ctx, param, h.slugs); err != nil {
			return toHTTPValidationError(err)
		}
	}

	article, err := h.commands.CreateArticle(ctx, param, user)
	if err != nil {
		if conv := toHTTPValidationError(err); conv != err {
			return conv
		}
		return fmt.Errorf("create article: %w", err)
	}

	data, found, err := h.queries.FindByID(ctx, article.ID, user)
	if err != nil {
		return fmt.Errorf("create article: load read model: %w", err)
	}
	if !found {
		// Source: Optional.get() → NoSuchElementException → 500.
		return fmt.Errorf("create article: article %q disappeared after save", article.ID)
	}
	// Source returns ResponseEntity.ok → 200 (not 201).
	writeErrorJSON(w, http.StatusOK, articlesCreateResponse{Article: data})
	return nil
}

// GetFeed writes 200 {"articles":[...],"articlesCount":n} for articles by
// authors the current user follows.
func (h *ArticlesHandler) GetFeed(w http.ResponseWriter, r *http.Request) error {
	user := currentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}
	offset, limit, msg, ok := parsePageParams(r)
	if !ok {
		writeArticlesBadRequest(w, r, msg)
		return nil
	}
	list, err := h.queries.FindUserFeed(r.Context(), user, app.NewPage(offset, limit))
	if err != nil {
		if errors.Is(err, app.ErrFeedRequiresUser) {
			w.WriteHeader(http.StatusUnauthorized)
			return nil
		}
		return fmt.Errorf("get feed: %w", err)
	}
	writeErrorJSON(w, http.StatusOK, list)
	return nil
}

// GetArticles writes 200 {"articles":[...],"articlesCount":n} for recent
// articles filtered by the optional tag, author and favorited parameters.
// Anonymous access is allowed.
func (h *ArticlesHandler) GetArticles(w http.ResponseWriter, r *http.Request) error {
	user := currentUser(r)
	offset, limit, msg, ok := parsePageParams(r)
	if !ok {
		writeArticlesBadRequest(w, r, msg)
		return nil
	}
	q := r.URL.Query()
	// MIGRATION_NOTE: an absent parameter (Java null) is passed as "", which
	// the query layer treats as "no filter".
	list, err := h.queries.FindRecentArticles(
		r.Context(), q.Get("tag"), q.Get("author"), q.Get("favorited"), app.NewPage(offset, limit), user,
	)
	if err != nil {
		return fmt.Errorf("get articles: %w", err)
	}
	writeErrorJSON(w, http.StatusOK, list)
	return nil
}
