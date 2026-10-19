// Package app contains the application (read-side) layer of the RealWorld
// application: query-model DTOs and the query services that populate them.
package app

// ProfileData is the read-side (CQRS query model) view of a user profile as
// seen by a particular viewer. It is populated by the read store and
// serialized in profile responses and as the author of articles/comments.
//
// The zero value (ProfileData{}) is the equivalent of the source's no-arg
// constructor: the read store scans into it and then fills Following
// afterwards (the source's setFollowing(true) in the query services). Fields
// are exported so they act as the getters/setters Lombok's @Data generated.
//
// All fields are comparable, so Go's == on ProfileData values compares every
// field, matching Lombok's generated equals/hashCode; Equal is provided for
// readability at call sites.
//
// Nullability decisions:
//   - ID, Username: string — always present for an existing user.
//   - Bio, Image: string — the users table allows NULL, but the application
//     always writes a value on registration; the read store is expected to
//     COALESCE NULLs to "" when scanning.
//     MIGRATION_NOTE: the Java DTO would serialize a NULL column as JSON
//     null; here it becomes "". Review if wire-exact null output matters.
//
// JSON: ID is never serialized or deserialized (source's @JsonIgnore); no
// omitempty is used, so empty strings and following=false are always
// emitted, matching Jackson's default behavior.
type ProfileData struct {
	ID        string `db:"id" json:"-"`
	Username  string `db:"username" json:"username"`
	Bio       string `db:"bio" json:"bio"`
	Image     string `db:"image" json:"image"`
	Following bool   `db:"following" json:"following"`
}

// NewProfileData builds a ProfileData with all fields, in the source's
// positional order (id, username, bio, image, following).
func NewProfileData(id, username, bio, image string, following bool) ProfileData {
	return ProfileData{
		ID:        id,
		Username:  username,
		Bio:       bio,
		Image:     image,
		Following: following,
	}
}

// Equal reports whether p and other have identical values in every field.
func (p ProfileData) Equal(other ProfileData) bool {
	return p == other
}
