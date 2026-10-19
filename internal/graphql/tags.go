package graphql

import (
	"context"
	"errors"
	"fmt"

	"migrated-app/internal/app"
)

// TagLister is the read-side dependency of TagDatafetcher. It is satisfied by
// *app.TagsQueryService and lets tests substitute a fake.
type TagLister interface {
	// AllTags returns the names of every tag known to the system.
	AllTags(ctx context.Context) ([]string, error)
}

// Compile-time proof that the application service satisfies TagLister.
var _ TagLister = (*app.TagsQueryService)(nil)

// ErrNilTagLister is returned by NewTagDatafetcher when no TagLister is given.
var ErrNilTagLister = errors.New("graphql: tags query service is nil")

// Schema coordinates resolved by TagDatafetcher. They replace the DGS
// codegen constants DgsConstants.QUERY_TYPE and DgsConstants.QUERY.Tags.
const (
	QueryType      = "Query"
	QueryFieldTags = "tags"
)

// TagDatafetcher resolves the root Query.tags field. It is the GraphQL
// counterpart of REST GET /tags.
//
// MIGRATION_NOTE: the source is a Netflix DGS @DgsComponent discovered by
// classpath scanning, with @DgsData binding getTags to Query.tags. Go has no
// annotation scanning, so the composition root must construct this type with
// NewTagDatafetcher and register Tags as the resolver for
// QueryType/QueryFieldTags with whatever GraphQL executor serves
// schema.graphqls.
type TagDatafetcher struct {
	tags TagLister
}

// NewTagDatafetcher creates a TagDatafetcher backed by tags
// (normally *app.TagsQueryService).
func NewTagDatafetcher(tags TagLister) (*TagDatafetcher, error) {
	if tags == nil {
		return nil, ErrNilTagLister
	}
	return &TagDatafetcher{tags: tags}, nil
}

// Tags resolves Query.tags ([String]) by returning every tag name exactly as
// the query service reports it. An empty result is a non-nil slice so it
// serializes as [] rather than null.
func (d *TagDatafetcher) Tags(ctx context.Context) ([]string, error) {
	tags, err := d.tags.AllTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("graphql: resolve %s.%s: %w", QueryType, QueryFieldTags, err)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, nil
}
