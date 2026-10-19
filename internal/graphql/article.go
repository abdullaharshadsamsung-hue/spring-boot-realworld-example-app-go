package graphql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
	"migrated-app/internal/httpapi"
)

// Schema coordinates resolved by ArticleDatafetcher. They replace the DGS
// codegen constants QUERY.Feed, QUERY.Articles, QUERY.Article, PROFILE.*,
// ARTICLEPAYLOAD.* and COMMENT.Article.
const (
	QueryFieldFeed          = "feed"
	QueryFieldArticles      = "articles"
	QueryFieldArticle       = "article"
	ProfileType             = "Profile"
	ProfileFieldFeed        = "feed"
	ProfileFieldFavorites   = "favorites"
	ProfileFieldArticles    = "articles"
	ArticlePayloadType      = "ArticlePayload"
	ArticlePayloadFieldArt  = "article"
	CommentFieldArticle     = "article"
)

// ErrFirstOrLastRequired is returned when neither first nor last is given.
// The message is kept verbatim from the source.
var ErrFirstOrLastRequired = errors.New("first 和 last 必须只存在一个")

// Errors returned by NewArticleDatafetcher.
var (
	// ErrNilArticleQuery is returned when no ArticleQuery is given.
	ErrNilArticleQuery = errors.New("graphql: article query service is nil")
	// ErrNilUserByUsernameFinder is returned when no user finder is given.
	ErrNilUserByUsernameFinder = errors.New("graphql: user repository is nil")
)

// ArticleQuery is the read-side dependency of ArticleDatafetcher.
// *app.ArticleQueryService satisfies it.
type ArticleQuery interface {
	// FindByID loads an article as seen by user (may be nil).
	FindByID(ctx context.Context, id string, user *core.User) (app.ArticleData, bool, error)
	// FindBySlug loads an article as seen by user (may be nil).
	FindBySlug(ctx context.Context, slug string, user *core.User) (app.ArticleData, bool, error)
	// FindRecentArticlesWithCursor returns a filtered cursor page.
	FindRecentArticlesWithCursor(ctx context.Context, tag, author, favoritedBy string, page app.CursorPageParameter, currentUser *core.User) (*app.CursorPager[app.ArticleNode], error)
	// FindUserFeedWithCursor returns a cursor page of user's feed.
	FindUserFeedWithCursor(ctx context.Context, user *core.User, page app.CursorPageParameter) (*app.CursorPager[app.ArticleNode], error)
}

var _ ArticleQuery = (*app.ArticleQueryService)(nil)

// UserByUsernameFinder looks up domain users by username (the source's
// UserRepository.findByUsername).
//
// MIGRATION_NOTE: the composition root adapts the store's user repository to
// this interface if its signature differs.
type UserByUsernameFinder interface {
	// FindByUsername returns the user; ok is false when none exists.
	FindByUsername(ctx context.Context, username string) (*core.User, bool, error)
}

// Article is the GraphQL Article type (author is resolved separately by
// ProfileDatafetcher.ArticleAuthor).
type Article struct {
	Body           string   `json:"body"`
	CreatedAt      string   `json:"createdAt"`
	Description    string   `json:"description"`
	Favorited      bool     `json:"favorited"`
	FavoritesCount int      `json:"favoritesCount"`
	Slug           string   `json:"slug"`
	TagList        []string `json:"tagList"`
	Title          string   `json:"title"`
	UpdatedAt      string   `json:"updatedAt"`
}

// ArticleEdge is the GraphQL ArticleEdge type.
type ArticleEdge struct {
	Cursor string   `json:"cursor"`
	Node   *Article `json:"node"`
}

// PageInfo is the Relay PageInfo type. Nil cursors render as null.
type PageInfo struct {
	StartCursor     *string `json:"startCursor"`
	EndCursor       *string `json:"endCursor"`
	HasPreviousPage bool    `json:"hasPreviousPage"`
	HasNextPage     bool    `json:"hasNextPage"`
}

// ArticlesConnection is the GraphQL ArticlesConnection type.
type ArticlesConnection struct {
	Edges    []ArticleEdge `json:"edges"`
	PageInfo PageInfo      `json:"pageInfo"`
}

// ArticlesConnectionResult replaces DataFetcherResult<ArticlesConnection>:
// LocalContext maps slug to ArticleData for child resolvers (Article.author).
type ArticlesConnectionResult struct {
	Data         *ArticlesConnection
	LocalContext map[string]app.ArticleData
}

// ArticleResult replaces DataFetcherResult<Article>.
type ArticleResult struct {
	Data         *Article
	LocalContext map[string]app.ArticleData
}

// CursorArgs holds the Relay pagination arguments. A nil First/Last and an
// empty After/Before mean "not supplied".
type CursorArgs struct {
	First  *int
	After  string
	Last   *int
	Before string
}

// ArticleFilter holds the optional Query.articles filters ("" = no filter).
type ArticleFilter struct {
	AuthoredBy  string
	FavoritedBy string
	WithTag     string
}

// ArticleDatafetcher resolves Query.feed, Query.articles, Query.article,
// Profile.feed, Profile.favorites, Profile.articles, ArticlePayload.article
// and Comment.article.
//
// MIGRATION_NOTE: the source is a DGS @DgsComponent. The composition root
// must register these methods with the GraphQL executor mounted at
// "POST /graphql" (behind the Authenticate middleware) and propagate the
// LocalContext maps to child resolvers.
type ArticleDatafetcher struct {
	articles ArticleQuery
	users    UserByUsernameFinder
}

// NewArticleDatafetcher creates an ArticleDatafetcher.
func NewArticleDatafetcher(articles ArticleQuery, users UserByUsernameFinder) (*ArticleDatafetcher, error) {
	if articles == nil {
		return nil, ErrNilArticleQuery
	}
	if users == nil {
		return nil, ErrNilUserByUsernameFinder
	}
	return &ArticleDatafetcher{articles: articles, users: users}, nil
}

// Feed resolves Query.feed for the current user (nil when anonymous; the
// query service then returns app.ErrFeedRequiresUser).
func (d *ArticleDatafetcher) Feed(ctx context.Context, args CursorArgs) (*ArticlesConnectionResult, error) {
	page, err := pageFor(args)
	if err != nil {
		return nil, err
	}
	current, _ := auth.UserFrom(ctx)
	pager, err := d.articles.FindUserFeedWithCursor(ctx, current, page)
	if err != nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", QueryType, QueryFieldFeed, err)
	}
	return connection(pager), nil
}

// ProfileFeed resolves Profile.feed: the feed of the parent profile's user.
func (d *ArticleDatafetcher) ProfileFeed(ctx context.Context, profile *Profile, args CursorArgs) (*ArticlesConnectionResult, error) {
	page, err := pageFor(args)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", ProfileType, ProfileFieldFeed, ErrMissingLocalContext)
	}
	target, ok, err := d.users.FindByUsername(ctx, profile.Username)
	if err != nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", ProfileType, ProfileFieldFeed, err)
	}
	if !ok || target == nil {
		return nil, httpapi.ErrResourceNotFound
	}
	pager, err := d.articles.FindUserFeedWithCursor(ctx, target, page)
	if err != nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", ProfileType, ProfileFieldFeed, err)
	}
	return connection(pager), nil
}

// ProfileFavorites resolves Profile.favorites: articles favorited by the
// parent profile, with flags computed for the current user.
func (d *ArticleDatafetcher) ProfileFavorites(ctx context.Context, profile *Profile, args CursorArgs) (*ArticlesConnectionResult, error) {
	if profile == nil {
		if _, err := pageFor(args); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("resolve %s.%s: %w", ProfileType, ProfileFieldFavorites, ErrMissingLocalContext)
	}
	return d.recent(ctx, args, ArticleFilter{FavoritedBy: profile.Username}, ProfileType, ProfileFieldFavorites)
}

// ProfileArticles resolves Profile.articles: articles authored by the
// parent profile.
func (d *ArticleDatafetcher) ProfileArticles(ctx context.Context, profile *Profile, args CursorArgs) (*ArticlesConnectionResult, error) {
	if profile == nil {
		if _, err := pageFor(args); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("resolve %s.%s: %w", ProfileType, ProfileFieldArticles, ErrMissingLocalContext)
	}
	return d.recent(ctx, args, ArticleFilter{AuthoredBy: profile.Username}, ProfileType, ProfileFieldArticles)
}

// Articles resolves Query.articles with optional tag/author/favoritedBy
// filters.
func (d *ArticleDatafetcher) Articles(ctx context.Context, args CursorArgs, filter ArticleFilter) (*ArticlesConnectionResult, error) {
	return d.recent(ctx, args, filter, QueryType, QueryFieldArticles)
}

// ArticlePayloadArticle resolves ArticlePayload.article by reloading the
// article (id from the parent mutation's domain Article) for the current user.
func (d *ArticleDatafetcher) ArticlePayloadArticle(ctx context.Context, article *core.Article) (*ArticleResult, error) {
	if article == nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", ArticlePayloadType, ArticlePayloadFieldArt, ErrMissingLocalContext)
	}
	return d.byID(ctx, article.ID, ArticlePayloadType, ArticlePayloadFieldArt)
}

// CommentArticle resolves Comment.article. articleID comes from the parent's
// CommentData local context.
func (d *ArticleDatafetcher) CommentArticle(ctx context.Context, articleID string) (*ArticleResult, error) {
	return d.byID(ctx, articleID, CommentType, CommentFieldArticle)
}

// ArticleBySlug resolves Query.article(slug). Unknown slugs yield
// httpapi.ErrResourceNotFound.
func (d *ArticleDatafetcher) ArticleBySlug(ctx context.Context, slug string) (*ArticleResult, error) {
	current, _ := auth.UserFrom(ctx)
	data, ok, err := d.articles.FindBySlug(ctx, slug, current)
	if err != nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", QueryType, QueryFieldArticle, err)
	}
	if !ok {
		return nil, httpapi.ErrResourceNotFound
	}
	return singleResult(data), nil
}

func (d *ArticleDatafetcher) byID(ctx context.Context, id, typ, field string) (*ArticleResult, error) {
	current, _ := auth.UserFrom(ctx)
	data, ok, err := d.articles.FindByID(ctx, id, current)
	if err != nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", typ, field, err)
	}
	if !ok {
		return nil, httpapi.ErrResourceNotFound
	}
	return singleResult(data), nil
}

func (d *ArticleDatafetcher) recent(ctx context.Context, args CursorArgs, f ArticleFilter, typ, field string) (*ArticlesConnectionResult, error) {
	page, err := pageFor(args)
	if err != nil {
		return nil, err
	}
	current, _ := auth.UserFrom(ctx)
	pager, err := d.articles.FindRecentArticlesWithCursor(ctx, f.WithTag, f.AuthoredBy, f.FavoritedBy, page, current)
	if err != nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", typ, field, err)
	}
	return connection(pager), nil
}

// pageFor validates first/last and builds the cursor parameter: first wins
// (NEXT with after), otherwise PREV with before.
func pageFor(args CursorArgs) (app.CursorPageParameter, error) {
	if args.First == nil && args.Last == nil {
		return app.CursorPageParameter{}, ErrFirstOrLastRequired
	}
	if args.First != nil {
		cursor, err := app.ParseDateTimeCursor(args.After)
		if err != nil {
			return app.CursorPageParameter{}, err
		}
		return app.NewCursorPageParameter(cursor, *args.First, app.DirectionNext), nil
	}
	cursor, err := app.ParseDateTimeCursor(args.Before)
	if err != nil {
		return app.CursorPageParameter{}, err
	}
	return app.NewCursorPageParameter(cursor, *args.Last, app.DirectionPrev), nil
}

func connection(pager *app.CursorPager[app.ArticleNode]) *ArticlesConnectionResult {
	nodes := pager.Data()
	edges := make([]ArticleEdge, 0, len(nodes))
	local := make(map[string]app.ArticleData, len(nodes))
	for _, n := range nodes {
		edges = append(edges, ArticleEdge{Cursor: n.Cursor(), Node: buildArticle(n.ArticleData)})
		local[n.Slug] = n.ArticleData
	}
	info := PageInfo{HasPreviousPage: pager.HasPrevious(), HasNextPage: pager.HasNext()}
	if c, ok := pager.StartCursor(); ok {
		info.StartCursor = &c
	}
	if c, ok := pager.EndCursor(); ok {
		info.EndCursor = &c
	}
	return &ArticlesConnectionResult{
		Data:         &ArticlesConnection{Edges: edges, PageInfo: info},
		LocalContext: local,
	}
}

func singleResult(data app.ArticleData) *ArticleResult {
	return &ArticleResult{
		Data:         buildArticle(data),
		LocalContext: map[string]app.ArticleData{data.Slug: data},
	}
}

func buildArticle(a app.ArticleData) *Article {
	return &Article{
		Body:           a.Body,
		CreatedAt:      isoUTC(a.CreatedAt),
		Description:    a.Description,
		Favorited:      a.Favorited,
		FavoritesCount: a.FavoritesCount,
		Slug:           a.Slug,
		TagList:        a.TagList,
		Title:          a.Title,
		UpdatedAt:      isoUTC(a.UpdatedAt),
	}
}

// isoUTC formats a timestamp like Joda's ISODateTimeFormat.dateTime() in UTC
// (yyyy-MM-ddTHH:mm:ss.SSSZ). It accepts time.Time, *time.Time or epoch-ms.
func isoUTC(v any) string {
	const layout = "2006-01-02T15:04:05.000Z07:00"
	switch t := v.(type) {
	case time.Time:
		return t.UTC().Format(layout)
	case *time.Time:
		if t == nil {
			return ""
		}
		return t.UTC().Format(layout)
	case int64:
		return time.UnixMilli(t).UTC().Format(layout)
	default:
		return ""
	}
}
