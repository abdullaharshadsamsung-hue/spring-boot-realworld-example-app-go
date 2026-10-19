// Package httpapi contains the REST transport layer of the RealWorld API:
// handler adapters, error-to-status mapping and JSON response helpers.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// ErrResourceNotFound signals that a requested resource (article, comment,
// profile, user, ...) does not exist. It carries no payload, mirroring the
// source's argument-less ResourceNotFoundException.
//
// Lower layers may return it directly or wrap it
// (fmt.Errorf("article %q: %w", slug, httpapi.ErrResourceNotFound)); any error
// for which errors.Is(err, ErrResourceNotFound) holds is rendered as
// 404 Not Found.
//
// MIGRATION_NOTE: the source relied on @ResponseStatus(NOT_FOUND) plus Spring's
// ResponseStatusExceptionResolver. In Go the mapping is explicit: handlers
// return the error and HandlerFunc/WriteError translate it to a status.
var ErrResourceNotFound = errors.New("resource not found")

// timestampLayout matches the ISO-8601 offset format used by Spring Boot's
// default error attributes.
const timestampLayout = "2006-01-02T15:04:05.000-07:00"

// now is the clock used for error timestamps; overridable in tests.
var now = time.Now

// ErrorBody is the Spring-Boot-style default error response body.
type ErrorBody struct {
	Timestamp string `json:"timestamp"`
	Status    int    `json:"status"`
	Error     string `json:"error"`
	Message   string `json:"message"`
	Path      string `json:"path"`
}

// StatusFor returns the HTTP status code an error escaping a handler maps to.
// A nil error maps to 200 OK; unrecognised errors map to 500.
func StatusFor(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrResourceNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// WriteError renders err as a Spring-style JSON error response. It is the
// fallback used when no more specific handling intercepted the error.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status := StatusFor(err)
	if status == http.StatusOK {
		status = http.StatusInternalServerError
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

// HandlerFunc is an http handler that may return an error instead of writing
// an error response itself. Returned errors are rendered with WriteError,
// replicating exceptions "escaping" a Spring MVC controller.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// ServeHTTP implements http.Handler.
func (h HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		WriteError(w, r, err)
	}
}
