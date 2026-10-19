package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// TagsQuerier is the application-layer dependency of TagsHandler. It is
// satisfied by *app.TagsQueryService; accepting an interface keeps the
// handler testable without a database.
type TagsQuerier interface {
	// AllTags returns every tag name in the system.
	AllTags(ctx context.Context) ([]string, error)
}

// ErrNilTagsQuerier is returned by NewTagsHandler when no TagsQuerier is
// supplied.
var ErrNilTagsQuerier = errors.New("httpapi: tags querier is nil")

// TagsHandler serves the public, read-only RealWorld "Get Tags" endpoint.
// It replaces the source's TagsApi @RestController.
type TagsHandler struct {
	tags TagsQuerier
}

// NewTagsHandler creates a TagsHandler backed by tags.
func NewTagsHandler(tags TagsQuerier) (*TagsHandler, error) {
	if tags == nil {
		return nil, ErrNilTagsQuerier
	}
	return &TagsHandler{tags: tags}, nil
}

// Register mounts the handler's routes on mux:
//
//	GET /tags → GetTags
//
// The route is public; no authentication middleware is required.
func (h *TagsHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /tags", ErrorHandlerFunc(h.GetTags))
}

// tagsResponse is the wire shape {"tags": [...]}.
type tagsResponse struct {
	Tags []string `json:"tags"`
}

// GetTags writes 200 {"tags":[...]} containing every tag name. When no tags
// exist the body is {"tags":[]} (never null). A query failure is returned
// and rendered by WriteError as a 500 Spring-style error body.
func (h *TagsHandler) GetTags(w http.ResponseWriter, r *http.Request) error {
	tags, err := h.tags.AllTags(r.Context())
	if err != nil {
		return fmt.Errorf("get tags: %w", err)
	}
	if tags == nil {
		tags = []string{}
	}
	writeErrorJSON(w, http.StatusOK, tagsResponse{Tags: tags})
	return nil
}
