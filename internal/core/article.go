// Package core contains the domain model and repository ports of the
// RealWorld application.
package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Tag
// ---------------------------------------------------------------------------

// Tag is a label attached to articles. Two tags are equal when their names
// are equal. ID is only an identity for persistence.
type Tag struct {
	ID   string
	Name string
}

// NewTag creates a Tag with a freshly generated random UUID (v4) identifier.
func NewTag(name string) (Tag, error) {
	id, err := newArticleUUID()
	if err != nil {
		return Tag{}, fmt.Errorf("core: new tag %q: %w", name, err)
	}
	return Tag{ID: id, Name: name}, nil
}

// Equal reports whether t and other have the same name. This mirrors the
// source's equality, which compared names only.
func (t Tag) Equal(other Tag) bool {
	return t.Name == other.Name
}

// Key returns the value that identifies a tag for de-duplication (its name).
func (t Tag) Key() string {
	return t.Name
}

// String returns the tag's name.
func (t Tag) String() string {
	return t.Name
}

// ---------------------------------------------------------------------------
// Article
// ---------------------------------------------------------------------------

// Article is the article aggregate root.
type Article struct {
	ID          string
	UserID      string
	Slug        string
	Title       string
	Description string
	Body        string
	Tags        []Tag
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewArticle creates an article with a random UUID, a slug derived from the
// title and de-duplicated tags. Both CreatedAt and UpdatedAt are set to
// createdAt; pass the zero time to use the current time.
func NewArticle(title, description, body string, tagList []string, userID string, createdAt time.Time) (*Article, error) {
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	id, err := newArticleUUID()
	if err != nil {
		return nil, fmt.Errorf("core: new article: %w", err)
	}

	seen := make(map[string]struct{}, len(tagList))
	tags := make([]Tag, 0, len(tagList))
	for _, name := range tagList {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		tag, err := NewTag(name)
		if err != nil {
			return nil, fmt.Errorf("core: new article: %w", err)
		}
		tags = append(tags, tag)
	}

	return &Article{
		ID:          id,
		UserID:      userID,
		Slug:        ToSlug(title),
		Title:       title,
		Description: description,
		Body:        body,
		Tags:        tags,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}, nil
}

// Update changes the article's title, description and body. Empty values
// leave the corresponding field unchanged. Changing the title regenerates
// the slug. Every applied change refreshes UpdatedAt.
func (a *Article) Update(title, description, body string) {
	if a == nil {
		return
	}
	if title != "" {
		a.Title = title
		a.Slug = ToSlug(title)
		a.UpdatedAt = time.Now()
	}
	if description != "" {
		a.Description = description
		a.UpdatedAt = time.Now()
	}
	if body != "" {
		a.Body = body
		a.UpdatedAt = time.Now()
	}
}

// Equal reports whether a and other have the same ID.
func (a *Article) Equal(other *Article) bool {
	if a == nil || other == nil {
		return a == other
	}
	return a.ID == other.ID
}

// slugSeparators matches runs of the characters the source replaced with a
// hyphen: & | U+FE30..U+FFA0 ' " whitespace ? , .
var slugSeparators = regexp.MustCompile(`[&|\x{FE30}-\x{FFA0}'"\s?,.]+`)

// ToSlug converts a title to its slug: the lower-cased title with each run
// of separator characters replaced by a single "-".
func ToSlug(title string) string {
	return slugSeparators.ReplaceAllString(strings.ToLower(title), "-")
}

// newArticleUUID returns a random RFC 4122 version 4 UUID string.
func newArticleUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32], nil
}

// ---------------------------------------------------------------------------
// ArticleRepository
// ---------------------------------------------------------------------------

// ArticleRepository is the persistence port for Article aggregates. The
// PostgreSQL implementation lives in the store layer and is injected through
// constructors.
//
// MIGRATION_NOTE: Java's Optional<Article> becomes (*Article, error). A
// missing article is reported as the sentinel ErrNotFound (internal/core/errors.go);
// callers should check it with errors.Is.
type ArticleRepository interface {
	// Save persists a. If no article with a.ID exists, it is inserted;
	// otherwise the existing row is updated. Its tags are created if they
	// do not exist yet and linked to the article.
	Save(ctx context.Context, a *Article) error

	// FindByID returns the article with the given id, or ErrNotFound.
	FindByID(ctx context.Context, id string) (*Article, error)

	// FindBySlug returns the article with the given slug, or ErrNotFound.
	FindBySlug(ctx context.Context, slug string) (*Article, error)

	// Remove deletes a from persistence.
	Remove(ctx context.Context, a *Article) error
}
