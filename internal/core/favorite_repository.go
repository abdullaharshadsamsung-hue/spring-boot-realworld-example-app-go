package core

import "context"

// ArticleFavoriteRepository is the domain port for persisting the
// "user favorited article" association. The concrete PostgreSQL
// implementation lives in the store package (the replacement for the
// source's MyBatisArticleFavoriteRepository).
//
// Implementations must be safe for concurrent use.
type ArticleFavoriteRepository interface {
	// Save persists favorite. It is idempotent per (ArticleID, UserID):
	// saving a pair that already exists must succeed without creating a
	// duplicate row (e.g. INSERT ... ON CONFLICT DO NOTHING, or a
	// find-then-insert as in the source).
	Save(ctx context.Context, favorite ArticleFavorite) error

	// Find looks up the favorite for the given article and user. The bool
	// result is false (with a nil error) when no such favorite exists;
	// a non-nil error is returned only for storage failures.
	Find(ctx context.Context, articleID, userID string) (ArticleFavorite, bool, error)

	// Remove deletes the favorite identified by favorite's ArticleID and
	// UserID. Removing a favorite that does not exist is not an error.
	Remove(ctx context.Context, favorite ArticleFavorite) error
}
