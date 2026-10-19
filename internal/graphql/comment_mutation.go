package graphql

import (
	"context"
	"errors"
	"fmt"

	"migrated-app/internal/app"
	"migrated-app/internal/core"
	"migrated-app/internal/httpapi"
)

// Schema coordinates resolved by CommentMutation. They replace the DGS
// codegen constants MUTATION.AddComment and MUTATION.DeleteComment.
const (
	MutationFieldAddComment    = "addComment"
	MutationFieldDeleteComment = "deleteComment"
)

// CommentMutationArticles is the article lookup used by CommentMutation.
// core.ArticleRepository satisfies it.
type CommentMutationArticles interface {
	// FindBySlug returns the article or an error matching core.ErrNotFound.
	FindBySlug(ctx context.Context, slug string) (*core.Article, error)
}

// CommentMutationStore is the write-side comment persistence used by
// CommentMutation. core.CommentRepository satisfies it.
type CommentMutationStore interface {
	// Save persists a new comment.
	Save(ctx context.Context, c *core.Comment) error
	// FindByID returns the comment with id belonging to articleID, or an
	// error matching core.ErrNotFound.
	FindByID(ctx context.Context, articleID, id string) (*core.Comment, error)
	// Remove deletes the comment.
	Remove(ctx context.Context, c *core.Comment) error
}

// CommentMutationQuery is the read-side dependency of CommentMutation.
// *app.CommentQueryService satisfies it.
type CommentMutationQuery interface {
	// FindByID returns the comment read model as seen by user; the bool is
	// false when it does not exist.
	FindByID(ctx context.Context, id string, user *core.User) (app.CommentData, bool, error)
}

// Errors returned by NewCommentMutation for missing dependencies.
var (
	ErrNilCommentMutationArticles = errors.New("graphql: comment mutation article repository is nil")
	ErrNilCommentMutationStore    = errors.New("graphql: comment mutation comment repository is nil")
	ErrNilCommentMutationQuery    = errors.New("graphql: comment mutation query service is nil")
)

// DeletionStatus is the GraphQL DeletionStatus type.
type DeletionStatus struct {
	Success bool `json:"success"`
}

// AddCommentResult replaces DataFetcherResult<CommentPayload>. The payload
// itself carries no fields; its "comment" field is resolved by
// CommentDatafetcher.CommentPayloadComment from LocalContext.
type AddCommentResult struct {
	LocalContext *app.CommentData
}

// CommentMutation resolves Mutation.addComment and Mutation.deleteComment.
//
// MIGRATION_NOTE: the source is a DGS @DgsComponent. The composition root must
// build it with NewCommentMutation and register AddComment/DeleteComment with
// the GraphQL executor at "POST /graphql" (behind the auth middleware), passing
// AddCommentResult.LocalContext to CommentDatafetcher.CommentPayloadComment.
type CommentMutation struct {
	articles CommentMutationArticles
	comments CommentMutationStore
	queries  CommentMutationQuery
}

// NewCommentMutation creates a CommentMutation.
func NewCommentMutation(articles CommentMutationArticles, comments CommentMutationStore, queries CommentMutationQuery) (*CommentMutation, error) {
	if articles == nil {
		return nil, ErrNilCommentMutationArticles
	}
	if comments == nil {
		return nil, ErrNilCommentMutationStore
	}
	if queries == nil {
		return nil, ErrNilCommentMutationQuery
	}
	return &CommentMutation{articles: articles, comments: comments, queries: queries}, nil
}

// AddComment resolves Mutation.addComment(slug, body). It requires an
// authenticated user (core.ErrUnauthenticated), returns
// httpapi.ErrResourceNotFound for an unknown slug, saves the comment and
// returns its read model as the local context. As in the source, body is
// not validated.
func (m *CommentMutation) AddComment(ctx context.Context, slug, body string) (*AddCommentResult, error) {
	user, err := requireCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	article, err := m.articles.FindBySlug(ctx, slug)
	if err != nil {
		return nil, notFoundOrWrap(err, "addComment: find article")
	}
	if article == nil {
		return nil, httpapi.ErrResourceNotFound
	}
	comment := core.NewComment(body, user.ID, article.ID)
	if err := m.comments.Save(ctx, comment); err != nil {
		return nil, fmt.Errorf("addComment: save: %w", err)
	}
	data, found, err := m.queries.FindByID(ctx, comment.ID(), user)
	if err != nil {
		return nil, fmt.Errorf("addComment: load read model: %w", err)
	}
	if !found {
		return nil, httpapi.ErrResourceNotFound
	}
	return &AddCommentResult{LocalContext: &data}, nil
}

// DeleteComment resolves Mutation.deleteComment(slug, id). It requires an
// authenticated user, returns httpapi.ErrResourceNotFound when the article or
// comment is missing and httpapi.ErrNoAuthorization unless the user authored
// the article or the comment; otherwise it removes the comment.
func (m *CommentMutation) DeleteComment(ctx context.Context, slug, commentID string) (*DeletionStatus, error) {
	user, err := requireCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	article, err := m.articles.FindBySlug(ctx, slug)
	if err != nil {
		return nil, notFoundOrWrap(err, "deleteComment: find article")
	}
	if article == nil {
		return nil, httpapi.ErrResourceNotFound
	}
	comment, err := m.comments.FindByID(ctx, article.ID, commentID)
	if err != nil {
		return nil, notFoundOrWrap(err, "deleteComment: find comment")
	}
	if comment == nil {
		return nil, httpapi.ErrResourceNotFound
	}
	if !core.CanWriteComment(user, article, comment) {
		return nil, httpapi.ErrNoAuthorization
	}
	if err := m.comments.Remove(ctx, comment); err != nil {
		return nil, fmt.Errorf("deleteComment: remove: %w", err)
	}
	return &DeletionStatus{Success: true}, nil
}
