package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"migrated-app/internal/core"
)

// ErrFeedRequiresUser is returned by the feed queries when no viewer is
// supplied. A feed only exists for an authenticated user.
//
// MIGRATION_NOTE: the source would throw a NullPointerException on
// user.getId(); Go returns this sentinel instead (agreed fix Q6/Q7).
var ErrFeedRequiresUser = errors.New("article feed requires an authenticated user")

// ArticleReader is the article read-model data source (the source's
// MyBatis ArticleReadService).
type ArticleReader interface {
	// FindByID returns the article; ok is false when it does not exist.
	FindByID(ctx context.Context, id string) (ArticleData, bool, error)
	// FindBySlug returns the article; ok is false when it does not exist.
	FindBySlug(ctx context.Context, slug string) (ArticleData, bool, error)
	// QueryArticles returns article IDs matching the optional filters (empty
	// string means "no filter"), newest first, for the given offset page.
	QueryArticles(ctx context.Context, tag, author, favoritedBy string, page Page) ([]string, error)
	// CountArticle counts articles matching the optional filters.
	CountArticle(ctx context.Context, tag, author, favoritedBy string) (int, error)
	// FindArticles loads the articles for ids, preserving the order of ids.
	FindArticles(ctx context.Context, ids []string) ([]ArticleData, error)
	// FindArticlesOfAuthors returns the offset page of articles by authors.
	FindArticlesOfAuthors(ctx context.Context, authorIDs []string, page Page) ([]ArticleData, error)
	// CountFeedSize counts articles written by authors.
	CountFeedSize(ctx context.Context, authorIDs []string) (int, error)
	// FindArticlesWithCursor returns up to page.QueryLimit() article IDs
	// matching the filters, ordered for the page direction.
	FindArticlesWithCursor(ctx context.Context, tag, author, favoritedBy string, page CursorPageParameter) ([]string, error)
	// FindArticlesOfAuthorsWithCursor returns up to page.QueryLimit()
	// articles by authors, ordered for the page direction.
	FindArticlesOfAuthorsWithCursor(ctx context.Context, authorIDs []string, page CursorPageParameter) ([]ArticleData, error)
}

// UserRelationshipReader answers follow-relation questions (the source's
// UserRelationshipQueryService).
type UserRelationshipReader interface {
	// IsUserFollowing reports whether userID follows anotherUserID.
	IsUserFollowing(ctx context.Context, userID, anotherUserID string) (bool, error)
	// FollowingAuthors returns the subset of authorIDs that userID follows.
	FollowingAuthors(ctx context.Context, userID string, authorIDs []string) (map[string]struct{}, error)
	// FollowedUsers returns the IDs of every user userID follows.
	FollowedUsers(ctx context.Context, userID string) ([]string, error)
}

// ArticleFavoritesReader answers favorite questions (the source's
// ArticleFavoritesReadService).
type ArticleFavoritesReader interface {
	// IsUserFavorite reports whether userID favorited articleID.
	IsUserFavorite(ctx context.Context, userID, articleID string) (bool, error)
	// ArticleFavoriteCount returns the number of favorites of articleID.
	ArticleFavoriteCount(ctx context.Context, articleID string) (int, error)
	// ArticlesFavoriteCount returns favorite counts keyed by article ID.
	// Articles without favorites may be absent from the map.
	ArticlesFavoriteCount(ctx context.Context, articleIDs []string) (map[string]int, error)
	// UserFavorites returns the subset of articleIDs favorited by user.
	UserFavorites(ctx context.Context, articleIDs []string, user *core.User) (map[string]struct{}, error)
}

// ArticleNode adapts ArticleData to the Node interface so it can be used in a
// CursorPager. It embeds ArticleData, so JSON output and fields are unchanged.
//
// MIGRATION_NOTE: ArticleData.Cursor returns a DateTimeCursor while Node
// requires Cursor() string; this wrapper bridges the two.
type ArticleNode struct {
	ArticleData
}

// Cursor returns the serialized (epoch-ms) cursor of the article.
func (n ArticleNode) Cursor() string {
	return n.ArticleData.Cursor().String()
}

// ArticleQueryService is the read side of the article feature: it loads
// article read models and enriches them with viewer-specific data.
type ArticleQueryService struct {
	articles  ArticleReader
	relations UserRelationshipReader
	favorites ArticleFavoritesReader
}

// NewArticleQueryService builds an ArticleQueryService.
func NewArticleQueryService(articles ArticleReader, relations UserRelationshipReader, favorites ArticleFavoritesReader) *ArticleQueryService {
	return &ArticleQueryService{articles: articles, relations: relations, favorites: favorites}
}

// FindByID loads the article with id. When user is non-nil, the favorited
// flag, favorites count and author-following flag are filled. ok is false
// when the article does not exist.
func (s *ArticleQueryService) FindByID(ctx context.Context, id string, user *core.User) (ArticleData, bool, error) {
	data, ok, err := s.articles.FindByID(ctx, id)
	if err != nil {
		return ArticleData{}, false, fmt.Errorf("find article by id %q: %w", id, err)
	}
	if !ok {
		return ArticleData{}, false, nil
	}
	if user != nil {
		if err := s.fillSingle(ctx, id, user, &data); err != nil {
			return ArticleData{}, false, err
		}
	}
	return data, true, nil
}

// FindBySlug is FindByID keyed by slug; enrichment uses the found article's ID.
func (s *ArticleQueryService) FindBySlug(ctx context.Context, slug string, user *core.User) (ArticleData, bool, error) {
	data, ok, err := s.articles.FindBySlug(ctx, slug)
	if err != nil {
		return ArticleData{}, false, fmt.Errorf("find article by slug %q: %w", slug, err)
	}
	if !ok {
		return ArticleData{}, false, nil
	}
	if user != nil {
		if err := s.fillSingle(ctx, data.ID, user, &data); err != nil {
			return ArticleData{}, false, err
		}
	}
	return data, true, nil
}

// FindRecentArticlesWithCursor returns a cursor page of articles filtered by
// the optional tag, author and favoritedBy (empty means no filter).
// currentUser may be nil.
func (s *ArticleQueryService) FindRecentArticlesWithCursor(
	ctx context.Context, tag, author, favoritedBy string, page CursorPageParameter, currentUser *core.User,
) (*CursorPager[ArticleNode], error) {
	ids, err := s.articles.FindArticlesWithCursor(ctx, tag, author, favoritedBy, page)
	if err != nil {
		return nil, fmt.Errorf("query article ids with cursor: %w", err)
	}
	if len(ids) == 0 {
		return NewCursorPager([]ArticleNode{}, page.Direction(), false), nil
	}
	hasExtra := len(ids) > page.Limit()
	if hasExtra {
		ids = ids[:page.Limit()]
	}
	if !page.IsNext() {
		slices.Reverse(ids)
	}
	articles, err := s.articles.FindArticles(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load articles: %w", err)
	}
	if err := s.fillList(ctx, articles, currentUser); err != nil {
		return nil, err
	}
	return NewCursorPager(toNodes(articles), page.Direction(), hasExtra), nil
}

// FindUserFeedWithCursor returns a cursor page of articles written by users
// that user follows. user must be non-nil.
func (s *ArticleQueryService) FindUserFeedWithCursor(ctx context.Context, user *core.User, page CursorPageParameter) (*CursorPager[ArticleNode], error) {
	if user == nil {
		return nil, ErrFeedRequiresUser
	}
	followed, err := s.relations.FollowedUsers(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("load followed users: %w", err)
	}
	if len(followed) == 0 {
		return NewCursorPager([]ArticleNode{}, page.Direction(), false), nil
	}
	articles, err := s.articles.FindArticlesOfAuthorsWithCursor(ctx, followed, page)
	if err != nil {
		return nil, fmt.Errorf("load feed articles with cursor: %w", err)
	}
	hasExtra := len(articles) > page.Limit()
	if hasExtra {
		articles = articles[:page.Limit()]
	}
	if !page.IsNext() {
		slices.Reverse(articles)
	}
	if err := s.fillList(ctx, articles, user); err != nil {
		return nil, err
	}
	return NewCursorPager(toNodes(articles), page.Direction(), hasExtra), nil
}

// FindRecentArticles returns an offset page of articles matching the
// optional filters, newest first, with the total matching count.
// currentUser may be nil.
func (s *ArticleQueryService) FindRecentArticles(
	ctx context.Context, tag, author, favoritedBy string, page Page, currentUser *core.User,
) (ArticleDataList, error) {
	ids, err := s.articles.QueryArticles(ctx, tag, author, favoritedBy, page)
	if err != nil {
		return ArticleDataList{}, fmt.Errorf("query article ids: %w", err)
	}
	count, err := s.articles.CountArticle(ctx, tag, author, favoritedBy)
	if err != nil {
		return ArticleDataList{}, fmt.Errorf("count articles: %w", err)
	}
	if len(ids) == 0 {
		return NewArticleDataList([]ArticleData{}, count), nil
	}
	articles, err := s.articles.FindArticles(ctx, ids)
	if err != nil {
		return ArticleDataList{}, fmt.Errorf("load articles: %w", err)
	}
	if err := s.fillList(ctx, articles, currentUser); err != nil {
		return ArticleDataList{}, err
	}
	return NewArticleDataList(articles, count), nil
}

// FindUserFeed returns an offset page of articles by authors that user
// follows, with the total feed size. user must be non-nil.
func (s *ArticleQueryService) FindUserFeed(ctx context.Context, user *core.User, page Page) (ArticleDataList, error) {
	if user == nil {
		return ArticleDataList{}, ErrFeedRequiresUser
	}
	followed, err := s.relations.FollowedUsers(ctx, user.ID)
	if err != nil {
		return ArticleDataList{}, fmt.Errorf("load followed users: %w", err)
	}
	if len(followed) == 0 {
		return NewArticleDataList([]ArticleData{}, 0), nil
	}
	articles, err := s.articles.FindArticlesOfAuthors(ctx, followed, page)
	if err != nil {
		return ArticleDataList{}, fmt.Errorf("load feed articles: %w", err)
	}
	if err := s.fillList(ctx, articles, user); err != nil {
		return ArticleDataList{}, err
	}
	count, err := s.articles.CountFeedSize(ctx, followed)
	if err != nil {
		return ArticleDataList{}, fmt.Errorf("count feed: %w", err)
	}
	return NewArticleDataList(articles, count), nil
}

// fillList sets favorite counts on every article, and the favorited and
// following flags when currentUser is non-nil.
func (s *ArticleQueryService) fillList(ctx context.Context, articles []ArticleData, currentUser *core.User) error {
	if len(articles) == 0 {
		return nil
	}
	ids := make([]string, 0, len(articles))
	for _, a := range articles {
		ids = append(ids, a.ID)
	}

	counts, err := s.favorites.ArticlesFavoriteCount(ctx, ids)
	if err != nil {
		return fmt.Errorf("load favorite counts: %w", err)
	}
	for i := range articles {
		// Missing entries mean zero favorites.
		// MIGRATION_NOTE: the source would unbox a null Integer here (NPE);
		// Go uses 0.
		articles[i].FavoritesCount = counts[articles[i].ID]
	}

	if currentUser == nil {
		return nil
	}

	favorited, err := s.favorites.UserFavorites(ctx, ids, currentUser)
	if err != nil {
		return fmt.Errorf("load user favorites: %w", err)
	}
	for i := range articles {
		if _, ok := favorited[articles[i].ID]; ok {
			articles[i].Favorited = true
		}
	}

	authorIDs := make([]string, 0, len(articles))
	for _, a := range articles {
		if a.Author != nil {
			authorIDs = append(authorIDs, a.Author.ID)
		}
	}
	following, err := s.relations.FollowingAuthors(ctx, currentUser.ID, authorIDs)
	if err != nil {
		return fmt.Errorf("load following authors: %w", err)
	}
	for i := range articles {
		if articles[i].Author == nil {
			continue
		}
		if _, ok := following[articles[i].Author.ID]; ok {
			articles[i].Author.Following = true
		}
	}
	return nil
}

// fillSingle fills favorited, favorites count and author following for one
// article as seen by user (non-nil).
func (s *ArticleQueryService) fillSingle(ctx context.Context, id string, user *core.User, data *ArticleData) error {
	fav, err := s.favorites.IsUserFavorite(ctx, user.ID, id)
	if err != nil {
		return fmt.Errorf("check user favorite: %w", err)
	}
	data.Favorited = fav

	count, err := s.favorites.ArticleFavoriteCount(ctx, id)
	if err != nil {
		return fmt.Errorf("count favorites: %w", err)
	}
	data.FavoritesCount = count

	if data.Author != nil {
		following, err := s.relations.IsUserFollowing(ctx, user.ID, data.Author.ID)
		if err != nil {
			return fmt.Errorf("check following: %w", err)
		}
		data.Author.Following = following
	}
	return nil
}

func toNodes(articles []ArticleData) []ArticleNode {
	nodes := make([]ArticleNode, 0, len(articles))
	for _, a := range articles {
		nodes = append(nodes, ArticleNode{ArticleData: a})
	}
	return nodes
}
