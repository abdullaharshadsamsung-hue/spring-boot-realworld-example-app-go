package httpapi

import (
	"encoding/json"
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

// writeErrorJSON encodes v as JSON with the given status code.
func writeErrorJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
