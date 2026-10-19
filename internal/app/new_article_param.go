package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"migrated-app/internal/core"
)

// MsgArticleNameExists is the message reported when an article title maps to
// a slug that already exists (source: @DuplicatedArticleConstraint message).
const MsgArticleNameExists = "article name exists"

// NewArticleParam is the payload for creating an article. On the wire it is
// wrapped under an "article" root key.
type NewArticleParam struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Body        string   `json:"body"`
	TagList     []string `json:"tagList"`
}

// newArticleEnvelope replaces Jackson's @JsonRootName("article").
type newArticleEnvelope struct {
	Article *newArticleFields `json:"article"`
}

type newArticleFields struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Body        string   `json:"body"`
	TagList     []string `json:"tagList"`
}

// UnmarshalJSON decodes {"article": {...}}. A missing root returns
// ErrMissingArticleRoot.
func (p *NewArticleParam) UnmarshalJSON(data []byte) error {
	var env newArticleEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("decode new article param: %w", err)
	}
	if env.Article == nil {
		return ErrMissingArticleRoot
	}
	*p = NewArticleParam(*env.Article)
	return nil
}

// MarshalJSON encodes the param wrapped in the "article" root object.
func (p NewArticleParam) MarshalJSON() ([]byte, error) {
	f := newArticleFields(p)
	return json.Marshal(newArticleEnvelope{Article: &f})
}

// NewArticleValidationError carries field-level validation failures for
// article creation, keyed by JSON field name.
type NewArticleValidationError struct {
	Fields map[string][]string
}

// Error implements error.
func (e *NewArticleValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+strings.Join(e.Fields[k], ", "))
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// FieldErrors exposes the failures so the HTTP layer renders a 422.
func (e *NewArticleValidationError) FieldErrors() map[string][]string {
	return e.Fields
}

func (e *NewArticleValidationError) add(field, msg string) {
	if e.Fields == nil {
		e.Fields = make(map[string][]string)
	}
	e.Fields[field] = append(e.Fields[field], msg)
}

// ArticleSlugChecker reports whether an article with the given slug exists.
// *DuplicatedArticleValidator satisfies it.
type ArticleSlugChecker interface {
	SlugExists(ctx context.Context, slug string) (bool, error)
}

// ValidateNewArticleParam checks p: title, description and body must not be
// blank, and the title's slug must not already exist. It returns a
// *NewArticleValidationError for constraint violations.
func ValidateNewArticleParam(ctx context.Context, p NewArticleParam, checker ArticleSlugChecker) error {
	if checker == nil {
		return errors.New("validate new article param: nil slug checker")
	}
	verr := &NewArticleValidationError{}
	if isBlank(p.Title) {
		verr.add("title", msgCantBeEmpty)
	} else {
		exists, err := checker.SlugExists(ctx, core.ToSlug(p.Title))
		if err != nil {
			return fmt.Errorf("validate new article param: %w", err)
		}
		if exists {
			verr.add("title", MsgArticleNameExists)
		}
	}
	if isBlank(p.Description) {
		verr.add("description", msgCantBeEmpty)
	}
	if isBlank(p.Body) {
		verr.add("body", msgCantBeEmpty)
	}
	if len(verr.Fields) > 0 {
		return verr
	}
	return nil
}
