# Spring Boot RealWorld (Go)

A Go port of the Spring Boot implementation of the [RealWorld](https://github.com/gothinkster/realworld) "Conduit" backend. It is a blogging-platform API covering users and authentication (JWT), profiles and follows, articles, favorites, tags and comments, backed by PostgreSQL.

This branch is the result of an automated migration from Java/Spring Boot to Go.

**Migration status**

- 72 of 103 source modules were translated. The other 31 were intentionally not carried over, as agreed in the migration plan (see [Known limitations](#known-limitations)).
- The migration tool rated its own confidence at 95%. This is a self-assessment, not a measured result.
- Verified so far:
  - The project builds.
  - The Go unit tests pass.
  - The REST API's responses were compared with the original application and matched.

---

## Tech stack

| Concern        | Choice                                                          |
|----------------|-----------------------------------------------------------------|
| Language       | Go (module `migrated-app`, `go 1.23` in `go.mod`; built and tested with the `golang:1.27-alpine` toolchain) |
| HTTP router    | `github.com/go-chi/chi/v5` v5.1.0 and `net/http` `ServeMux`     |
| Database       | PostgreSQL via `database/sql` and `github.com/lib/pq` v1.10.9   |
| Configuration  | Environment variables via `github.com/spf13/viper` v1.19.0      |
| Logging        | `github.com/rs/zerolog` v1.33.0                                 |
| Auth           | JWT (`internal/auth`)                                           |
| Container      | `Dockerfile` and `docker-compose.yml` (Postgres `16-alpine`)    |

---

## Prerequisites

- **Go toolchain.** The build and tests were run with the toolchain from the `golang:1.27-alpine` image. `go.mod` declares `go 1.23` as the minimum version.
- **git and a C build toolchain.** On Alpine, install these with the `git` and `build-base` packages.
- **PostgreSQL**, reachable from where you run the server. `docker-compose.yml` uses `postgres:16-alpine`.
- **Docker with Compose** (optional), only if you want to run the containerised setup.

---

## Database

| | Database |
|---|---|
| **Now** | PostgreSQL only. The connection is configured through `DATABASE_URL`, which must be a PostgreSQL DSN. |
| **Original** | SQLite (a local database file, with an empty username and password). |

**Why it changed:** the migrated project targets PostgreSQL. As a result, the SQLite file path was replaced by a DSN (see the note on `Config.DatabaseURL` in `internal/config/config.go`).

**Schema creation is automatic.** On startup the server runs `store.EnsureSchema` (`internal/store/schema.go`), which executes `CREATE TABLE IF NOT EXISTS` for these tables:

- `users`
- `articles`
- `article_favorites`
- `follows`
- `tags`
- `article_tags`
- `comments`

These statements mirror `src/main/resources/db/migration/V1__create_tables.sql`. No separate migration step is needed. The database itself must already exist.

---

## Getting started

Run all commands from the root of a fresh clone.

### 1. Install dependencies and build

On Alpine (as in the build image), install the system packages first:

```sh
apk add --no-cache git build-base
```

Then download the Go modules and build the server:

```sh
go mod download
go mod tidy
go build -o bin/server ./cmd/server
```

### 2. Configure the environment

`DATABASE_URL` is required. Point it at an existing PostgreSQL database. Set `JWT_SECRET` too, because the built-in default is a secret that was committed to source control.

```sh
export DATABASE_URL="postgres://app:app@localhost:5432/app?sslmode=disable"
export JWT_SECRET="change-me"
export PORT=8080
```

All variables are listed in [Environment variables](#environment-variables).

### 3. Database setup

No manual step is required. Tables are created on startup (see [Database](#database)).

### 4. Run

```sh
./bin/server
```

The server listens on **port 8080** and logs `server started on :8080`. It shuts down gracefully, with a 10-second timeout, on SIGINT or SIGTERM.

### 5. First requests

A liveness probe that needs no database data:

```sh
curl http://localhost:8080/healthz
```

The following requests were verified to get the same answer from the migrated server as from the original:

```
GET /articles/feed -> 200
GET /user -> 200
GET /tags -> 200
GET /articles/how-to-build-a-rest-api-with-spring-boot -> 200
GET /articles/how-to-build-a-rest-api-with-spring-boot/comments -> 200
GET /profiles/migrade_b80f5fd587 -> 200
GET /articles/building-a-production-ready-rest-api-with-spring-boot -> 200
GET /articles/building-a-production-ready-rest-api-with-spring-boot -> 404
```

The simplest one to try is:

```sh
curl http://localhost:8080/tags
```

Keep these points in mind when trying the others:

- **Slugs and usernames:** they refer to data created during the verification run. A freshly created database is empty, so those article and profile requests will return 404 until matching data exists.
- **The two `building-a-production-ready…` entries:** the same request appears with both 200 and 404 because the result depends on whether the article exists at that moment.
- **`/user` and `/articles/feed`:** these are user-specific endpoints. Expect to send an authenticated request (a JWT issued by the server).

### Alternative: Docker Compose

```sh
docker compose up --build
```

This starts two services:

- **`postgres`** (`postgres:16-alpine`): user, password and database are all `app`. A health check gates app startup. Data is persisted in the `postgres-data` volume.
- **`app`**: built from `Dockerfile`. Port 8080 is published on the host as `localhost:8080`.

Compose sets the following variables:

| Variable | Value |
|---|---|
| `DATABASE_URL` | `postgres://app:app@postgres:5432/app?sslmode=disable` |
| `JWT_SECRET` | `test-secret-key-for-ci` |
| `PORT` | `8080` |

Compose also sets `DB_HOST`, `DB_NAME`, `DB_PASSWORD`, `DB_PORT`, `DB_USER`, `DB_USERNAME` and `POSTGRES_URL`. The configuration loader does not read any of these.

The Postgres service only `expose`s port 5432 to other containers. It is **not** published to the host, so a server binary running directly on your machine cannot use it as-is.

---

## Running tests

```sh
go test ./...
```

The unit tests passed in the migration run. That run had a PostgreSQL instance available, but the tests that depend on one have not been identified.

---

## Environment variables

All variables are read by `internal/config/config.go`. Defaults were taken from the original `application.properties` where it defined them.

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `DATABASE_URL` | **yes** | — | PostgreSQL DSN. Startup fails with `config: DATABASE_URL must be set` if it is empty. |
| `PORT` | no | `8080` | Parsed into the config, but `cmd/server/main.go` currently hard-codes `:8080`. See [Known limitations](#known-limitations). |
| `JWT_SECRET` | no (must not be empty) | A long secret copied from the original project | HMAC secret for signing and verifying tokens. **Override it everywhere except local development**, because the default was committed to source control. |
| `JWT_SESSION_TIME` | no | `86400` | Token lifetime in seconds. Must be a positive integer. |
| `IMAGE_DEFAULT` | no | `https://static.productionready.io/images/smiley-cyrus.jpg` | Avatar given to users who register without an image. |
| `DB_STATEMENT_TIMEOUT` | no | `3000` | Per-statement database timeout in seconds. Must be a positive integer. It is parsed and validated, but currently not applied to database calls (see [Known limitations](#known-limitations)). |
| `SQL_LOG_LEVEL` | no | `debug` | zerolog level for the data-access layer (`trace`, `debug`, `info`, `warn`, `error`, …). |

---

## Architecture overview

```
cmd/server/
  main.go          HTTP server on :8080, graceful shutdown on SIGINT/SIGTERM
  router.go        chi router, dependency wiring, password hasher,
                   read-side PostgreSQL adapters
internal/config/   environment-based configuration (replaces application.properties)
internal/core/     domain model: users, articles, comments, favorites, tokens,
                   authorization rules (authz.go), domain errors
internal/app/      application services: article/comment/profile/tag/user queries,
                   article commands, user service, validation, offset paging
                   (page.go, paging.go), date-time cursors, DTOs
internal/auth/     JWT service (jwt.go) and request-context user helpers (context.go)
internal/httpapi/  REST handlers (users, current user, profiles, articles,
                   favorites, comments, tags), error mapping, security middleware
internal/graphql/  GraphQL resolvers and mutations (see Known limitations)
internal/store/    write-side repositories (users, articles, favorites, comments),
                   schema bootstrap, UTC time codec
src/main/resources/db/migration/V1__create_tables.sql   original schema (reference)
src/main/resources/schema/schema.graphqls               original GraphQL schema
```

### Request flow

1. The chi router applies its `Logger` and `Recoverer` middleware.
2. `GET /healthz` is answered directly.
3. Every other path (`/*`) is passed to the API handler.
4. Each `internal/httpapi` handler registers its routes on a `net/http` `ServeMux`.
5. That mux is wrapped by `httpapi.SecurityChain`, which applies CORS, then JWT authentication, then authorization rules. A `subjectResolver` adapts `auth.JWTService` for this chain.

### Data access

- **Writes:** commands go through the repositories in `internal/store`.
- **Reads:** query services use small read adapters defined in `cmd/server/router.go`. These are `userReads`, `relationReads`, `favoriteReads`, `articleReads`, `commentReads` and `tagReads`.

  These adapters replace the MyBatis read services and XML mappers. Offset pagination uses `LIMIT`/`OFFSET`. Cursor pagination filters and orders on `updated_at` (articles) or `created_at` (comments).

- **Tags:** tags for article lists are loaded in a second query, after the page of article IDs has been selected.

### Startup sequence

1. Load the configuration.
2. Open the database and ping it. A failed ping only logs a warning.
3. Ensure the schema exists. A failure here is fatal.
4. Build the services and handlers.

---

## Migration notes

Changes from the original Spring codebase, each with its reason:

1. **Password updates are now hashed.** Updating a password through `PUT /user` or the GraphQL `updateUser` mutation now stores the hash. The original stored the new password in plain text, so that user could never log in again.
   - *Why:* Quirk Q1 in the plan. This is the one deliberate behaviour fix, and **the plan says it should be confirmed with the owner.**

2. **Personal feed pagination is fixed** (both the offset and cursor variants). The feed now paginates by article ID first and loads tags afterwards. Pages are no longer cut short, tag lists are no longer truncated, and the response shape is unchanged.
   - *Why:* Quirk Q4. The original applied `LIMIT` to joined tag rows, which truncated both tags and pages.

3. **GraphQL comment pagination** (`Article.comments`) now applies `LIMIT limit+1`.
   - *Why:* Quirk Q5. The original cursor query had no `LIMIT`, so it could return the wrong number of rows and wrong `hasNext`/`hasPrevious` values.

4. **Creating an article without `tagList`** (REST or GraphQL) now succeeds with an empty tag list instead of returning HTTP 500.
   - *Why:* Quirk Q6. The Java code threw a `NullPointerException` on a null `tagList`.

5. **A missing or blank article title** now returns a normal 422 `can't be empty` error instead of a 500 from the duplicate-title validator.
   - *Why:* Quirk Q7. The duplicate check is now skipped for blank titles, and the NotBlank rule reports the error instead.

6. **The GraphQL `feed` query without an authenticated user** now returns an authentication error (`errorType` `INTERNAL`) instead of crashing.
   - *Why:* Plan section ArticleQueryService. `FindUserFeedWithCursor(nil user)` returns `ErrUnauthenticated` instead of causing a `NullPointerException`.

7. **An empty GraphQL pagination cursor (`""`) is treated as no cursor.** Non-numeric cursors still produce an error.
   - *Why:* Go strings cannot be null, so `""` stands in for Java's `null` (plan Phase 3, `ParseDateTimeCursor`). The original threw `NumberFormatException` on `""`.

8. **Generic GraphQL errors** no longer carry a Java exception class-name prefix. They use a fixed message with `errorType` `INTERNAL`.
   - *Why:* Plan section GraphQL errors. There are no Java classes in the Go runtime.

9. **Multiple validation messages for one field** are emitted in a fixed order: NotBlank, then format, then duplicate. The original order was unspecified.
   - *Why:* Quirk Q9. Go emits messages in declaration order.

10. **MyBatis cache and statement-timeout settings are removed.** The 3000-second statement timeout is meant to be applied per request through a context timeout (`DB_STATEMENT_TIMEOUT`); in this branch that is not yet wired up.
    - *Why:* Plan Phase 0. These settings have no equivalent once MyBatis is removed.

11. **The Gradle `clean` task** (which deleted the SQLite database file) was planned to be replaced by a Makefile `clean` target.
    - *Why:* Plan Phase 0. Gradle is replaced by the Go toolchain.
    - No Makefile is included in this branch. See [Known limitations](#known-limitations).

12. **Spring HATEOAS support is dropped.**
    - *Why:* Plan section 4. The `spring-boot-starter-hateoas` dependency was unused.

13. **New endpoint: unauthenticated `GET /healthz`**, which returns `ok`.
    - *Why:* It serves as a liveness probe for running the Go binary under an orchestrator. This was not in the plan; it was inferred.

14. **The database changed from SQLite to PostgreSQL.** See [Database](#database).

15. **The password encoder changed.** BCrypt was replaced by an iterated, salted SHA-512 encoder (`passwordHasher` in `cmd/server/router.go`).
    - *Why:* `golang.org/x/crypto` (bcrypt) is not a module dependency.
    - Stored passwords use the format `sha512$<iterations>$<salt>$<hash>`, with 10000 iterations.

---

## Known limitations

### Source files intentionally not translated

There are 31 such files, all excluded per the agreed plan. Their behaviour was folded into other Go code.

| Original | Count | Where the behaviour went |
|---|---|---|
| MyBatis XML mappers | 11 | SQL and result maps were ported into Go row scanning. |
| MyBatis mapper interfaces: `UserMapper`, `ArticleMapper`, `CommentMapper`, `ArticleFavoriteMapper` | 4 | `internal/store` repositories. |
| MyBatis read services: `UserReadService`, `ArticleReadService`, `CommentReadService`, `TagReadService`, `ArticleFavoritesReadService`, `UserRelationshipQueryService` | 6 | Read-side SQL (see the next section for its actual location). |
| Constraint annotations: `DuplicatedArticleConstraint`, `DuplicatedEmailConstraint`, `DuplicatedUsernameConstraint` | 3 | Annotations have no Go meaning; only the validator logic survives. |
| `PageCursor`, `Node` | 2 | Collapsed into a single `Node` interface plus `FormatCursor`. |
| `JacksonCustomizations` | 1 | No serializer framework is needed; replaced by JSON time handling in Go. |
| `ArticleFavoriteCount` | 1 | Replaced by `map[string]int`. |
| `FieldErrorResource`, `ErrorResource`, `ErrorResourceSerializer` | 3 | `internal/httpapi/errors.go` and the validation error type. |

### Gaps found in this branch

- **The read-side SQL is not where the plan says.** The plan names separate read-side files under `internal/store`. In this branch the read-side SQL lives in `cmd/server/router.go`.
- **GraphQL is not served.** `internal/graphql` contains resolvers and mutations with unit tests, but `cmd/server/router.go` mounts only the REST handlers from `internal/httpapi`. The GraphQL-related migration notes (items 3 and 6–8) describe that package's behaviour; no GraphQL endpoint is reachable from the running server.
- **`PORT` has no effect on the listen port.** `cmd/server/main.go` hard-codes `Addr: ":8080"` instead of using `Config.Addr()`.
- **Existing passwords from the original database will not work.** Hashes produced by the original BCrypt encoder do not match the SHA-512 encoder, so users imported from the original database cannot log in until their passwords are reset.
- **The default JWT secret is public.** It was copied from the original project, where it was committed to source control. Always set `JWT_SECRET` outside local development.
- **The statement timeout is not applied.** `DB_STATEMENT_TIMEOUT` is parsed into `Config.StatementTimeout`, but `cmd/server/router.go` never uses it: the repositories and read adapters are constructed with only the `*sql.DB`, so no per-request context timeout is applied to database calls.
- **No Makefile.** It was referenced by the plan for the `clean` target but is not present.
- **Startup tolerates an unreachable database at first.** A failed ping is only a warning. The subsequent schema creation then fails and the process exits.
- **The Docker build can change `go.mod`.** The `Dockerfile` runs `go mod tidy` during the build, so `go.mod`/`go.sum` inside the image can differ from the committed files.

---

## Manual review required

### Low-confidence modules

The migration flagged the following original modules as low confidence. Review their Go counterparts against the original behaviour.

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
- `internal/graphql/user.go`
- `internal/graphql/profile.go`
- `internal/graphql/relation_mutation.go`
- `internal/graphql/tags.go`
- `internal/graphql/user_mutation.go`
- `internal/graphql/errors.go`

**Application layer**
- `internal/app/article_query.go`
- `internal/app/article_command.go`
- `internal/app/new_article_param.go`
- `internal/app/user_service.go`
- `internal/app/dto.go`

**Domain and persistence**
- `internal/core/user.go`
- `internal/core/article.go`
- `internal/store/user_repo.go`

**Configuration**
- `src/main/resources/application.properties` (now `internal/config/config.go`)

### Additional items to verify

- **Password update fix (migration note 1):** confirm with the project owner that this deliberate behaviour change is wanted.
- **Password encoder** (`passwordHasher` in `cmd/server/router.go`): review whether iterated SHA-512 is acceptable in place of BCrypt, and plan how to handle existing BCrypt hashes.
- **Article command service wiring:** `app.NewArticleCommandService(articleRepo, nil)` in `cmd/server/router.go` passes `nil` as its second dependency. Confirm this is intended.
- **Read-side SQL in `cmd/server/router.go`:** check it against the original MyBatis mappers. This covers filtering by tag, author and favorited-by; cursor direction and ordering; and tag loading.
- **Statement timeout:** `Config.StatementTimeout` is currently not applied to database calls (migration note 10); decide whether to wire it in.
- **GraphQL:** decide whether to mount `internal/graphql` in `cmd/server/router.go`. If it is mounted, verify it against `src/main/resources/schema/schema.graphqls`.
- **Listen port:** decide whether `cmd/server/main.go` should use `Config.Addr()` so that `PORT` takes effect.
