package graphql

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/core"
	"migrated-app/internal/httpapi"
)

// Schema coordinates resolved by MeDatafetcher. They replace the DGS codegen
// constants QUERY.Me, USERPAYLOAD.TYPE_NAME and USERPAYLOAD.User.
const (
	QueryType            = "Query"
	QueryFieldMe         = "me"
	UserPayloadType      = "UserPayload"
	UserPayloadFieldUser = "user"
)

// UserByIDFinder is the read-side dependency of MeDatafetcher.
// *app.UserQueryService satisfies it.
type UserByIDFinder interface {
	// FindByID returns the user read model. The bool is false when no such
	// user exists.
	FindByID(ctx context.Context, id string) (app.UserData, bool, error)
}

// Compile-time proof that the collaborators satisfy the ports.
var (
	_ UserByIDFinder = (*app.UserQueryService)(nil)
)

// Errors returned by NewMeDatafetcher and the resolvers.
var (
	// ErrNilUserByIDFinder is returned when no UserByIDFinder is given.
	ErrNilUserByIDFinder = errors.New("graphql: user query service is nil")
	// ErrNilMeTokenService is returned when no core.TokenService is given.
	ErrNilMeTokenService = errors.New("graphql: token service is nil")
	// ErrMalformedAuthorization is returned by Me when the Authorization
	// header has no "<scheme> <token>" form. The source threw
	// ArrayIndexOutOfBoundsException in that case.
	ErrMalformedAuthorization = errors.New("graphql: malformed Authorization header")
)

// User is the GraphQL User type (schema fields email, username, token; the
// profile field is resolved separately by ProfileDatafetcher.UserProfile).
type User struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

// UserResult replaces DGS's DataFetcherResult<User>: Data is the value sent
// to the client and LocalContext is the domain user handed to child
// resolvers such as User.profile.
type UserResult struct {
	Data         *User
	LocalContext *core.User
}

// MeDatafetcher resolves Query.me and UserPayload.user.
//
// MIGRATION_NOTE: the source is a DGS @DgsComponent. No GraphQL executor is
// wired yet; the composition root must build this with NewMeDatafetcher and
// register Me / UserPayloadUser for the coordinates above with the executor
// mounted at "POST /graphql" (behind the Authenticate middleware so the
// current user is on the context). The executor must pass the raw
// Authorization header into Me and propagate UserResult.LocalContext to
// child resolvers (User.profile).
type MeDatafetcher struct {
	users  UserByIDFinder
	tokens core.TokenService
}

// NewMeDatafetcher creates a MeDatafetcher.
func NewMeDatafetcher(users UserByIDFinder, tokens core.TokenService) (*MeDatafetcher, error) {
	if users == nil {
		return nil, ErrNilUserByIDFinder
	}
	if tokens == nil {
		return nil, ErrNilMeTokenService
	}
	return &MeDatafetcher{users: users, tokens: tokens}, nil
}

// Me resolves Query.me. It returns (nil, nil) for anonymous requests, which
// GraphQL renders as null. Otherwise it loads the current user's read model
// (httpapi.ErrResourceNotFound if missing) and returns its email and
// username together with the token taken verbatim from the Authorization
// header (the second space-separated part). The domain user becomes the
// local context.
func (d *MeDatafetcher) Me(ctx context.Context, authorization string) (*UserResult, error) {
	current, ok := auth.UserFrom(ctx)
	if !ok || current == nil {
		return nil, nil
	}
	userData, found, err := d.users.FindByID(ctx, current.ID)
	if err != nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", QueryType, QueryFieldMe, err)
	}
	if !found {
		return nil, httpapi.ErrResourceNotFound
	}
	// Mirrors Java's authorization.split(" ")[1].
	parts := strings.Split(authorization, " ")
	if len(parts) < 2 {
		return nil, ErrMalformedAuthorization
	}
	uwt := app.NewUserWithToken(userData, parts[1])
	return &UserResult{
		Data: &User{
			Email:    uwt.Email(),
			Username: uwt.Username(),
			Token:    uwt.Token(),
		},
		LocalContext: current,
	}, nil
}

// UserPayloadUser resolves UserPayload.user for createUser, login and
// updateUser. user is the domain user the parent mutation placed in the
// local context; a fresh JWT is issued for it.
func (d *MeDatafetcher) UserPayloadUser(ctx context.Context, user *core.User) (*UserResult, error) {
	if user == nil {
		return nil, fmt.Errorf("resolve %s.%s: %w", UserPayloadType, UserPayloadFieldUser, ErrMissingLocalContext)
	}
	token, err := d.tokens.ToToken(user)
	if err != nil {
		return nil, fmt.Errorf("resolve %s.%s: issue token: %w", UserPayloadType, UserPayloadFieldUser, err)
	}
	return &UserResult{
		Data: &User{
			Email:    user.Email,
			Username: user.Username,
			Token:    token,
		},
		LocalContext: user,
	}, nil
}
