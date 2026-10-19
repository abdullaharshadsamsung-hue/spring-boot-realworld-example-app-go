package app

import (
	"encoding/json"
	"time"
)

// CommentData is the read model for a comment, including its author's
// profile. It is filled by the store layer's read queries and serialized for
// the comment endpoints.
//
// The zero value is the empty instance (the source's no-args constructor).
// Fields are exported so callers can read and set them directly, replacing
// Lombok's generated getters and setters. Author may be nil if the author
// could not be resolved.
type CommentData struct {
	ID        string
	Body      string
	ArticleID string // internal only; never serialized (source @JsonIgnore)
	CreatedAt time.Time
	UpdatedAt time.Time
	Author    *ProfileData
}

// NewCommentData returns a CommentData populated in the source's all-args
// constructor order: id, body, articleId, createdAt, updatedAt, profileData.
func NewCommentData(
	id, body, articleID string,
	createdAt, updatedAt time.Time,
	profileData *ProfileData,
) CommentData {
	return CommentData{
		ID:        id,
		Body:      body,
		ArticleID: articleID,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Author:    profileData,
	}
}

// Cursor returns the pagination cursor for this comment, derived from its
// CreatedAt timestamp (the source's getCursor()).
func (c CommentData) Cursor() DateTimeCursor {
	return NewDateTimeCursor(c.CreatedAt)
}

// Equal reports whether c and other hold equal values in all six fields,
// replacing Lombok's generated equals. Timestamps are compared as instants
// (time.Time.Equal) and authors by value.
func (c CommentData) Equal(other CommentData) bool {
	return c.ID == other.ID &&
		c.Body == other.Body &&
		c.ArticleID == other.ArticleID &&
		c.CreatedAt.Equal(other.CreatedAt) &&
		c.UpdatedAt.Equal(other.UpdatedAt) &&
		profilesEqual(c.Author, other.Author)
}

// commentDataJSON is the wire shape of CommentData. articleId is omitted.
type commentDataJSON struct {
	ID        string            `json:"id"`
	Body      string            `json:"body"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
	Author    *ProfileData      `json:"author"`
	Cursor    articleCursorJSON `json:"cursor"`
}

// MarshalJSON serializes the comment for API responses. The profile is
// written under "author", ArticleID is never emitted, and the cursor
// property (derived from CreatedAt) is preserved for wire compatibility.
//
// MIGRATION_NOTE: Jackson serialized the getCursor() getter as a "cursor"
// property; it is reproduced explicitly here, matching ArticleData.
func (c CommentData) MarshalJSON() ([]byte, error) {
	return json.Marshal(commentDataJSON{
		ID:        c.ID,
		Body:      c.Body,
		CreatedAt: formatISO(c.CreatedAt),
		UpdatedAt: formatISO(c.UpdatedAt),
		Author:    c.Author,
		Cursor:    articleCursorJSON{Data: formatISO(c.CreatedAt)},
	})
}
