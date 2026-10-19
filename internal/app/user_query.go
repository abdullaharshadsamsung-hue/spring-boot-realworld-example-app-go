package app

import (
	"context"
	"errors"
	"fmt"
)

// UserByIDReader is the read-side port for looking up a user query model by
// its ID. It complements UserReadService (which covers username lookups);
// the store-layer PostgreSQL implementation of the source's MyBatis
// UserReadService is expected to satisfy both interfaces.
type UserByIDReader interface {
	// FindByID returns the user with the given ID. The bool is false (with a
	// nil error) when no such user exists.
	FindByID(ctx context.Context, id string) (UserData, bool, error)
}

// ErrNilUserByIDReader is returned by NewUserQueryService when no
// UserByIDReader is supplied.
var ErrNilUserByIDReader = errors.New("app: user by-id reader is nil")

// UserQueryService fetches user read models (UserData).
//
// MIGRATION_NOTE: the source is a Spring @Service singleton with Lombok
// constructor injection; here it is constructed explicitly in the
// composition root via NewUserQueryService.
type UserQueryService struct {
	users UserByIDReader
}

// NewUserQueryService creates a UserQueryService backed by users.
func NewUserQueryService(users UserByIDReader) (*UserQueryService, error) {
	if users == nil {
		return nil, ErrNilUserByIDReader
	}
	return &UserQueryService{users: users}, nil
}

// FindByID returns the read model of the user with the given ID. The bool
// result is false (with a nil error) when no such user exists, replacing the
// source's Optional.ofNullable(...) / Optional.empty().
func (s *UserQueryService) FindByID(ctx context.Context, id string) (UserData, bool, error) {
	u, found, err := s.users.FindByID(ctx, id)
	if err != nil {
		return UserData{}, false, fmt.Errorf("app: find user by id %q: %w", id, err)
	}
	if !found {
		return UserData{}, false, nil
	}
	return u, true, nil
}
