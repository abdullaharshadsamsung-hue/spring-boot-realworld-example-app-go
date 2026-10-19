// Package core contains the domain entities of the RealWorld (Conduit)
// application, independent of persistence and transport concerns.
package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// User is an application user: identity, credentials and profile fields.
//
// The zero value User{} has every field empty, including ID. It replaces
// the source's Lombok @NoArgsConstructor, which existed so MyBatis could
// build the object by reflection. The store package fills a zero-value
// User directly from query results.
//
// MIGRATION_NOTE: Java's String fields could be null. Go strings cannot,
// so "null" and "" are both the empty string here. Update treats them the
// same way, which matches the source's Util.isEmpty check (null or "").
type User struct {
	// ID is an application-generated UUID v4 string. It is the entity's
	// identity, and Update never changes it.
	ID string
	// Email is the user's e-mail address.
	Email string
	// Username is the user's unique public handle.
	Username string
	// Password is the user's stored (hashed) password.
	Password string
	// Bio is free-form profile text.
	Bio string
	// Image is the URL of the user's profile image.
	Image string
}

// NewUser creates a user with a freshly generated random UUID v4 as its ID.
// The other fields are stored exactly as given, with no validation or
// transformation.
//
// MIGRATION_NOTE: Java's UUID.randomUUID() cannot fail. Reading from the
// OS CSPRNG in Go can, at least in principle, so NewUser returns an error
// rather than panicking.
func NewUser(email, username, password, bio, image string) (*User, error) {
	id, err := newUUIDv4()
	if err != nil {
		return nil, fmt.Errorf("new user: %w", err)
	}
	return &User{
		ID:       id,
		Email:    email,
		Username: username,
		Password: password,
		Bio:      bio,
		Image:    image,
	}, nil
}

// Update partially updates the user. Each field is overwritten only when
// the supplied value is non-empty; otherwise the current value is kept.
// The ID is never changed.
func (u *User) Update(email, username, password, bio, image string) {
	if email != "" {
		u.Email = email
	}
	if username != "" {
		u.Username = username
	}
	if password != "" {
		u.Password = password
	}
	if bio != "" {
		u.Bio = bio
	}
	if image != "" {
		u.Image = image
	}
}

// Equal reports whether u and other are the same entity. Like the source's
// @EqualsAndHashCode(of = "id"), it compares only the ID. Two nil users are
// equal; a nil user is never equal to a non-nil one.
//
// MIGRATION_NOTE: Go has no hashCode(). To key a map or set by user
// identity, use User.ID as the key. That matches the source's id-based
// hash.
func (u *User) Equal(other *User) bool {
	if u == nil || other == nil {
		return u == other
	}
	return u.ID == other.ID
}

// String returns a debug representation of the user. The password is
// redacted so it never ends up in logs.
func (u *User) String() string {
	if u == nil {
		return "User<nil>"
	}
	return fmt.Sprintf("User{ID:%q, Email:%q, Username:%q, Bio:%q, Image:%q}",
		u.ID, u.Email, u.Username, u.Bio, u.Image)
}

// newUUIDv4 returns a random RFC 4122 version-4 UUID in the canonical
// 8-4-4-4-12 lowercase hex form, the same format as Java's
// UUID.randomUUID().toString().
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
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
	return string(out[:]), nil
}
