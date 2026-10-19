package httpapi

import (
	"errors"
	"net/http"
	"time"
)

// timestampLayout is the format of ErrorBody.Timestamp, matching the ISO-8601
// timestamps of Spring Boot's default error body.
const timestampLayout = "2006-01-02T15:04:05.000Z07:00"

// now returns the current time; it is a variable so tests can override it.
var now = time.Now

// ErrorBody is the Spring Boot style JSON error response body.
type ErrorBody struct {
	Timestamp string `json:"timestamp"`
	Status    int    `json:"status"`
	Error     string `json:"error"`
	Message   string `json:"message"`
	Path      string `json:"path"`
}

// StatusFor maps err to the HTTP status code WriteError would use for it.
// A nil error maps to 200 OK.
func StatusFor(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrInvalidAuthentication):
		return http.StatusUnprocessableEntity
	case errors.Is(err, ErrNoAuthorization):
		return http.StatusForbidden
	case errors.Is(err, ErrResourceNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
