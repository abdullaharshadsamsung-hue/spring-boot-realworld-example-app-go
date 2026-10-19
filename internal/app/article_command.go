package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"migrated-app/internal/core"
)

// articleParamError is a constant-capable error type. It lets sentinel
// errors be declared as consts so they cannot be reassigned at runtime.
type articleParamError string

// Error implements the error interface.
func (e articleParamError) Error() string { return string(e) }

// ErrMissingArticleRoot is returned by UpdateArticleParam.UnmarshalJSON when
// the payload is not wrapped in the {"article": {...}} root object. This
// replaces Jackson's @JsonRootName("article") with UNWRAP_ROOT_VALUE enabled.
const ErrMissingArticleRoot articleParamError = `request body must be wrapped in an "article" root object`

// UpdateArticleParam carries the editable fields of an article. A field that
// is absent from the request body is the empty string, which
// core.Article.Update treats as "leave unchanged" (the source's "" defaults).
type UpdateArticleParam struct {
	title       string
	body        string
	description string
}

// NewUpdateArticleParam builds an UpdateArticleParam from explicit values.
func NewUpdateArticleParam(title, body, description string) UpdateArticleParam {
	return UpdateArticleParam{title: title, body: body, description: description}
}

// Title returns the requested new title, or "" when not provided.
func (p UpdateArticleParam) Title() string { return p.title }

// Body returns the requested new body, or "" when not provided.
func (p UpdateArticleParam) Body() string { return p.body }

// Description returns the requested new description, or "" when not provided.
func (p UpdateArticleParam) Description() string { return p.description }

// updateArticleFields is the wire shape of the inner "article" object.
type updateArticleFields struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	Description string `json:"description"`
}

// updateArticleEnvelope is the wire shape of the full request body.
type updateArticleEnvelope struct {
	Article *updateArticleFields `json:"article"`
}

// UnmarshalJSON decodes {"article": {"title": ..., "body": ..., "description": ...}}.
// Missing inner fields default to "". A missing or null root returns
// ErrMissingArticleRoot.
func (p *UpdateArticleParam) UnmarshalJSON(data []byte) error {
	var env updateArticleEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("decode update article param: %w", err)
	}
	if env.Article == nil {
		return ErrMissingArticleRoot
	}
	p.title = env.Article.Title
	p.body = env.Article.Body
	p.description = env.Article.Description
	return nil
}

// MarshalJSON encodes the param wrapped in the "article" root object,
// symmetric with UnmarshalJSON.
func (p UpdateArticleParam) MarshalJSON() ([]byte, error) {
	return json.Marshal(updateArticleEnvelope{Article: &updateArticleFields{
		Title:       p.title,
		Body:        p.body,
		Description: p.description,
	}})
}

// ArticleWriter is the persistence port used by ArticleCommandService.
// core.ArticleRepository satisfies it.
type ArticleWriter interface {
	// Save inserts or updates the article.
	Save(ctx context.Context, article *core.Article) error
}

// ArticleParamValidator validates a NewArticleParam before an article is
// created: "can't be empty" for blank title, description or body, followed
// by the duplicated-slug check. It returns a *ValidationError on failure.
//
// MIGRATION_NOTE: replaces Spring's @Validated/@Valid method validation and
// the DB-backed @DuplicatedArticleConstraint validator (see
// validate_article.go). Spring threw ConstraintViolationException via an AOP
// proxy; here the validation error is returned explicitly.
type ArticleParamValidator interface {
	// Validate returns nil when the param is acceptable.
	Validate(ctx context.Context, param NewArticleParam) error
}

// ArticleCommandService creates and updates Article aggregates (the write
// side of the CQRS split; reads go through ArticleQueryService).
//
// MIGRATION_NOTE: the source had no @Transactional; each Save runs under the
// repository's own transaction semantics, which is preserved here.
type ArticleCommandService struct {
	repo      ArticleWriter
	validator ArticleParamValidator
	now       func() time.Time
}

// NewArticleCommandService constructs an ArticleCommandService.
func NewArticleCommandService(repo ArticleWriter, validator ArticleParamValidator) *ArticleCommandService {
	return &ArticleCommandService{repo: repo, validator: validator, now: time.Now}
}

// CreateArticle validates param, builds a new Article owned by creator and
// persists it. Validation failures are returned unchanged (a *ValidationError)
// so the HTTP/GraphQL layers can map them to 422 responses.
func (s *ArticleCommandService) CreateArticle(ctx context.Context, param NewArticleParam, creator *core.User) (*core.Article, error) {
	if creator == nil {
		return nil, fmt.Errorf("create article: creator is required")
	}
	if s.validator != nil {
		if err := s.validator.Validate(ctx, param); err != nil {
			return nil, err
		}
	}
	article := core.NewArticle(
		param.Title,
		param.Description,
		param.Body,
		param.TagList,
		creator.ID,
		s.now(),
	)
	if err := s.repo.Save(ctx, article); err != nil {
		return nil, fmt.Errorf("create article: save: %w", err)
	}
	return article, nil
}

// UpdateArticle applies param to article via Article.Update, persists it and
// returns the same instance. As in the source, UpdateArticleParam carries no
// constraints, so no validation is performed.
func (s *ArticleCommandService) UpdateArticle(ctx context.Context, article *core.Article, param UpdateArticleParam) (*core.Article, error) {
	if article == nil {
		return nil, fmt.Errorf("update article: article is required")
	}
	article.Update(param.Title(), param.Description(), param.Body())
	if err := s.repo.Save(ctx, article); err != nil {
		return nil, fmt.Errorf("update article: save: %w", err)
	}
	return article, nil
}
