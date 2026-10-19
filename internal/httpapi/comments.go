package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"migrated-app/internal/app"
	"migrated-app/internal/core"
)

// CommentArticleFinder is the write-side article lookup used by
// CommentsHandler. core.ArticleRepository satisfies it.
type CommentArticleFinder interface {
	// FindBySlug returns the article or an error matching core.ErrNotFound.
	FindBySlug(ctx context.Context, slug string) (*core.Article, error)
}

// CommentStore is the write-side comment persistence used by CommentsHandler.
type CommentStore interface {
	// Save persists a new comment.
	Save(ctx context.Context, c *core.Comment) error
	// FindByID returns the comment with id that belongs to articleID, or an
	// error matching core.ErrNotFound.
	FindByID(ctx context.Context, articleID, id string) (*core.Comment, error)
	// Remove deletes the comment.
	Remove(ctx context.Context, c *core.Comment) error
}

// CommentQuerier is the read-side dependency of CommentsHandler.
type CommentQuerier interface {
	// FindByID returns the comment read model as seen by user (nil for
	// anonymous). The bool is false when it does not exist.
	FindByID(ctx context.Context, id string, user *core.User) (app.CommentData, bool, error)
	// FindByArticleID returns every comment of the article as seen by user.
	FindByArticleID(ctx context.Context, articleID string, user *core.User) ([]app.CommentData, error)
}

// ErrNilCommentArticleFinder is returned by NewCommentsHandler when no article repository is supplied.
var ErrNilCommentArticleFinder = errors.New("httpapi: comment article repository is nil")

// ErrNilCommentStore is returned by NewCommentsHandler when no comment repository is supplied.
var ErrNilCommentStore = errors.New("httpapi: comment repository is nil")

// ErrNilCommentQuerier is returned by NewCommentsHandler when no comment query service is supplied.
var ErrNilCommentQuerier = errors.New("httpapi: comment query service is nil")

// CommentsHandler serves /articles/{slug}/comments. It replaces the source's
// CommentsApi @RestController.
type CommentsHandler struct {
	articles CommentArticleFinder
	comments CommentStore
	queries  CommentQuerier
}

// NewCommentsHandler creates a CommentsHandler.
func NewCommentsHandler(articles CommentArticleFinder, comments CommentStore, queries CommentQuerier) (*CommentsHandler, error) {
	if articles == nil {
		return nil, ErrNilCommentArticleFinder
	}
	if comments == nil {
		return nil, ErrNilCommentStore
	}
	if queries == nil {
		return nil, ErrNilCommentQuerier
	}
	return &CommentsHandler{articles: articles, comments: comments, queries: queries}, nil
}

// Register mounts the handler's routes on mux:
//
//	POST   /articles/{slug}/comments      → CreateComment (auth required)
//	GET    /articles/{slug}/comments      → GetComments   (auth optional)
//	DELETE /articles/{slug}/comments/{id} → DeleteComment (auth required)
func (h *CommentsHandler) Register(mux *http.ServeMux) {
	mux.Handle("POST /articles/{slug}/comments", requireUser(h.CreateComment))
	mux.Handle("GET /articles/{slug}/comments", ErrorHandlerFunc(h.GetComments))
	mux.Handle("DELETE /articles/{slug}/comments/{id}", requireUser(h.DeleteComment))
}

// NewCommentParam is the request body of CreateComment, sent wrapped as
// {"comment": {"body": "..."}} (source: @JsonRootName("comment")).
type NewCommentParam struct {
	Body *string `json:"body"`
}

// Validate enforces @NotBlank(message = "can't be empty") on body.
func (p NewCommentParam) Validate() error {
	if p.Body == nil || strings.TrimSpace(*p.Body) == "" {
		errs := FieldErrors{}
		errs.Add("body", "can't be empty")
		return NewInvalidRequestError(errs)
	}
	return nil
}

type newCommentRequest struct {
	Comment NewCommentParam `json:"comment"`
}

type commentResponse struct {
	Comment app.CommentData `json:"comment"`
}

type commentsResponse struct {
	Comments []app.CommentData `json:"comments"`
}

// CreateComment creates a comment on {slug} authored by the current user and
// writes 201 {"comment":...}; 404 if the article does not exist, 422 if the
// body is blank.
func (h *CommentsHandler) CreateComment(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	user := currentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}

	var req newCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// MIGRATION_NOTE: Spring answers unreadable JSON with 400.
		writeErrorJSON(w, http.StatusBadRequest, ErrorBody{
			Timestamp: now().UTC().Format(timestampLayout),
			Status:    http.StatusBadRequest,
			Error:     http.StatusText(http.StatusBadRequest),
			Message:   "malformed request body",
			Path:      r.URL.Path,
		})
		return nil
	}
	// Validation (@Valid) runs before the handler body in Spring, so a blank
	// body yields 422 even when the article does not exist.
	if err := req.Comment.Validate(); err != nil {
		return err
	}

	article, err := h.articles.FindBySlug(ctx, r.PathValue("slug"))
	if err != nil {
		return notFoundOr(err, "create comment: find article")
	}

	comment := core.NewComment(*req.Comment.Body, user.ID, article.ID)
	if err := h.comments.Save(ctx, comment); err != nil {
		return fmt.Errorf("create comment: save: %w", err)
	}

	data, found, err := h.queries.FindByID(ctx, comment.ID(), user)
	if err != nil {
		return fmt.Errorf("create comment: load read model: %w", err)
	}
	if !found {
		// Source: Optional.get() → NoSuchElementException → 500.
		return fmt.Errorf("create comment: comment %q disappeared after save", comment.ID())
	}
	writeErrorJSON(w, http.StatusCreated, commentResponse{Comment: data})
	return nil
}

// GetComments writes 200 {"comments":[...]} for {slug}; 404 if the article
// does not exist. The current user may be nil (anonymous).
func (h *CommentsHandler) GetComments(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	user := currentUser(r)
	article, err := h.articles.FindBySlug(ctx, r.PathValue("slug"))
	if err != nil {
		return notFoundOr(err, "get comments: find article")
	}
	comments, err := h.queries.FindByArticleID(ctx, article.ID, user)
	if err != nil {
		return fmt.Errorf("get comments: %w", err)
	}
	if comments == nil {
		comments = []app.CommentData{}
	}
	writeErrorJSON(w, http.StatusOK, commentsResponse{Comments: comments})
	return nil
}

// DeleteComment deletes comment {id} of article {slug}. 404 if the article or
// the comment (scoped to that article) is missing, 403 unless the current
// user authored the article or the comment, otherwise 204.
func (h *CommentsHandler) DeleteComment(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	user := currentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}
	article, err := h.articles.FindBySlug(ctx, r.PathValue("slug"))
	if err != nil {
		return notFoundOr(err, "delete comment: find article")
	}
	comment, err := h.comments.FindByID(ctx, article.ID, r.PathValue("id"))
	if err != nil {
		return notFoundOr(err, "delete comment: find comment")
	}
	if comment == nil {
		return ErrResourceNotFound
	}
	if !canWriteComment(user, article, comment) {
		WriteAuthzError(w, r, ErrNoAuthorization)
		return nil
	}
	if err := h.comments.Remove(ctx, comment); err != nil {
		return fmt.Errorf("delete comment: remove: %w", err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// canWriteComment mirrors AuthorizationService.canWriteComment: the user may
// write when they authored the article or the comment.
//
// MIGRATION_NOTE: internal/core/authz.go holds the domain port; prefer calling
// it directly once its exact signature is confirmed (manual review).
func canWriteComment(user *core.User, article *core.Article, comment *core.Comment) bool {
	if user == nil || article == nil || comment == nil {
		return false
	}
	return user.ID == article.UserID || user.ID == comment.UserID()
}
