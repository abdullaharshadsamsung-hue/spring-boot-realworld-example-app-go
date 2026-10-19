package core

import "errors"

// ErrUnauthenticated means an operation needs an authenticated user and
// the request has none. GraphQL resolvers (ArticleMutation, CommentMutation,
// RelationMutation) return it when the current user is missing. Like the
// source's argument-less AuthenticationException, it carries no payload.
//
// Callers may return it directly or wrap it with
// fmt.Errorf("create article: %w", core.ErrUnauthenticated). Check for it with
// errors.Is(err, core.ErrUnauthenticated).
//
// MIGRATION_NOTE: In the source, GraphQLCustomizeExceptionHandler only handles
// InvalidAuthenticationException. AuthenticationException goes to the DGS
// default handler instead, which reports it as errorType INTERNAL and does
// not set a message (Java's no-arg RuntimeException has a null message). The
// GraphQL error mapper should keep that behaviour: do not map this error to
// UNAUTHENTICATED/401 unless that change is deliberately accepted.
var ErrUnauthenticated = errors.New("authentication required")
