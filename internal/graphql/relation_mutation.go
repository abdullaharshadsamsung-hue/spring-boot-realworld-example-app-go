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

// Schema coordinates resolved by RelationMutation. They replace the DGS
// codegen constants MUTATION.TYPE_NAME, MUTATION.FollowUser and
// MUTATION.UnfollowUser.
const (
	MutationType              = "Mutation"
	MutationFieldFollowUser   = "followUser"
	MutationFieldUnfollowUser = "unfollowUser"
)

// RelationRepository is the write-side dependency of RelationMutation.
// core.UserRepository satisfies it.
type RelationRepository interface {
	// FindByUsername returns the user or an error matching core.ErrNotFound.
	FindByUsername(ctx context.Context, username string) (*core.User, error)
	// SaveRelation persists a follow relation.
	SaveRelation(ctx context.Context, r *core.FollowRelation) error
	// FindRelation returns the relation or an error matching core.ErrNotFound.
	FindRelation(ctx context.Context, userID, targetID string) (*core.FollowRelation, error)
	// RemoveRelation deletes the relation.
	RemoveRelation(ctx context.Context, r *core.FollowRelation) error
}

// Compile-time checks that the real collaborators satisfy the ports.
var (
	_ RelationRepository = (core.UserRepository)(nil)
	_ ProfileFinder      = (*app.ProfileQueryService)(nil)
)

// ErrNilRelationRepository is returned by NewRelationMutation when no
// RelationRepository is given.
var ErrNilRelationRepository = errors.New("graphql: relation repository is nil")

// RelationMutation resolves Mutation.followUser and Mutation.unfollowUser.
//
// MIGRATION_NOTE: the source is a DGS @DgsComponent served on POST /graphql.
// No GraphQL executor is wired in this project yet; the composition root must
// build this with NewRelationMutation and register FollowUser/UnfollowUser
// for the fields above with the executor mounted at "POST /graphql" (behind
// the Authenticate middleware so the current user is on the context). Errors
// returned here must be passed through CustomizeExceptionHandler so they
// appear in the GraphQL "errors" array with HTTP 200.
type RelationMutation struct {
	users    RelationRepository
	profiles ProfileFinder
}

// NewRelationMutation creates a RelationMutation.
func NewRelationMutation(users RelationRepository, profiles ProfileFinder) (*RelationMutation, error) {
	if users == nil {
		return nil, ErrNilRelationRepository
	}
	if profiles == nil {
		return nil, ErrNilProfileFinder
	}
	return &RelationMutation{users: users, profiles: profiles}, nil
}

// FollowUser resolves Mutation.followUser(username). It requires an
// authenticated user (core.ErrUnauthenticated otherwise), returns
// httpapi.ErrResourceNotFound for an unknown target, saves the relation and
// returns the target's profile as seen by the current user.
func (m *RelationMutation) FollowUser(ctx context.Context, username string) (*ProfilePayload, error) {
	user, err := requireCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	target, err := m.users.FindByUsername(ctx, username)
	if err != nil {
		return nil, notFoundOrWrap(err, "followUser: find target")
	}
	if err := m.users.SaveRelation(ctx, core.NewFollowRelation(user.ID, target.ID)); err != nil {
		return nil, fmt.Errorf("followUser: save relation: %w", err)
	}
	return m.buildPayload(ctx, username, user)
}

// UnfollowUser resolves Mutation.unfollowUser(username). It requires an
// authenticated user and returns httpapi.ErrResourceNotFound when the target
// or the follow relation is missing. Otherwise it removes the relation and
// returns the target's profile.
func (m *RelationMutation) UnfollowUser(ctx context.Context, username string) (*ProfilePayload, error) {
	user, err := requireCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	target, err := m.users.FindByUsername(ctx, username)
	if err != nil {
		return nil, notFoundOrWrap(err, "unfollowUser: find target")
	}
	relation, err := m.users.FindRelation(ctx, user.ID, target.ID)
	if err != nil {
		return nil, notFoundOrWrap(err, "unfollowUser: find relation")
	}
	if err := m.users.RemoveRelation(ctx, relation); err != nil {
		return nil, fmt.Errorf("unfollowUser: remove relation: %w", err)
	}
	return m.buildPayload(ctx, username, user)
}

// buildPayload loads username's profile as seen by current.
//
// MIGRATION_NOTE: the source called Optional.get(), which throws
// NoSuchElementException if the user vanished concurrently; here that race
// returns httpapi.ErrResourceNotFound instead.
func (m *RelationMutation) buildPayload(ctx context.Context, username string, current *core.User) (*ProfilePayload, error) {
	pd, found, err := m.profiles.FindByUsername(ctx, username, current)
	if err != nil {
		return nil, fmt.Errorf("load profile %q: %w", username, err)
	}
	if !found {
		return nil, httpapi.ErrResourceNotFound
	}
	return &ProfilePayload{Profile: &Profile{
		Username:  pd.Username,
		Bio:       pd.Bio,
		Image:     pd.Image,
		Following: pd.Following,
	}}, nil
}

// requireCurrentUser returns the authenticated user or core.ErrUnauthenticated.
func requireCurrentUser(ctx context.Context) (*core.User, error) {
	u, ok := auth.UserFrom(ctx)
	if !ok || u == nil {
		return nil, core.ErrUnauthenticated
	}
	return u, nil
}

// notFoundOrWrap maps core.ErrNotFound to httpapi.ErrResourceNotFound and
// wraps any other error with op.
func notFoundOrWrap(err error, op string) error {
	if errors.Is(err, core.ErrNotFound) {
		return httpapi.ErrResourceNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}
