package core

import (
	"crypto/rand"
	"encoding/hex"
	mrand "math/rand/v2"
	"regexp"
	"strings"
	"time"
)

// Tag is a label attached to an article. Two tags are the same when their
// names match, regardless of their IDs, as with the source's Tag
// (@EqualsAndHashCode(of = "name")).
type Tag struct {
	ID   string
	Name string
}

// NewTag returns a Tag with the given name and a new random UUID ID.
func NewTag(name string) Tag {
	return Tag{ID: newRandomUUIDForCore(), Name: name}
}

// Equal reports whether t and other have the same name, matching the
// source's name-based equality.
func (t Tag) Equal(other Tag) bool {
	return t.Name == other.Name
}

// Key returns the value that identifies a tag (its name). Use it as a map key
// to deduplicate tags.
func (t Tag) Key() string {
	return t.Name
}

// String returns the tag name.
func (t Tag) String() string {
	return t.Name
}

// Article is the aggregate root for a blog article.
//
// Fields are exported so the persistence layer can hydrate the struct. The
// source achieved the same with Lombok @Getter, @NoArgsConstructor and
// reflection. Domain code should change an article only through Update, so
// the slug and UpdatedAt stay consistent.
type Article struct {
	UserID      string
	ID          string
	Slug        string
	Title       string
	Description string
	Body        string
	Tags        []Tag
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewArticle creates an article with a new random UUID ID. The slug is
// derived from title, and duplicate tag names are removed. A nil tagList
// gives an empty tag list.
//
// createdAt sets both CreatedAt and UpdatedAt. If createdAt is the zero time,
// the current time is used. This covers both source constructors: the 5-arg
// one that defaulted to new DateTime() and the 6-arg one that took createdAt.
func NewArticle(title, description, body string, tagList []string, userID string, createdAt time.Time) *Article {
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	return &Article{
		UserID:      userID,
		ID:          newRandomUUIDForCore(),
		Slug:        ToSlug(title),
		Title:       title,
		Description: description,
		Body:        body,
		Tags:        dedupeTags(tagList),
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
}

// Update changes the title, description and body. Empty values are ignored.
// Changing the title also recomputes the slug. Any applied change sets
// UpdatedAt to the current time, in memory only.
func (a *Article) Update(title, description, body string) {
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

// Equal reports whether a and other are the same article, compared by ID
// only (source: @EqualsAndHashCode(of = {"id"})). Two nil articles are equal.
func (a *Article) Equal(other *Article) bool {
	if a == nil || other == nil {
		return a == other
	}
	return a.ID == other.ID
}

// slugSeparators matches runs of the characters the source's slug regex
// replaced: '&', '|', U+FE30–U+FFA0, '’', '”', whitespace, '?', ',' and '.'.
//
// MIGRATION_NOTE: Java's \s matches [ \t\n\x0B\f\r], but Go's RE2 \s does not
// include \x0B (vertical tab). \x0B is added explicitly to keep the behavior
// the same.
var slugSeparators = regexp.MustCompile(`[&|\x{FE30}-\x{FFA0}’”\s\x0B?,.]+`)

// ToSlug lowercases title and replaces each run of separator characters with
// a single '-'.
//
// MIGRATION_NOTE: Java's String.toLowerCase() depends on the locale, while
// strings.ToLower uses Unicode case mapping. Results can differ only for
// locale-specific cases such as the Turkish dotted I.
func ToSlug(title string) string {
	return slugSeparators.ReplaceAllString(strings.ToLower(title), "-")
}

// dedupeTags turns tag names into Tags and removes duplicate names, keeping
// the first occurrence of each.
//
// MIGRATION_NOTE: The source used a HashSet, so its tag order was
// unspecified. Keeping input order is deterministic and still satisfies the
// source's behavior.
func dedupeTags(names []string) []Tag {
	tags := make([]Tag, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		tags = append(tags, NewTag(name))
	}
	return tags
}

// newRandomUUIDForCore returns a random RFC 4122 version 4 UUID string, like
// Java's UUID.randomUUID().toString().
//
// MIGRATION_NOTE: No UUID library is in go.mod, so the UUID is built from
// crypto/rand. On the very rare chance that crypto/rand fails, math/rand/v2
// is used instead. These are identifiers, not secrets, so this avoids a panic
// and avoids changing the constructor signatures that callers already use.
// Review this if IDs ever need to be unguessable.
func newRandomUUIDForCore() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		for i := range b {
			b[i] = byte(mrand.Uint32())
		}
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
