package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
)

// ErrNoAuthorization signals that the authenticated user is not permitted to
// perform the requested action on a resource — for example updating or
// deleting another user's article, or deleting a comment they neither wrote
// nor own the article of. It carries no payload, mirroring the source's
// argument-less NoAuthorizationException.
//
// Handlers and services may return it directly or wrap it
// (fmt.Errorf("delete comment %s: %w", id, httpapi.ErrNoAuthorization)); any
// error for which errors.Is(err, ErrNoAuthorization) holds is rendered as
// 403 Forbidden with a Spring-style error body.
//
// MIGRATION_NOTE: the source relied on @ResponseStatus(FORBIDDEN) plus Spring's
// ResponseStatusExceptionResolver. In Go the mapping is explicit: see
// ForbiddenStatusFor / WriteAuthzError and AuthzHandlerFunc.
var ErrNoAuthorization = errors.New("no authorization")

// ForbiddenStatusFor extends StatusFor with the 403 mapping for
// ErrNoAuthorization. A nil error maps to 200 OK; other errors fall through to
// StatusFor.
func ForbiddenStatusFor(err error) int {
	if errors.Is(err, ErrNoAuthorization) {
		return http.StatusForbidden
	}
	return StatusFor(err)
}

// WriteAuthzError renders err as a Spring-style JSON error response, mapping
// ErrNoAuthorization to 403 Forbidden and delegating every other error to
// WriteError.
func WriteAuthzError(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil && !errors.Is(err, ErrNoAuthorization) {
		var fe FieldErrorer
		if errors.As(err, &fe) || errors.Is(err, ErrInvalidAuthentication) {
			WriteError(w, r, err)
			return
		}
	}
	status := http.StatusInternalServerError
	if err != nil {
		status = ForbiddenStatusFor(err)
	}
	body := ErrorBody{
		Timestamp: now().Format(timestampLayout),
		Status:    status,
		Error:     http.StatusText(status),
		// Spring Boot (2.3+) omits exception messages by default; the source
		// exception has no message anyway.
		Message: "",
		Path:    r.URL.Path,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// AuthzHandlerFunc is like HandlerFunc but renders returned errors with
// WriteAuthzError, so ErrNoAuthorization escaping a handler becomes
// 403 Forbidden — the equivalent of NoAuthorizationException escaping a
// Spring MVC controller.
type AuthzHandlerFunc func(w http.ResponseWriter, r *http.Request) error

// ServeHTTP implements http.Handler.
func (h AuthzHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		WriteAuthzError(w, r, err)
	}
}
