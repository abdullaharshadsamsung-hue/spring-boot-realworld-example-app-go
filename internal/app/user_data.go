package app

import "fmt"

// UserData is the read-side (CQRS query model) view of a user, carrying the
// fields the read store returns for a user (id, email, username, bio,
// image). Services use it to build API responses such as the
// user-with-token payload and ProfileData.
//
// The zero value (UserData{}) is the equivalent of the source's no-arg
// constructor: every field unset. Fields are exported, so they serve as the
// getters/setters Lombok's @Data generated.
//
// All fields are comparable, so == compares every field, matching Lombok's
// generated equals/hashCode; Equal is provided for readability.
//
// Nullability follows ProfileData: Bio and Image are plain strings and the
// read store is expected to COALESCE NULL columns to "".
// MIGRATION_NOTE: a NULL bio/image in Java would be null; here it is "".
type UserData struct {
	ID       string `db:"id" json:"id"`
	Email    string `db:"email" json:"email"`
	Username string `db:"username" json:"username"`
	Bio      string `db:"bio" json:"bio"`
	Image    string `db:"image" json:"image"`
}

// NewUserData builds a UserData with all fields, in the source's positional
// order (id, email, username, bio, image).
func NewUserData(id, email, username, bio, image string) UserData {
	return UserData{
		ID:       id,
		Email:    email,
		Username: username,
		Bio:      bio,
		Image:    image,
	}
}

// Equal reports whether u and other have identical values in every field.
func (u UserData) Equal(other UserData) bool {
	return u == other
}

// String returns a representation including the type name and every field
// value, mirroring Lombok's generated toString.
func (u UserData) String() string {
	return fmt.Sprintf("UserData(id=%s, email=%s, username=%s, bio=%s, image=%s)",
		u.ID, u.Email, u.Username, u.Bio, u.Image)
}
