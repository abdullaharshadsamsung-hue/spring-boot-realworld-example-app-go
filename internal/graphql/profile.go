package graphql

import (
	"context"
	"errors"
	"fmt"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
	"migrated-app/internal/httpapi"
)

// Schema coordinates resolved by ProfileDatafetcher. They replace the DGS
// codegen constants USER.Profile, ARTICLE.Author, COMMENT.Author and
// QUERY.Profile.
const (
	UserType            = "User"
	ArticleType         = "Article"
	CommentType         = "Comment"
	QueryFieldProfile   = "profile"
	UserFieldProfile    = "profile"
	ArticleFieldAuthor  = "author"
	CommentFieldAuthor  = "author"
	profileArgUsername  = "username"
)

// ProfileFinder is the read-side dependency of ProfileDatafetcher. It is
// satisfied by *app.ProfileQueryService.
type ProfileFinder interface {
	// FindByUsername returns the profile of username as seen by current
	// (nil for anonymous). The bool is false when no such user exists.
	FindByUsername(ctx context.Context, username string, current *core.User) (app.ProfileData, bool, error)
}

// Compile-time proof that the application service satisfies ProfileFinder.
var _ ProfileFinder = (*app.ProfileQueryService)(nil)

// ErrNilProfileFinder is returned by NewProfileDatafetcher when no
// ProfileFinder is given.
var ErrNilProfileFinder = errors.New("graphql: profile query service is nil")

// ErrMissingLocalContext is returned when a nested resolver cannot find the
// parent's data in the local context. In the source this was a
// NullPointerException.
var ErrMissingLocalContext = errors.New("graphql: parent data missing from local context")

// Profile is the GraphQL Profile type.
type Profile struct {
	Username  string `json:"username"`
	Bio       string `json:"bio"`
	Image     string `json:"image"`
	Following bool   `json:"following"`
}

// ProfilePayload is the GraphQL ProfilePayload type returned by
// Query.profile.
type ProfilePayload struct {
	Profile *Profile `json:"profile"`
}

// AuthorLookup maps a parent key (for example a comment id) to its author's
// username. ok is false when the key is unknown.
type AuthorLookup func(key string) (username string, ok bool)

// ProfileDatafetcher resolves Query.profile, User.profile, Article.author
// and Comment.author.
//
// MIGRATION_NOTE: the source is a DGS @DgsComponent registered by classpath
// scanning. The composition root must build it with NewProfileDatafetcher and
// register QueryProfile, UserProfile, ArticleAuthor and CommentAuthor with
// the GraphQL executor serving schema.graphqls (no executor is wired yet).
type ProfileDatafetcher struct {
	profiles ProfileFinder
}

// NewProfileDatafetcher creates a ProfileDatafetcher backed by profiles
// (normally *app.ProfileQueryService).
func NewProfileDatafetcher(profiles ProfileFinder) (*ProfileDatafetcher, error) {
	if profiles == nil {
		return nil, ErrNilProfileFinder
	}
	return &ProfileDatafetcher{profiles: profiles}, nil
}

// QueryProfile resolves Query.profile(username) and wraps the result in a
// ProfilePayload. An unknown username returns httpapi.ErrResourceNotFound.
func (d *ProfileDatafetcher) QueryProfile(ctx context.Context, username string) (*ProfilePayload, error) {
	p, err := d.queryProfile(ctx, username)
	if err != nil {
		return nil, err
	}
	return &ProfilePayload{Profile: p}, nil
}

// UserProfile resolves User.profile. user is the domain user that the parent
// resolver put in the local context.
func (d *ProfileDatafetcher) UserProfile(ctx context.Context, user *core.User) (*Profile, error) {
	if user == nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", UserType, UserFieldProfile, ErrMissingLocalContext)
	}
	return d.queryProfile(ctx, user.Username)
}

// ArticleAuthor resolves Article.author. local is the parent's local context,
// which maps article slug to ArticleData. slug is the source article's slug.
func (d *ProfileDatafetcher) ArticleAuthor(ctx context.Context, slug string, local map[string]app.ArticleData) (*Profile, error) {
	a, ok := local[slug]
	if !ok || a.Author == nil {
		return nil, fmt.Errorf("resolve %s.%s for slug %q: %w", ArticleType, ArticleFieldAuthor, slug, ErrMissingLocalContext)
	}
	return d.queryProfile(ctx, a.Author.Username)
}

// CommentAuthor resolves Comment.author. commentID is the source comment's
// id. lookup reads the author's username from the parent's local context,
// which maps comment id to CommentData.
//
// MIGRATION_NOTE: the caller builds lookup from its map[string]app.CommentData
// (returning the entry's profile username), so this file does not depend on
// CommentData's field layout.
func (d *ProfileDatafetcher) CommentAuthor(ctx context.Context, commentID string, lookup AuthorLookup) (*Profile, error) {
	if lookup == nil {
		return nil, fmt.Errorf("resolve %s.%s for id %q: %w", CommentType, CommentFieldAuthor, commentID, ErrMissingLocalContext)
	}
	username, ok := lookup(commentID)
	if !ok {
		return nil, fmt.Errorf("resolve %s.%s for id %q: %w", CommentType, CommentFieldAuthor, commentID, ErrMissingLocalContext)
	}
	return d.queryProfile(ctx, username)
}

// queryProfile looks up username as seen by the authenticated user (if any)
// and maps it to the GraphQL Profile type.
func (d *ProfileDatafetcher) queryProfile(ctx context.Context, username string) (*Profile, error) {
	current, _ := auth.UserFrom(ctx)
	pd, found, err := d.profiles.FindByUsername(ctx, username, current)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, httpapi.ErrResourceNotFound
	}
	return &Profile{
		Username:  pd.Username,
		Bio:       pd.Bio,
		Image:     pd.Image,
		Following: pd.Following,
	}, nil
}
