package graphql

import (
	"context"
	"errors"
	"fmt"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
)

// Schema coordinates resolved by CommentDatafetcher. They replace the DGS
// codegen constants COMMENTPAYLOAD.* and ARTICLE.Comments.
const (
	CommentPayloadType         = "CommentPayload"
	CommentPayloadFieldComment = "comment"
	ArticleFieldComments       = "comments"
)

// ErrNilCommentQuery is returned by NewCommentDatafetcher when no
// CommentQuery is given.
var ErrNilCommentQuery = errors.New("graphql: comment query service is nil")

// CommentQuery is the read-side dependency of CommentDatafetcher.
// *app.CommentQueryService satisfies it.
type CommentQuery interface {
	// FindByArticleIDWithCursor returns a cursor page of the article's
	// comments as seen by user (may be nil).
	FindByArticleIDWithCursor(ctx context.Context, articleID string, user *core.User, page app.CursorPageParameter) (*app.CursorPager[app.CommentNode], error)
}

var _ CommentQuery = (*app.CommentQueryService)(nil)

// Comment is the GraphQL Comment type (author and article are resolved by
// child resolvers using the local context).
type Comment struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// CommentEdge is the GraphQL CommentEdge type.
type CommentEdge struct {
	Cursor string   `json:"cursor"`
	Node   *Comment `json:"node"`
}

// CommentsConnection is the GraphQL CommentsConnection type.
type CommentsConnection struct {
	Edges    []CommentEdge `json:"edges"`
	PageInfo PageInfo      `json:"pageInfo"`
}

// CommentResult replaces DataFetcherResult<Comment>: LocalContext maps the
// comment id to its CommentData for child resolvers (Comment.author,
// Comment.article).
type CommentResult struct {
	Data         *Comment
	LocalContext map[string]app.CommentData
}

// CommentsConnectionResult replaces DataFetcherResult<CommentsConnection>.
type CommentsConnectionResult struct {
	Data         *CommentsConnection
	LocalContext map[string]app.CommentData
}

// CommentDatafetcher resolves CommentPayload.comment and Article.comments.
//
// MIGRATION_NOTE: the source is a DGS @DgsComponent. The composition root
// must build it with NewCommentDatafetcher and register CommentPayloadComment
// and ArticleComments with the GraphQL executor serving schema.graphqls at
// "POST /graphql", propagating LocalContext to child resolvers (use
// CommentAuthorLookup for ProfileDatafetcher.CommentAuthor).
type CommentDatafetcher struct {
	comments CommentQuery
}

// NewCommentDatafetcher creates a CommentDatafetcher.
func NewCommentDatafetcher(comments CommentQuery) (*CommentDatafetcher, error) {
	if comments == nil {
		return nil, ErrNilCommentQuery
	}
	return &CommentDatafetcher{comments: comments}, nil
}

// CommentPayloadComment resolves CommentPayload.comment. comment is the
// CommentData placed in the local context by the addComment mutation.
func (d *CommentDatafetcher) CommentPayloadComment(_ context.Context, comment *app.CommentData) (*CommentResult, error) {
	if comment == nil {
		// MIGRATION_NOTE: source would NPE; we return a typed error instead.
		return nil, fmt.Errorf("resolve %s.%s: %w", CommentPayloadType, CommentPayloadFieldComment, ErrMissingLocalContext)
	}
	return &CommentResult{
		Data:         buildComment(*comment),
		LocalContext: map[string]app.CommentData{comment.ID: *comment},
	}, nil
}

// ArticleComments resolves Article.comments(first, after, last, before).
// article is the parent GraphQL Article; local is the parent's local context
// mapping slug to ArticleData.
func (d *CommentDatafetcher) ArticleComments(ctx context.Context, article *Article, local map[string]app.ArticleData, args CursorArgs) (*CommentsConnectionResult, error) {
	// first/last validation and NEXT(after)/PREV(before) selection.
	page, err := pageFor(args)
	if err != nil {
		return nil, err
	}
	if article == nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", ArticleType, ArticleFieldComments, ErrMissingLocalContext)
	}
	articleData, ok := local[article.Slug]
	if !ok {
		return nil, fmt.Errorf("resolve %s.%s for slug %q: %w", ArticleType, ArticleFieldComments, article.Slug, ErrMissingLocalContext)
	}
	current, _ := auth.UserFrom(ctx)
	pager, err := d.comments.FindByArticleIDWithCursor(ctx, articleData.ID, current, page)
	if err != nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", ArticleType, ArticleFieldComments, err)
	}

	nodes := pager.Data()
	edges := make([]CommentEdge, 0, len(nodes))
	ctxMap := make(map[string]app.CommentData, len(nodes))
	for _, n := range nodes {
		edges = append(edges, CommentEdge{Cursor: n.Cursor(), Node: buildComment(n.CommentData)})
		ctxMap[n.CommentData.ID] = n.CommentData
	}
	info := PageInfo{HasPreviousPage: pager.HasPrevious(), HasNextPage: pager.HasNext()}
	if c, ok := pager.StartCursor(); ok {
		info.StartCursor = &c
	}
	if c, ok := pager.EndCursor(); ok {
		info.EndCursor = &c
	}
	return &CommentsConnectionResult{
		Data:         &CommentsConnection{Edges: edges, PageInfo: info},
		LocalContext: ctxMap,
	}, nil
}

// CommentAuthorLookup adapts a comment local context to the AuthorLookup
// used by ProfileDatafetcher.CommentAuthor.
func CommentAuthorLookup(local map[string]app.CommentData) AuthorLookup {
	return func(id string) (string, bool) {
		c, ok := local[id]
		if !ok || c.Author == nil {
			return "", false
		}
		return c.Author.Username, true
	}
}

// buildComment maps CommentData to the GraphQL Comment. As in the source,
// both createdAt and updatedAt are formatted from CreatedAt (ISO-8601 UTC).
func buildComment(c app.CommentData) *Comment {
	ts := isoUTC(c.CreatedAt)
	return &Comment{
		ID:        c.ID,
		Body:      c.Body,
		CreatedAt: ts,
		UpdatedAt: ts,
	}
}
