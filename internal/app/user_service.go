package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"sort"
	"strings"

	"migrated-app/internal/core"
)

// Validation messages copied from the source annotations so the wire output
// stays compatible with the Spring implementation.
const (
	msgCantBeEmpty       = "can't be empty"
	msgShouldBeEmail     = "should be an email"
	msgDuplicatedEmail   = "duplicated email"
	msgDuplicatedUsernam = "duplicated username"
)

// RegisterParam is the payload of POST /users. On the wire it is wrapped
// under a "user" root key: {"user": {"email": ..., "username": ..., "password": ...}}.
type RegisterParam struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// registerParamEnvelope replaces Jackson's @JsonRootName("user") together
// with UNWRAP_ROOT_VALUE.
type registerParamEnvelope struct {
	User RegisterParam `json:"user"`
}

// NewRegisterParam builds a RegisterParam from its fields.
func NewRegisterParam(email, username, password string) RegisterParam {
	return RegisterParam{Email: email, Username: username, Password: password}
}

// DecodeRegisterParam reads a root-wrapped registration payload from r.
//
// A missing "user" key yields a zero RegisterParam, which then fails
// validation with "can't be empty" on every field, matching the source.
func DecodeRegisterParam(r io.Reader) (RegisterParam, error) {
	var env registerParamEnvelope
	if err := json.NewDecoder(r).Decode(&env); err != nil {
		return RegisterParam{}, fmt.Errorf("decode register param: %w", err)
	}
	return env.User, nil
}

// RegisterValidationError carries field-level validation failures, keyed by
// JSON field name. It renders as {"errors": {"email": ["..."]}} in the HTTP
// layer.
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

func (e *RegisterValidationError) add(field, msg string) {
	if e.Fields == nil {
		e.Fields = make(map[string][]string)
	}
	e.Fields[field] = append(e.Fields[field], msg)
}

// RegisterUserFinder is the lookup dependency of registration validation
// (replacing DuplicatedEmailValidator / DuplicatedUsernameValidator).
// Implementations return an error matching core.ErrNotFound when no user exists.
type RegisterUserFinder interface {
	// FindByEmail returns the user with the given email.
	FindByEmail(ctx context.Context, email string) (*core.User, error)
	// FindByUsername returns the user with the given username.
	FindByUsername(ctx context.Context, username string) (*core.User, error)
}

// isBlank mirrors @NotBlank: empty or only whitespace.
func isBlank(s string) bool {
	return strings.TrimSpace(s) == ""
}

// isWellFormedEmail approximates Hibernate's @Email. Empty values are
// considered valid (blankness is reported by @NotBlank separately).
//
// MIGRATION_NOTE: Hibernate's regex and net/mail differ on exotic edge cases;
// manual review if strict parity matters.
func isWellFormedEmail(s string) bool {
	if s == "" {
		return true
	}
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return false
	}
	return addr.Address == s && strings.Contains(s, "@")
}

// exists reports whether a lookup found a user, distinguishing not-found from
// real failures.
func exists(u *core.User, err error) (bool, error) {
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return u != nil, nil
}

// Validate checks p like the source Bean Validation constraints: no field may
// be blank, email must be well-formed, and email/username must not already be
// taken. It returns *RegisterValidationError for constraint violations and a
// wrapped error for lookup failures.
func (p RegisterParam) Validate(ctx context.Context, finder RegisterUserFinder) error {
	if finder == nil {
		return errors.New("validate register param: nil user finder")
	}
	verr := &RegisterValidationError{}

	if isBlank(p.Email) {
		verr.add("email", msgCantBeEmpty)
	}
	if !isWellFormedEmail(p.Email) {
		verr.add("email", msgShouldBeEmail)
	}
	// DuplicatedEmailValidator: null/empty values pass.
	if p.Email != "" {
		found, err := exists(finder.FindByEmail(ctx, p.Email))
		if err != nil {
			return fmt.Errorf("validate register param: find by email: %w", err)
		}
		if found {
			verr.add("email", msgDuplicatedEmail)
		}
	}

	if isBlank(p.Username) {
		verr.add("username", msgCantBeEmpty)
	}
	// DuplicatedUsernameValidator: null/empty values pass.
	if p.Username != "" {
		found, err := exists(finder.FindByUsername(ctx, p.Username))
		if err != nil {
			return fmt.Errorf("validate register param: find by username: %w", err)
		}
		if found {
			verr.add("username", msgDuplicatedUsernam)
		}
	}

	if isBlank(p.Password) {
		verr.add("password", msgCantBeEmpty)
	}

	if len(verr.Fields) > 0 {
		return verr
	}
	return nil
}

// UserRegistrationRepository is the persistence dependency of UserService.
type UserRegistrationRepository interface {
	RegisterUserFinder
	// Save persists the user.
	Save(ctx context.Context, user *core.User) error
}

// PasswordHasher hashes plaintext passwords before storage (Q1 fix).
type PasswordHasher interface {
	// Hash returns the encoded password.
	Hash(plain string) (string, error)
}

// UserService registers new users (source: io.spring.application.user.UserService).
type UserService struct {
	repo         UserRegistrationRepository
	hasher       PasswordHasher
	defaultImage string
}

// NewUserService builds a UserService. defaultImage replaces the source's
// ${image.default} property.
func NewUserService(repo UserRegistrationRepository, hasher PasswordHasher, defaultImage string) (*UserService, error) {
	if repo == nil {
		return nil, errors.New("new user service: nil repository")
	}
	if hasher == nil {
		return nil, errors.New("new user service: nil password hasher")
	}
	return &UserService{repo: repo, hasher: hasher, defaultImage: defaultImage}, nil
}

// CreateUser validates p and persists a new user with an empty bio and the
// default image. Validation failures are returned as *RegisterValidationError.
func (s *UserService) CreateUser(ctx context.Context, p RegisterParam) (*core.User, error) {
	if err := p.Validate(ctx, s.repo); err != nil {
		return nil, err
	}
	hashed, err := s.hasher.Hash(p.Password)
	if err != nil {
		return nil, fmt.Errorf("create user: hash password: %w", err)
	}
	// MIGRATION_NOTE: assumes core.NewUser(email, username, password, bio, image)
	// mirrors the Java User constructor; verify against internal/core/user.go.
	user := core.NewUser(p.Email, p.Username, hashed, "", s.defaultImage)
	if err := s.repo.Save(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: save: %w", err)
	}
	return user, nil
}
