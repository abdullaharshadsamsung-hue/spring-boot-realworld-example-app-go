package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"sync/atomic"
	"time"
)

// Comment is the domain aggregate for a comment left by a user on an article.
//
// Identity is determined solely by ID, mirroring the source's Lombok
// @EqualsAndHashCode(of = {"id"}). Fields are unexported so a Comment can
// only be built through NewComment (fresh comment) or RestoreComment
// (rehydration from persistence).
type Comment struct {
	id        string
	body      string
	userID    string
	articleID string
	createdAt time.Time
}

// NewComment creates a new comment with a freshly generated random UUID and a
// creation timestamp of "now" (UTC, millisecond precision to match the
// epoch-ms wire format used throughout the API).
func NewComment(body, userID, articleID string) *Comment {
	return &Comment{
		id:        newCommentID(),
		body:      body,
		userID:    userID,
		articleID: articleID,
		createdAt: time.Now().UTC().Truncate(time.Millisecond),
	}
}

// RestoreComment rebuilds a Comment from persisted state. It performs no
// validation and generates nothing; it is intended for repository
// implementations reading rows back from the database.
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

// Body returns the comment text.
func (c *Comment) Body() string { return c.body }

// UserID returns the id of the user who wrote the comment.
func (c *Comment) UserID() string { return c.userID }

// ArticleID returns the id of the article the comment belongs to.
func (c *Comment) ArticleID() string { return c.articleID }

// CreatedAt returns the instant the comment was created.
func (c *Comment) CreatedAt() time.Time { return c.createdAt }

// Equal reports whether c and other denote the same comment. As in the
// source, only the id participates in equality. Two nil comments are equal;
// a nil and a non-nil comment are not.
func (c *Comment) Equal(other *Comment) bool {
	if c == nil || other == nil {
		return c == other
	}
	return c.id == other.id
}

// HashKey returns a value suitable for use as a map key that is consistent
// with Equal (it is the comment's id).
func (c *Comment) HashKey() string {
	if c == nil {
		return ""
	}
	return c.id
}

// CommentRepository is the domain-owned persistence port for the Comment
// aggregate. The PostgreSQL implementation lives in the store package and is
// injected explicitly by the composition root (replacing Spring's autowiring
// of the MyBatisCommentRepository bean).
//
// MIGRATION_NOTE: the Java interface declared no checked exceptions; failures
// propagated as unchecked runtime exceptions. In Go every operation returns
// an explicit error instead.
type CommentRepository interface {
	// Save persists the comment so that it can later be retrieved with
	// FindByID using its article id and comment id. In the reference
	// implementation this is an insert of a new comment record.
	Save(ctx context.Context, comment *Comment) error

	// FindByID looks up a comment by the composite key (articleID, id).
	// It replaces Java's Optional<Comment>: found is false (with a nil
	// comment and nil error) when no such comment exists; err is non-nil
	// only for genuine persistence failures.
	FindByID(ctx context.Context, articleID, id string) (comment *Comment, found bool, err error)

	// Remove deletes the given comment from persistence.
	Remove(ctx context.Context, comment *Comment) error
}

// commentIDFallbackSeq disambiguates fallback ids generated within the same
// nanosecond if the system random source is unavailable.
var commentIDFallbackSeq atomic.Uint64

// newCommentID returns a random RFC 4122 version-4 UUID string, matching the
// source's UUID.randomUUID().toString().
//
// MIGRATION_NOTE: crypto/rand.Read essentially never fails on supported
// platforms; rather than panicking in library code, a time+counter based id
// is used as a last-resort fallback. Review if strict UUID format is required
// in that (practically unreachable) case.
func newCommentID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16) + "-" +
			strconv.FormatUint(commentIDFallbackSeq.Add(1), 16)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant

	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:])
}
