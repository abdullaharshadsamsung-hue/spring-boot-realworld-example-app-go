package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
)

// ProfileQuerier is the read-side dependency of ProfilesHandler.
// *app.ProfileQueryService satisfies it.
type ProfileQuerier interface {
	// FindByUsername returns the profile of username as seen by current
	// (nil for anonymous). The bool is false when no such user exists.
	FindByUsername(ctx context.Context, username string, current *core.User) (app.ProfileData, bool, error)
}

// FollowRepository is the write-side dependency of ProfilesHandler.
// core.UserRepository satisfies it.
type FollowRepository interface {
	// FindByUsername returns the user or an error matching core.ErrNotFound.
	FindByUsername(ctx context.Context, username string) (*core.User, error)
	// SaveRelation idempotently persists a follow relation.
	SaveRelation(ctx context.Context, r *core.FollowRelation) error
	// FindRelation returns the relation or an error matching core.ErrNotFound.
	FindRelation(ctx context.Context, userID, targetID string) (*core.FollowRelation, error)
	// RemoveRelation deletes the relation.
	RemoveRelation(ctx context.Context, r *core.FollowRelation) error
}

// Compile-time checks that the real collaborators satisfy the ports.
var (
	_ ProfileQuerier   = (*app.ProfileQueryService)(nil)
	_ FollowRepository = (core.UserRepository)(nil)
)

// ErrNilProfileQuerier is returned by NewProfilesHandler when no ProfileQuerier is supplied.
var ErrNilProfileQuerier = errors.New("httpapi: profile querier is nil")

// ErrNilFollowRepository is returned by NewProfilesHandler when no FollowRepository is supplied.
var ErrNilFollowRepository = errors.New("httpapi: follow repository is nil")

// ProfilesHandler serves the /profiles/{username} endpoints. It replaces the
// source's ProfileApi @RestController.
type ProfilesHandler struct {
	profiles ProfileQuerier
	users    FollowRepository
}

// NewProfilesHandler creates a ProfilesHandler.
func NewProfilesHandler(profiles ProfileQuerier, users FollowRepository) (*ProfilesHandler, error) {
	if profiles == nil {
		return nil, ErrNilProfileQuerier
	}
	if users == nil {
		return nil, ErrNilFollowRepository
	}
	return &ProfilesHandler{profiles: profiles, users: users}, nil
}

// Register mounts the handler's routes on mux:
//
//	GET    /profiles/{username}        → GetProfile (auth optional)
//	POST   /profiles/{username}/follow → Follow     (auth required)
//	DELETE /profiles/{username}/follow → Unfollow   (auth required)
//
// The Authenticate middleware must wrap mux so the current user is on the
// request context.
func (h *ProfilesHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /profiles/{username}", ErrorHandlerFunc(h.GetProfile))
	mux.Handle("POST /profiles/{username}/follow", requireUser(h.Follow))
	mux.Handle("DELETE /profiles/{username}/follow", requireUser(h.Unfollow))
}

// requireUser rejects anonymous requests with 401 and an empty body,
// mirroring Spring Security's HttpStatusEntryPoint(UNAUTHORIZED).
func requireUser(next ErrorHandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := auth.UserFrom(r.Context()); !ok || u == nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// profileResponse is the wire shape {"profile": {...}}.
type profileResponse struct {
	Profile app.ProfileData `json:"profile"`
}

// currentUser returns the authenticated user or nil.
func currentUser(r *http.Request) *core.User {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		return nil
	}
	return u
}

// GetProfile writes 200 {"profile":...} or 404 when the user does not exist.
func (h *ProfilesHandler) GetProfile(w http.ResponseWriter, r *http.Request) error {
	username := r.PathValue("username")
	return h.writeProfile(w, r, username, currentUser(r))
}

// Follow makes the current user follow {username}; 404 if the target is missing.
func (h *ProfilesHandler) Follow(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	user := currentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}
	username := r.PathValue("username")
	target, err := h.users.FindByUsername(ctx, username)
	if err != nil {
		return notFoundOr(err, "follow: find target")
	}
	if err := h.users.SaveRelation(ctx, core.NewFollowRelation(user.ID, target.ID)); err != nil {
		return fmt.Errorf("follow: save relation: %w", err)
	}
	return h.writeProfile(w, r, username, user)
}

// Unfollow removes the current user's follow relation to {username}; 404 if
// the target or the relation is missing.
func (h *ProfilesHandler) Unfollow(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	user := currentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}
	username := r.PathValue("username")
	target, err := h.users.FindByUsername(ctx, username)
	if err != nil {
		return notFoundOr(err, "unfollow: find target")
	}
	relation, err := h.users.FindRelation(ctx, user.ID, target.ID)
	if err != nil {
		return notFoundOr(err, "unfollow: find relation")
	}
	if err := h.users.RemoveRelation(ctx, relation); err != nil {
		return fmt.Errorf("unfollow: remove relation: %w", err)
	}
	return h.writeProfile(w, r, username, user)
}

// writeProfile loads the profile from the viewer's perspective and writes it.
// MIGRATION_NOTE: the source called Optional.get() after follow/unfollow,
// which would throw (500) if the user vanished concurrently; here that race
// yields 404 instead.
func (h *ProfilesHandler) writeProfile(w http.ResponseWriter, r *http.Request, username string, viewer *core.User) error {
	profile, found, err := h.profiles.FindByUsername(r.Context(), username, viewer)
	if err != nil {
		return fmt.Errorf("profile %q: %w", username, err)
	}
	if !found {
		return ErrResourceNotFound
	}
	writeErrorJSON(w, http.StatusOK, profileResponse{Profile: profile})
	return nil
}

// notFoundOr maps core.ErrNotFound to ErrResourceNotFound (404) and wraps
// any other error (500).
func notFoundOr(err error, op string) error {
	if errors.Is(err, core.ErrNotFound) {
		return ErrResourceNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}
