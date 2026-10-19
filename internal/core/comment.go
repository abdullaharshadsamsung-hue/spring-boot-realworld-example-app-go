// Package core contains the domain entities of the RealWorld (Conduit) application.
package core

import (
	"crypto/rand"
	"fmt"
	"time"
)

// Comment is a domain entity representing a comment on an article.
//
// Its fields are unexported and exposed through read-only accessors so that,
// like the source entity, a Comment cannot be mutated after construction.
// Identity is self-assigned in the domain (a random UUID) rather than by the
// database.
type Comment struct {
	id        string
	body      string
	userID    string
	articleID string
	createdAt time.Time
}

// NewComment constructs a new Comment for the given article authored by the
// given user. It assigns a random UUID (v4) as the id and sets the creation
// time to now. An error is returned only if the system's secure random source
// fails.
func NewComment(body, userID, articleID string) (*Comment, error) {
	id, err := generateCommentID()
	if err != nil {
		return nil, fmt.Errorf("new comment: %w", err)
	}
	return &Comment{
		id:        id,
		body:      body,
		userID:    userID,
		articleID: articleID,
		createdAt: time.Now(),
	}, nil
}

// RestoreComment rebuilds a Comment from persisted state. It is intended for
// the persistence layer only (the equivalent of the source's no-args
// constructor plus reflective field hydration) and performs no id or
// timestamp generation.
func RestoreComment(id, body, userID, articleID string, createdAt time.Time) *Comment {
	return &Comment{
		id:        id,
		body:      body,
		userID:    userID,
		articleID: articleID,
		createdAt: createdAt,
	}
}

// ID returns the comment's unique identifier.
func (c *Comment) ID() string { return c.id }

// Body returns the comment's text.
func (c *Comment) Body() string { return c.body }

// UserID returns the id of the user who authored the comment.
func (c *Comment) UserID() string { return c.userID }

// ArticleID returns the id of the article the comment belongs to.
func (c *Comment) ArticleID() string { return c.articleID }

// CreatedAt returns the time at which the comment was created.
func (c *Comment) CreatedAt() time.Time { return c.createdAt }

// Equal reports whether c and other represent the same comment. Equality is
// based solely on id, mirroring the source's @EqualsAndHashCode(of = "id").
// Two nil comments are considered equal.
func (c *Comment) Equal(other *Comment) bool {
	if c == nil || other == nil {
		return c == other
	}
	return c.id == other.id
}

// HashKey returns the value that identifies this comment for use as a map key
// or set member; it is derived solely from the id, consistent with Equal.
func (c *Comment) HashKey() string {
	if c == nil {
		return ""
	}
	return c.id
}

// generateCommentID returns a random RFC 4122 version 4 UUID in canonical
// string form, equivalent to Java's UUID.randomUUID().toString().
func generateCommentID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
