package app

import (
	"context"
	"errors"
	"fmt"

	"migrated-app/internal/core"
)

// UserByUsernameFinder is the lookup dependency of DuplicatedUsernameValidator.
// core.UserRepository and RegisterUserFinder both satisfy it. Implementations
// return an error matching core.ErrNotFound when no user has the username.
type UserByUsernameFinder interface {
	// FindByUsername returns the user registered with username.
	FindByUsername(ctx context.Context, username string) (*core.User, error)
}

// Compile-time checks.
var (
	_ UserByUsernameFinder = (core.UserRepository)(nil)
	_ UserByUsernameFinder = (RegisterUserFinder)(nil)
)

// ErrNilUserByUsernameFinder is returned by NewDuplicatedUsernameValidator
// when no finder is supplied.
var ErrNilUserByUsernameFinder = errors.New("app: nil UserByUsernameFinder")

// DuplicatedUsernameValidator rejects a username that already belongs to a
// registered user. It replaces the source's Bean Validation
// ConstraintValidator behind @DuplicatedUsernameConstraint.
//
// MIGRATION_NOTE: the source received its UserRepository through @Autowired
// field injection performed by Spring's constraint validator factory, and was
// invoked implicitly by @Valid. Go uses constructor injection and the caller
// invokes IsValid/ValidateUsername explicitly.
type DuplicatedUsernameValidator struct {
	users UserByUsernameFinder
}

// NewDuplicatedUsernameValidator builds a DuplicatedUsernameValidator backed
// by users (usually a core.UserRepository).
func NewDuplicatedUsernameValidator(users UserByUsernameFinder) (*DuplicatedUsernameValidator, error) {
	if users == nil {
		return nil, ErrNilUserByUsernameFinder
	}
	return &DuplicatedUsernameValidator{users: users}, nil
}

// IsValid reports whether username is acceptable. An empty username is valid
// (the "can't be empty" constraint covers it, mirroring the source's
// null/empty-tolerant validator); otherwise it is valid only when no user is
// registered with that username. Lookup failures other than core.ErrNotFound
// are returned as errors.
func (v *DuplicatedUsernameValidator) IsValid(ctx context.Context, username string) (bool, error) {
	if username == "" {
		return true, nil
	}
	found, err := exists(v.users.FindByUsername(ctx, username))
	if err != nil {
		return false, fmt.Errorf("check duplicated username: %w", err)
	}
	return !found, nil
}

// ValidateUsername returns a *RegisterValidationError carrying
// "duplicated username" on the "username" field when username is already
// taken, nil when it is acceptable, or a wrapped lookup error.
func (v *DuplicatedUsernameValidator) ValidateUsername(ctx context.Context, username string) error {
	ok, err := v.IsValid(ctx, username)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	verr := &RegisterValidationError{}
	verr.add("username", msgDuplicatedUsername)
	return verr
}
