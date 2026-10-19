package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"migrated-app/internal/app"
	"migrated-app/internal/core"
)

// CurrentUserUpdater applies profile changes to a user. *app.UserService
// satisfies it. Validation failures come back as one or more (possibly
// joined) *app.FieldError values.
type CurrentUserUpdater interface {
	// UpdateUser validates and persists the command's changes.
	UpdateUser(ctx context.Context, cmd *app.UpdateUserCommand) error
}

// CurrentUserResolver returns the authenticated user that the JWT middleware
// stored on the request context. It replaces @AuthenticationPrincipal. The
// bool is false when the request is unauthenticated.
type CurrentUserResolver func(ctx context.Context) (*core.User, bool)

// Constructor errors for NewCurrentUserHandler.
var (
	ErrNilCurrentUserUpdater  = errors.New("httpapi: current user updater is nil")
	ErrNilCurrentUserQueries  = errors.New("httpapi: current user data loader is nil")
	ErrNilCurrentUserResolver = errors.New("httpapi: current user resolver is nil")
)

// currentUserStatusError is an error that carries its own HTTP status, which
// WriteError picks up through the StatusCoder interface.
type currentUserStatusError struct {
	status int
	msg    string
}

// Error implements the error interface.
func (e *currentUserStatusError) Error() string { return e.msg }

// HTTPStatus implements StatusCoder.
func (e *currentUserStatusError) HTTPStatus() int { return e.status }

var _ StatusCoder = (*currentUserStatusError)(nil)

var (
	errCurrentUserUnauthenticated = &currentUserStatusError{status: http.StatusUnauthorized, msg: "unauthenticated"}
	errCurrentUserMissingHeader   = &currentUserStatusError{status: http.StatusBadRequest, msg: "Required request header 'Authorization' is not present"}
	errCurrentUserMissingRoot     = &currentUserStatusError{status: http.StatusBadRequest, msg: `request body must be wrapped in a "user" object`}
	errCurrentUserMalformedBody   = &currentUserStatusError{status: http.StatusBadRequest, msg: "malformed request body"}
)

// CurrentUserHandler serves GET /user and PUT /user. It replaces the
// source's CurrentUserApi @RestController.
type CurrentUserHandler struct {
	updater CurrentUserUpdater
	queries UserDataLoader
	current CurrentUserResolver
}

// NewCurrentUserHandler creates a CurrentUserHandler; every dependency is required.
func NewCurrentUserHandler(updater CurrentUserUpdater, queries UserDataLoader, current CurrentUserResolver) (*CurrentUserHandler, error) {
	switch {
	case updater == nil:
		return nil, ErrNilCurrentUserUpdater
	case queries == nil:
		return nil, ErrNilCurrentUserQueries
	case current == nil:
		return nil, ErrNilCurrentUserResolver
	}
	return &CurrentUserHandler{updater: updater, queries: queries, current: current}, nil
}

// Register mounts the handler's routes on mux:
//
//	GET /user → CurrentUser   (200)
//	PUT /user → UpdateProfile (200)
//
// Both routes require authentication; the JWT middleware must run first so
// the resolver can find the user.
func (h *CurrentUserHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /user", ErrorHandlerFunc(h.CurrentUser))
	mux.Handle("PUT /user", ErrorHandlerFunc(h.UpdateProfile))
}

// CurrentUser writes 200 {"user": {..., "token"}} for the authenticated user,
// echoing the token from the Authorization header.
func (h *CurrentUserHandler) CurrentUser(w http.ResponseWriter, r *http.Request) error {
	user, ok := h.current(r.Context())
	if !ok || user == nil {
		return errCurrentUserUnauthenticated
	}
	token, err := currentUserToken(r)
	if err != nil {
		return err
	}
	return h.writeCurrentUser(w, r, user.ID, token)
}

// UpdateProfile applies a partial update from {"user": {...}} and writes
// 200 {"user": {..., "token"}} carrying the same token the client sent.
func (h *CurrentUserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) error {
	user, ok := h.current(r.Context())
	if !ok || user == nil {
		return errCurrentUserUnauthenticated
	}
	token, err := currentUserToken(r)
	if err != nil {
		return err
	}

	var param app.UpdateUserParam
	if err := json.NewDecoder(r.Body).Decode(&param); err != nil {
		if errors.Is(err, app.ErrMissingUserRoot) {
			return errCurrentUserMissingRoot
		}
		if isBadRequest(err) {
			return fmt.Errorf("update profile: decode body: %w", err)
		}
		return errCurrentUserMalformedBody
	}

	if err := h.updater.UpdateUser(r.Context(), app.NewUpdateUserCommand(user, &param)); err != nil {
		if fe := collectAppFieldErrors(err); fe.HasErrors() {
			return NewInvalidRequestError(fe)
		}
		return fmt.Errorf("update profile: %w", err)
	}
	return h.writeCurrentUser(w, r, user.ID, token)
}

// writeCurrentUser re-reads the read model and writes the response envelope.
// MIGRATION_NOTE: the source called Optional.get() (NoSuchElementException →
// 500); a missing read model is likewise reported as an internal error.
func (h *CurrentUserHandler) writeCurrentUser(w http.ResponseWriter, r *http.Request, userID, token string) error {
	data, found, err := h.queries.FindByID(r.Context(), userID)
	if err != nil {
		return fmt.Errorf("load current user %s: %w", userID, err)
	}
	if !found {
		return fmt.Errorf("load current user %s: user no longer exists", userID)
	}
	writeJSON(w, r, http.StatusOK, usersResponse{User: usersWithToken{UserData: data, Token: token}})
	return nil
}

// currentUserToken reproduces authorization.split(" ")[1]: the header is
// required (400 if absent) and must contain a space; otherwise the source
// threw ArrayIndexOutOfBoundsException, preserved here as a 500.
func currentUserToken(r *http.Request) (string, error) {
	header, present := r.Header["Authorization"]
	if !present || len(header) == 0 {
		return "", errCurrentUserMissingHeader
	}
	parts := strings.Split(header[0], " ")
	if len(parts) < 2 {
		return "", errors.New("authorization header has no token part")
	}
	return parts[1], nil
}
