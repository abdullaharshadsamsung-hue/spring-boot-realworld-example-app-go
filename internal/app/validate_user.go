package app

import (
	"context"
	"errors"
	"fmt"

	"migrated-app/internal/core"
)

// UserByEmailFinder is the lookup dependency of DuplicatedEmailValidator.
// core.UserRepository and RegisterUserFinder both satisfy it. Implementations
// return an error matching core.ErrNotFound when no user has the email.
type UserByEmailFinder interface {
	// FindByEmail returns the user registered with email.
	FindByEmail(ctx context.Context, email string) (*core.User, error)
}

// Compile-time checks.
var (
	_ UserByEmailFinder = (core.UserRepository)(nil)
	_ UserByEmailFinder = (RegisterUserFinder)(nil)
)

// ErrNilUserByEmailFinder is returned by NewDuplicatedEmailValidator when no
// finder is supplied.
var ErrNilUserByEmailFinder = errors.New("app: nil UserByEmailFinder")

// DuplicatedEmailValidator rejects an email that already belongs to a
// registered user. It replaces the source's Bean Validation
// ConstraintValidator behind @DuplicatedEmailConstraint.
//
// MIGRATION_NOTE: the source received its UserRepository through @Autowired
// field injection performed by Spring's constraint validator factory, and was
// invoked implicitly by @Valid. Go uses constructor injection and the caller
// invokes IsValid/ValidateEmail explicitly.
type DuplicatedEmailValidator struct {
	users UserByEmailFinder
}

// NewDuplicatedEmailValidator builds a DuplicatedEmailValidator backed by
// users (usually a core.UserRepository).
func NewDuplicatedEmailValidator(users UserByEmailFinder) (*DuplicatedEmailValidator, error) {
	if users == nil {
		return nil, ErrNilUserByEmailFinder
	}
	return &DuplicatedEmailValidator{users: users}, nil
}

// IsValid reports whether email is acceptable. An empty email is valid (other
// constraints such as "can't be empty" and "should be an email" cover it);
// otherwise it is valid only when no user is registered with that email.
// Lookup failures other than core.ErrNotFound are returned as errors.
func (v *DuplicatedEmailValidator) IsValid(ctx context.Context, email string) (bool, error) {
	if email == "" {
		return true, nil
	}
	found, err := exists(v.users.FindByEmail(ctx, email))
	if err != nil {
		return false, fmt.Errorf("check duplicated email: %w", err)
	}
	return !found, nil
}

// ValidateEmail returns a *RegisterValidationError carrying "duplicated email"
// on the "email" field when email is already registered, nil when it is
// acceptable, or a wrapped lookup error.
func (v *DuplicatedEmailValidator) ValidateEmail(ctx context.Context, email string) error {
	ok, err := v.IsValid(ctx, email)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	verr := &RegisterValidationError{}
	verr.add("email", msgDuplicatedEmail)
	return verr
}
