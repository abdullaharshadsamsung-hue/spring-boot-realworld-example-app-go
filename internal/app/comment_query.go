package app

import (
	"context"
	"fmt"
	"slices"

	"migrated-app/internal/core"
)

// CommentReader is the comment read-model data source (the source's MyBatis
// CommentReadService).
type CommentReader interface {
	// FindByID returns the comment; ok is false when it does not exist.
	FindByID(ctx context.Context, id string) (CommentData, bool, error)
	// FindByArticleID returns every comment of the article.
	FindByArticleID(ctx context.Context, articleID string) ([]CommentData, error)
	// FindByArticleIDWithCursor returns up to page.QueryLimit() (limit+1)
	// comments of the article, ordered for the page direction.
	FindByArticleIDWithCursor(ctx context.Context, articleID string, page CursorPageParameter) ([]CommentData, error)
}

// CommentNode adapts CommentData to the Node interface so it can be used in
// a CursorPager. It embeds CommentData, so fields and JSON are unchanged.
//
// MIGRATION_NOTE: CommentData.Cursor returns a DateTimeCursor while Node
// requires Cursor() string; this wrapper bridges the two (same as ArticleNode).
type CommentNode struct {
	CommentData
}

// Cursor returns the serialized cursor of the comment.
func (n CommentNode) Cursor() string {
	return n.CommentData.Cursor().String()
}

// MarshalJSON delegates to CommentData so the wire shape is unchanged.
func (n CommentNode) MarshalJSON() ([]byte, error) {
	return n.CommentData.MarshalJSON()
}

// CommentQueryService is the read side of the comment feature: it loads
// comment read models and enriches author profiles with the viewer's
// following flag.
type CommentQueryService struct {
	comments  CommentReader
	relations UserRelationshipReader
}

// NewCommentQueryService builds a CommentQueryService.
func NewCommentQueryService(comments CommentReader, relations UserRelationshipReader) *CommentQueryService {
	return &CommentQueryService{comments: comments, relations: relations}
}

// FindByID loads the comment with id and sets the author's following flag
// according to whether user follows the author. ok is false when the comment
// does not exist.
//
// MIGRATION_NOTE: the source dereferences user unconditionally (NPE when
// null). Here a nil user, or a nil author profile, simply skips the
// following lookup (agreed fix Q6/Q7).
func (s *CommentQueryService) FindByID(ctx context.Context, id string, user *core.User) (CommentData, bool, error) {
	data, ok, err := s.comments.FindByID(ctx, id)
	if err != nil {
		return CommentData{}, false, fmt.Errorf("find comment by id %q: %w", id, err)
	}
	if !ok {
		return CommentData{}, false, nil
	}
	if user != nil && data.Author != nil {
		following, err := s.relations.IsUserFollowing(ctx, user.ID, data.Author.ID)
		if err != nil {
			return CommentData{}, false, fmt.Errorf("check following: %w", err)
		}
		data.Author.Following = following
	}
	return data, true, nil
}

// FindByArticleID returns all comments of the article. When user is non-nil
// and there are comments, authors followed by user are marked as followed.
func (s *CommentQueryService) FindByArticleID(ctx context.Context, articleID string, user *core.User) ([]CommentData, error) {
	comments, err := s.comments.FindByArticleID(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("find comments of article %q: %w", articleID, err)
	}
	if comments == nil {
		comments = []CommentData{}
	}
	if len(comments) > 0 && user != nil {
		if err := s.fillFollowing(ctx, comments, user); err != nil {
			return nil, err
		}
	}
	return comments, nil
}

// FindByArticleIDWithCursor returns a cursor page of the article's comments.
// The reader is expected to return up to limit+1 rows; the extra row signals
// that another page exists and is trimmed. PREV pages are reversed so the
// result is always in natural order. user may be nil.
func (s *CommentQueryService) FindByArticleIDWithCursor(
	ctx context.Context, articleID string, user *core.User, page CursorPageParameter,
) (*CursorPager[CommentNode], error) {
	comments, err := s.comments.FindByArticleIDWithCursor(ctx, articleID, page)
	if err != nil {
		return nil, fmt.Errorf("find comments of article %q with cursor: %w", articleID, err)
	}
	if len(comments) == 0 {
		return NewCursorPager([]CommentNode{}, page.Direction(), false), nil
	}
	if user != nil {
		if err := s.fillFollowing(ctx, comments, user); err != nil {
			return nil, err
		}
	}
	hasExtra := len(comments) > page.Limit()
	if hasExtra {
		// Source removes only the element at index limit; the reader never
		// returns more than limit+1 rows, so truncation is equivalent.
		comments = comments[:page.Limit()]
	}
	if !page.IsNext() {
		slices.Reverse(comments)
	}
	nodes := make([]CommentNode, 0, len(comments))
	for _, c := range comments {
		nodes = append(nodes, CommentNode{CommentData: c})
	}
	return NewCursorPager(nodes, page.Direction(), hasExtra), nil
}

// fillFollowing batch-loads which comment authors user follows (one query,
// avoiding N+1) and sets Following=true on those authors' profiles.
func (s *CommentQueryService) fillFollowing(ctx context.Context, comments []CommentData, user *core.User) error {
	authorIDs := make([]string, 0, len(comments))
	for _, c := range comments {
		if c.Author != nil {
			authorIDs = append(authorIDs, c.Author.ID)
		}
	}
	following, err := s.relations.FollowingAuthors(ctx, user.ID, authorIDs)
	if err != nil {
		return fmt.Errorf("load following authors: %w", err)
	}
	for i := range comments {
		if comments[i].Author == nil {
			continue
		}
		if _, ok := following[comments[i].Author.ID]; ok {
			comments[i].Author.Following = true
		}
	}
	return nil
}
