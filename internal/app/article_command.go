package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// newArticleRootKey is the JSON root key that wraps a NewArticleParam in a
// request body. It replaces Jackson's @JsonRootName("article") combined with
// DeserializationFeature.UNWRAP_ROOT_VALUE.
const newArticleRootKey = "article"

// Validation messages. They match the source's Bean Validation messages, so
// the error JSON stays wire-compatible.
const (
	// MsgCantBeEmpty is reported when a required field is blank. It is the
	// source's @NotBlank(message = "can't be empty").
	MsgCantBeEmpty = "can't be empty"
	// MsgArticleNameExists is reported when the title maps to the slug of an
	// existing article. It is the default message of the source's
	// @DuplicatedArticleConstraint.
	MsgArticleNameExists = "article name exists"
)

// ErrMissingArticleRoot is returned by DecodeNewArticleParam when the body
// has no "article" root key. Jackson fails the same way when root unwrapping
// is enabled and the wrapper name does not match.
var ErrMissingArticleRoot = errors.New("app: request body is missing root key \"article\"")

// NewArticleParam is the command used to create an article.
//
// MIGRATION_NOTE: Java String fields that may be null become plain Go
// strings, so null and "" are the same value. Both fail the @NotBlank check
// in the source, so this has no observable effect on validation. TagList
// stays nil when the client omits it, which mirrors the Java null. Callers
// must treat a nil TagList and an empty TagList the same way.
type NewArticleParam struct {
	// Title is the article title. It is required and must be unique by slug.
	Title string `json:"title"`
	// Description is a short summary. It is required.
	Description string `json:"description"`
	// Body is the article content. It is required.
	Body string `json:"body"`
	// TagList is the optional list of tag names.
	TagList []string `json:"tagList"`
}

// NewEmptyNewArticleParam returns a zero-valued NewArticleParam. It replaces
// Lombok's @NoArgsConstructor.
func NewEmptyNewArticleParam() NewArticleParam {
	return NewArticleParam{}
}

// NewNewArticleParam returns a NewArticleParam built from all of its fields,
// in source order: title, description, body, tagList. It replaces Lombok's
// @AllArgsConstructor.
func NewNewArticleParam(title, description, body string, tagList []string) NewArticleParam {
	return NewArticleParam{
		Title:       title,
		Description: description,
		Body:        body,
		TagList:     tagList,
	}
}

// NewArticleParamBuilder builds a NewArticleParam step by step. It replaces
// Lombok's @Builder.
type NewArticleParamBuilder struct {
	p NewArticleParam
}

// NewArticleParamBuilderFor returns an empty builder. It is the equivalent of
// NewArticleParam.builder() in the source.
func NewArticleParamBuilderFor() *NewArticleParamBuilder {
	return &NewArticleParamBuilder{}
}

// Title sets the title and returns the builder.
func (b *NewArticleParamBuilder) Title(title string) *NewArticleParamBuilder {
	b.p.Title = title
	return b
}

// Description sets the description and returns the builder.
func (b *NewArticleParamBuilder) Description(description string) *NewArticleParamBuilder {
	b.p.Description = description
	return b
}

// Body sets the body and returns the builder.
func (b *NewArticleParamBuilder) Body(body string) *NewArticleParamBuilder {
	b.p.Body = body
	return b
}

// TagList sets the tag list and returns the builder.
func (b *NewArticleParamBuilder) TagList(tagList []string) *NewArticleParamBuilder {
	b.p.TagList = tagList
	return b
}

// Build returns the assembled NewArticleParam. The tag slice is copied, so
// later changes to the builder do not affect the result.
func (b *NewArticleParamBuilder) Build() NewArticleParam {
	out := b.p
	if b.p.TagList != nil {
		out.TagList = append([]string(nil), b.p.TagList...)
	}
	return out
}

// DecodeNewArticleParam reads a JSON body of the form
// {"article": {"title": ..., "description": ..., "body": ..., "tagList": [...]}}
// and returns the inner NewArticleParam. Unknown fields are ignored, as
// Jackson does in the source configuration.
func DecodeNewArticleParam(r io.Reader) (NewArticleParam, error) {
	var envelope map[string]json.RawMessage
	if err := json.NewDecoder(r).Decode(&envelope); err != nil {
		return NewArticleParam{}, fmt.Errorf("decode new article body: %w", err)
	}
	raw, ok := envelope[newArticleRootKey]
	if !ok {
		return NewArticleParam{}, ErrMissingArticleRoot
	}
	var p NewArticleParam
	if err := json.Unmarshal(raw, &p); err != nil {
		return NewArticleParam{}, fmt.Errorf("decode new article param: %w", err)
	}
	return p, nil
}

// ArticleSlugChecker reports whether an article with the given slug already
// exists. It replaces the ArticleQueryService.findBySlug(slug, null)
// dependency used by the source's DuplicatedArticleValidator.
type ArticleSlugChecker interface {
	// SlugExists returns true if an article with this slug exists.
	SlugExists(ctx context.Context, slug string) (bool, error)
}

// ArticleSlugCheckerFunc adapts a plain function to ArticleSlugChecker.
type ArticleSlugCheckerFunc func(ctx context.Context, slug string) (bool, error)

// SlugExists calls f.
func (f ArticleSlugCheckerFunc) SlugExists(ctx context.Context, slug string) (bool, error) {
	return f(ctx, slug)
}

// NewArticleValidationError holds failed field constraints, keyed by JSON
// field name. The HTTP layer renders it as
// {"errors": {"title": ["can't be empty"]}}.
type NewArticleValidationError struct {
	// Fields maps a field name to its violation messages.
	Fields map[string][]string
}

// Error implements error. Fields are listed in sorted order so the output is
// deterministic.
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

func (e *NewArticleValidationError) add(field, msg string) {
	if e.Fields == nil {
		e.Fields = make(map[string][]string)
	}
	e.Fields[field] = append(e.Fields[field], msg)
}

// ErrNilArticleSlugChecker is returned when validation runs without a slug
// checker.
var ErrNilArticleSlugChecker = errors.New("app: nil ArticleSlugChecker")

// ValidateNewArticleParam applies the source's Bean Validation constraints.
// Title, description and body must be non-blank, and the title must not
// produce the slug of an existing article.
//
// It returns *NewArticleValidationError when constraints fail. Any other
// error comes from the checker itself.
//
// MIGRATION_NOTE: Hibernate Validator evaluates every constraint, so the
// duplicate check runs even when the title is blank. That behavior is kept
// here. The source would raise an NPE on a null title; Go has no null
// string, so that case reduces to a lookup of slug "".
func ValidateNewArticleParam(ctx context.Context, p NewArticleParam, checker ArticleSlugChecker) error {
	if checker == nil {
		return ErrNilArticleSlugChecker
	}
	verr := &NewArticleValidationError{}

	if isBlankArticleField(p.Title) {
		verr.add("title", MsgCantBeEmpty)
	}
	exists, err := checker.SlugExists(ctx, ArticleTitleToSlug(p.Title))
	if err != nil {
		return fmt.Errorf("check duplicated article title: %w", err)
	}
	if exists {
		verr.add("title", MsgArticleNameExists)
	}
	if isBlankArticleField(p.Description) {
		verr.add("description", MsgCantBeEmpty)
	}
	if isBlankArticleField(p.Body) {
		verr.add("body", MsgCantBeEmpty)
	}

	if len(verr.Fields) > 0 {
		return verr
	}
	return nil
}

// isBlankArticleField mirrors @NotBlank. A value is blank when it is empty or
// contains only whitespace.
func isBlankArticleField(s string) bool {
	return strings.TrimSpace(s) == ""
}

// articleSlugPattern is the Go form of the source regex
// "[\\&|[\\uFE30-\\uFFA0]|\\’|\\”|\\s\\?\\,\\.]+". The Java character-class
// union matches &, |, U+FE30..U+FFA0, ’, ”, whitespace, ?, comma and period.
var articleSlugPattern = regexp.MustCompile(`[&|\x{FE30}-\x{FFA0}’”\s?,.]+`)

// ArticleTitleToSlug converts a title to a slug the same way as the source's
// Article.toSlug. It lowercases the title and replaces each run of separator
// characters with "-".
//
// MIGRATION_NOTE: this must stay identical to the slug function the core
// Article uses. If internal/core exposes one, delegate to it. Manual review
// required.
func ArticleTitleToSlug(title string) string {
	return articleSlugPattern.ReplaceAllString(strings.ToLower(title), "-")
}
