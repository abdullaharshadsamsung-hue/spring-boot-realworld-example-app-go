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

// Schema coordinates resolved by UserMutation. They replace the DGS codegen
// constants MUTATION.CreateUser, MUTATION.Login and MUTATION.UpdateUser.
const (
	MutationFieldCreateUser = "createUser"
	MutationFieldLogin      = "login"
	MutationFieldUpdateUser = "updateUser"
)

// UserMutationService is the application service used by UserMutation.
// *app.UserService satisfies it.
type UserMutationService interface {
	// CreateUser validates and registers a new user. Validation failures are
	// returned as (joined) *app.FieldError values.
	CreateUser(ctx context.Context, email, username, password string) (*core.User, error)
	// UpdateUser validates the command and applies it to its target user in place.
	UpdateUser(ctx context.Context, cmd *app.UpdateUserCommand) error
}

var _ UserMutationService = (*app.UserService)(nil)

// UserMutationUsers is the user lookup used by login.
// core.UserRepository satisfies it.
type UserMutationUsers interface {
	// FindByEmail returns the user or an error matching core.ErrNotFound.
	FindByEmail(ctx context.Context, email string) (*core.User, error)
}

// PasswordMatcher checks a raw password against a stored hash. It replaces
// Spring Security's PasswordEncoder.matches.
type PasswordMatcher interface {
	// Matches reports whether raw corresponds to encoded.
	Matches(raw, encoded string) bool
}

// Errors returned by NewUserMutation for missing dependencies.
var (
	ErrNilUserMutationUsers   = errors.New("graphql: user mutation user repository is nil")
	ErrNilUserMutationMatcher = errors.New("graphql: user mutation password matcher is nil")
	ErrNilUserMutationService = errors.New("graphql: user mutation user service is nil")
)

// CreateUserInput is the GraphQL CreateUserInput type.
type CreateUserInput struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// UpdateUserInput is the GraphQL UpdateUserInput type. Nil fields are left
// unchanged (core.User.Update ignores empty values).
type UpdateUserInput struct {
	Email    *string `json:"email"`
	Username *string `json:"username"`
	Password *string `json:"password"`
	Image    *string `json:"image"`
	Bio      *string `json:"bio"`
}

// UserPayloadResult replaces DataFetcherResult<UserPayload>. The payload has
// no fields of its own; its "user" field is resolved by
// MeDatafetcher.UserPayloadUser from LocalContext.
type UserPayloadResult struct {
	LocalContext *core.User
}

// CreateUserResult is the GraphQL union UserResult = UserPayload | Error.
// Exactly one of Payload and Error is non-nil.
type CreateUserResult struct {
	Payload *UserPayloadResult
	Error   *Error
}

// TypeName returns the concrete union member name for __typename resolution.
func (r *CreateUserResult) TypeName() string {
	if r != nil && r.Error != nil {
		return "Error"
	}
	return UserPayloadType
}

// UserMutation resolves Mutation.createUser, Mutation.login and
// Mutation.updateUser.
//
// MIGRATION_NOTE: the source is a DGS @DgsComponent. The composition root must
// build it with NewUserMutation and register its methods with the GraphQL
// executor at "POST /graphql" (behind the auth middleware so the current user
// is on the context), passing UserPayloadResult.LocalContext to
// MeDatafetcher.UserPayloadUser. Errors returned from Login/UpdateUser must be
// routed through CustomizeExceptionHandler.
type UserMutation struct {
	users    UserMutationUsers
	matcher  PasswordMatcher
	services UserMutationService
}

// NewUserMutation creates a UserMutation.
func NewUserMutation(users UserMutationUsers, matcher PasswordMatcher, services UserMutationService) (*UserMutation, error) {
	if users == nil {
		return nil, ErrNilUserMutationUsers
	}
	if matcher == nil {
		return nil, ErrNilUserMutationMatcher
	}
	if services == nil {
		return nil, ErrNilUserMutationService
	}
	return &UserMutation{users: users, matcher: matcher, services: services}, nil
}

// CreateUser resolves Mutation.createUser(input). Validation failures are
// returned as data (the Error union member, message "BAD_REQUEST"), not as
// a GraphQL error. Any other failure is returned as an error.
func (m *UserMutation) CreateUser(ctx context.Context, input CreateUserInput) (*CreateUserResult, error) {
	user, err := m.services.CreateUser(ctx, input.Email, input.Username, input.Password)
	if err != nil {
		if cve, ok := fieldErrorsToViolations(err, "createUser.registerParam"); ok {
			data := ErrorsAsData(cve)
			return &CreateUserResult{Error: &data}, nil
		}
		return nil, fmt.Errorf("%s: %w", MutationFieldCreateUser, err)
	}
	return &CreateUserResult{Payload: &UserPayloadResult{LocalContext: user}}, nil
}

// Login resolves Mutation.login(email, password). An unknown email or a
// password mismatch yields httpapi.ErrInvalidAuthentication, which the
// exception handler renders as an UNAUTHENTICATED error.
func (m *UserMutation) Login(ctx context.Context, email, password string) (*UserPayloadResult, error) {
	user, err := m.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, httpapi.ErrInvalidAuthentication
		}
		return nil, fmt.Errorf("%s: find user: %w", MutationFieldLogin, err)
	}
	if user == nil || !m.matcher.Matches(password, user.Password) {
		return nil, httpapi.ErrInvalidAuthentication
	}
	return &UserPayloadResult{LocalContext: user}, nil
}

// UpdateUser resolves Mutation.updateUser(changes). Anonymous requests get
// (nil, nil), rendered as a silent null. Validation failures are returned as
// *ConstraintViolationError so the exception handler emits a BAD_REQUEST
// GraphQL error. The current user is updated in place and returned as the
// local context.
func (m *UserMutation) UpdateUser(ctx context.Context, changes UpdateUserInput) (*UserPayloadResult, error) {
	current, ok := auth.UserFrom(ctx)
	if !ok || current == nil {
		return nil, nil
	}
	param := app.NewUpdateUserParamBuilder().
		Username(deref(changes.Username)).
		Email(deref(changes.Email)).
		Bio(deref(changes.Bio)).
		Password(deref(changes.Password)).
		Image(deref(changes.Image)).
		Build()

	if err := m.services.UpdateUser(ctx, app.NewUpdateUserCommand(current, param)); err != nil {
		if cve, ok := fieldErrorsToViolations(err, "updateUser.command"); ok {
			return nil, cve
		}
		return nil, fmt.Errorf("%s: %w", MutationFieldUpdateUser, err)
	}
	return &UserPayloadResult{LocalContext: current}, nil
}

// fieldErrorsToViolations converts the *app.FieldError values contained in
// err (possibly joined via errors.Join) into a ConstraintViolationError. The
// prefix supplies the "method.argument" segments that fieldParam strips. It
// returns false when err contains no field errors, or also contains non-field
// errors (which must surface as unexpected failures).
func fieldErrorsToViolations(err error, prefix string) (*ConstraintViolationError, bool) {
	var leaves []error
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		leaves = joined.Unwrap()
	} else {
		leaves = []error{err}
	}
	violations := make([]Violation, 0, len(leaves))
	for _, leaf := range leaves {
		var fe *app.FieldError
		if !errors.As(leaf, &fe) {
			return nil, false
		}
		violations = append(violations, Violation{
			PropertyPath: prefix + "." + fe.Field,
			Message:      fe.Message,
		})
	}
	if len(violations) == 0 {
		return nil, false
	}
	return NewConstraintViolationError(violations...), true
}
