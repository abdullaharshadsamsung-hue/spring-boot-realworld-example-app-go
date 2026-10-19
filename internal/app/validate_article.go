package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"migrated-app/internal/core"
)

// ArticleBySlugFinder is the read-side dependency of
// DuplicatedArticleValidator. *ArticleQueryService satisfies it.
type ArticleBySlugFinder interface {
	// FindBySlug returns the article; ok is false when it does not exist.
	// user may be nil (anonymous viewer).
	FindBySlug(ctx context.Context, slug string, user *core.User) (ArticleData, bool, error)
}

// Compile-time checks.
var (
	_ ArticleBySlugFinder = (*ArticleQueryService)(nil)
	_ ArticleSlugChecker  = (*DuplicatedArticleValidator)(nil)
)

// ErrNilArticleBySlugFinder is returned by NewDuplicatedArticleValidator when
// no finder is supplied.
var ErrNilArticleBySlugFinder = errors.New("app: nil ArticleBySlugFinder")

// DuplicatedArticleValidator rejects an article title whose slug already
// belongs to an existing article. It replaces the source's Bean Validation
// ConstraintValidator behind @DuplicatedArticleConstraint.
//
// MIGRATION_NOTE: the source got its ArticleQueryService through @Autowired
// field injection by Spring's constraint validator factory. Go uses
// constructor injection instead.
type DuplicatedArticleValidator struct {
	articles ArticleBySlugFinder
}

// NewDuplicatedArticleValidator builds a DuplicatedArticleValidator backed by
// articles (usually *ArticleQueryService).
func NewDuplicatedArticleValidator(articles ArticleBySlugFinder) (*DuplicatedArticleValidator, error) {
	if articles == nil {
		return nil, ErrNilArticleBySlugFinder
	}
	return &DuplicatedArticleValidator{articles: articles}, nil
}

// IsValid reports whether title is acceptable: it is valid when no existing
// article has the slug core.ToSlug(title). A blank title is reported as valid
// here; the "can't be empty" constraint covers that case.
//
// MIGRATION_NOTE: the source would throw an NPE on a null title. Go has no
// null string, so a blank title skips the lookup (agreed fix Q7).
func (v *DuplicatedArticleValidator) IsValid(ctx context.Context, title string) (bool, error) {
	if strings.TrimSpace(title) == "" {
		return true, nil
	}
	exists, err := v.SlugExists(ctx, core.ToSlug(title))
	if err != nil {
		return false, err
	}
	return !exists, nil
}

// SlugExists reports whether an article with slug exists. The lookup is
// anonymous (nil user), as in the source's findBySlug(slug, null). It lets the
// validator be passed to ValidateNewArticleParam as its ArticleSlugChecker.
func (v *DuplicatedArticleValidator) SlugExists(ctx context.Context, slug string) (bool, error) {
	_, ok, err := v.articles.FindBySlug(ctx, slug, nil)
	if err != nil {
		return false, fmt.Errorf("check duplicated article slug %q: %w", slug, err)
	}
	return ok, nil
}

// ValidateTitle returns a *NewArticleValidationError with
// MsgArticleNameExists on the "title" field when title is a duplicate, nil
// when it is acceptable, or a lookup error.
func (v *DuplicatedArticleValidator) ValidateTitle(ctx context.Context, title string) error {
	ok, err := v.IsValid(ctx, title)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	verr := &NewArticleValidationError{}
	verr.add("title", MsgArticleNameExists)
	return verr
}
