// Package core holds the domain model of the RealWorld (Conduit) API: the
// user, article, comment and favorite aggregates and the repository ports
// that the store layer implements.
package core

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
)

// ErrNilUser is returned when a nil *User is passed where a user is required.
var ErrNilUser = errors.New("core: user is nil")

// ErrNilFollowRelation is returned when a nil *FollowRelation is passed where
// a relation is required.
var ErrNilFollowRelation = errors.New("core: follow relation is nil")

// User is the user aggregate. It mirrors io.spring.core.user.User.
//
// MIGRATION_NOTE: the source's nullable Java Strings for Bio and Image are
// plain strings here. The core layer treats "" as "absent". The REST and
// GraphQL layers decide how an empty value is serialized. RealWorld requires
// JSON null for an absent bio/image, so those layers map "" to null; the
// store layer maps SQL NULL to "".
//
// MIGRATION_NOTE: per the project plan (fix Q1), Password holds the
// already-hashed password. Hashing happens in the app layer before NewUser
// or Update is called.
type User struct {
	// ID is the immutable random UUID (v4) that identifies the user.
	ID string
	// Email is the user's unique email address.
	Email string
	// Username is the user's unique username.
	Username string
	// Password is the user's hashed password.
	Password string
	// Bio is the user's free-text biography ("" when absent).
	Bio string
	// Image is the URL of the user's avatar ("" when absent).
	Image string
}

// NewUser creates a new User with a freshly generated random UUID as its ID.
// This matches the source constructor, which uses UUID.randomUUID().
// It returns an error only if the system's secure random source fails.
func NewUser(email, username, password, bio, image string) (*User, error) {
	id, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("core: generate user id: %w", err)
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

// Update overwrites each mutable field whose new value is non-empty. An empty
// string leaves the corresponding field unchanged. This mirrors the source's
// Util.isEmpty checks.
func (u *User) Update(email, username, password, bio, image string) {
	if u == nil {
		return
	}
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

// Equal reports whether u and other identify the same user. Identity is the
// ID only, matching Lombok's @EqualsAndHashCode(of = {"id"}).
func (u *User) Equal(other *User) bool {
	if u == nil || other == nil {
		return u == other
	}
	return u.ID == other.ID
}

// String returns a debug representation of the user. The password is
// deliberately omitted so it never ends up in logs.
func (u *User) String() string {
	if u == nil {
		return "User<nil>"
	}
	return fmt.Sprintf("User{id=%s, email=%s, username=%s, bio=%s, image=%s}",
		u.ID, u.Email, u.Username, u.Bio, u.Image)
}

// FollowRelation records that the user UserID follows the user TargetID.
// It mirrors io.spring.core.user.FollowRelation.
type FollowRelation struct {
	// UserID is the ID of the follower.
	UserID string
	// TargetID is the ID of the followed user.
	TargetID string
}

// NewFollowRelation creates a relation meaning userID follows targetID.
func NewFollowRelation(userID, targetID string) *FollowRelation {
	return &FollowRelation{UserID: userID, TargetID: targetID}
}

// Equal reports whether r and other describe the same relation. All fields
// are compared, matching Lombok's @Data equality.
func (r *FollowRelation) Equal(other *FollowRelation) bool {
	if r == nil || other == nil {
		return r == other
	}
	return r.UserID == other.UserID && r.TargetID == other.TargetID
}

// String returns a debug representation of the relation.
func (r *FollowRelation) String() string {
	if r == nil {
		return "FollowRelation<nil>"
	}
	return fmt.Sprintf("FollowRelation{userId=%s, targetId=%s}", r.UserID, r.TargetID)
}

// UserRepository is the domain port for persisting and querying User
// aggregates and FollowRelation entities. It replaces the source's
// @Repository interface io.spring.core.user.UserRepository. The PostgreSQL
// implementation (the MyBatisUserRepository equivalent) lives in the store
// layer and is injected by constructor in cmd/server/main.go.
//
// MIGRATION_NOTE: Java's Optional<T> results become (*T, error). A missing
// row is reported as an error matching ErrNotFound (internal/core/errors.go)
// via errors.Is. Callers must not treat a nil error as possible absence.
//
// MIGRATION_NOTE: @Repository exception translation has no Go equivalent.
// Implementations wrap driver errors with fmt.Errorf("...: %w", err).
type UserRepository interface {
	// Save persists u. If no user with the same ID exists, it inserts u.
	// Otherwise it updates the stored user's mutable fields (email,
	// username, password, bio, image). It returns ErrNilUser if u is nil.
	Save(ctx context.Context, u *User) error

	// FindByID looks up a user by unique ID. It returns ErrNotFound if no
	// user has that ID.
	FindByID(ctx context.Context, id string) (*User, error)

	// FindByUsername looks up a user by exact username. It returns
	// ErrNotFound if no user has that username.
	FindByUsername(ctx context.Context, username string) (*User, error)

	// FindByEmail looks up a user by exact email. It returns ErrNotFound if
	// no user has that email.
	FindByEmail(ctx context.Context, email string) (*User, error)

	// SaveRelation persists a relation meaning r.UserID follows r.TargetID.
	// It is a no-op if the relation already exists. It returns
	// ErrNilFollowRelation if r is nil.
	SaveRelation(ctx context.Context, r *FollowRelation) error

	// FindRelation finds the relation where userID follows targetID. It
	// returns ErrNotFound if no such relation exists.
	FindRelation(ctx context.Context, userID, targetID string) (*FollowRelation, error)

	// RemoveRelation deletes the relation identified by r.UserID and
	// r.TargetID. Deleting a relation that does not exist is not an error,
	// which matches the source's unconditional DELETE. It returns
	// ErrNilFollowRelation if r is nil.
	RemoveRelation(ctx context.Context, r *FollowRelation) error
}

// newUUID returns a random RFC 4122 version-4 UUID in canonical text form.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
