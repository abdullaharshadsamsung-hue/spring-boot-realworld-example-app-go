// Package app contains the application (query/command) layer: read-model
// DTOs, pagination helpers and services sitting between the HTTP/GraphQL
// adapters and the core domain.
package app

import (
	"encoding/json"
	"time"
)

// isoLayout is the wire format used for timestamps in API responses. It
// mirrors the source's Jackson DateTime serializer, which writes ISO-8601 in
// UTC with millisecond precision.
const isoLayout = "2006-01-02T15:04:05.000Z"

// formatISO renders t in UTC using the wire layout.
func formatISO(t time.Time) string {
	return t.UTC().Format(isoLayout)
}

// ProfileData is the read model for a user's public profile. The ID is used
// internally (for example to check follow relations) and is never written to
// JSON, matching the source's @JsonIgnore on id.
type ProfileData struct {
	ID        string `json:"-"`
	Username  string `json:"username"`
	Bio       string `json:"bio"`
	Image     string `json:"image"`
	Following bool   `json:"following"`
}

// NewProfileData returns a ProfileData populated in the source's
// constructor order: id, username, bio, image, following.
func NewProfileData(id, username, bio, image string, following bool) ProfileData {
	return ProfileData{
		ID:        id,
		Username:  username,
		Bio:       bio,
		Image:     image,
		Following: following,
	}
}

// Equal reports whether p and other hold identical values in every field.
func (p ProfileData) Equal(other ProfileData) bool {
	return p == other
}

// profilesEqual compares two optional profiles. Two nil profiles are equal;
// a nil and a non-nil profile are not.
func profilesEqual(a, b *ProfileData) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// ArticleData is the read model for an article, filled by the store layer's
// read queries and serialized for the article endpoints.
//
// The zero value is an empty, ready-to-populate instance; fields are
// exported so the store layer can set them directly (replacing the setters
// MyBatis used). Author may be nil if the author could not be resolved.
type ArticleData struct {
	ID             string
	Slug           string
	Title          string
	Description    string
	Body           string
	Favorited      bool
	FavoritesCount int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	TagList        []string
	Author         *ProfileData
}

// NewArticleData returns an ArticleData populated in the source's
// all-args constructor order: id, slug, title, description, body,
// favorited, favoritesCount, createdAt, updatedAt, tagList, profileData.
// Test fixtures rely on this positional order.
func NewArticleData(
	id, slug, title, description, body string,
	favorited bool,
	favoritesCount int,
	createdAt, updatedAt time.Time,
	tagList []string,
	profileData *ProfileData,
) ArticleData {
	return ArticleData{
		ID:             id,
		Slug:           slug,
		Title:          title,
		Description:    description,
		Body:           body,
		Favorited:      favorited,
		FavoritesCount: favoritesCount,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		TagList:        tagList,
		Author:         profileData,
	}
}

// Cursor returns the pagination cursor for this article, derived from its
// UpdatedAt timestamp (the source's getCursor()).
func (a ArticleData) Cursor() DateTimeCursor {
	return NewDateTimeCursor(a.UpdatedAt)
}

// Equal reports whether a and other hold equal values in every field,
// replacing Lombok's generated equals. Timestamps are compared as instants
// (time.Time.Equal) and tag lists element by element; a nil and an empty
// tag list are treated as equal.
func (a ArticleData) Equal(other ArticleData) bool {
	if a.ID != other.ID ||
		a.Slug != other.Slug ||
		a.Title != other.Title ||
		a.Description != other.Description ||
		a.Body != other.Body ||
		a.Favorited != other.Favorited ||
		a.FavoritesCount != other.FavoritesCount ||
		!a.CreatedAt.Equal(other.CreatedAt) ||
		!a.UpdatedAt.Equal(other.UpdatedAt) {
		return false
	}
	if len(a.TagList) != len(other.TagList) {
		return false
	}
	for i := range a.TagList {
		if a.TagList[i] != other.TagList[i] {
			return false
		}
	}
	return profilesEqual(a.Author, other.Author)
}

// articleCursorJSON is the wire shape of the cursor property Jackson emitted
// from getCursor(): {"data": "<iso timestamp>"}.
type articleCursorJSON struct {
	Data string `json:"data"`
}

// articleDataJSON is the wire shape of ArticleData.
type articleDataJSON struct {
	ID             string            `json:"id"`
	Slug           string            `json:"slug"`
	Title          string            `json:"title"`
	Description    string            `json:"description"`
	Body           string            `json:"body"`
	Favorited      bool              `json:"favorited"`
	FavoritesCount int               `json:"favoritesCount"`
	CreatedAt      string            `json:"createdAt"`
	UpdatedAt      string            `json:"updatedAt"`
	TagList        []string          `json:"tagList"`
	Author         *ProfileData      `json:"author"`
	Cursor         articleCursorJSON `json:"cursor"`
}

// MarshalJSON serializes the article for API responses. The profile is
// written under "author", tagList is always an array (never null), and the
// cursor property is preserved for wire compatibility with the source.
//
// MIGRATION_NOTE: Jackson serialized the getCursor() getter as a "cursor"
// property; it is reproduced explicitly here. Review whether clients
// actually depend on it before removing.
func (a ArticleData) MarshalJSON() ([]byte, error) {
	tags := a.TagList
	if tags == nil {
		tags = []string{}
	}
	return json.Marshal(articleDataJSON{
		ID:             a.ID,
		Slug:           a.Slug,
		Title:          a.Title,
		Description:    a.Description,
		Body:           a.Body,
		Favorited:      a.Favorited,
		FavoritesCount: a.FavoritesCount,
		CreatedAt:      formatISO(a.CreatedAt),
		UpdatedAt:      formatISO(a.UpdatedAt),
		TagList:        tags,
		Author:         a.Author,
		Cursor:         articleCursorJSON{Data: formatISO(a.UpdatedAt)},
	})
}
