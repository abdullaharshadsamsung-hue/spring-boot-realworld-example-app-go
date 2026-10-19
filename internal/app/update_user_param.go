package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// userRootKey is the JSON root key that wraps user request bodies, mirroring
// the source's @JsonRootName("user") with UNWRAP_ROOT_VALUE enabled.
const userRootKey = "user"

// ErrMissingUserRoot is returned when a JSON request body is not wrapped in
// the expected "user" root key (or that key's value is null).
var ErrMissingUserRoot = errors.New(`app: request body must be wrapped in a "user" object`)

// UpdateUserParam is the command parameter for updating the current user.
//
// Every field is optional: an empty string means "leave this field
// unchanged", matching core.User.Update. The zero value corresponds to the
// source's no-args constructor (all fields "").
type UpdateUserParam struct {
	email    string
	password string
	username string
	bio      string
	image    string
}

// NewUpdateUserParam creates an update parameter with the given values.
func NewUpdateUserParam(email, password, username, bio, image string) UpdateUserParam {
	return UpdateUserParam{email: email, password: password, username: username, bio: bio, image: image}
}

// Email returns the new email, or "" for no change.
func (p UpdateUserParam) Email() string { return p.email }

// Password returns the new password, or "" for no change.
func (p UpdateUserParam) Password() string { return p.password }

// Username returns the new username, or "" for no change.
func (p UpdateUserParam) Username() string { return p.username }

// Bio returns the new bio, or "" for no change.
func (p UpdateUserParam) Bio() string { return p.bio }

// Image returns the new image URL, or "" for no change.
func (p UpdateUserParam) Image() string { return p.image }

// updateUserFields is the wire shape of the inner "user" object.
type updateUserFields struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Username string `json:"username"`
	Bio      string `json:"bio"`
	Image    string `json:"image"`
}

// UnmarshalJSON decodes a body of the form {"user":{...}}. Missing or null
// inner fields decode to "" (no change).
func (p *UpdateUserParam) UnmarshalJSON(data []byte) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("app: decode update user param: %w", err)
	}
	raw, ok := envelope[userRootKey]
	if !ok {
		return ErrMissingUserRoot
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		*p = UpdateUserParam{}
		return nil
	}
	var f updateUserFields
	if err := json.Unmarshal(raw, &f); err != nil {
		return fmt.Errorf("app: decode update user param: %w", err)
	}
	*p = NewUpdateUserParam(f.Email, f.Password, f.Username, f.Bio, f.Image)
	return nil
}

// MarshalJSON encodes the parameter wrapped in the "user" root key.
func (p UpdateUserParam) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]updateUserFields{
		userRootKey: {Email: p.email, Password: p.password, Username: p.username, Bio: p.bio, Image: p.image},
	})
}
