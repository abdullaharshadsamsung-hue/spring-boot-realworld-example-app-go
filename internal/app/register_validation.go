package app

import (
	"context"
	"errors"
	"sort"
	"strings"

	"migrated-app/internal/core"
)

// RegisterUserFinder is the lookup dependency used by registration
// validation. core.UserRepository satisfies it. Implementations return an
// error matching core.ErrNotFound when no matching user exists.
type RegisterUserFinder interface {
	// FindByEmail returns the user registered with email.
	FindByEmail(ctx context.Context, email string) (*core.User, error)
	// FindByUsername returns the user registered with username.
	FindByUsername(ctx context.Context, username string) (*core.User, error)
}

var _ RegisterUserFinder = (core.UserRepository)(nil)

// RegisterValidationError carries field-level validation failures for user
// registration, keyed by JSON field name.
type RegisterValidationError struct {
	Fields map[string][]string
}

// Error implements error.
func (e *RegisterValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+strings.Join(e.Fields[k], ", "))
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// FieldErrors exposes the failures so the HTTP layer renders a 422.
func (e *RegisterValidationError) FieldErrors() map[string][]string {
	return e.Fields
}

func (e *RegisterValidationError) add(field, msg string) {
	if e.Fields == nil {
		e.Fields = make(map[string][]string)
	}
	e.Fields[field] = append(e.Fields[field], msg)
}

// exists converts a repository lookup result into a found flag. A
// core.ErrNotFound error (or a nil user) means not found.
func exists(u *core.User, err error) (bool, error) {
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return u != nil, nil
}

// isBlank reports whether s is empty or contains only whitespace
// (Bean Validation's @NotBlank).
func isBlank(s string) bool {
	return strings.TrimSpace(s) == ""
}
