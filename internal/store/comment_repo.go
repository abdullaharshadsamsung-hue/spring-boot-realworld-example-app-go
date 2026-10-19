package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"migrated-app/internal/core"
)

// commentQuerier is the subset of *sql.DB / *sql.Tx / *sql.Conn used by
// CommentRepo. Accepting it (rather than *sql.DB) lets callers run the
// repository inside a caller-owned transaction, matching the source's
// implicit participation in any surrounding transaction.
type commentQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// CommentRepo is the PostgreSQL implementation of core.CommentRepository.
// It replaces the source's MyBatisCommentRepository + CommentMapper pair.
type CommentRepo struct {
	db commentQuerier
}

// NewCommentRepo returns a CommentRepo backed by db (a *sql.DB, *sql.Tx or
// *sql.Conn).
func NewCommentRepo(db commentQuerier) *CommentRepo {
	return &CommentRepo{db: db}
}

// Compile-time check that CommentRepo satisfies the domain port.
var _ core.CommentRepository = (*CommentRepo)(nil)

const (
	insertCommentSQL = `INSERT INTO comments (id, body, user_id, article_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)`

	findCommentSQL = `SELECT id, body, user_id, article_id, created_at
FROM comments
WHERE id = $1 AND article_id = $2`

	deleteCommentSQL = `DELETE FROM comments WHERE id = $1`
)

// Save inserts comment as a new row. updated_at is set to the comment's
// creation time, as the source mapper did.
func (r *CommentRepo) Save(ctx context.Context, comment *core.Comment) error {
	if comment == nil {
		return errors.New("store: save comment: nil comment")
	}
	_, err := r.db.ExecContext(ctx, insertCommentSQL,
		comment.ID(),
		comment.Body(),
		comment.UserID(),
		comment.ArticleID(),
		NewUTCTime(comment.CreatedAt()),
	)
	if err != nil {
		return fmt.Errorf("store: save comment %s: %w", comment.ID(), err)
	}
	return nil
}

// FindByID returns the comment with the given id that belongs to articleID.
// found is false (with nil comment and nil error) when no such row exists.
func (r *CommentRepo) FindByID(ctx context.Context, articleID, id string) (*core.Comment, bool, error) {
	var (
		cID                 string
		body, userID, artID sql.NullString
		createdAt           UTCTime
	)
	err := r.db.QueryRowContext(ctx, findCommentSQL, id, articleID).
		Scan(&cID, &body, &userID, &artID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("store: find comment %s for article %s: %w", id, articleID, err)
	}
	return core.RestoreComment(cID, body.String, userID.String, artID.String, createdAt.Time), true, nil
}

// Remove deletes the comment identified by comment's id. Deleting a comment
// that no longer exists is not an error, matching the source.
func (r *CommentRepo) Remove(ctx context.Context, comment *core.Comment) error {
	if comment == nil {
		return errors.New("store: remove comment: nil comment")
	}
	if _, err := r.db.ExecContext(ctx, deleteCommentSQL, comment.ID()); err != nil {
		return fmt.Errorf("store: remove comment %s: %w", comment.ID(), err)
	}
	return nil
}
