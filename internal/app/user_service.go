// Package app contains the application-layer commands and services of the
// RealWorld (Conduit) API.
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// userRootKey is the JSON root key that wraps user request bodies, mirroring
// the source's @JsonRootName("user") with UNWRAP_ROOT_VALUE enabled.
const userRootKey = "user"

// ErrMissingUserRoot is returned when a JSON request body is not wrapped in
// the expected "user" root key (or that key's value is null).
var ErrMissingUserRoot = errors.New(`app: request body must be wrapped in a "user" object`)

// FieldError describes a single failed constraint on a request field, the
// equivalent of a Bean Validation ConstraintViolation.
type FieldError struct {
	// Field is the JSON name of the offending field.
	Field string
	// Message is the human-readable constraint message.
	Message string
}

// Error implements the error interface.
func (e *FieldError) Error() string {
	return fmt.Sprintf("%s %s", e.Field, e.Message)
}

// UpdateUserParam is the command parameter for PUT /user.
//
// Every field is optional: an empty string means "leave unchanged". The zero
// value is the equivalent of the source's no-args constructor (all fields ""),
// and JSON decoding starts from it, so absent fields stay "".
//
// Fields are unexported and exposed through read-only accessors only.
type UpdateUserParam struct {
	email    string
	password string
	username string
	bio      string
	image    string
}

// NewUpdateUserParam creates an update parameter in the order
// (email, password, username, bio, image), the source's all-args constructor.
func NewUpdateUserParam(email, password, username, bio, image string) UpdateUserParam {
	return UpdateUserParam{
		email:    email,
		password: password,
		username: username,
		bio:      bio,
		image:    image,
	}
}

// Email returns the new email, or "" if it should be left unchanged.
func (p UpdateUserParam) Email() string { return p.email }

// Password returns the new password, or "" if it should be left unchanged.
func (p UpdateUserParam) Password() string { return p.password }

// Username returns the new username, or "" if it should be left unchanged.
func (p UpdateUserParam) Username() string { return p.username }

// Bio returns the new bio, or "" if it should be left unchanged.
func (p UpdateUserParam) Bio() string { return p.bio }

// Image returns the new image URL, or "" if it should be left unchanged.
func (p UpdateUserParam) Image() string { return p.image }

// Validate checks the parameter's constraints, replacing the source's
// @Email(message = "should be an email") triggered by @Valid. An empty email
// is valid (it means "no change"), matching Hibernate's EmailValidator.
// On failure a *FieldError is returned.
func (p UpdateUserParam) Validate() error {
	if !isValidEmail(p.email) {
		return &FieldError{Field: "email", Message: "should be an email"}
	}
	return nil
}

// isValidEmail approximates javax.validation @Email semantics: "" is valid,
// otherwise the value must be a bare RFC 5322 addr-spec (local@domain).
//
// MIGRATION_NOTE: Hibernate's EmailValidator uses its own regexes; net/mail
// is close but may differ on exotic edge cases (e.g. quoted local parts).
func isValidEmail(s string) bool {
	if s == "" {
		return true
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return false
	}
	at := strings.LastIndexByte(s, '@')
	return at > 0 && at < len(s)-1
}

// updateUserFields is the wire shape of the inner "user" object.
type updateUserFields struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Username string `json:"username"`
	Bio      string `json:"bio"`
	Image    string `json:"image"`
}

// UnmarshalJSON decodes a body of the form {"user":{...}}. Missing or null
// inner fields decode to "" (no change); unknown properties are ignored,
// matching Spring Boot's default Jackson configuration. A body without the
// "user" root key yields ErrMissingUserRoot.
func (p *UpdateUserParam) UnmarshalJSON(data []byte) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("app: decode update user param: %w", err)
	}
	raw, ok := envelope[userRootKey]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ErrMissingUserRoot
	}
	var f updateUserFields
	if err := json.Unmarshal(raw, &f); err != nil {
		return fmt.Errorf("app: decode update user param: %w", err)
	}
	*p = NewUpdateUserParam(f.Email, f.Password, f.Username, f.Bio, f.Image)
	return nil
}

// MarshalJSON encodes the parameter wrapped in the "user" root key, the
// inverse of UnmarshalJSON.
func (p UpdateUserParam) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]updateUserFields{
		userRootKey: {
			Email:    p.email,
			Password: p.password,
			Username: p.username,
			Bio:      p.bio,
			Image:    p.image,
		},
	})
}

// UpdateUserParamBuilder is a fluent builder for UpdateUserParam (the
// source's Lombok @Builder). Unset fields default to "" (@Builder.Default).
type UpdateUserParamBuilder struct {
	p UpdateUserParam
}

// NewUpdateUserParamBuilder returns a builder with every field set to "".
func NewUpdateUserParamBuilder() *UpdateUserParamBuilder {
	return &UpdateUserParamBuilder{}
}

// Email sets the email.
func (b *UpdateUserParamBuilder) Email(v string) *UpdateUserParamBuilder {
	b.p.email = v
	return b
}

// Password sets the password.
func (b *UpdateUserParamBuilder) Password(v string) *UpdateUserParamBuilder {
	b.p.password = v
	return b
}

// Username sets the username.
func (b *UpdateUserParamBuilder) Username(v string) *UpdateUserParamBuilder {
	b.p.username = v
	return b
}

// Bio sets the bio.
func (b *UpdateUserParamBuilder) Bio(v string) *UpdateUserParamBuilder {
	b.p.bio = v
	return b
}

// Image sets the image URL.
func (b *UpdateUserParamBuilder) Image(v string) *UpdateUserParamBuilder {
	b.p.image = v
	return b
}

// Build returns the configured UpdateUserParam.
func (b *UpdateUserParamBuilder) Build() UpdateUserParam {
	return b.p
}
