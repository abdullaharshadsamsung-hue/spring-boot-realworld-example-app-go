# Spring Boot RealWorld (Go port)

A Go port of the Spring Boot implementation of the [RealWorld](https://github.com/gothinkster/realworld) backend, a Medium-style blogging API. It covers users and authentication (JWT), profiles and following, articles, comments, favorites and tags. The original exposes the domain through a REST API and a GraphQL API, and this port keeps both: REST handlers live in `internal/httpapi` and GraphQL resolvers in `internal/graphql`.

> **Migration status: incomplete and unverified. Read this before relying on the code.**
>
> | Check | Result |
> |---|---|
> | Build (`go build ./cmd/server`) | ✅ passed |
> | Unit tests (`go test ./...`) | ❌ **failed** |
> | Behavior compared with the original Spring app | ⚠️ not run |
> | Modules migrated | 72 of 103 |
> | Confidence | 90%, self-assessed by the migration tool. This is not a measured result. |
>
> No HTTP or GraphQL request has been confirmed to behave like the original.

---

## Tech stack

- **Language:** Go 1.27
- **HTTP:** Go standard library approach. The migration target was "go/standard", so there is no web framework.
- **Database:** PostgreSQL. The migration run used a PostgreSQL instance.
- **Auth:** JWT (`internal/auth/jwt.go`)
- **API surfaces:** REST and GraphQL. The schema is in `src/main/resources/schema/schema.graphqls`.
- **Schema:** SQL migration in `src/main/resources/db/migration/V1__create_tables.sql`

The full dependency list is in `go.mod`.

## Prerequisites

The migration was built in the `golang:1.27-alpine` image. To reproduce that toolchain locally, you need:

- Go 1.27
- git
- A C toolchain. The build environment installed Alpine's `build-base` package, which provides gcc, make and libc headers.
- A reachable PostgreSQL server
- The PostgreSQL command-line client (`psql`), to apply the schema

On Alpine, install the system packages like this:

```sh
apk add --no-cache git build-base
```

## Getting started

Run all commands from the root of a fresh clone.

### 1. Download dependencies

```sh
go mod download
```

### 2. Tidy modules

The migration needed this step to resolve dependencies. Run it before building:

```sh
go mod tidy
```

### 3. Build the server

```sh
go build -o bin/server ./cmd/server
```

### 4. Configure the environment

The server reads three variables (see [Environment variables](#environment-variables)):

```sh
export DATABASE_URL='<your PostgreSQL connection string>'
export PORT=8080
export JWT_SECRET='<a long random secret>'
```

### 5. Prepare the database

The migration run did not record a database setup step.

- The table definitions are in `src/main/resources/db/migration/V1__create_tables.sql`.
- Create the database referenced by `DATABASE_URL` (for example with `createdb` or `CREATE DATABASE`).
- Apply that SQL file to it:

```sh
psql "$DATABASE_URL" -f src/main/resources/db/migration/V1__create_tables.sql
```

It has not been verified whether the Go server applies this schema itself on startup.

### 6. Run

```sh
./bin/server
```

The server listens on port **8080**.

### A first request to try

None yet. No request has been verified against the migrated server, so there is no known-good example to give here. The route table is defined in `cmd/server/router.go`.

When you confirm a request returns the same response as the original Spring app, add it to this section.

## Running tests

```sh
go test ./...
```

> ❌ This command **currently fails**. Fixing the failing tests is the first task for anyone picking up this branch.

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL connection string used by the store layer (`internal/store`). |
| `PORT` | Yes | Port for the HTTP server. The migration run listened on `8080`. |
| `JWT_SECRET` | Yes | Secret used to sign and verify JWTs (`internal/auth/jwt.go`). |

Configuration is loaded in `internal/config/config.go`. Its defaults and the expected `DATABASE_URL` format were not recorded during migration.

## Architecture overview

```
cmd/server/            entry point (main.go) and route wiring (router.go)
internal/config/       environment-based configuration
internal/auth/         JWT handling and request-context user storage
internal/core/         domain model: users, articles, comments, favorites,
                       authorization rules, domain errors
internal/app/          application layer: query services, command services,
                       DTOs, input params, validation, cursor/offset paging
internal/store/        PostgreSQL repositories (users, articles, comments,
                       favorites) and time encoding
internal/httpapi/      REST handlers, middleware, error mapping
internal/graphql/      GraphQL resolvers and mutations, error mapping
src/main/resources/    SQL schema and GraphQL schema carried over from the
                       original project
```

### Request flow

1. **Routing.** `cmd/server/router.go` registers the REST handlers (`internal/httpapi`) and the GraphQL endpoint (`internal/graphql`).
2. **Authentication.** `internal/httpapi/middleware.go` and `internal/auth` take the place of the Spring Security filter chain. They parse the JWT and place the current user in the request `context.Context` (`internal/auth/context.go`).
3. **Application layer.** Handlers and resolvers call `internal/app`:
   - Reads go through `*_query.go`.
   - Writes go through `article_command.go` and `user_service.go`.
   - Input validation is in `validate_article.go`, `validate_user.go` and `register_validation.go`.
4. **Domain and persistence.** The application layer works with the domain types in `internal/core`. It persists through `internal/store`, which replaces the original MyBatis mappers.
5. **Errors.** Domain errors (`internal/core/errors.go`) are translated to responses in two places:
   - REST: `internal/httpapi/errors.go`
   - GraphQL: `internal/graphql/errors.go`

### Layer mapping from the Spring project

| Spring package | Go package |
|---|---|
| `io.spring.graphql` | `internal/graphql` |
| `io.spring.application` | `internal/app` |
| `io.spring.infrastructure` (MyBatis) | `internal/store` |
| `io.spring.api.security` | `internal/auth`, `internal/httpapi/middleware.go` |

## Migration notes

**No intentional behavioral changes were recorded.** The aim was a one-to-one port of the Spring codebase. The structural changes below were forced by the platform, because Spring and JVM mechanisms have no Go equivalent:

| Original mechanism | Replacement in Go | Reason |
|---|---|---|
| Spring Security filter chain (`OncePerRequestFilter`, `SecurityContextHolder`) | Explicit middleware, with the user stored in `context.Context` | Spring's thread-local security context does not exist in Go. |
| MyBatis mappers and XML result maps | Hand-written repositories in `internal/store` | MyBatis builds mapper implementations at runtime from XML. Go has no equivalent. |
| `@Transactional` | Explicit transaction handling, where implemented | Spring's AOP-based declarative transactions have no Go equivalent. |
| Netflix DGS datafetchers and annotations | Resolver code in `internal/graphql` | DGS registration and its exception SPI are JVM-only. |
| Bean Validation annotations | Explicit validation functions in `internal/app` | The `javax.validation` runtime does not exist in Go. |

### Unaccounted-for modules

72 of 103 modules are reported as migrated, but no source files were listed as "not translated". The remaining 31 modules have no recorded status. Compare against the original Spring project to find out what is missing.

## Known limitations

- **Tests fail.** `go test ./...` does not pass.
- **No behavioral parity check.** Response shapes, status codes and error formats have not been compared with the original.
- **No verified requests.** No request has been confirmed working end to end.
- **Database setup is unverified.** No DB setup command was run during migration. Schema application is a manual step (see step 5).
- **Components that could not be migrated directly.** Each of these needed a manual rewrite. The Go file listed is the likely counterpart, based on naming.

  | Original component | Problem | Likely Go counterpart |
  |---|---|---|
  | `GraphQLCustomizeExceptionHandler`: DGS `TypedGraphQLError` / `DefaultDataFetcherExceptionHandler` integration | Tied to the DGS and graphql-java exception SPI. The `extensions` keys (`errorType`/classification plus the field → messages map) must be reproduced by hand. | `internal/graphql/errors.go` |
  | `GraphQLCustomizeExceptionHandler`: `ConstraintViolation` handling | Depends on the Bean Validation runtime model. Validation errors must be mapped to the same `{field: [messages]}` structure. | `internal/graphql/errors.go`, `internal/app/validate_*.go` |
  | `JwtTokenFilter`: `SecurityContextHolder` / `UsernamePasswordAuthenticationToken` | No Go equivalent. Needs the user stored in `context.Context` with a `CurrentUser(ctx)`-style accessor and explicit auth-required middleware on protected routes. | `internal/auth/context.go`, `internal/httpapi/middleware.go` |
  | `JwtTokenFilter`: `OncePerRequestFilter` / filter-chain ordering (`addFilterBefore`) | Servlet and Spring Security mechanics. Middleware order must be set explicitly where the stack is composed. | `cmd/server/router.go`, `internal/httpapi/middleware.go` |
  | `JwtTokenFilter`: `WebAuthenticationDetailsSource` | Spring-specific and not used by business logic. Expected to be dropped. | — |
  | `ProfileDatafetcher`: DGS annotations (`@DgsComponent`, `@DgsData`, `@InputArgument`) | Must be re-expressed as resolvers for `Query.profile` and `User.profile`, plus the article and comment author fields. | `internal/graphql/profile.go` |
  | `ProfileDatafetcher`: `DataFetchingEnvironment.getLocalContext` | graphql-java-specific parent-to-child context. Needs parent data embedded in the resolved object, or a per-request context or loader. | `internal/graphql/profile.go`, `internal/graphql/article.go`, `internal/graphql/comment.go` |
  | `ProfileDatafetcher`: `SecurityUtil.getCurrentUser` | Thread-local security context. The resolver must read the user from the request context instead. | `internal/graphql/profile.go`, `internal/auth/context.go` |
  | `MyBatisArticleRepository`: `@Transactional` on `save` | Needs explicit `BeginTx`/`Commit`/`Rollback`. Article and tag writes must be atomic. | `internal/store/article_repo.go` |
  | `MyBatisArticleRepository`: `ArticleMapper` XML result maps and dynamic SQL | Each SQL statement must be hand-ported. Tag aggregation means grouping join rows by article id. | `internal/store/article_repo.go` |

## Manual review required

The migration flagged the following original modules as **low confidence**. Review each one's Go counterpart against the Spring source. Counterparts below are matched by name and are best guesses. Modules marked "—" have no obvious counterpart and may be among the 31 unaccounted-for modules.

### API / REST (`internal/httpapi`)

| Original | Likely Go counterpart |
|---|---|
| `UsersApi` | `internal/httpapi/users.go` |
| `CurrentUserApi` | `internal/httpapi/current_user.go` |
| `ArticlesApi` | `internal/httpapi/articles.go` |
| `ArticleApi` | `internal/httpapi/article.go` |
| `CommentsApi` | `internal/httpapi/comments.go` |
| `CustomizeExceptionHandler` | `internal/httpapi/errors.go` |
| `InvalidRequestException` | `internal/httpapi/errors.go`, `internal/core/errors.go` |
| `NoAuthorizationException` | `internal/httpapi/no_authorization.go` |
| `WebSecurityConfig` (which routes are public and which require auth) | `cmd/server/router.go`, `internal/httpapi/middleware.go` |

### GraphQL (`internal/graphql`)

| Original | Likely Go counterpart |
|---|---|
| `ArticleDatafetcher` | `internal/graphql/article.go` |
| `ArticleMutation` | `internal/graphql/article_mutation.go` |
| `CommentDatafetcher` | `internal/graphql/comment.go` |
| `ProfileDatafetcher` | `internal/graphql/profile.go` |
| `MeDatafetcher` | `internal/graphql/user.go` |
| `UserMutation` | `internal/graphql/user_mutation.go` |
| `RelationMutation` | `internal/graphql/relation_mutation.go` |
| `TagDatafetcher` | `internal/graphql/tags.go` |
| `GraphQLCustomizeExceptionHandler` | `internal/graphql/errors.go` |

### Application layer (`internal/app`)

| Original | Likely Go counterpart |
|---|---|
| `ArticleQueryService` | `internal/app/article_query.go` |
| `ArticleCommandService` | `internal/app/article_command.go` |
| `NewArticleParam` | `internal/app/new_article_param.go` |
| `UserService` | `internal/app/user_service.go` |
| `UpdateUserCommand` | `internal/app/update_user_param.go`, `internal/app/user_service.go` |
| `RegisterParam` | `internal/app/register_validation.go`, `internal/app/validate_user.go` |
| `DuplicatedUsernameValidator` | `internal/app/register_validation.go` |
| `ArticleData` | `internal/app/dto.go`, `internal/app/article_data_list.go` |
| `ProfileData` | `internal/app/dto.go`, `internal/app/profile_query.go` |
| `CursorPager` | `internal/app/page.go`, `internal/app/paging.go` |
| `CursorPageParameter` | `internal/app/datetime_cursor.go`, `internal/app/paging.go` |

### Domain and persistence (`internal/core`, `internal/store`)

| Original | Likely Go counterpart |
|---|---|
| `User` | `internal/core/user.go` |
| `Article` | `internal/core/article.go` |
| `UserRepository` | `internal/core/user.go`, `internal/store/user_repo.go` |
| `ArticleRepository` | `internal/core/article.go`, `internal/store/article_repo.go` |
| `CommentRepository` | `internal/core/comment.go`, `internal/store/comment_repo.go` |
| `MyBatisUserRepository` | `internal/store/user_repo.go` |

### Configuration

| Original | Likely Go counterpart |
|---|---|
| `application.properties`: confirm every setting the Spring app relied on is reflected in `DATABASE_URL`, `PORT` and `JWT_SECRET`. This includes the JWT secret and session lifetime. | `internal/config/config.go` |

### Suggested order of work

1. Make `go test ./...` pass.
2. Account for the 31 modules with no recorded migration status.
3. Run the same REST and GraphQL requests against the original Spring app and this server, and compare the responses. Start with the error formats and the auth-protected routes.
4. Add the first verified request to [A first request to try](#a-first-request-to-try).
