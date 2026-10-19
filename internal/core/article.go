// Package core contains the persistence-agnostic domain model of the
// RealWorld (Conduit) application.
package core

import (
	"crypto/rand"
	"fmt"
)

// Tag is an article tag: a name plus a domain-generated UUID identifier.
//
// Two tags are considered equal when their names match; the ID is ignored
// (mirrors Lombok's @EqualsAndHashCode(of = "name") in the source).
//
// The zero value (Tag{}) is the equivalent of the source's no-args
// constructor and is what sqlx / encoding/json populate when scanning or
// decoding. Fields are exported so they serve directly as the accessors and
// mutators Lombok's @Data generated.
type Tag struct {
	ID   string `db:"id" json:"id"`
	Name string `db:"name" json:"name"`
}

// NewTag returns a Tag with the given name and a freshly generated random
// (version 4) UUID string as its ID.
func NewTag(name string) *Tag {
	return &Tag{
		ID:   newRandomUUID(),
		Name: name,
	}
}

// Equal reports whether t and other represent the same tag. Equality is
// based solely on Name; ID is ignored. Two nil tags are equal; a nil and a
// non-nil tag are not.
func (t *Tag) Equal(other *Tag) bool {
	if t == nil || other == nil {
		return t == other
	}
	return t.Name == other.Name
}

// Key returns the value that identifies the tag for hashing purposes (for
// example as a map key or set member). It is consistent with Equal: it
// depends only on Name.
func (t *Tag) Key() string {
	if t == nil {
		return ""
	}
	return t.Name
}

// String returns a human-readable representation including both id and
// name, matching the shape of Lombok's generated toString.
func (t *Tag) String() string {
	if t == nil {
		return "Tag(<nil>)"
	}
	return fmt.Sprintf("Tag(id=%s, name=%s)", t.ID, t.Name)
}

// newRandomUUID generates an RFC 4122 version 4 UUID in its canonical
// 36-character string form, equivalent to Java's UUID.randomUUID().toString().
func newRandomUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// MIGRATION_NOTE: crypto/rand failing means the OS entropy source is
		// broken; since Go 1.24 rand.Read itself crashes the process in this
		// case and never returns an error. Treating it as unrecoverable keeps
		// NewTag's single-value signature, matching the source constructor.
		panic(fmt.Sprintf("core: crypto/rand unavailable: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
