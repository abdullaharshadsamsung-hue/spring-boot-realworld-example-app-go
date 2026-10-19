package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"migrated-app/internal/core"
)

// MIGRATION_NOTE: This file replaces io.spring.application.user.UserService
// together with the package-private UpdateUserConstraint/UpdateUserValidator
// pair. Spring's @Validated AOP proxy and Bean Validation have no Go
// equivalent. The checks run explicitly at the top of CreateUser and
// UpdateUser before any state is changed or persisted.
//
// MANUAL REVIEW: the signatures of core.UserRepository (FindByEmail,
// FindByUsername, Save), core.NewUser, (*core.User).Update and
// (*core.User).Equal are assumed from sibling files. Confirm them against
// internal/core/user.go.

// Validation messages shared with the original Bean Validation annotations.
const (
	msgCantBeEmpty           = "can't be empty"
	msgShouldBeAnEmail       = "should be an email"
	msgDuplicatedEmail       = "duplicated email"
	msgDuplicatedUsername    = "duplicated username"
	msgEmailAlreadyExist     = "email already exist"
	msgUsernameAlreadyExist  = "username already exist"
	userParamJSONRoot        = "user"
	userServiceFieldEmail    = "email"
	userServiceFieldUsername = "username"
	userServiceFieldPassword = "password"
)

// userServiceEmailPattern approximates Hibernate's lenient @Email check.
// Empty strings are handled separately and count as valid, as in Hibernate.
var userServiceEmailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+$`)

func isValidUserEmail(email string) bool {
	return email == "" || userServiceEmailPattern.MatchString(email)
}

// FieldError is a validation failure bound to a single input field. It
// replaces a Bean Validation ConstraintViolation with a property node.
type FieldError struct {
	// Field is the JSON name of the offending property.
	Field string
	// Message is the human-readable violation message.
	Message string
}

// Error implements the error interface.
func (e *FieldError) Error() string {
	return e.Field + " " + e.Message
}

// UpdateUserParam, its constructor, accessors and JSON (un)marshalling live
// in update_user_param.go.

// Validate applies the field-level constraints of UpdateUserParam. Only the
// email format is checked (@Email(message = "should be an email")).
func (p UpdateUserParam) Validate() error {
	if !isValidUserEmail(p.email) {
		return &FieldError{Field: userServiceFieldEmail, Message: msgShouldBeAnEmail}
	}
	return nil
}

// UpdateUserParamBuilder builds an UpdateUserParam fluently. It replaces
// Lombok's @Builder.
type UpdateUserParamBuilder struct {
	p UpdateUserParam
}

// NewUpdateUserParamBuilder returns an empty builder.
func NewUpdateUserParamBuilder() *UpdateUserParamBuilder {
	return &UpdateUserParamBuilder{}
}

// Email sets the email.
func (b *UpdateUserParamBuilder) Email(v string) *UpdateUserParamBuilder {
	b.p.email = v
	return b
}

// Password sets the raw password.
func (b *UpdateUserParamBuilder) Password(v string) *UpdateUserParamBuilder {
	b.p.password = v
	return b
}

// Username sets the username.
func (b *UpdateUserParamBuilder) Username(v string) *UpdateUserParamBuilder {
	b.p.username = v
	return b
}

// Bio sets the bio.
func (b *UpdateUserParamBuilder) Bio(v string) *UpdateUserParamBuilder {
	b.p.bio = v
	return b
}

// Image sets the image URL.
func (b *UpdateUserParamBuilder) Image(v string) *UpdateUserParamBuilder {
	b.p.image = v
	return b
}

// Build returns a copy of the accumulated param.
func (b *UpdateUserParamBuilder) Build() *UpdateUserParam {
	p := b.p
	return &p
}

// UpdateUserCommand pairs the user being updated with the requested changes.
type UpdateUserCommand struct {
	targetUser *core.User
	param      *UpdateUserParam
}

// NewUpdateUserCommand builds an UpdateUserCommand.
func NewUpdateUserCommand(targetUser *core.User, param *UpdateUserParam) *UpdateUserCommand {
	return &UpdateUserCommand{targetUser: targetUser, param: param}
}

// TargetUser returns the user being updated.
func (c *UpdateUserCommand) TargetUser() *core.User { return c.targetUser }

// Param returns the requested changes.
func (c *UpdateUserCommand) Param() *UpdateUserParam { return c.param }

// UserPasswordEncoder hashes raw passwords. It replaces Spring Security's
// PasswordEncoder. A bcrypt-backed implementation is wired in main.
type UserPasswordEncoder interface {
	// Encode returns the hash of raw.
	Encode(raw string) (string, error)
}

// Constructor errors for NewUserService.
var (
	ErrNilUserServiceRepository = errors.New("app: nil user repository")
	ErrNilUserServiceEncoder    = errors.New("app: nil password encoder")
)

// ErrNilUpdateUserCommand is returned by UpdateUser when the command, its
// target user or its param is nil.
var ErrNilUpdateUserCommand = errors.New("app: nil update user command")

// UserService registers and updates users.
type UserService struct {
	repo         core.UserRepository
	defaultImage string
	encoder      UserPasswordEncoder
}

// NewUserService builds a UserService. defaultImage is the configured
// image.default value applied to new users.
func NewUserService(repo core.UserRepository, defaultImage string, encoder UserPasswordEncoder) (*UserService, error) {
	if repo == nil {
		return nil, ErrNilUserServiceRepository
	}
	if encoder == nil {
		return nil, ErrNilUserServiceEncoder
	}
	return &UserService{repo: repo, defaultImage: defaultImage, encoder: encoder}, nil
}

// CreateUser validates the registration input, then creates and saves a new
// user. The password is hashed, bio is "" and image is the default image.
// Validation failures are returned as one or more joined *FieldError values.
func (s *UserService) CreateUser(ctx context.Context, email, username, password string) (*core.User, error) {
	if err := s.validateRegistration(ctx, email, username, password); err != nil {
		return nil, err
	}
	hashed, err := s.encoder.Encode(password)
	if err != nil {
		return nil, fmt.Errorf("app: hash password: %w", err)
	}
	user := core.NewUser(email, username, hashed, "", s.defaultImage)
	if err := s.repo.Save(ctx, user); err != nil {
		return nil, fmt.Errorf("app: save new user: %w", err)
	}
	return user, nil
}

func (s *UserService) validateRegistration(ctx context.Context, email, username, password string) error {
	var errs []error

	switch {
	case strings.TrimSpace(email) == "":
		errs = append(errs, &FieldError{Field: userServiceFieldEmail, Message: msgCantBeEmpty})
	case !isValidUserEmail(email):
		errs = append(errs, &FieldError{Field: userServiceFieldEmail, Message: msgShouldBeAnEmail})
	}
	if email != "" {
		_, found, err := s.lookupByEmail(ctx, email)
		if err != nil {
			return err
		}
		if found {
			errs = append(errs, &FieldError{Field: userServiceFieldEmail, Message: msgDuplicatedEmail})
		}
	}

	if strings.TrimSpace(username) == "" {
		errs = append(errs, &FieldError{Field: userServiceFieldUsername, Message: msgCantBeEmpty})
	}
	if username != "" {
		_, found, err := s.lookupByUsername(ctx, username)
		if err != nil {
			return err
		}
		if found {
			errs = append(errs, &FieldError{Field: userServiceFieldUsername, Message: msgDuplicatedUsername})
		}
	}

	if strings.TrimSpace(password) == "" {
		errs = append(errs, &FieldError{Field: userServiceFieldPassword, Message: msgCantBeEmpty})
	}
	return errors.Join(errs...)
}

// UpdateUser validates the command, including the email/username uniqueness
// rule of the original UpdateUserConstraint, then applies the changes to the
// target user and saves it.
//
// The new password is hashed before it is stored. This is the documented Q1
// fix: the Java source stored the raw value.
func (s *UserService) UpdateUser(ctx context.Context, cmd *UpdateUserCommand) error {
	if cmd == nil || cmd.TargetUser() == nil || cmd.Param() == nil {
		return ErrNilUpdateUserCommand
	}
	param := cmd.Param()
	if err := param.Validate(); err != nil {
		return err
	}
	if err := s.ValidateUpdateUser(ctx, cmd); err != nil {
		return err
	}

	password := param.Password()
	if password != "" {
		hashed, err := s.encoder.Encode(password)
		if err != nil {
			return fmt.Errorf("app: hash password: %w", err)
		}
		password = hashed
	}

	user := cmd.TargetUser()
	user.Update(param.Email(), param.Username(), password, param.Bio(), param.Image())
	if err := s.repo.Save(ctx, user); err != nil {
		return fmt.Errorf("app: save updated user: %w", err)
	}
	return nil
}

// ValidateUpdateUser reports whether the command's email and username are
// each unused or already owned by the target user. It replaces
// UpdateUserValidator. Conflicts are returned as joined *FieldError values
// ("email already exist" and/or "username already exist").
func (s *UserService) ValidateUpdateUser(ctx context.Context, cmd *UpdateUserCommand) error {
	if cmd == nil || cmd.TargetUser() == nil || cmd.Param() == nil {
		return ErrNilUpdateUserCommand
	}
	target := cmd.TargetUser()

	emailValid := true
	if existing, found, err := s.lookupByEmail(ctx, cmd.Param().Email()); err != nil {
		return err
	} else if found {
		emailValid = existing.Equal(target)
	}

	usernameValid := true
	if existing, found, err := s.lookupByUsername(ctx, cmd.Param().Username()); err != nil {
		return err
	} else if found {
		usernameValid = existing.Equal(target)
	}

	var errs []error
	if !emailValid {
		errs = append(errs, &FieldError{Field: userServiceFieldEmail, Message: msgEmailAlreadyExist})
	}
	if !usernameValid {
		errs = append(errs, &FieldError{Field: userServiceFieldUsername, Message: msgUsernameAlreadyExist})
	}
	return errors.Join(errs...)
}

func (s *UserService) lookupByEmail(ctx context.Context, email string) (*core.User, bool, error) {
	u, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("app: find user by email: %w", err)
	}
	if u == nil {
		return nil, false, nil
	}
	return u, true, nil
}

func (s *UserService) lookupByUsername(ctx context.Context, username string) (*core.User, bool, error) {
	u, err := s.repo.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("app: find user by username: %w", err)
	}
	if u == nil {
		return nil, false, nil
	}
	return u, true, nil
}
