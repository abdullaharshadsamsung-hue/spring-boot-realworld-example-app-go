# Spring Boot RealWorld (Go port)

This is a Go implementation of the [RealWorld](https://github.com/gothinkster/realworld) "Conduit" backend API, migrated from the Spring Boot RealWorld example application. It provides a REST API for:

- user registration and login
- profiles and follows
- articles, including feed, favorites and tags
- comments

Authentication uses JWT, and data is stored in PostgreSQL.

**Migration status**

| Item | Status |
|---|---|
| Modules migrated | 72 of 103 |
| Modules not translated | The other 31 were intentionally not translated, per the agreed plan. See [Known limitations](#known-limitations). |
| Build | passed |
| Target unit tests (`go test ./...`) | passed |
| Behaviour comparison against the original | passed for the requests listed under [First requests to try](#first-requests-to-try) |
| Model self-assessed confidence | 95%. This figure was not measured. |

---

## Tech stack

| Concern | Choice |
|---|---|
| Language | Go. `go.mod` declares `go 1.23`, and the project was built with the `golang:1.27-alpine` toolchain. |
| HTTP | `net/http` and [`go-chi/chi/v5`](https://github.com/go-chi/chi) v5.1.0, with chi's `Logger` and `Recoverer` middleware |
| Database driver | [`lib/pq`](https://github.com/lib/pq) v1.10.9 through `database/sql` |
| Database | PostgreSQL. `docker-compose.yml` uses `postgres:16-alpine`. |
| Configuration | [`spf13/viper`](https://github.com/spf13/viper) v1.19.0, which reads environment variables |
| Logging | [`rs/zerolog`](https://github.com/rs/zerolog) v1.33.0 |
| Module name | `migrated-app` |

---

## Prerequisites

- **Go 1.27.** This matches the `golang:1.27-alpine` image the project was built and tested with. `go.mod` requires at least 1.23.
- **git.** It is needed by `go mod download`.
- **PostgreSQL**, reachable from your machine. Version 16 is what `docker-compose.yml` uses. If you do not have one, step 3 below shows how to start one with Docker.
- *(Optional)* **Docker with Docker Compose**, for the containerised setup or for running PostgreSQL locally.

---

## Database

| | Database |
|---|---|
| Original (Spring) | SQLite. It used a local SQLite database file with an empty username and password. |
| This project | PostgreSQL only, using the `lib/pq` driver and `$n` placeholders. |

**Why the database changed:** the migrated project targets PostgreSQL. As a result, the SQLite file path became a PostgreSQL DSN in `DATABASE_URL`, and the Gradle task that deleted the SQLite database file no longer applies.

**How the schema is created**

- No separate migration step is needed. On startup, `store.EnsureSchema` in `internal/store/schema.go` runs idempotent `CREATE TABLE IF NOT EXISTS` statements for all tables: `users`, `articles`, `article_favorites`, `follows`, `tags`, `article_tags` and `comments`.
- These statements mirror `src/main/resources/db/migration/V1__create_tables.sql`. That file is kept for reference only. Nothing in the Go code executes it, and no migration tool (Flyway or similar) is used.
- You only need an existing, empty PostgreSQL database and a user that can create tables in it.

**Startup behaviour when the database is unavailable**

- If the initial database ping fails, the server logs a warning and keeps going.
- If schema creation then fails, startup aborts.

---

## Getting started

### 1. Install dependencies and build

Run these from the repository root:

```sh
go mod download
go mod tidy
go build -o bin/server ./cmd/server
```

`go mod tidy` is part of the build sequence that was verified. It may rewrite `go.mod` and `go.sum`, so check `git diff` afterwards.

### 2. Configure the environment

`DATABASE_URL` is required. The example below uses the same credentials as `docker-compose.yml` and matches the PostgreSQL container started in step 3; if you use your own PostgreSQL server, change the host, user, password and database name in the DSN accordingly.

```sh
export DATABASE_URL="postgres://app:app@localhost:5432/app?sslmode=disable"
export JWT_SECRET="change-me-to-a-long-random-secret"
export PORT=8080
```

Notes on these variables:

- **`JWT_SECRET`:** if you leave it unset, the secret that was committed in the original project is used. Always set your own value outside local development.
- **`PORT`:** it is parsed but does not change the listen port. See [Known limitations](#known-limitations).

All supported variables are listed under [Environment variables](#environment-variables).

### 3. Set up the database

You need a running PostgreSQL database. If you do not already have one, start one with Docker using the same user, password and database as `docker-compose.yml` (published on `localhost:5432`, matching the `DATABASE_URL` above):

```sh
docker run -d --name conduit-postgres \
  -e POSTGRES_USER=app -e POSTGRES_PASSWORD=app -e POSTGRES_DB=app \
  -p 5432:5432 postgres:16-alpine
```

No schema commands are required. The tables are created automatically when the server starts (see [Database](#database)).

### 4. Run the server

```sh
./bin/server
```

The server listens on **http://localhost:8080**. Stop it with `Ctrl+C` or `SIGTERM`; the server then shuts down gracefully, allowing up to 10 seconds.

### First requests to try

The following requests were verified to return the same status as the original application:

```
GET  /articles            -> 200
GET  /tags                -> 200
GET  /articles/feed       -> 200
GET  /user                -> 200
GET  /articles/how-to-train-your-dragon           -> 200
GET  /articles/how-to-train-your-dragon/comments  -> 200
POST /users               -> 201
POST /articles            -> 200
```

The two GET requests on `how-to-train-your-dragon` only succeed after an article with that slug exists.

To start with something that needs no data or authentication:

```sh
curl http://localhost:8080/tags
curl http://localhost:8080/articles
```

There is also a liveness endpoint that returns `ok` and needs no authentication:

```sh
curl http://localhost:8080/healthz
```

### Alternative: Docker Compose

`docker-compose.yml` starts two services:

- **`postgres`** (`postgres:16-alpine`). It has the user, password and database all set to `app` and a health check. Its port 5432 is exposed only to the compose network, not published to the host.
- **`app`**, built from the `Dockerfile`. It starts once Postgres is healthy and publishes port `8080`.

```sh
docker compose up --build
```

The API is then available at http://localhost:8080.

The compose file sets `JWT_SECRET` to `test-secret-key-for-ci`, which is suitable for local or CI use only.

---

## Running tests

```sh
go test ./...
```

Test files sit next to the code they cover, under `internal/app`, `internal/auth`, `internal/config`, `internal/core`, `internal/graphql`, `internal/httpapi` and `internal/store`.

The verified test run had a PostgreSQL database available; start it and export the variables from steps 2 and 3 before running the tests.

---

## Environment variables

These are read by `internal/config/config.go`.

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | **yes** | none | PostgreSQL DSN. If it is missing or blank, startup fails. |
| `JWT_SECRET` | no (but set it) | The secret committed in the original `application.properties` | HMAC secret used to sign and verify JWTs. It must not be empty. |
| `PORT` | no | `8080` | Parsed and validated, but the listener in `cmd/server/main.go` is hard-coded to `:8080`. |
| `JWT_SESSION_TIME` | no | `86400` | Token lifetime in seconds. Must be a positive integer. |
| `IMAGE_DEFAULT` | no | `https://static.productionready.io/images/smiley-cyrus.jpg` | Avatar URL assigned to users who register without an image. |
| `DB_STATEMENT_TIMEOUT` | no | `3000` | Per-statement database timeout in seconds. Must be a positive integer. |
| `SQL_LOG_LEVEL` | no | `debug` | zerolog level for the data-access layer, for example `debug`, `info`, `warn`, `error` or `trace`. Matching is case-insensitive. |

`docker-compose.yml` also sets `DB_HOST`, `DB_NAME`, `DB_PASSWORD`, `DB_PORT`, `DB_USER`, `DB_USERNAME` and `POSTGRES_URL`. `internal/config` does not read any of these; only `DATABASE_URL` is used.

---

## Architecture overview

```
cmd/server/          entry point and dependency wiring
internal/config/     environment-based configuration (replaces application.properties)
internal/core/       domain model: users, articles, comments, favorites, authorization rules, errors
internal/app/        application services: queries, commands, DTOs, paging/cursors, validation
internal/httpapi/    REST handlers, error mapping, security middleware
internal/auth/       JWT service and request-context user helpers
internal/store/      PostgreSQL repositories, schema bootstrap, time encoding
internal/graphql/    GraphQL resolvers and mutations (not mounted, see Known limitations)
src/main/resources/  original schema files kept for reference
```

### `cmd/server/main.go`

Creates an HTTP server on `:8080` and handles graceful shutdown on `SIGINT` or `SIGTERM`.

### `cmd/server/router.go`

Builds the routing layer and wires every dependency by hand. There is no DI container. The steps are:

1. Builds a chi router with logging and recovery middleware, and mounts `GET /healthz`.
2. Loads configuration, opens the database, and runs `store.EnsureSchema`.
3. Constructs the parts of the dependency graph:
   - the JWT service
   - the password hasher
   - the write-side repositories from `internal/store`
   - the read-side PostgreSQL adapters (`userReads`, `articleReads`, `commentReads`, `favoriteReads`, `relationReads`, `tagReads`), which are defined in this file
   - the `internal/app` services
4. Registers the `internal/httpapi` handlers (tags, users, current user, profiles, articles, single article, favorites, comments) on a `net/http` `ServeMux`.
5. Wraps that mux in `httpapi.SecurityChain`, which applies CORS, then JWT authentication, then the authorization rules. The wrapped mux handles every path except `/healthz`.

### `internal/store`

Contains the write-side repositories (users, articles, comments, favorites). They port the SQL that previously lived in the MyBatis mapper XML files. `timecodec.go` handles timestamp encoding, including cursor values.

### `internal/app`

Mirrors the Spring `application` layer. It contains:

- query services (articles, comments, profiles, tags, users)
- `ArticleCommandService` and `UserService`
- offset paging (`page.go`, `paging.go`) and date-time cursor paging (`datetime_cursor.go`)
- validation (`validate_article.go`, `validate_user.go`, `register_validation.go`)

---

## Migration notes

### Behaviour changes

Each change from the original Spring codebase is listed with its reason.

| # | Change | Reason |
|---|---|---|
| 1 | Updating a user's password through `PUT /user` or the GraphQL `updateUser` mutation now stores the password **hashed**. The original stored the new password in plain text, so that user could never log in again. | Quirk Q1 in the plan. This is the one deliberate behaviour fix, and the plan says it **should be confirmed with the owner**. |
| 2 | The personal feed (offset and cursor variants) now paginates by article ID first and loads tags afterwards. Pages are no longer cut short and tag lists are no longer truncated. The response shape is unchanged. | Quirk Q4. The original applied `LIMIT` to joined tag rows, which truncated both tags and pages. |
| 3 | GraphQL comment pagination (`Article.comments`) now applies `LIMIT limit+1`. | Quirk Q5. The original comment cursor query had no `LIMIT`, so it could return the wrong number of rows and wrong `hasNext`/`hasPrevious` values. |
| 4 | Creating an article with no `tagList` (REST or GraphQL) succeeds with an empty tag list. The original returned HTTP 500. | Quirk Q6. The Java code threw a `NullPointerException` on a null `tagList`. |
| 5 | Creating an article with a missing or blank title returns a normal 422 "can't be empty" error. The original returned 500 from the duplicate-title validator. | Quirk Q7. The duplicate check is skipped for blank titles, and `NotBlank` reports the error instead. |
| 6 | The GraphQL `feed` query with no authenticated user returns an authentication error (`errorType INTERNAL`). The original crashed. | Plan section *ArticleQueryService*: `FindUserFeedWithCursor(nil user)` returns `ErrUnauthenticated` instead of causing an NPE. |
| 7 | An empty GraphQL pagination cursor string (`""`) is treated as "no cursor". Non-numeric cursors still produce an error. | Go strings cannot be null, so `""` stands in for Java's `null` (plan Phase 3, `ParseDateTimeCursor`). The original threw `NumberFormatException` on `""`. |
| 8 | Generic GraphQL errors no longer carry the Java exception class-name prefix. They use a fixed message with `errorType INTERNAL`. | Plan section *GraphQL errors*. There are no Java classes in the Go runtime. |
| 9 | Multiple validation messages for the same field are emitted in a fixed order: NotBlank, then format, then duplicate. In the original the order was unspecified. | Quirk Q9. Go emits messages in declaration order. |
| 10 | The MyBatis cache and statement-timeout settings are gone. The 3000 s statement timeout is meant to be applied per request through a context timeout (`DB_STATEMENT_TIMEOUT`). | Plan Phase 0. MyBatis-specific settings have no equivalent once MyBatis is removed. See [Manual review](#manual-review-required). |
| 11 | The Gradle `clean` task, which deleted the SQLite database file, is replaced by a Makefile `clean` target. | Plan Phase 0. Gradle is replaced by the Go toolchain. **No Makefile is included in this repository**, so this target is currently unavailable. |
| 12 | Spring HATEOAS support is dropped. | Plan section 4. The `spring-boot-starter-hateoas` dependency was unused. |
| 13 | New unauthenticated `GET /healthz` endpoint that returns `ok`. | Liveness probe for running the Go binary under an orchestrator. This was not mentioned in the plan; the reason is inferred. |

### Other structural changes

- **Database:** the database moved from SQLite to PostgreSQL. See [Database](#database).
- **Password hashing:** password hashing uses an iterated, salted SHA-512 encoder (10,000 iterations) defined in `cmd/server/router.go`. It replaces Spring Security's BCrypt because `golang.org/x/crypto` is not a module dependency.
- **Configuration:** `application.properties` is replaced by environment variables (`internal/config`). The defaults are copied from the original file.

---

## Known limitations

### Source files intentionally not translated (31, per the agreed plan)

| Source | Replacement or reason |
|---|---|
| `src/main/resources/mapper/*.xml` (10 files) | Their SQL is ported into `internal/store`. |
| The `TransferData` mapper XML file | Its result maps are ported as row scanning in Go. |
| MyBatis mapper interfaces for comments, article favorites, users and articles | Their SQL is ported into the `internal/store` repositories (`comment_repo.go`, `favorite_repo.go`, `user_repo.go`, `article_repo.go`). |
| MyBatis read services: `UserRelationshipQueryService`, `TagReadService`, `UserReadService`, `ArticleFavoritesReadService`, `ArticleReadService`, `CommentReadService` | Their SQL is ported to Go. The plan placed these in separate `internal/store` files; in this tree the read-side queries live in the adapters in `cmd/server/router.go`. |
| `DuplicatedArticleConstraint`, `DuplicatedEmailConstraint`, `DuplicatedUsernameConstraint` | These are annotation types with no Go meaning. Only the validator logic survives. |
| The page-cursor and node abstract classes | The abstract hierarchy is collapsed into a single `Node` interface (`Cursor() time.Time`) plus `FormatCursor`. |
| The Jackson customizations | No serializer framework is needed. Its behaviour moves into a `JSONTime` type. |
| The article favorite-count class | Replaced by `map[string]int`. |
| The field-error, error-resource and error-resource serializer classes | Not needed as classes. Their behaviour moves into `internal/httpapi/errors.go`. |

### Runtime limitations

- **GraphQL is not served.**
  - `internal/graphql` contains resolvers, mutations and tests, and `src/main/resources/schema/schema.graphqls` is kept.
  - However, `cmd/server/router.go` registers only the REST handlers, and `go.mod` has no GraphQL server library.
  - The GraphQL behaviour changes listed above therefore describe the package code, not a reachable endpoint.
- **`PORT` is ignored for listening.** The server always binds `:8080`. `internal/config` provides `Config.Addr()`, but `cmd/server/main.go` does not use it.
- **Existing password hashes are incompatible.** Users whose passwords were hashed with BCrypt by the original application cannot log in with the new SHA-512 encoder.
- **The committed default JWT secret.** If `JWT_SECRET` is unset, the secret that was committed in the original project is used.
- **No schema migration tooling.** Startup only runs `CREATE TABLE IF NOT EXISTS`. Later schema changes to existing tables are not applied automatically.

---

## Manual review required

### Low-confidence modules

The following original modules were flagged as low confidence. Verify their Go counterparts:

**REST API and security**
- `internal/httpapi/users.go`
- `internal/httpapi/current_user.go`
- `internal/httpapi/articles.go`
- `internal/httpapi/article.go`
- `internal/httpapi/comments.go`
- `internal/httpapi/middleware.go`
- `internal/httpapi/errors.go`
- `internal/httpapi/no_authorization.go`

**GraphQL**
- `internal/graphql/article.go`
- `internal/graphql/article_mutation.go`
- `internal/graphql/comment.go`
- `internal/graphql/profile.go`
- `internal/graphql/user.go`
- `internal/graphql/tags.go`
- `internal/graphql/relation_mutation.go`
- `internal/graphql/user_mutation.go`
- `internal/graphql/errors.go`

**Application layer**
- `internal/app/article_query.go`
- `internal/app/paging.go`
- `internal/app/datetime_cursor.go`
- `internal/app/article_command.go`
- `internal/app/new_article_param.go`
- `internal/app/user_service.go`
- `internal/app/update_user_param.go`
- `internal/app/register_validation.go`
- `internal/app/dto.go`

**Domain and persistence**
- `internal/core/user.go`
- `internal/core/article.go`
- `internal/core/comment.go`
- `internal/store/user_repo.go`

**Configuration**
- `src/main/resources/application.properties`

### Specific points to check

1. **Password-update fix (Q1).** Confirm with the project owner that hashing passwords on `PUT /user` and `updateUser` is the intended behaviour.
2. **Password encoder.** Decide whether the SHA-512 encoder in `cmd/server/router.go` is acceptable, or whether `golang.org/x/crypto/bcrypt` should be added. Bcrypt would restore compatibility with existing hashes and use a stronger algorithm.
3. **Statement timeout.** `cmd/server/router.go` does not visibly use `cfg.StatementTimeout`. Verify that the per-request context timeout described in migration note 10 is actually applied.
4. **`app.NewArticleCommandService(articleRepo, nil)`.** The second dependency is passed as `nil` in `cmd/server/router.go`. Confirm this is safe for every code path.
5. **GraphQL wiring.** Decide whether to mount `internal/graphql`, which needs a GraphQL server library, or to remove it.
6. **`PORT` handling.** Wire `Config.Addr()` into `cmd/server/main.go` if a configurable port is required.
7. **Missing Makefile.** Add the Makefile `clean` target described in the plan, or drop that requirement.
