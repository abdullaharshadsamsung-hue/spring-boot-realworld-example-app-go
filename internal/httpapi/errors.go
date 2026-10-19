package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
)

// ErrResourceNotFound signals that the requested resource (article, comment,
// profile, ...) does not exist. Any error for which
// errors.Is(err, ErrResourceNotFound) holds is rendered as 404 Not Found with
// a Spring-style error body.
//
// It replaces the source's ResourceNotFoundException, which used
// @ResponseStatus(NOT_FOUND).
var ErrResourceNotFound = errors.New("resource not found")

// ErrInvalidAuthentication signals a failed login, such as an unknown email or
// a wrong password. It is rendered as 422 Unprocessable Entity with the body
// {"message":"invalid email or password"}, which matches the source's
// CustomizeExceptionHandler.handleInvalidAuthentication.
var ErrInvalidAuthentication = errors.New("invalid email or password")

// FieldErrors maps a request field name to the validation messages reported
// for it. It is the Go replacement for Spring Validation's Errors object
// (BindingResult, MapBindingResult, ...) as used by InvalidRequestException.
type FieldErrors map[string][]string

// Add records message as a validation failure for field.
// Messages are kept in the order they were added.
func (f FieldErrors) Add(field, message string) {
	f[field] = append(f[field], message)
}

// HasErrors reports whether at least one field failure has been recorded.
func (f FieldErrors) HasErrors() bool {
	return len(f) > 0
}

// FieldErrorer is implemented by any error that carries field-level
// validation failures. WriteError renders all such errors as
// 422 {"errors":{field:[messages]}}.
//
// Because this is an interface, validation errors from other packages (for
// example a *app.ValidationError) can be rendered without httpapi importing
// those packages. They only need to expose a FieldErrors() method.
type FieldErrorer interface {
	error
	FieldErrors() map[string][]string
}

// InvalidRequestError is returned when request input fails validation, for
// example a duplicated username or email or a blank required field. It wraps
// the collected field errors.
//
// It replaces the source's unchecked InvalidRequestException(Errors).
// Instead of throwing, Go code returns it as an ordinary error.
type InvalidRequestError struct {
	errs FieldErrors
}

// NewInvalidRequestError builds an InvalidRequestError that wraps errs.
// A nil map is replaced with an empty one, so Errors never returns nil.
func NewInvalidRequestError(errs FieldErrors) *InvalidRequestError {
	if errs == nil {
		errs = FieldErrors{}
	}
	return &InvalidRequestError{errs: errs}
}

// Error returns the error message. As in the source, which called super(""),
// the message is intentionally empty. The details are in Errors().
//
// MIGRATION_NOTE: an empty Error() string matches the source exactly, but it
// makes plain log lines uninformative. Callers that log this error should also
// log Errors(). Manual review: decide whether a non-empty message is preferable.
func (e *InvalidRequestError) Error() string {
	return ""
}

// Errors returns the field errors supplied at construction.
func (e *InvalidRequestError) Errors() FieldErrors {
	return e.errs
}

// FieldErrors implements FieldErrorer.
func (e *InvalidRequestError) FieldErrors() map[string][]string {
	return e.errs
}

// validationBody is the 422 response body for field validation failures.
type validationBody struct {
	Errors map[string][]string `json:"errors"`
}

// messageBody is the 422 response body for authentication failures.
type messageBody struct {
	Message string `json:"message"`
}

// WriteError renders err as an HTTP error response. It replaces the source's
// @RestControllerAdvice CustomizeExceptionHandler and Spring's
// @ResponseStatus resolution:
//
//   - FieldErrorer (including *InvalidRequestError) → 422 {"errors":{field:[msgs]}}
//   - ErrInvalidAuthentication                       → 422 {"message":"invalid email or password"}
//   - everything else                                → StatusFor(err) with a Spring-style ErrorBody
//
// A nil err writes nothing.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}

	var fe FieldErrorer
	if errors.As(err, &fe) {
		fields := fe.FieldErrors()
		if fields == nil {
			fields = map[string][]string{}
		}
		writeErrorJSON(w, http.StatusUnprocessableEntity, validationBody{Errors: fields})
		return
	}

	if errors.Is(err, ErrInvalidAuthentication) {
		writeErrorJSON(w, http.StatusUnprocessableEntity, messageBody{Message: ErrInvalidAuthentication.Error()})
		return
	}

	status := StatusFor(err)
	message := err.Error()
	if status >= http.StatusInternalServerError {
		// Do not leak internal error details to clients.
		message = http.StatusText(status)
	}
	path := ""
	if r != nil && r.URL != nil {
		path = r.URL.Path
	}
	writeErrorJSON(w, status, ErrorBody{
		Timestamp: now().UTC().Format(timestampLayout),
		Status:    status,
		Error:     http.StatusText(status),
		Message:   message,
		Path:      path,
	})
}

// writeErrorJSON writes v as a JSON response with the given status code.
func writeErrorJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// The status line has already been sent, so an encoding failure can only
	// mean that the client has gone away. There is nothing useful left to do.
	_ = json.NewEncoder(w).Encode(v)
}

// ErrorHandlerFunc is an HTTP handler that returns an error instead of
// writing error responses itself. Any returned error is rendered with
// WriteError.
type ErrorHandlerFunc func(w http.ResponseWriter, r *http.Request) error

// ServeHTTP implements http.Handler.
func (f ErrorHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := f(w, r); err != nil {
		WriteError(w, r, err)
	}
}
