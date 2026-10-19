// Package httpapi contains the REST transport layer: handlers, middleware and
// the centralised error-to-response mapping that replaces Spring's
// @RestControllerAdvice CustomizeExceptionHandler.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/rs/zerolog"

	"migrated-app/internal/core"
)

// sentinelError is a string-backed error type so package sentinels can be
// declared as constants and compared with errors.Is.
type sentinelError string

// Error implements the error interface.
func (e sentinelError) Error() string { return string(e) }

// Sentinel errors produced by the REST layer.
const (
	// ErrResourceNotFound replaces ResourceNotFoundException and maps to 404.
	ErrResourceNotFound sentinelError = "resource not found"
	// ErrInvalidAuthentication replaces InvalidAuthenticationException
	// (wrong email/password on login) and maps to 422 with a message body.
	ErrInvalidAuthentication sentinelError = "invalid email or password"
)

// FieldErrors groups validation messages by field name. It is the Go
// equivalent of a list of FieldErrorResource values, already in the shape
// produced by the source's ErrorResource serializer:
//
//	{"errors": {"email": ["can't be empty", "should be an email"]}}
type FieldErrors map[string][]string

// Add records msg against field. The receiver must be non-nil.
func (f FieldErrors) Add(field, msg string) {
	f[field] = append(f[field], msg)
}

// HasErrors reports whether any field error has been recorded.
func (f FieldErrors) HasErrors() bool {
	return len(f) > 0
}

// FieldErrorer is implemented by any error that carries per-field validation
// failures. It unifies InvalidRequestException, MethodArgumentNotValidException
// and ConstraintViolationException from the source: all three produce HTTP 422
// with an ErrorResource body.
type FieldErrorer interface {
	error
	// FieldErrors returns the validation failures grouped by field.
	FieldErrors() FieldErrors
}

// InvalidRequestError replaces InvalidRequestException. It is returned by
// handlers and validators when request content fails validation.
type InvalidRequestError struct {
	errs FieldErrors
}

// NewInvalidRequestError wraps errs in an InvalidRequestError. A nil map is
// replaced by an empty one so the JSON body is always an object.
func NewInvalidRequestError(errs FieldErrors) *InvalidRequestError {
	if errs == nil {
		errs = FieldErrors{}
	}
	return &InvalidRequestError{errs: errs}
}

// Error implements the error interface.
func (e *InvalidRequestError) Error() string {
	if e == nil || len(e.errs) == 0 {
		return "invalid request"
	}
	fields := make([]string, 0, len(e.errs))
	for field := range e.errs {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, field+": "+strings.Join(e.errs[field], ", "))
	}
	return "invalid request: " + strings.Join(parts, "; ")
}

// Errors returns the grouped field errors (InvalidRequestException.getErrors).
func (e *InvalidRequestError) Errors() FieldErrors {
	if e == nil {
		return FieldErrors{}
	}
	return e.errs
}

// FieldErrors implements FieldErrorer.
func (e *InvalidRequestError) FieldErrors() FieldErrors {
	return e.Errors()
}

var _ FieldErrorer = (*InvalidRequestError)(nil)

// FieldErrorBody is the JSON body written for validation failures; it replaces
// ErrorResource and its custom serializer.
type FieldErrorBody struct {
	Errors FieldErrors `json:"errors"`
}

// messageBody is the JSON body for authentication failures; it replaces the
// double-brace-initialised HashMap {"message": ...} from the source.
type messageBody struct {
	Message string `json:"message"`
}

// StatusCoder may be implemented by errors defined elsewhere (for example the
// no-authorization or unauthenticated errors) to choose their own HTTP status.
type StatusCoder interface {
	// HTTPStatus returns the status code to send for this error.
	HTTPStatus() int
}

// ParamFromPropertyPath reproduces CustomizeExceptionHandler.getParam: a
// ConstraintViolation property path such as "createArticle.param.title" has
// its first two segments (method and parameter name) stripped. A single
// segment is returned unchanged; exactly two segments yield "".
func ParamFromPropertyPath(path string) string {
	splits := strings.Split(path, ".")
	if len(splits) == 1 {
		return path
	}
	return strings.Join(splits[2:], ".")
}

// isBadRequest reports whether err is a malformed-body or bad-query error.
func isBadRequest(err error) bool {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var numErr *strconv.NumError
	switch {
	case errors.As(err, &syntaxErr),
		errors.As(err, &typeErr),
		errors.As(err, &numErr),
		errors.Is(err, io.EOF),
		errors.Is(err, io.ErrUnexpectedEOF):
		return true
	}
	return false
}

// StatusFor maps an error returned by a handler to its HTTP status code.
func StatusFor(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var fe FieldErrorer
	if errors.As(err, &fe) {
		return http.StatusUnprocessableEntity
	}
	if errors.Is(err, ErrInvalidAuthentication) {
		return http.StatusUnprocessableEntity
	}
	if errors.Is(err, ErrNoAuthorization) {
		return http.StatusForbidden
	}
	var sc StatusCoder
	if errors.As(err, &sc) {
		return sc.HTTPStatus()
	}
	if errors.Is(err, ErrResourceNotFound) || errors.Is(err, core.ErrNotFound) {
		return http.StatusNotFound
	}
	if isBadRequest(err) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

// writeJSON encodes v with the given status and application/json content type.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger := zerolog.Ctx(r.Context())
		logger.Error().Err(err).Msg("httpapi: encode error response")
	}
}

// WriteError converts err into an HTTP response. It is the single dispatch
// point replacing every @ExceptionHandler in CustomizeExceptionHandler:
//
//   - FieldErrorer (InvalidRequestException, @Valid failures,
//     ConstraintViolationException) → 422 {"errors": {...}}
//   - ErrInvalidAuthentication → 422 {"message": "..."}
//   - StatusCoder → its status; 401 is written with an empty body
//   - not found → 404, malformed body / bad query int → 400
//   - anything else → logged, 500
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	var fe FieldErrorer
	if errors.As(err, &fe) {
		errs := fe.FieldErrors()
		if errs == nil {
			errs = FieldErrors{}
		}
		writeJSON(w, r, http.StatusUnprocessableEntity, FieldErrorBody{Errors: errs})
		return
	}
	if errors.Is(err, ErrInvalidAuthentication) {
		writeJSON(w, r, http.StatusUnprocessableEntity, messageBody{Message: err.Error()})
		return
	}

	status := StatusFor(err)
	switch status {
	case http.StatusUnauthorized:
		w.WriteHeader(status)
	case http.StatusInternalServerError:
		logger := zerolog.Ctx(r.Context())
		logger.Error().Err(err).Str("path", r.URL.Path).Msg("httpapi: unhandled error")
		// MIGRATION_NOTE: Spring's ResponseEntityExceptionHandler would emit its
		// default error body; we emit a generic message to avoid leaking internals.
		writeJSON(w, r, status, messageBody{Message: http.StatusText(status)})
	default:
		writeJSON(w, r, status, messageBody{Message: err.Error()})
	}
}

// ErrorHandlerFunc is an HTTP handler that returns an error instead of writing
// it. Its ServeHTTP routes any returned error through WriteError, which gives
// every route the cross-cutting behaviour of @RestControllerAdvice.
type ErrorHandlerFunc func(w http.ResponseWriter, r *http.Request) error

// ServeHTTP implements http.Handler.
func (h ErrorHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		WriteError(w, r, err)
	}
}

// HandlerFunc is an alias of ErrorHandlerFunc kept for call-site brevity.
type HandlerFunc = ErrorHandlerFunc

var _ http.Handler = ErrorHandlerFunc(nil)
