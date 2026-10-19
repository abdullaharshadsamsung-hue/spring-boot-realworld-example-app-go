// Package app contains the application-layer commands and services of the
// RealWorld (Conduit) API.
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// articleRootKey is the JSON root key that wraps article request bodies,
// mirroring the source's @JsonRootName("article") with
// UNWRAP_ROOT_VALUE enabled.
const articleRootKey = "article"

// ErrMissingArticleRoot is returned when a JSON request body is not wrapped
// in the expected "article" root key (or that key's value is null).
var ErrMissingArticleRoot = errors.New(`app: request body must be wrapped in an "article" object`)

// UpdateArticleParam is the command parameter for updating an existing
// article.
//
// Every field is optional: an empty string means "leave this field
// unchanged". The zero value therefore is the equivalent of the source's
// no-args constructor (all fields ""), and is what JSON decoding starts
// from, so fields absent from the request body stay "".
//
// Fields are unexported and exposed through read-only accessors only; no
// setters are provided.
//
// Field type decisions: title, body and description are plain strings.
// The source defaults them to "" and treats null and "" identically
// ("no change"), so null is never observable and *string is unnecessary.
type UpdateArticleParam struct {
	title       string
	body        string
	description string
}

// NewUpdateArticleParam creates an update parameter with the given values,
// in the order title, body, description (the source's all-args
// constructor). Pass "" for any field that should be left unchanged.
func NewUpdateArticleParam(title, body, description string) UpdateArticleParam {
	return UpdateArticleParam{title: title, body: body, description: description}
}

// Title returns the new title, or "" if the title should be left unchanged.
func (p UpdateArticleParam) Title() string { return p.title }

// Body returns the new body, or "" if the body should be left unchanged.
func (p UpdateArticleParam) Body() string { return p.body }

// Description returns the new description, or "" if the description should
// be left unchanged.
func (p UpdateArticleParam) Description() string { return p.description }

// updateArticleFields is the wire shape of the inner "article" object.
type updateArticleFields struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	Description string `json:"description"`
}

// UnmarshalJSON decodes a body of the form
//
//	{"article":{"title":"...","body":"...","description":"..."}}
//
// Missing or null inner fields decode to "" (no change). Unknown properties
// are ignored, matching Spring Boot's default Jackson configuration. A body
// without the "article" root key yields ErrMissingArticleRoot.
func (p *UpdateArticleParam) UnmarshalJSON(data []byte) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("app: decode update article param: %w", err)
	}
	raw, ok := envelope[articleRootKey]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ErrMissingArticleRoot
	}
	var fields updateArticleFields
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("app: decode update article param: %w", err)
	}
	*p = NewUpdateArticleParam(fields.Title, fields.Body, fields.Description)
	return nil
}

// MarshalJSON encodes the parameter wrapped in the "article" root key, the
// inverse of UnmarshalJSON.
func (p UpdateArticleParam) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]updateArticleFields{
		articleRootKey: {Title: p.title, Body: p.body, Description: p.description},
	})
}
