package app

import "encoding/json"

// UserWithToken is the immutable read-side view returned by the
// authentication and registration endpoints: a user's public profile fields
// copied from UserData plus a freshly issued auth token.
//
// Fields are unexported and exposed only through accessor methods, matching
// the source's getter-only (@Getter, no setters) class. The user ID is
// deliberately not carried, as in the source.
//
// JSON: serialized as {"email","username","bio","image","token"} via
// MarshalJSON, matching Jackson's getter-based output (no omitempty).
type UserWithToken struct {
	email    string
	username string
	bio      string
	image    string
	token    string
}

// NewUserWithToken builds a UserWithToken from userData's email, username,
// bio and image together with the supplied token.
func NewUserWithToken(userData UserData, token string) UserWithToken {
	return UserWithToken{
		email:    userData.Email,
		username: userData.Username,
		bio:      userData.Bio,
		image:    userData.Image,
		token:    token,
	}
}

// Email returns the user's email address.
func (u UserWithToken) Email() string { return u.email }

// Username returns the user's username.
func (u UserWithToken) Username() string { return u.username }

// Bio returns the user's bio.
func (u UserWithToken) Bio() string { return u.bio }

// Image returns the user's image URL.
func (u UserWithToken) Image() string { return u.image }

// Token returns the auth token issued for the user.
func (u UserWithToken) Token() string { return u.token }

// userWithTokenJSON is the wire shape of UserWithToken.
type userWithTokenJSON struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Bio      string `json:"bio"`
	Image    string `json:"image"`
	Token    string `json:"token"`
}

// MarshalJSON encodes u with the fields email, username, bio, image and
// token, matching the source's Jackson serialization.
func (u UserWithToken) MarshalJSON() ([]byte, error) {
	return json.Marshal(userWithTokenJSON{
		Email:    u.email,
		Username: u.username,
		Bio:      u.bio,
		Image:    u.image,
		Token:    u.token,
	})
}
