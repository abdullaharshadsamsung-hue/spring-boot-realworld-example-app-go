package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"migrated-app/internal/app"
	"migrated-app/internal/core"
)

// UserRegistrar creates new users. *app.UserService satisfies it. Validation
// failures are returned as one or more (possibly joined) *app.FieldError values.
type UserRegistrar interface {
	// CreateUser validates the input and persists a new user.
	CreateUser(ctx context.Context, email, username, password string) (*core.User, error)
}

// UserByEmailFinder looks users up by email. core.UserRepository satisfies it.
type UserByEmailFinder interface {
	// FindByEmail returns the user or an error matching core.ErrNotFound.
	FindByEmail(ctx context.Context, email string) (*core.User, error)
}

// UserDataLoader is the read-side lookup used to build the response body.
// The bool is false when no user has the given id.
type UserDataLoader interface {
	// FindByID returns the read model of the user with id.
	FindByID(ctx context.Context, id string) (app.UserData, bool, error)
}

// PasswordMatcher verifies a raw password against its stored hash. It
// replaces Spring Security's PasswordEncoder.matches.
type PasswordMatcher interface {
	// Matches reports whether raw hashes to encoded.
	Matches(raw, encoded string) bool
}

// TokenIssuer issues JWTs for users. It replaces JwtService.toToken.
type TokenIssuer interface {
	// ToToken returns a signed token for user.
	ToToken(user *core.User) (string, error)
}

// Constructor errors for NewUsersHandler.
var (
	ErrNilUserRegistrar   = errors.New("httpapi: user registrar is nil")
	ErrNilUserEmailFinder = errors.New("httpapi: user email finder is nil")
	ErrNilUserDataLoader  = errors.New("httpapi: user data loader is nil")
	ErrNilPasswordMatcher = errors.New("httpapi: password matcher is nil")
	ErrNilTokenIssuer     = errors.New("httpapi: token issuer is nil")
)

// UsersHandler serves POST /users and POST /users/login. It replaces the
// source's UsersApi @RestController.
type UsersHandler struct {
	registrar UserRegistrar
	users     UserByEmailFinder
	queries   UserDataLoader
	passwords PasswordMatcher
	tokens    TokenIssuer
}

// NewUsersHandler creates a UsersHandler; every dependency is required.
func NewUsersHandler(registrar UserRegistrar, users UserByEmailFinder, queries UserDataLoader, passwords PasswordMatcher, tokens TokenIssuer) (*UsersHandler, error) {
	switch {
	case registrar == nil:
		return nil, ErrNilUserRegistrar
	case users == nil:
		return nil, ErrNilUserEmailFinder
	case queries == nil:
		return nil, ErrNilUserDataLoader
	case passwords == nil:
		return nil, ErrNilPasswordMatcher
	case tokens == nil:
		return nil, ErrNilTokenIssuer
	}
	return &UsersHandler{registrar: registrar, users: users, queries: queries, passwords: passwords, tokens: tokens}, nil
}

// Register mounts the handler's routes on mux:
//
//	POST /users       → CreateUser (201)
//	POST /users/login → Login      (200)
func (h *UsersHandler) Register(mux *http.ServeMux) {
	mux.Handle("POST /users", ErrorHandlerFunc(h.CreateUser))
	mux.Handle("POST /users/login", ErrorHandlerFunc(h.Login))
}

// usersRegisterParam replaces RegisterParam (root name "user").
type usersRegisterParam struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// usersLoginParam replaces LoginParam (root name "user").
type usersLoginParam struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// usersWithToken is the wire shape of UserWithToken: the user's fields plus token.
type usersWithToken struct {
	app.UserData
	Token string `json:"token"`
}

// usersResponse is the {"user": {...}} envelope.
type usersResponse struct {
	User usersWithToken `json:"user"`
}

// usersEmailPattern approximates Hibernate's lenient @Email; "" counts as valid.
var usersEmailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+$`)

const (
	usersMsgCantBeEmpty     = "can't be empty"
	usersMsgShouldBeAnEmail = "should be an email"
)

// decodeUserRoot decodes {"user": {...}} into dst.
// MIGRATION_NOTE: Jackson's UNWRAP_ROOT_VALUE would reject a body without the
// "user" root; here a missing root decodes to empty fields and fails validation (422).
func decodeUserRoot(r *http.Request, dst any) error {
	var envelope struct {
		User json.RawMessage `json:"user"`
	}
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	if len(envelope.User) == 0 || string(envelope.User) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.User, dst); err != nil {
		return fmt.Errorf("decode user: %w", err)
	}
	return nil
}

// CreateUser registers a user and writes 201 {"user": {..., "token"}}.
func (h *UsersHandler) CreateUser(w http.ResponseWriter, r *http.Request) error {
	var param usersRegisterParam
	if err := decodeUserRoot(r, &param); err != nil {
		return err
	}
	user, err := h.registrar.CreateUser(r.Context(), param.Email, param.Username, param.Password)
	if err != nil {
		if fe := collectAppFieldErrors(err); fe.HasErrors() {
			return NewInvalidRequestError(fe)
		}
		return fmt.Errorf("create user: %w", err)
	}
	return h.writeUser(w, r, http.StatusCreated, user)
}

// Login authenticates by email/password and writes 200 {"user": {..., "token"}}.
// Unknown email or wrong password yields 422 {"message":"invalid email or password"}.
func (h *UsersHandler) Login(w http.ResponseWriter, r *http.Request) error {
	var param usersLoginParam
	if err := decodeUserRoot(r, &param); err != nil {
		return err
	}
	if errs := validateLoginParam(param); errs.HasErrors() {
		return NewInvalidRequestError(errs)
	}
	user, err := h.users.FindByEmail(r.Context(), param.Email)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return ErrInvalidAuthentication
		}
		return fmt.Errorf("login: find user by email: %w", err)
	}
	if user == nil || !h.passwords.Matches(param.Password, user.Password) {
		return ErrInvalidAuthentication
	}
	return h.writeUser(w, r, http.StatusOK, user)
}

// validateLoginParam applies LoginParam's @NotBlank/@Email constraints.
func validateLoginParam(p usersLoginParam) FieldErrors {
	errs := FieldErrors{}
	if strings.TrimSpace(p.Email) == "" {
		errs.Add("email", usersMsgCantBeEmpty)
	}
	if p.Email != "" && !usersEmailPattern.MatchString(p.Email) {
		errs.Add("email", usersMsgShouldBeAnEmail)
	}
	if strings.TrimSpace(p.Password) == "" {
		errs.Add("password", usersMsgCantBeEmpty)
	}
	return errs
}

// writeUser loads the read model, issues a token and writes the envelope.
// MIGRATION_NOTE: the source called Optional.get() (500 on absence); a missing
// read model is reported as an internal error here as well.
func (h *UsersHandler) writeUser(w http.ResponseWriter, r *http.Request, status int, user *core.User) error {
	data, found, err := h.queries.FindByID(r.Context(), user.ID)
	if err != nil {
		return fmt.Errorf("load user %s: %w", user.ID, err)
	}
	if !found {
		return fmt.Errorf("load user %s: read model missing after write", user.ID)
	}
	token, err := h.tokens.ToToken(user)
	if err != nil {
		return fmt.Errorf("issue token: %w", err)
	}
	writeJSON(w, r, status, usersResponse{User: usersWithToken{UserData: data, Token: token}})
	return nil
}

// collectAppFieldErrors flattens (possibly joined) *app.FieldError values.
func collectAppFieldErrors(err error) FieldErrors {
	out := FieldErrors{}
	var walk func(error)
	walk = func(e error) {
		if e == nil {
			return
		}
		var fe *app.FieldError
		if errors.As(e, &fe) && fe != nil {
			if multi, ok := e.(interface{ Unwrap() []error }); ok {
				for _, inner := range multi.Unwrap() {
					walk(inner)
				}
				return
			}
			out.Add(fe.Field, fe.Message)
			return
		}
		if multi, ok := e.(interface{ Unwrap() []error }); ok {
			for _, inner := range multi.Unwrap() {
				walk(inner)
			}
		}
	}
	walk(err)
	return out
}
