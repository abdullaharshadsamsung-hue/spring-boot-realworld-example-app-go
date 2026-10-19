package graphql

import (
	"context"
	"errors"
	"fmt"

	"migrated-app/internal/app"
	"migrated-app/internal/core"
	"migrated-app/internal/httpapi"
)

// Schema coordinates resolved by ArticleMutation. They replace the DGS
// codegen constants MUTATION.CreateArticle, MUTATION.UpdateArticle,
// MUTATION.FavoriteArticle, MUTATION.UnfavoriteArticle and
// MUTATION.DeleteArticle.
const (
	MutationFieldCreateArticle     = "createArticle"
	MutationFieldUpdateArticle     = "updateArticle"
	MutationFieldFavoriteArticle   = "favoriteArticle"
	MutationFieldUnfavoriteArticle = "unfavoriteArticle"
	MutationFieldDeleteArticle     = "deleteArticle"
)

// ArticleMutationCommands is the write service used by ArticleMutation.
// *app.ArticleCommandService satisfies it.
type ArticleMutationCommands interface {
	// CreateArticle validates and persists a new article owned by creator.
	CreateArticle(ctx context.Context, param app.NewArticleParam, creator *core.User) (*core.Article, error)
	// UpdateArticle applies param to article and persists it.
	UpdateArticle(ctx context.Context, article *core.Article, param app.UpdateArticleParam) (*core.Article, error)
}

var _ ArticleMutationCommands = (*app.ArticleCommandService)(nil)

// ArticleMutationArticles is the article repository used by ArticleMutation.
// core.ArticleRepository satisfies it.
type ArticleMutationArticles interface {
	// FindBySlug returns the article or an error matching core.ErrNotFound.
	FindBySlug(ctx context.Context, slug string) (*core.Article, error)
	// Remove deletes the article.
	Remove(ctx context.Context, article *core.Article) error
}

// Errors returned by NewArticleMutation for missing dependencies.
var (
	ErrNilArticleMutationCommands  = errors.New("graphql: article mutation command service is nil")
	ErrNilArticleMutationFavorites = errors.New("graphql: article mutation favorite repository is nil")
	ErrNilArticleMutationArticles  = errors.New("graphql: article mutation article repository is nil")
)

// CreateArticleInput is the GraphQL CreateArticleInput type.
type CreateArticleInput struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Body        string   `json:"body"`
	TagList     []string `json:"tagList"`
}

// UpdateArticleInput is the GraphQL UpdateArticleInput type. Nil fields are
// left unchanged.
type UpdateArticleInput struct {
	Title       *string `json:"title"`
	Body        *string `json:"body"`
	Description *string `json:"description"`
}

// ArticlePayloadResult replaces DataFetcherResult<ArticlePayload>. The
// payload has no fields of its own; its "article" field is resolved by
// ArticleDatafetcher.ArticlePayloadArticle from LocalContext.
type ArticlePayloadResult struct {
	LocalContext *core.Article
}

// ArticleMutation resolves the article write mutations.
//
// MIGRATION_NOTE: the source is a DGS @DgsComponent. The composition root must
// build it with NewArticleMutation and register its methods with the GraphQL
// executor at "POST /graphql" (behind the auth middleware), passing
// ArticlePayloadResult.LocalContext to ArticleDatafetcher.ArticlePayloadArticle.
type ArticleMutation struct {
	commands  ArticleMutationCommands
	favorites core.ArticleFavoriteRepository
	articles  ArticleMutationArticles
}

// NewArticleMutation creates an ArticleMutation.
func NewArticleMutation(commands ArticleMutationCommands, favorites core.ArticleFavoriteRepository, articles ArticleMutationArticles) (*ArticleMutation, error) {
	if commands == nil {
		return nil, ErrNilArticleMutationCommands
	}
	if favorites == nil {
		return nil, ErrNilArticleMutationFavorites
	}
	if articles == nil {
		return nil, ErrNilArticleMutationArticles
	}
	return &ArticleMutation{commands: commands, favorites: favorites, articles: articles}, nil
}

// CreateArticle resolves Mutation.createArticle(input). It requires an
// authenticated user; a nil tag list becomes empty. Validation errors from
// the command service are returned unchanged.
func (m *ArticleMutation) CreateArticle(ctx context.Context, input CreateArticleInput) (*ArticlePayloadResult, error) {
	user, err := requireCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	tags := input.TagList
	if tags == nil {
		tags = []string{}
	}
	param := app.NewArticleParam{
		Title:       input.Title,
		Description: input.Description,
		Body:        input.Body,
		TagList:     tags,
	}
	article, err := m.commands.CreateArticle(ctx, param, user)
	if err != nil {
		return nil, err
	}
	return &ArticlePayloadResult{LocalContext: article}, nil
}

// UpdateArticle resolves Mutation.updateArticle(slug, changes). As in the
// source, the slug is looked up BEFORE authentication is checked.
func (m *ArticleMutation) UpdateArticle(ctx context.Context, slug string, changes UpdateArticleInput) (*ArticlePayloadResult, error) {
	article, err := m.findArticle(ctx, slug, MutationFieldUpdateArticle)
	if err != nil {
		return nil, err
	}
	user, err := requireCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	if !core.CanWriteArticle(user, article) {
		return nil, httpapi.ErrNoAuthorization
	}
	// Nil fields map to "", which core.Article.Update treats as unchanged.
	param := app.NewUpdateArticleParam(deref(changes.Title), deref(changes.Body), deref(changes.Description))
	updated, err := m.commands.UpdateArticle(ctx, article, param)
	if err != nil {
		return nil, err
	}
	return &ArticlePayloadResult{LocalContext: updated}, nil
}

// FavoriteArticle resolves Mutation.favoriteArticle(slug).
func (m *ArticleMutation) FavoriteArticle(ctx context.Context, slug string) (*ArticlePayloadResult, error) {
	user, err := requireCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	article, err := m.findArticle(ctx, slug, MutationFieldFavoriteArticle)
	if err != nil {
		return nil, err
	}
	fav := core.NewArticleFavorite(article.ID, user.ID)
	if err := m.favorites.Save(ctx, fav); err != nil {
		return nil, fmt.Errorf("%s: save favorite: %w", MutationFieldFavoriteArticle, err)
	}
	return &ArticlePayloadResult{LocalContext: article}, nil
}

// UnfavoriteArticle resolves Mutation.unfavoriteArticle(slug). A missing
// favorite is silently ignored.
func (m *ArticleMutation) UnfavoriteArticle(ctx context.Context, slug string) (*ArticlePayloadResult, error) {
	user, err := requireCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	article, err := m.findArticle(ctx, slug, MutationFieldUnfavoriteArticle)
	if err != nil {
		return nil, err
	}
	fav, found, err := m.favorites.Find(ctx, article.ID, user.ID)
	if err != nil {
		return nil, fmt.Errorf("%s: find favorite: %w", MutationFieldUnfavoriteArticle, err)
	}
	if found {
		if err := m.favorites.Remove(ctx, fav); err != nil {
			return nil, fmt.Errorf("%s: remove favorite: %w", MutationFieldUnfavoriteArticle, err)
		}
	}
	return &ArticlePayloadResult{LocalContext: article}, nil
}

// DeleteArticle resolves Mutation.deleteArticle(slug) and returns
// {success: true} on success.
func (m *ArticleMutation) DeleteArticle(ctx context.Context, slug string) (*DeletionStatus, error) {
	user, err := requireCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	article, err := m.findArticle(ctx, slug, MutationFieldDeleteArticle)
	if err != nil {
		return nil, err
	}
	if !core.CanWriteArticle(user, article) {
		return nil, httpapi.ErrNoAuthorization
	}
	if err := m.articles.Remove(ctx, article); err != nil {
		return nil, fmt.Errorf("%s: remove article: %w", MutationFieldDeleteArticle, err)
	}
	return &DeletionStatus{Success: true}, nil
}

// findArticle loads an article by slug, mapping absence to
// httpapi.ErrResourceNotFound.
func (m *ArticleMutation) findArticle(ctx context.Context, slug, field string) (*core.Article, error) {
	article, err := m.articles.FindBySlug(ctx, slug)
	if err != nil {
		return nil, notFoundOrWrap(err, field+": find article")
	}
	if article == nil {
		return nil, httpapi.ErrResourceNotFound
	}
	return article, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
