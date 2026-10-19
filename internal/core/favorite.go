// Package core contains the domain models of the RealWorld application.
package core

import "hash/fnv"

// ArticleFavorite records that a user has favorited an article.
//
// It is a plain value object: the zero value is an empty favorite with no
// article or user set (the equivalent of the source's no-arg constructor),
// and because every field is comparable, ArticleFavorite values can be
// compared with == and used directly as map keys, giving value-based
// equality and hashing over (ArticleID, UserID).
type ArticleFavorite struct {
	ArticleID string `json:"articleId"`
	UserID    string `json:"userId"`
}

// NewArticleFavorite creates a favorite linking the article to the user.
func NewArticleFavorite(articleID, userID string) ArticleFavorite {
	return ArticleFavorite{ArticleID: articleID, UserID: userID}
}

// GetArticleID returns the identifier of the favorited article.
func (f ArticleFavorite) GetArticleID() string { return f.ArticleID }

// GetUserID returns the identifier of the user who favorited the article.
func (f ArticleFavorite) GetUserID() string { return f.UserID }

// Equal reports whether f and other refer to the same article/user pair.
func (f ArticleFavorite) Equal(other ArticleFavorite) bool {
	return f == other
}

// Hash returns a hash code consistent with Equal: equal favorites always
// produce the same hash.
func (f ArticleFavorite) Hash() uint64 {
	h := fnv.New64a()
	// A zero-byte separator stops ("ab","c") and ("a","bc") from colliding
	// trivially. Writes to an fnv hash never return an error.
	_, _ = h.Write([]byte(f.ArticleID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(f.UserID))
	return h.Sum64()
}
