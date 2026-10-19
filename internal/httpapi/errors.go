package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
)

// invalidAuthenticationMessage is the fixed message for every failed login,
// copied from the source exception. It never says whether the email or the
// password was wrong, so callers cannot use it to find out which emails are
// registered.
const invalidAuthenticationMessage = "invalid email or password"

// ErrInvalidAuthentication signals a failed authentication attempt, such as
// an unknown email or a wrong password. Its message is always
// "invalid email or password".
//
// Services and handlers may return it directly or wrap it, for example
// fmt.Errorf("login %s: %w", email, httpapi.ErrInvalidAuthentication).
// Any error for which errors.Is(err, ErrInvalidAuthentication) holds is
// rendered as 422 Unprocessable Entity with the body
// {"message":"invalid email or password"}.
//
// MIGRATION_NOTE: the source used an unchecked InvalidAuthenticationException
// that CustomizeExceptionHandler turned into a 422 response. In Go it is a
// returned sentinel error, mapped to HTTP by ErrorHandlerFunc and WriteError.
// If a core.ErrInvalidCredentials sentinel also exists, MANUAL REVIEW: alias
// one to the other, or add it to the errors.Is check in
// invalidAuthenticationBody, so only one sentinel is in use.
var ErrInvalidAuthentication = errors.New(invalidAuthenticationMessage)

// ErrResourceNotFound signals that a requested resource (article, comment,
// profile, ...) does not exist. Any error for which
// errors.Is(err, ErrResourceNotFound) holds is rendered as 404 Not Found.
var ErrResourceNotFound = errors.New("resource not found")

// ErrorHandlerFunc is an HTTP handler that returns an error instead of
// writing error responses itself. Its ServeHTTP method maps known sentinel
// errors to the matching HTTP response, in place of Spring's
// @ExceptionHandler / @ResponseStatus handling.
type ErrorHandlerFunc func(w http.ResponseWriter, r *http.Request) error

// ServeHTTP implements http.Handler. It calls f and, if f returns an error,
// writes the matching error response with WriteError.
func (f ErrorHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := f(w, r); err != nil {
		WriteError(w, r, err)
	}
}

// WriteError writes the HTTP response for err:
//
//   - ErrInvalidAuthentication → 422 {"message":"invalid email or password"}
//   - ErrNoAuthorization       → 403 (delegated to WriteAuthzError)
//   - ErrResourceNotFound      → 404 with an empty body
//   - anything else            → 500 with a generic message, so internal
//     details are never exposed
//
// The request is passed through so that delegated writers such as
// WriteAuthzError have access to it. A nil err writes nothing.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	if body, ok := invalidAuthenticationBody(err); ok {
		writeJSONError(w, http.StatusUnprocessableEntity, body)
		return
	}
	switch {
	case errors.Is(err, ErrNoAuthorization):
		WriteAuthzError(w, r, err)
	case errors.Is(err, ErrResourceNotFound):
		w.WriteHeader(http.StatusNotFound)
	default:
		writeJSONError(w, http.StatusInternalServerError,
			map[string]string{"message": http.StatusText(http.StatusInternalServerError)})
	}
}

// invalidAuthenticationBody reports whether err is a failed-authentication
// error. If it is, it also returns the response body for it. The message is
// always the fixed string, even when the error was wrapped with extra
// context, so the response never leaks which credential was wrong.
func invalidAuthenticationBody(err error) (map[string]string, bool) {
	if !errors.Is(err, ErrInvalidAuthentication) {
		return nil, false
	}
	return map[string]string{"message": invalidAuthenticationMessage}, true
}

// writeJSONError writes body as JSON with the given status code. If encoding
// fails after the header has been sent, the status cannot be changed, so the
// failure is dropped.
func writeJSONError(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
