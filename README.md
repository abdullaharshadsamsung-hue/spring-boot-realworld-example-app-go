# Spring Boot RealWorld: Go port

A Go port of the Spring Boot implementation of the [RealWorld](https://github.com/gothinkster/realworld) "Conduit" API, a Medium-style blogging backend. It covers users and authentication (JWT), profiles and follows, articles, favorites, tags and comments. The original project was written in Java with Spring Boot, MyBatis, Spring Security and Netflix DGS (GraphQL).

> **Migration status.** This port is incomplete. 72 of 103 source modules have been migrated. The model's self-assessed confidence is 91%, which is an estimate and was not measured.
>
> | Check | Result |
> |---|---|
> | Build | passed |
> | Target unit tests (`go test ./...`) | passed |
> | Behaviour compared with the original Spring app | **not run** |
>
> No HTTP request has been verified against the original's responses. Treat every endpoint as unverified until it has been compared.

---

## Tech stack

| Concern | Library / tool |
|---|---|
| Language | Go (`go.mod` declares `go 1.23`; the migration was built with the `golang:1.27-alpine` toolchain) |
| HTTP routing | `net/http`, plus `github.com/go-chi/chi/v5` v5.1.0 for the outer router and middleware |
| Database driver | `github.com/lib/pq` v1.10.9 (PostgreSQL) |
| SQL helpers | `database/sql`, `github.com/jmoiron/sqlx` v1.4.0 |
| Logging | `github.com/rs/zerolog` v1.33.0 |
| Configuration | `github.com/spf13/viper` v1.19.0 (environment variables only) |
| Database | PostgreSQL |

The Go module is named `migrated-app`.

---

## Prerequisites

- **Go**: the migration ran on the Go toolchain from `golang:1.27-alpine`. `go.mod` requires at least 1.23.
- **git**: needed by `go mod download`.
- **A C toolchain** (gcc, make, libc headers). The build environment installed this with Alpine's `build-base` package. On Alpine:
  ```sh
  apk add --no-cache git build-base
  ```
- **PostgreSQL**: a reachable server and a database for the app.
- `psql`, if you apply the schema the way shown below.

---

## Database

| | Database |
|---|---|
| Original (Spring) | SQLite (empty username and password) |
| This port | PostgreSQL only. The connection is opened with the `postgres` driver from `lib/pq`. |

**Why it changed:** the migrated project targets PostgreSQL, so the SQLite file path became a PostgreSQL DSN in `DATABASE_URL`. This is recorded as a `MIGRATION_NOTE` in `internal/config/config.go`. The read-side SQL in `cmd/server/router.go` uses PostgreSQL-specific features (`= ANY($1)`, `$n` placeholders), so it will not run on SQLite.

The schema is in `src/main/resources/db/migration/V1__create_tables.sql`. It creates these tables:

- `users`
- `articles`
- `article_favorites`
- `follows`
- `tags`
- `article_tags`
- `comments`

**The application does not apply this file.** No migration runner exists in the code, so you must apply it yourself.

---

## Getting started

Run these steps in order from the root of a fresh clone.

### 1. Download dependencies, tidy the module and build

```sh
go mod download
go mod tidy
go build -o bin/server ./cmd/server
```

`go mod tidy` was run successfully during migration and is required before the build.

### 2. Set environment variables

```sh
export DATABASE_URL='postgres://USER:PASSWORD@localhost:5432/DBNAME?sslmode=disable'
export JWT_SECRET='replace-with-a-long-random-secret'
export PORT=8080
```

Notes:

- `DATABASE_URL` is required. Without it, startup stops with `config: DATABASE_URL must be set`.
- `JWT_SECRET` has a built-in default, but that value was committed to source control in the original project. Always set your own.
- `PORT` is read by the config loader but currently has **no effect**: `cmd/server/main.go` hard-codes `Addr: ":8080"`. See [Known limitations](#known-limitations).

All supported variables are listed in [Environment variables](#environment-variables).

### 3. Create the schema

The migration run had no DB setup step, and the app does not create tables. Apply the schema file to your database before first use, for example:

```sh
psql "$DATABASE_URL" -f src/main/resources/db/migration/V1__create_tables.sql
```

This `psql` invocation was **not** part of the verified migration run. Use whatever migration tool your team prefers.

### 4. Run the server

```sh
./bin/server
```

The server listens on **port 8080** and logs `server started on :8080`. It shuts down gracefully on SIGINT or SIGTERM, with a 10-second timeout.

If the database cannot be pinged at startup, the server only logs a warning (`database ping failed; continuing`) and keeps starting. Check the logs if requests fail.

### 5. First request

No requests have been verified against the original application. The simplest route defined in the code is the health check in `cmd/server/router.go`:

```sh
curl http://localhost:8080/healthz
```

The code returns `ok` for this route. The health check was added in this port and does not exist in the original.

---

## Running tests

```sh
go test ./...
```

This passed during migration. These are unit tests of the Go code only. They do not compare behaviour with the Spring application.

---

## Environment variables

These are defined in `internal/config/config.go`. Defaults are copied from the original `application.properties` where it defined one.

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `DATABASE_URL` | **yes** | none | PostgreSQL DSN. Startup fails if it is unset or empty. |
| `PORT` | no | `8080` | Intended HTTP listen port. **Currently ignored**: `main.go` hard-codes `:8080`. |
| `JWT_SECRET` | no, but override it | the secret committed in the original project | HMAC secret used to sign and verify JWTs. Must not be empty. |
| `JWT_SESSION_TIME` | no | `86400` | Token lifetime in seconds. Must be a positive integer. |
| `IMAGE_DEFAULT` | no | `https://static.productionready.io/images/smiley-cyrus.jpg` | Avatar URL given to users who register without an image. |
| `DB_STATEMENT_TIMEOUT` | no | `3000` | Intended per-statement database timeout in seconds. Must be a positive integer. **Currently has no effect**: it is validated but not passed to any component. |
| `SQL_LOG_LEVEL` | no | `debug` | Intended zerolog level for the data-access layer (`debug`, `info`, `warn`, and so on). **Currently has no effect**: it is validated but not passed to any component. |

Startup also fails if `JWT_SESSION_TIME` or `DB_STATEMENT_TIMEOUT` is not a positive integer, or if `SQL_LOG_LEVEL` is not a valid zerolog level.

---

## Architecture overview

```
cmd/server/          entry point and dependency wiring
internal/config/     environment-based configuration (replaces application.properties)
internal/auth/       JWT service and request-context user accessor
internal/core/       domain types and rules: users, articles, comments, favorites, authorization, errors
internal/app/        application services: queries, commands, DTOs, validation, offset and cursor paging
internal/store/      write-side PostgreSQL repositories and time encoding
internal/httpapi/    REST handlers, security middleware, error rendering
internal/graphql/    GraphQL resolvers and mutations (not mounted, see Known limitations)
src/main/resources/  SQL schema and GraphQL SDL carried over from the original
```

### Request flow

The request path is defined in `cmd/server/main.go` and `cmd/server/router.go`.

1. `main.go` starts an HTTP server on `:8080` with the handler returned by `buildRouter()`.
2. `buildRouter()` creates a chi router with request-logging and `middleware.Recoverer` middleware. It handles `GET /healthz` itself and forwards every other path (`/*`) to the API handler.
3. `buildAPI()` builds the dependency graph:
   1. It loads the config and opens the PostgreSQL connection.
   2. It creates the JWT service.
   3. It creates the write-side repositories from `internal/store`:
      - `store.NewUserRepository`
      - `store.NewArticleRepository`
      - `store.NewFavoriteRepo`
      - `store.NewCommentRepo`
   4. It creates the read-side adapters, which are defined in `router.go` itself:
      - `userReads`
      - `relationReads`
      - `favoriteReads`
      - `articleReads`
      - `commentReads`
      - `tagReads`
   5. It creates the services from `internal/app`.
   6. It creates the REST handlers from `internal/httpapi`.
4. Each handler registers itself on a standard `http.ServeMux`. The handlers are:
   - tags
   - users
   - current user
   - profiles
   - articles
   - single article
   - favorites
   - comments
5. The mux is wrapped in `httpapi.SecurityChain`, which applies CORS, then JWT authentication, then authorization rules.

### Read and write split

Writes go through the repositories in `internal/store`. Reads go through the hand-written SQL adapters in `cmd/server/router.go`. This mirrors the original's split between MyBatis repositories and read services.

Article lists are fetched in two steps:

1. Fetch the article IDs.
2. Load the article rows and their tags in a separate query (`fillTags`).

### Password hashing

Passwords are hashed by `passwordHasher` in `cmd/server/router.go`. It uses iterated, salted SHA-512 with 10,000 iterations, in the format `sha512$<iter>$<salt-hex>$<hash-hex>`. It does not use bcrypt. See [Known limitations](#known-limitations).

---

## Migration notes

### Behaviour changes from the Spring original

| # | Change | Reason |
|---|---|---|
| 1 | Updating a user's password through `PUT /user` or the GraphQL `updateUser` mutation now stores the password hashed. The original stored the new password in plain text, so that user could never log in again. | Quirk Q1 in the plan. This is the one deliberate behaviour fix. **The plan says it should be confirmed with the owner.** |
| 2 | The personal feed (offset and cursor variants) now paginates by article ID first and loads tags afterwards. Pages are no longer cut short and tag lists are no longer truncated. The response shape is unchanged. | Quirk Q4 in the plan. The original applied `LIMIT` to joined tag rows, which truncated both tags and pages. |
| 3 | GraphQL comment pagination (`Article.comments`) now applies `LIMIT limit+1`. | Quirk Q5 in the plan. The original comment cursor query had no `LIMIT`, so it could return the wrong number of rows and wrong `hasNext`/`hasPrevious` values. |
| 4 | Creating an article with no `tagList` (REST or GraphQL) succeeds with an empty tag list. The original returned HTTP 500. | Quirk Q6 in the plan. The Java code threw a `NullPointerException` on a null `tagList`. |
| 5 | Creating an article with a missing or blank title returns a normal 422 "can't be empty" error. The original returned 500 from the duplicate-title validator. | Quirk Q7 in the plan. The duplicate check is skipped for blank titles, and NotBlank reports the error instead. |
| 6 | The GraphQL `feed` query with no authenticated user returns an authentication error (`errorType` `INTERNAL`). The original crashed with a `NullPointerException`. | Plan section ArticleQueryService: `FindUserFeedWithCursor(nil user)` returns `ErrUnauthenticated` instead of an NPE. |
| 7 | An empty GraphQL pagination cursor string is treated as no cursor. The original threw `NumberFormatException` on `""`. Non-numeric cursors still produce an error. | Go strings cannot be null, so `""` stands in for Java's `null` (plan Phase 3, `ParseDateTimeCursor`). |
| 8 | Generic GraphQL errors no longer carry the Java exception class-name prefix in their message. They use a fixed message with `errorType` `INTERNAL`. | Plan section GraphQL errors: there are no Java classes in the Go runtime. |
| 9 | Multiple validation messages for the same field are emitted in a fixed order: NotBlank, then format, then duplicate. In the original the order was unspecified. | Quirk Q9 in the plan. Go emits messages in declaration order. |
| 10 | The MyBatis cache and statement-timeout settings are gone. The 3000 s statement timeout is intended to be applied per request through a context timeout (`DB_STATEMENT_TIMEOUT`), but it is not wired in yet (see [Known limitations](#known-limitations)). | Plan Phase 0: MyBatis-specific settings have no equivalent once MyBatis is removed. |
| 11 | The Gradle `clean` task, which deleted the SQLite database file, is replaced by a Makefile `clean` target. | Plan Phase 0: Gradle is replaced by the Go toolchain. **There is no Makefile in this repository**, so this target does not exist yet. |
| 12 | Spring HATEOAS support is dropped. | Plan section 4: the `spring-boot-starter-hateoas` dependency was unused. |
| 13 | A new unauthenticated `GET /healthz` endpoint returns `ok`. | Liveness probe for running the Go binary under an orchestrator. This was not in the plan; it was inferred. |

### Infrastructure changes

| Change | Reason |
|---|---|
| The database is now PostgreSQL instead of SQLite. | See [Database](#database). |
| Configuration comes from environment variables instead of `application.properties`. | See `internal/config/config.go`. |
| Password hashing is now iterated, salted SHA-512 instead of BCrypt. | `golang.org/x/crypto` is not a module dependency. This is recorded as a `MIGRATION_NOTE` in `cmd/server/router.go`. |

### Source files intentionally not translated (per the agreed plan)

| Original source | Count | Where the behaviour went |
|---|---|---|
| MyBatis mapper XML files | 10 | The SQL was ported into `internal/store`. |
| MyBatis result-map XML file | 1 | Ported as row scanning in `internal/store`. |
| MyBatis mapper interfaces: `UserMapper`, `ArticleMapper`, `CommentMapper`, `ArticleFavoriteMapper` | 4 | `internal/store/user_repo.go`, `article_repo.go`, `comment_repo.go`, `favorite_repo.go` |
| MyBatis read services: `UserReadService`, `ArticleReadService`, `CommentReadService`, `TagReadService`, `ArticleFavoritesReadService`, `UserRelationshipQueryService` | 6 | The plan named separate read files under `internal/store`. Those files do not exist. The read SQL is in the read adapters in `cmd/server/router.go`. |
| Constraint annotations: `DuplicatedArticleConstraint`, `DuplicatedEmailConstraint`, `DuplicatedUsernameConstraint` | 3 | The annotations have no Go meaning. Only the validator logic survives, in `internal/app/validate_article.go`, `validate_user.go` and `register_validation.go`. |
| `JacksonCustomizations` | 1 | The plan says this moved into a `JSONTime` type in `internal/app`. Verify where JSON time formatting actually lives. |
| `PageCursor`, `Node` | 2 | The abstract hierarchy was collapsed into a `Node` interface (`Cursor() time.Time`) plus `FormatCursor`. |
| `ArticleFavoriteCount` | 1 | Replaced by `map[string]int`. |
| `ErrorResource`, `ErrorResourceSerializer`, `FieldErrorResource` | 3 | `internal/httpapi/errors.go` and the validation error type. |

---

## Known limitations

### GraphQL is not served

The `internal/graphql` package and `src/main/resources/schema/schema.graphqls` exist. However, `cmd/server/router.go` mounts no GraphQL handler, and `go.mod` contains no GraphQL server library. The GraphQL-related behaviour changes above (rows 3, 6, 7 and 8) cannot be reached over HTTP until a GraphQL server is wired in.

### Known code issues

- **Password hashes are incompatible with the original.** Hashes produced by the Java BCrypt encoder will not match the SHA-512 format used here. Users migrated from an existing database cannot log in until their passwords are reset or a BCrypt-compatible verifier is added.
- **`PORT` is ignored.** `main.go` hard-codes `Addr: ":8080"` instead of using `cfg.Addr()`.
- **The statement timeout and SQL log level are not applied.** `cfg.StatementTimeout` and `cfg.SQLLogLevel` are loaded and validated, but `buildAPI()` in `cmd/server/router.go` only passes `DatabaseURL`, `JWTSecret`, `JWTSessionTime` and `DefaultImage` on. The config object is not handed to any handler or repository, so `DB_STATEMENT_TIMEOUT` and `SQL_LOG_LEVEL` currently have no effect.
- **The schema is not applied automatically.** There is no migration runner. Apply `V1__create_tables.sql` manually.
- **Database outages are only logged at startup.** A failed ping is logged as a warning and the server still starts.
- **The default JWT secret is public.** It was committed in the original project.
- **There is no Makefile.** Migration note 11 refers to a `clean` target that does not exist.
- **31 of 103 modules are not migrated.** Some of these are intentional (see the table above). The rest have not been accounted for in this README.

### Unmigrable components

These components are tied to JVM frameworks and need hand-written Go replacements.

| Original component | Why it couldn't be migrated directly | Suggested approach / current state |
|---|---|---|
| `GraphQLCustomizeExceptionHandler.onException` (DGS `TypedGraphQLError` and `DefaultDataFetcherExceptionHandler` integration) | Tied to the Netflix DGS and graphql-java exception-handling SPI and its typed-error extension format. | Rewrite it as the chosen Go GraphQL server's error formatter. Reproduce the `errorType`/classification extensions and the field-to-messages map by hand. Related code: `internal/graphql/errors.go`. |
| `GraphQLCustomizeExceptionHandler`: `ConstraintViolation` handling | Depends on the Bean Validation runtime model, which does not exist outside Java. | Map the Go validation errors to the same `{field: [messages]}` structure. |
| `JwtTokenFilter`: `SecurityContextHolder` / `UsernamePasswordAuthenticationToken` | Spring Security's thread-local context has no Go equivalent. | Store the authenticated user in `context.Context` and use explicit auth middleware. Related code: `internal/auth/context.go`, `internal/httpapi/middleware.go`. |
| `JwtTokenFilter`: `OncePerRequestFilter` / filter-chain registration | Servlet and Spring Security filter mechanics. | Use standard Go middleware with explicit ordering. Related code: `httpapi.SecurityChain`. |
| `JwtTokenFilter`: `WebAuthenticationDetailsSource` | Spring-specific details object that business logic does not use. | Drop it. Optionally store `r.RemoteAddr` in the context for auditing. |
| `ProfileDatafetcher`: DGS annotations (`@DgsComponent`, `@DgsData`, `@InputArgument`) | DGS-specific registration. | Re-express as a resolver map (`Query.profile`, `User.profile`, and the author fields on articles and comments) in a Go GraphQL library such as gqlgen. |
| `ProfileDatafetcher`: `DataFetchingEnvironment.getLocalContext` | graphql-java-specific local context propagation. | Embed the needed data in the parent resolved object, or use a per-request context or DataLoader. |
| `ProfileDatafetcher`: `SecurityUtil.getCurrentUser` | Depends on Spring Security's thread-local context. | Read the JWT user from the GraphQL request context. |
| `MyBatisArticleRepository`: `@Transactional` on save | Spring AOP declarative transactions have no Go equivalent. | Use explicit `BeginTx`/`Commit`/`Rollback`, or a `WithTx` helper. Verify that `internal/store/article_repo.go` saves articles and their tags atomically. |
| `MyBatisArticleRepository`: `ArticleMapper` (mapper plus XML result maps) | MyBatis generates the implementation at runtime from XML. | The SQL was hand-ported into `internal/store/article_repo.go` and the read adapters in `cmd/server/router.go`. Verify tag aggregation and the dynamic SQL. |

---

## Manual review required

### Low-confidence modules

These modules were flagged as low confidence. Each is listed with the Go code that most likely implements it.

| Original (Java) | Go code to review |
|---|---|
| `application.properties` | `internal/config/config.go` |
| `WebSecurityConfig`, `NoAuthorizationException`, `InvalidRequestException`, `CustomizeExceptionHandler` | `internal/httpapi/middleware.go`, `no_authorization.go`, `errors.go`, `status.go`, plus `internal/core/errors.go` and `internal/core/authz.go` |
| `UsersApi`, `CurrentUserApi` | `internal/httpapi/users.go`, `current_user.go` |
| `ArticlesApi`, `ArticleApi` | `internal/httpapi/articles.go`, `article.go` |
| `CommentsApi` | `internal/httpapi/comments.go` |
| `UserService`, `UpdateUserCommand`, `RegisterParam`, `DuplicatedUsernameValidator` | `internal/app/user_service.go`, `update_user_param.go`, `register_validation.go`, `validate_user.go` |
| `ArticleCommandService`, `NewArticleParam` | `internal/app/article_command.go`, `new_article_param.go`, `validate_article.go` |
| `ArticleQueryService`, `ArticleData`, `ProfileData` | `internal/app/article_query.go`, `article_data_list.go`, `dto.go` |
| `CursorPager`, `CursorPageParameter` | `internal/app/paging.go`, `page.go`, `datetime_cursor.go` |
| `User`, `Article`, `UserRepository`, `ArticleRepository`, `CommentRepository` | `internal/core/user.go`, `article.go`, `comment.go`, `favorite_repository.go` |
| `MyBatisUserRepository` | `internal/store/user_repo.go` |
| `ArticleDatafetcher`, `CommentDatafetcher`, `ProfileDatafetcher`, `MeDatafetcher`, `TagDatafetcher` | `internal/graphql/article.go`, `comment.go`, `profile.go`, `user.go`, `tags.go` |
| `ArticleMutation`, `UserMutation`, `RelationMutation` | `internal/graphql/article_mutation.go`, `user_mutation.go`, `relation_mutation.go`, `comment_mutation.go` |
| `GraphQLCustomizeExceptionHandler` | `internal/graphql/errors.go` |

### Specific items to verify

1. **Behaviour parity.** No comparison with the original has been run. Run the RealWorld API test suite, or replay recorded requests, against both implementations.
2. **Password-update fix (Q1).** Confirm it with the project owner, as the plan requires.
3. **Password hashing.** Decide between adding `golang.org/x/crypto/bcrypt` for compatibility with existing data and keeping the SHA-512 scheme in `cmd/server/router.go`.
4. **`app.NewArticleCommandService(articleRepo, nil)`.** The second dependency is passed as `nil` in `cmd/server/router.go`. Confirm that this is safe.
5. **GraphQL wiring.** Choose a GraphQL library, add it to `go.mod`, and mount `internal/graphql` in the router.
6. **`PORT`, `DB_STATEMENT_TIMEOUT` and `SQL_LOG_LEVEL`.** Wire these config values into the server and data layer.
7. **Timestamps.** The schema uses `TIMESTAMP` without a time zone. Check `internal/store/timecodec.go` (`UTCTime`, `ToDB`) for correct UTC round-tripping and cursor encoding.
8. **Transactions.** Confirm that article saves with tags are atomic in `internal/store/article_repo.go`.
9. **The remaining unmigrated modules.** Account for the 31 modules that were not migrated.
