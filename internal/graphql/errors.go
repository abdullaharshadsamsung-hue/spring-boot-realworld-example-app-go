// Package graphql holds the GraphQL layer of the RealWorld (Conduit) API.
// This file replaces the source's GraphQLCustomizeExceptionHandler, the global
// Netflix DGS DataFetcherExceptionHandler.
package graphql

import (
	"errors"
	"sort"
	"strings"

	"migrated-app/internal/httpapi"
)

// ErrorType is the DGS typed-error classification, written to
// extensions.errorType and extensions.classification.
type ErrorType string

// Error types used by the handler. They match the names in
// com.netflix.graphql.types.errors.ErrorType.
const (
	ErrorTypeUnauthenticated ErrorType = "UNAUTHENTICATED"
	ErrorTypeBadRequest      ErrorType = "BAD_REQUEST"
	ErrorTypeInternal        ErrorType = "INTERNAL"
)

// Violation is one failed constraint. It is the Go equivalent of a
// javax.validation ConstraintViolation.
type Violation struct {
	// RootType names the validated root object. It corresponds to
	// getRootBeanClass().getName() and is informational only.
	RootType string
	// PropertyPath is the full constraint path, for example
	// "createArticle.input.title" (method.argument.field).
	PropertyPath string
	// Constraint is the constraint's short name, for example "NotBlank".
	Constraint string
	// Message is the human-readable constraint message.
	Message string
}

// ConstraintViolationError is the Go equivalent of
// javax.validation.ConstraintViolationException. Resolvers return it when
// method-level validation of their arguments fails.
type ConstraintViolationError struct {
	Violations []Violation
}

// NewConstraintViolationError builds a ConstraintViolationError from violations.
func NewConstraintViolationError(violations ...Violation) *ConstraintViolationError {
	return &ConstraintViolationError{Violations: violations}
}

// Error matches Hibernate Validator's message format:
// "path: message" entries joined with ", ".
func (e *ConstraintViolationError) Error() string {
	parts := make([]string, 0, len(e.Violations))
	for _, v := range e.Violations {
		parts = append(parts, v.PropertyPath+": "+v.Message)
	}
	return strings.Join(parts, ", ")
}

// FieldErrors groups violation messages by field. The field is the property
// path with its method and argument segments removed (see fieldParam). This
// method also lets the error satisfy httpapi.FieldErrorer.
func (e *ConstraintViolationError) FieldErrors() map[string][]string {
	out := make(map[string][]string, len(e.Violations))
	for _, v := range e.Violations {
		field := fieldParam(v.PropertyPath)
		out[field] = append(out[field], v.Message)
	}
	return out
}

// GraphQLError is a spec-shaped GraphQL error, the equivalent of DGS's
// TypedGraphQLError. It serialises to {"message","path","extensions"}.
type GraphQLError struct {
	Message    string         `json:"message"`
	Path       []any          `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

// HandlerResult is the outcome of handling a data-fetcher error. It is the
// equivalent of DataFetcherExceptionHandlerResult.
type HandlerResult struct {
	Errors []GraphQLError
}

// ExceptionHandler converts errors returned by data fetchers (resolvers)
// into GraphQL errors.
type ExceptionHandler interface {
	HandleError(err error, path []any) HandlerResult
}

// DefaultExceptionHandler stands in for DGS's DefaultDataFetcherExceptionHandler.
// It reports every error as a single INTERNAL typed error.
type DefaultExceptionHandler struct{}

// HandleError implements ExceptionHandler.
//
// MIGRATION_NOTE: DGS prefixes the message with the Java class name
// ("pkg.SomeException: msg"). Go errors have no class name, so the message is
// just err.Error(). Like the source, the message is passed to the client, so
// resolvers should not return errors that carry sensitive internal details.
// Manual review: consider replacing the message with a generic one.
func (DefaultExceptionHandler) HandleError(err error, path []any) HandlerResult {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return HandlerResult{Errors: []GraphQLError{newTypedError(ErrorTypeInternal, msg, path, nil)}}
}

// CustomizeExceptionHandler is the global handler that replaces
// GraphQLCustomizeExceptionHandler. It maps:
//   - httpapi.ErrInvalidAuthentication → one UNAUTHENTICATED error
//   - *ConstraintViolationError        → one BAD_REQUEST error whose
//     extensions map each field to its messages
//   - everything else                  → the fallback handler
type CustomizeExceptionHandler struct {
	fallback ExceptionHandler
}

// NewCustomizeExceptionHandler creates the handler. A nil fallback means
// DefaultExceptionHandler is used.
func NewCustomizeExceptionHandler(fallback ExceptionHandler) *CustomizeExceptionHandler {
	if fallback == nil {
		fallback = DefaultExceptionHandler{}
	}
	return &CustomizeExceptionHandler{fallback: fallback}
}

// HandleError implements ExceptionHandler.
func (h *CustomizeExceptionHandler) HandleError(err error, path []any) HandlerResult {
	if errors.Is(err, httpapi.ErrInvalidAuthentication) {
		// The source uses exception.getMessage(), which is always
		// "invalid email or password".
		return HandlerResult{Errors: []GraphQLError{
			newTypedError(ErrorTypeUnauthenticated, httpapi.ErrInvalidAuthentication.Error(), path, nil),
		}}
	}

	var cve *ConstraintViolationError
	if errors.As(err, &cve) {
		fields := cve.FieldErrors()
		ext := make(map[string]any, len(fields))
		for k, v := range fields {
			ext[k] = v
		}
		return HandlerResult{Errors: []GraphQLError{
			newTypedError(ErrorTypeBadRequest, cve.Error(), path, ext),
		}}
	}

	return h.fallback.HandleError(err, path)
}

// ErrorItem is the GraphQL ErrorItem type: one field key and its messages.
type ErrorItem struct {
	Key   string   `json:"key"`
	Value []string `json:"value"`
}

// Error is the GraphQL Error payload type. Mutations return it as data to
// report validation failures.
type Error struct {
	Message string      `json:"message"`
	Errors  []ErrorItem `json:"errors"`
}

// ErrorsAsData converts cve into the GraphQL Error payload, with the message
// "BAD_REQUEST" and one ErrorItem per field. It replaces the source's static
// getErrorsAsData. A nil cve gives an empty item list.
//
// MIGRATION_NOTE: the source iterates a HashMap, so its item order is not
// defined. Here the items are sorted by key so the output is deterministic.
func ErrorsAsData(cve *ConstraintViolationError) Error {
	items := []ErrorItem{}
	if cve != nil {
		fields := cve.FieldErrors()
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			items = append(items, ErrorItem{Key: k, Value: fields[k]})
		}
	}
	return Error{Message: string(ErrorTypeBadRequest), Errors: items}
}

// fieldParam drops the method and argument segments from a property path
// ("method.arg.field.sub" → "field.sub"). A path with no dot is returned
// unchanged. A two-segment path gives "", which matches the source's getParam.
func fieldParam(s string) string {
	splits := strings.Split(s, ".")
	if len(splits) == 1 {
		return s
	}
	return strings.Join(splits[2:], ".")
}

// newTypedError builds a typed error the way TypedGraphQLError.Builder does:
// the supplied extensions are merged with errorType/classification.
func newTypedError(t ErrorType, msg string, path []any, ext map[string]any) GraphQLError {
	merged := make(map[string]any, len(ext)+2)
	for k, v := range ext {
		merged[k] = v
	}
	merged["errorType"] = string(t)
	merged["classification"] = string(t)
	return GraphQLError{Message: msg, Path: path, Extensions: merged}
}
