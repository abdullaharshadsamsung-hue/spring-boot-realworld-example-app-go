package app

import (
	"context"
	"errors"
	"fmt"
)

// TagReadService is the read-side port that lists every tag name stored in
// the system. It replaces the source's MyBatis @Mapper TagReadService
// (SELECT name FROM tags); the PostgreSQL implementation lives in the store
// layer and is injected through NewTagsQueryService.
type TagReadService interface {
	// All returns the names of all tags, in the order the store yields them.
	All(ctx context.Context) ([]string, error)
}

// ErrNilTagReadService is returned by NewTagsQueryService when no
// TagReadService is supplied.
var ErrNilTagReadService = errors.New("app: tag read service is nil")

// TagsQueryService is the application-layer query service behind
// GET /tags (TagsApi) and the GraphQL "tags" query (TagDatafetcher).
//
// MIGRATION_NOTE: the source is a Spring @Service singleton wired by
// component scanning; here it is constructed explicitly once in the
// composition root (cmd/server/main.go) and handed to the REST and GraphQL
// handlers.
type TagsQueryService struct {
	tags TagReadService
}

// NewTagsQueryService creates a TagsQueryService backed by tags.
func NewTagsQueryService(tags TagReadService) (*TagsQueryService, error) {
	if tags == nil {
		return nil, ErrNilTagReadService
	}
	return &TagsQueryService{tags: tags}, nil
}

// AllTags returns every tag name exactly as the underlying read service
// reports it — no filtering, sorting or de-duplication. When no tags exist
// the result is an empty, non-nil slice so it serializes as [] rather than
// null.
func (s *TagsQueryService) AllTags(ctx context.Context) ([]string, error) {
	tags, err := s.tags.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("app: list all tags: %w", err)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, nil
}
