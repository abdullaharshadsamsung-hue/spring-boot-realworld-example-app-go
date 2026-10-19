package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"migrated-app/internal/core"
)

// favoriteQuerier is the subset of *sql.DB / *sql.Tx / *sql.Conn used by
// FavoriteRepo, so the repository can run inside a caller-owned transaction.
type favoriteQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// FavoriteRepo is the PostgreSQL implementation of
// core.ArticleFavoriteRepository. It replaces the source's
// MyBatisArticleFavoriteRepository + ArticleFavoriteMapper pair.
type FavoriteRepo struct {
	db favoriteQuerier
}

// NewFavoriteRepo returns a FavoriteRepo backed by db (a *sql.DB, *sql.Tx or
// *sql.Conn).
func NewFavoriteRepo(db favoriteQuerier) *FavoriteRepo {
	return &FavoriteRepo{db: db}
}

// Compile-time check that FavoriteRepo satisfies the domain port.
var _ core.ArticleFavoriteRepository = (*FavoriteRepo)(nil)

const (
	// MIGRATION_NOTE: the source did find-then-insert, which races under
	// concurrent requests. ON CONFLICT DO NOTHING against the
	// (article_id, user_id) primary key gives the same observable behaviour
	// (no insert when the pair already exists) atomically.
	insertFavoriteSQL = `INSERT INTO article_favorites (article_id, user_id)
VALUES ($1, $2)
ON CONFLICT (article_id, user_id) DO NOTHING`

	findFavoriteSQL = `SELECT article_id, user_id
FROM article_favorites
WHERE article_id = $1 AND user_id = $2`

	deleteFavoriteSQL = `DELETE FROM article_favorites
WHERE article_id = $1 AND user_id = $2`
)

// Save persists favorite, inserting a row only if no row for the
// (ArticleID, UserID) pair already exists. Saving an existing pair is a no-op.
func (r *FavoriteRepo) Save(ctx context.Context, favorite core.ArticleFavorite) error {
	if _, err := r.db.ExecContext(ctx, insertFavoriteSQL, favorite.ArticleID, favorite.UserID); err != nil {
		return fmt.Errorf("store: save favorite (article %s, user %s): %w",
			favorite.ArticleID, favorite.UserID, err)
	}
	return nil
}

// Find looks up the favorite for articleID and userID. found is false (with a
// nil error) when no such favorite exists; an error is returned only for
// storage failures.
func (r *FavoriteRepo) Find(ctx context.Context, articleID, userID string) (core.ArticleFavorite, bool, error) {
	var fav core.ArticleFavorite
	err := r.db.QueryRowContext(ctx, findFavoriteSQL, articleID, userID).
		Scan(&fav.ArticleID, &fav.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ArticleFavorite{}, false, nil
	}
	if err != nil {
		return core.ArticleFavorite{}, false, fmt.Errorf("store: find favorite (article %s, user %s): %w",
			articleID, userID, err)
	}
	return fav, true, nil
}

// Remove deletes the favorite identified by favorite's ArticleID and UserID.
// Removing a favorite that does not exist is not an error, matching the source.
func (r *FavoriteRepo) Remove(ctx context.Context, favorite core.ArticleFavorite) error {
	if _, err := r.db.ExecContext(ctx, deleteFavoriteSQL, favorite.ArticleID, favorite.UserID); err != nil {
		return fmt.Errorf("store: remove favorite (article %s, user %s): %w",
			favorite.ArticleID, favorite.UserID, err)
	}
	return nil
}
