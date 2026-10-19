package app

import (
	"context"
	"errors"
	"fmt"

	"migrated-app/internal/core"
)

// UserReadService is the read-side port for looking up user query models.
// It replaces the source's MyBatis @Mapper UserReadService; the PostgreSQL
// implementation lives in the store layer.
type UserReadService interface {
	// FindByUsername returns the user with the given username. The bool is
	// false (with a nil error) when no such user exists.
	FindByUsername(ctx context.Context, username string) (UserData, bool, error)
}

// UserRelationshipQueryService is the read-side port answering follow
// relationship questions. It replaces the source's MyBatis @Mapper
// UserRelationshipQueryService.
type UserRelationshipQueryService interface {
	// IsUserFollowing reports whether the user userID follows anotherUserID.
	IsUserFollowing(ctx context.Context, userID, anotherUserID string) (bool, error)
}

// ErrNilUserReadService is returned by NewProfileQueryService when no
// UserReadService is supplied.
var ErrNilUserReadService = errors.New("app: user read service is nil")

// ErrNilUserRelationshipQueryService is returned by NewProfileQueryService
// when no UserRelationshipQueryService is supplied.
var ErrNilUserRelationshipQueryService = errors.New("app: user relationship query service is nil")

// ProfileQueryService builds the public ProfileData view of a user as seen
// by an optional current (authenticated) user.
//
// MIGRATION_NOTE: the source is a Spring @Component singleton; here it is
// constructed explicitly in the composition root. Like the source, it runs
// no transaction: each read is an independent query.
type ProfileQueryService struct {
	users     UserReadService
	relations UserRelationshipQueryService
}

// NewProfileQueryService creates a ProfileQueryService from its read ports.
func NewProfileQueryService(users UserReadService, relations UserRelationshipQueryService) (*ProfileQueryService, error) {
	if users == nil {
		return nil, ErrNilUserReadService
	}
	if relations == nil {
		return nil, ErrNilUserRelationshipQueryService
	}
	return &ProfileQueryService{users: users, relations: relations}, nil
}

// FindByUsername returns the profile of the user named username as seen by
// current. The bool result is false (with a nil error) when no user has that
// username, replacing the source's Optional.empty().
//
// current may be nil for an anonymous caller; in that case Following is
// false and the relationship store is not queried (the source's
// short-circuit &&).
func (s *ProfileQueryService) FindByUsername(ctx context.Context, username string, current *core.User) (ProfileData, bool, error) {
	u, found, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		return ProfileData{}, false, fmt.Errorf("app: find user by username %q: %w", username, err)
	}
	if !found {
		return ProfileData{}, false, nil
	}

	following := false
	if current != nil {
		following, err = s.relations.IsUserFollowing(ctx, current.ID, u.ID)
		if err != nil {
			return ProfileData{}, false, fmt.Errorf("app: check whether user %s follows %s: %w", current.ID, u.ID, err)
		}
	}

	return NewProfileData(u.ID, u.Username, u.Bio, u.Image, following), true, nil
}
