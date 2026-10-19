# Spring Boot RealWorld: Go port

A Go implementation of the [RealWorld](https://github.com/gothinkster/realworld) "Conduit" backend API, migrated from the Spring Boot RealWorld example. The API covers users and authentication (JWT), profiles and follows, articles, favorites, comments and tags. Data is stored in PostgreSQL.

**Migration status:** 72 of 103 source modules were translated. The other 31 were intentionally not carried over, as agreed in the migration plan (see [Known limitations](#known-limitations)). The migration tool self-assessed its confidence at 95%. That figure is an estimate, not a measured result.

**Verified so far:**

| Check | Result |
|---|---|
| Build (`go build ./cmd/server`) | passed |
| Unit tests (`go test ./...`) | passed |
| Requests against the running Go server (the requests listed in [First requests](#first-requests-to-try)) | returned the expected status codes |

---

## Tech stack

- **Language:** Go. `go.mod` declares `go 1.23`, and the project was built and tested with the `golang:1.27-alpine` toolchain.
- **HTTP routing:** [`github.com/go-chi/chi/v5`](https://github.com/go-chi/chi) for the outer router and middleware, and the standard library `net/http` `ServeMux` for API handlers.
- **Database:** PostgreSQL through `database/sql` and [`github.com/lib/pq`](https://github.com/lib/pq).
- **Configuration:** [`github.com/spf13/viper`](https://github.com/spf13/viper), which reads environment variables only.
- **Logging:** [`github.com/rs/zerolog`](https://github.com/rs/zerolog).
- **Auth:** JWT, implemented in `internal/auth/jwt.go`.
- **Container:** `Dockerfile` and `docker-compose.yml`, which run the app plus `postgres:16-alpine`.

## Prerequisites

- Go toolchain 1.27, the same version as the `golang:1.27-alpine` image used for the verified build.
- `git` and a C build toolchain. The Alpine build installs both through `build-base`. On other systems, install your distribution's equivalent (`gcc`, `make`, …).
- A reachable PostgreSQL server. The compose setup uses PostgreSQL 16.
- `psql`, if you apply the schema the way shown below.
- Optional: Docker with the Compose plugin, for the containerised setup.

---

## Database

| | Database |
|---|---|
| **Original (Spring)** | SQLite (a local database file, empty username/password), accessed through MyBatis |
| **Migrated (Go)** | PostgreSQL only, accessed through `database/sql` and `lib/pq` |

**Why it changed:** the migrated project targets PostgreSQL, so the SQLite file path was replaced by a PostgreSQL DSN (`DATABASE_URL`). This is recorded as a `MIGRATION_NOTE` in `internal/config/config.go`. The queries use PostgreSQL-specific features, such as `$n` placeholders and `= ANY($1)` with array parameters, so SQLite is no longer supported.

**Schema:** the table definitions are in `src/main/resources/db/migration/V1__create_tables.sql`. The file creates these tables: `users`, `articles`, `article_favorites`, `follows`, `tags`, `article_tags` and `comments`.

**Schema caveats:**

- The server does not run migrations in `cmd/server/main.go` or `cmd/server/router.go`.
- `docker-compose.yml` does not mount the schema into the Postgres container.
- At startup the server only pings the database. If the ping fails, it logs a warning and keeps running.

---

## Getting started

Run all commands from the root of a fresh clone.

### 1. Install dependencies and build

```sh
go mod download
go mod tidy
go build -o bin/server ./cmd/server
```

This produces the binary `bin/server`. `go mod tidy` was part of the verified build. It may rewrite `go.mod`/`go.sum` if they are out of date.

### 2. Configure the environment

`DATABASE_URL` is required. Without it, the server exits at startup with `config: DATABASE_URL must be set`. The value below matches the credentials used in `docker-compose.yml`. Adjust the host, port and credentials to your PostgreSQL server.

```sh
export DATABASE_URL="postgres://app:app@localhost:5432/app?sslmode=disable"
export JWT_SECRET="change-me"
export PORT=8080
```

See [Environment variables](#environment-variables) for all options. Note that `PORT` currently has no effect on the listen address.

### 3. Set up the database

The migration run recorded no database setup command. Create the tables from the bundled schema before the first start. This `psql` invocation is a suggestion and was not part of the recorded run:

```sh
psql "$DATABASE_URL" -f src/main/resources/db/migration/V1__create_tables.sql
```

### 4. Run

```sh
./bin/server
```

The server listens on **port 8080** (`http://localhost:8080`). Press Ctrl+C or send SIGTERM to stop it. Shutdown is graceful, with a 10-second timeout.

Health check (unauthenticated):

```sh
curl http://localhost:8080/healthz
# ok
```

### First requests to try

The following requests were sent to the running Go server and returned these status codes:

| Request | Status |
|---|---|
| `GET /articles/feed` | 200 |
| `GET /articles` | 200 |
| `GET /user` | 200 |
| `GET /tags` | 200 |
| `GET /articles/1` | 404 |
| `GET /articles/1/comments` | 404 |
| `GET /profiles/1` | 404 |
| `POST /users` | 201 |

For example:

```sh
curl -i http://localhost:8080/tags
curl -i http://localhost:8080/articles
```

### Alternative: Docker Compose

```sh
docker compose up --build
```

This builds the image from `Dockerfile` and starts two services:

- **`app`:** published on `localhost:8080`, configured with `DATABASE_URL=postgres://app:app@postgres:5432/app?sslmode=disable` and `JWT_SECRET=test-secret-key-for-ci`.
- **`postgres`:** `postgres:16-alpine`, with user, password and database all set to `app`, and data in the `postgres-data` volume. The app waits for its health check to pass.

Notes on the compose setup:

- Port 5432 is only `expose`d, not published to the host. A `bin/server` running on the host therefore cannot reach the compose database unless you add a `ports:` mapping.
- The schema is not applied automatically. Apply it to the compose database yourself once the services are up (not part of the recorded run):

  ```sh
  docker compose exec -T postgres psql -U app -d app < src/main/resources/db/migration/V1__create_tables.sql
  ```

- Compose also sets `DB_HOST`, `DB_NAME`, `DB_PASSWORD`, `DB_PORT`, `DB_USER`, `DB_USERNAME` and `POSTGRES_URL`. `internal/config/config.go` reads none of them; only `DATABASE_URL` is used.
- `JWT_SECRET` in the compose file is a test value. Do not use it in production.

---

## Running tests

```sh
go test ./...
```

This passed during migration. The run had a PostgreSQL server reachable as `postgres`. Whether any tests, for example in `internal/store`, need a live database is not documented. If tests fail with connection errors, provide a database.

---

## Environment variables

All variables are read by `internal/config/config.go`.

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | **yes** | none | PostgreSQL DSN. Startup fails if it is missing or blank. |
| `JWT_SECRET` | no (but override it) | A secret copied from the original `application.properties`, committed to source control | HMAC secret for signing and verifying JWTs. An empty value is rejected. **Always set this outside development.** |
| `PORT` | no | `8080` | Parsed by `internal/config/config.go`, but **currently ignored**: `cmd/server/main.go` hard-codes `Addr: ":8080"`. |
| `JWT_SESSION_TIME` | no | `86400` | Token lifetime in seconds. Must be a positive integer. |
| `IMAGE_DEFAULT` | no | `https://static.productionready.io/images/smiley-cyrus.jpg` | Avatar assigned to users who register without an image. |
| `DB_STATEMENT_TIMEOUT` | no | `3000` | Per-statement database timeout in seconds. Must be a positive integer. See [Manual review](#manual-review-required). |
| `SQL_LOG_LEVEL` | no | `debug` | zerolog level for the data-access layer (`trace`, `debug`, `info`, `warn`, `error`, …). Invalid values fail startup. |

---

## Architecture overview

```
cmd/server/
  main.go          HTTP server on :8080, graceful shutdown on SIGINT/SIGTERM
  router.go        chi router, middleware, /healthz, dependency wiring,
                   read-side PostgreSQL adapters, password hasher
internal/
  config/          Environment-based configuration (replaces application.properties)
  core/            Domain model: users, articles, comments, favorites, tokens,
                   authorization rules (authz.go), domain errors
  app/             Application services: article/comment/profile/tag/user queries,
                   article commands, user service, validation, paging and
                   date-time cursors, DTOs
  auth/            JWT service (jwt.go) and request-context user (context.go)
  httpapi/         REST handlers (users, current user, profiles, articles,
                   article, favorites, comments, tags), security middleware,
                   error rendering
  graphql/         GraphQL resolvers and mutations (articles, comments, profiles,
                   relations, tags, users) and GraphQL error mapping
  store/           PostgreSQL write-side repositories (users, articles,
                   comments, favorites) and a UTC time codec
src/main/resources/
  db/migration/V1__create_tables.sql   Database schema
  schema/schema.graphqls               GraphQL schema carried over from the original
```

**Request flow:**

1. The chi router applies the `Logger` and `Recoverer` middleware.
2. `/healthz` is served directly.
3. Every other path goes to an `http.ServeMux` holding all `internal/httpapi` handlers.
4. That mux is wrapped in `httpapi.SecurityChain`, which runs CORS, then JWT authentication, then authorization rules.

**Read/write split:**

- **Writes** go through the repositories in `internal/store`.
- **Reads** go through small `database/sql` adapters defined in `cmd/server/router.go`: `userReads`, `relationReads`, `favoriteReads`, `articleReads`, `commentReads` and `tagReads`. These replace the original MyBatis read services and mapper XML.
- **Application layer:** the services in `internal/app` sit between the handlers and these adapters.

**Pagination:** offset paging uses `LIMIT/OFFSET`. Cursor paging uses `updated_at` for articles and `created_at` for comments.

---

## Migration notes

These are the changes from the original Spring codebase, with the reason for each.

### Behaviour changes

1. **Password updates are hashed.**
   - **Change:** a password changed through `PUT /user` or the GraphQL `updateUser` mutation is now stored hashed.
   - **Reason:** the original stored the new password in plain text, so that user could never log in again (Quirk Q1). This is the one deliberate behaviour fix, and the plan says it should be confirmed with the project owner.
2. **Personal feed pagination is fixed.**
   - **Change:** the feed, in both its offset and cursor variants, now paginates by article ID first and loads tags afterwards. The response shape is unchanged.
   - **Reason:** the original applied `LIMIT` to joined tag rows. This cut pages short and truncated tag lists (Quirk Q4).
3. **GraphQL comment pagination uses a limit.**
   - **Change:** `Article.comments` now applies `LIMIT limit+1`.
   - **Reason:** the original comment cursor query had no `LIMIT`. It could return the wrong number of rows and wrong `hasNext`/`hasPrevious` values (Quirk Q5).
4. **Articles without `tagList` succeed.**
   - **Change:** creating an article with no `tagList`, through REST or GraphQL, succeeds with an empty tag list.
   - **Reason:** the Java code threw a `NullPointerException` on a null `tagList`, which produced HTTP 500 (Quirk Q6).
5. **Missing or blank article titles return 422.**
   - **Change:** these requests now get a normal 422 "can't be empty" error.
   - **Reason:** the original returned 500 from the duplicate-title validator. The duplicate check is now skipped for blank titles, and the NotBlank check reports the error instead (Quirk Q7).
6. **Unauthenticated GraphQL `feed` returns an error.**
   - **Change:** the query returns an authentication error with `errorType INTERNAL`.
   - **Reason:** the original crashed with a `NullPointerException`. `FindUserFeedWithCursor(nil user)` now returns `ErrUnauthenticated`.
7. **An empty GraphQL cursor string means "no cursor".**
   - **Change:** `""` is treated as no cursor. Non-numeric cursors still produce an error.
   - **Reason:** the original threw `NumberFormatException` on `""`. Go strings cannot be null, so `""` stands in for Java's `null` (plan Phase 3, `ParseDateTimeCursor`).
8. **Generic GraphQL errors have a fixed message.**
   - **Change:** they use a fixed message with `errorType INTERNAL` and no Java exception class-name prefix.
   - **Reason:** there are no Java classes in the Go runtime.
9. **Validation messages come in a fixed order.**
   - **Change:** multiple messages for one field are always emitted as NotBlank, then format, then duplicate.
   - **Reason:** Go emits them in declaration order. In the original the order was unspecified (Quirk Q9).
10. **New endpoint: `GET /healthz`.**
    - **Change:** an unauthenticated endpoint that returns `ok`.
    - **Reason:** a liveness probe for running the binary under an orchestrator. The plan does not mention it; it was inferred.

### Infrastructure changes

- **MyBatis cache and statement-timeout settings are removed.**
  - **Change:** the 3000 s statement timeout is meant to be applied per request through a context timeout (`DB_STATEMENT_TIMEOUT`).
  - **Reason:** MyBatis-specific settings have no equivalent once MyBatis is removed (plan Phase 0).
- **The Gradle `clean` task is replaced by a Makefile `clean` target.**
  - **Change:** the Gradle task deleted the SQLite database file; the plan replaces it with a Makefile target.
  - **Reason:** Gradle is replaced by the Go toolchain (plan Phase 0).
  - **Note:** no Makefile exists in this repository yet, so this target is not available.
- **Spring HATEOAS is dropped.**
  - **Reason:** the `spring-boot-starter-hateoas` dependency was unused (plan section 4).
- **SQLite is replaced by PostgreSQL.** See [Database](#database).

---

## Known limitations

- **GraphQL is not served.**
  - `internal/graphql` and `src/main/resources/schema/schema.graphqls` exist, but `cmd/server/router.go` mounts only the REST handlers.
  - No GraphQL endpoint is reachable from the running server.
  - The GraphQL-related migration notes above describe the code in `internal/graphql`, not a live endpoint.
- **Existing password hashes are incompatible.**
  - Passwords are hashed with an iterated, salted SHA-512 scheme (`passwordHasher` in `cmd/server/router.go`), because `golang.org/x/crypto/bcrypt` is not a dependency.
  - BCrypt hashes produced by the original Java app will not match, so users migrated from an existing database cannot log in until their passwords are reset.
- **`PORT` is ignored.** The listen address is hard-coded to `:8080` in `cmd/server/main.go`.
- **The schema is not applied automatically.** Neither the binary nor `docker-compose.yml` applies `V1__create_tables.sql`.
- **A database ping failure is not fatal.** The server starts anyway and logs `database ping failed; continuing`. Requests then fail at query time.
- **The documented Makefile `clean` target does not exist.**
- **Some modules were intentionally not translated.** These 31 source files were left out per the agreed plan; none were deemed unmigrable:
  - **10 MyBatis mapper XML files.** Their SQL is ported into `internal/store`.
  - **A MyBatis result-map XML file.** Its result maps are ported as row scanning in Go.
  - **4 MyBatis mapper interfaces.** Their SQL is ported into `internal/store/comment_repo.go`, `favorite_repo.go`, `user_repo.go` and `article_repo.go`.
  - **MyBatis read services** (`UserRelationshipQueryService`, `TagReadService`, `UserReadService`, `ArticleFavoritesReadService`, `ArticleReadService`, `CommentReadService`).
    - The plan assigned their SQL to `internal/store`.
    - In this tree, the read-side SQL is in the adapters in `cmd/server/router.go`.
  - **3 constraint annotations** (`DuplicatedArticleConstraint`, `DuplicatedEmailConstraint`, `DuplicatedUsernameConstraint`). They have no Go meaning; only the validator logic survives.
  - **The 2 page-cursor classes.** The abstract hierarchy is collapsed into a single `Node` interface (`Cursor() time.Time`) plus `FormatCursor`.
  - **The Jackson customization class.**
    - No serializer framework is needed.
    - The plan moves this behaviour into a JSON time type in `internal/app`, but the file the plan names is not present in this tree.
  - **The favorite-count class.** Replaced by `map[string]int`.
  - **The 3 error-resource classes.** Their behaviour moves into `internal/httpapi/errors.go`.

---

## Manual review required

### Issues found in the Go code

- **Q1 password hashing:** confirm the behaviour change with the project owner, as the plan requires.
- **Statement timeout:**
  - `cmd/server/router.go` loads `cfg.StatementTimeout` but never passes it anywhere.
  - Verify that the 3000 s per-request context timeout is actually applied.
  - Likewise, `cfg.SQLLogLevel` is parsed but not visibly used.
- **`PORT` wiring:** decide whether `main.go` should use `cfg.Addr()` instead of the hard-coded `":8080"`.
- **`app.NewArticleCommandService(articleRepo, nil)`** in `cmd/server/router.go`: verify that passing `nil` as the second argument is intended and safe.
- **GraphQL:** decide whether and how to mount `internal/graphql`.
- **Password hashing:** decide between keeping the SHA-512 scheme and adding bcrypt for compatibility with existing data.
- **Schema provisioning:** decide how `V1__create_tables.sql` should be applied in each environment.

### Low-confidence modules

The migration flagged these original Spring source files as low confidence. Their Go counterparts are listed below; review them against the original behaviour. `src/main/resources/application.properties` refers to the original repository and does not exist in this one.

**Configuration and security**
- `src/main/resources/application.properties`
- `internal/httpapi/middleware.go`

**REST API and error handling**
- `internal/httpapi/users.go`
- `internal/httpapi/current_user.go`
- `internal/httpapi/articles.go`
- `internal/httpapi/article.go`
- `internal/httpapi/comments.go`
- `internal/httpapi/errors.go`
- `internal/httpapi/no_authorization.go`

**GraphQL**
- `internal/graphql/article.go`
- `internal/graphql/article_mutation.go`
- `internal/graphql/comment.go`
- `internal/graphql/profile.go`
- `internal/graphql/relation_mutation.go`
- `internal/graphql/user.go`
- `internal/graphql/tags.go`
- `internal/graphql/user_mutation.go`
- `internal/graphql/errors.go`

**Application layer**
- `internal/app/article_query.go`
- `internal/app/paging.go`
- `internal/app/dto.go`
- `internal/app/article_command.go`
- `internal/app/new_article_param.go`
- `internal/app/user_service.go`
- `internal/app/update_user_param.go`
- `internal/app/register_validation.go`
- `internal/app/validate_user.go`

**Domain and persistence**
- `internal/core/user.go`
- `internal/core/article.go`
- `internal/core/comment.go`
- `internal/store/user_repo.go`
