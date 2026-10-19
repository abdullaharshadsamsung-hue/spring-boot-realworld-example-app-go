# Migration Notes

**Model self-assessed confidence:** 90% (not a measured result)  
**Build at time of writing:** passed  
**Target unit tests:** failed  
**Behavior compared with the original:** not run

The final recommendation is in the pull request description.

---

## What was migrated

- `src/main/java/io/spring/Util.java` → `internal/core/strings.go` (92% confidence)
- `src/main/java/io/spring/core/favorite/ArticleFavorite.java` → `internal/core/favorite.go` (93% confidence)
- `src/main/java/io/spring/core/article/Tag.java` → `internal/core/article.go` (89% confidence)
- `src/main/java/io/spring/core/comment/Comment.java` → `internal/core/comment.go` (89% confidence)
- `src/main/java/io/spring/application/data/ProfileData.java` → `internal/app/dto.go` (84% confidence) ⚠️ needs review
- `src/main/java/io/spring/application/Page.java` → `internal/app/paging.go` (94% confidence)
- `src/main/java/io/spring/application/article/UpdateArticleParam.java` → `internal/app/article_command.go` (89% confidence)
- `src/main/java/io/spring/api/exception/ResourceNotFoundException.java` → `internal/httpapi/errors.go` (85% confidence)
- `src/main/java/io/spring/application/user/UpdateUserParam.java` → `internal/app/user_service.go` (86% confidence)
- `src/main/java/io/spring/application/data/UserData.java` → `internal/app/user_data.go` (89% confidence)
- `src/main/java/io/spring/core/user/FollowRelation.java` → `internal/core/user.go` (93% confidence)
- `src/main/resources/application.properties` → `internal/config/config.go` (78% confidence) ⚠️ needs review
- `src/main/java/io/spring/api/exception/NoAuthorizationException.java` → `internal/httpapi/no_authorization.go` (77% confidence) ⚠️ needs review
- `src/main/java/io/spring/infrastructure/mybatis/DateTimeHandler.java` → `internal/store/timecodec.go` (85% confidence)
- `src/main/java/io/spring/api/exception/InvalidAuthenticationException.java` → `internal/httpapi/errors.go` (87% confidence)
- `src/main/java/io/spring/graphql/exception/AuthenticationException.java` → `internal/core/errors.go` (87% confidence)
- `src/main/java/io/spring/api/exception/InvalidRequestException.java` → `internal/httpapi/errors.go` (72% confidence) ⚠️ needs review
- `src/main/java/io/spring/core/user/User.java` → `internal/core/user.go` (76% confidence) ⚠️ needs review
- `src/main/java/io/spring/core/article/Article.java` → `internal/core/article.go` (75% confidence) ⚠️ needs review
- `src/main/java/io/spring/core/favorite/ArticleFavoriteRepository.java` → `internal/core/favorite_repository.go` (91% confidence)
- `src/main/java/io/spring/core/comment/CommentRepository.java` → `internal/core/comment.go` (75% confidence) ⚠️ needs review
- `src/main/java/io/spring/application/data/UserWithToken.java` → `internal/app/user_with_token.go` (91% confidence)
- `src/main/java/io/spring/application/DateTimeCursor.java` → `internal/app/datetime_cursor.go` (89% confidence)
- `src/main/java/io/spring/graphql/exception/GraphQLCustomizeExceptionHandler.java` → `internal/graphql/errors.go` (80% confidence) ⚠️ needs review
- `src/main/java/io/spring/core/service/JwtService.java` → `internal/core/token.go` (87% confidence)
- `src/main/java/io/spring/application/TagsQueryService.java` → `internal/app/tags_query.go` (91% confidence)
- `src/main/java/io/spring/application/user/UpdateUserCommand.java` → `internal/app/user_service.go` (66% confidence) ⚠️ needs review
- `src/main/java/io/spring/core/user/UserRepository.java` → `internal/core/user.go` (68% confidence) ⚠️ needs review
- `src/main/java/io/spring/graphql/SecurityUtil.java` → `internal/auth/context.go` (91% confidence)
- `src/main/java/io/spring/core/service/AuthorizationService.java` → `internal/core/authz.go` (94% confidence)
- `src/main/java/io/spring/core/article/ArticleRepository.java` → `internal/core/article.go` (66% confidence) ⚠️ needs review
- `src/main/java/io/spring/application/CursorPager.java` → `internal/app/paging.go` (74% confidence) ⚠️ needs review
- `src/main/java/io/spring/infrastructure/repository/MyBatisCommentRepository.java` → `internal/store/comment_repo.go` (88% confidence)
- `src/main/java/io/spring/application/ProfileQueryService.java` → `internal/app/profile_query.go` (90% confidence)
- `src/main/java/io/spring/application/data/ArticleData.java` → `internal/app/dto.go` (72% confidence) ⚠️ needs review
- `src/main/java/io/spring/infrastructure/repository/MyBatisArticleFavoriteRepository.java` → `internal/store/favorite_repo.go` (92% confidence)
- `src/main/java/io/spring/application/UserQueryService.java` → `internal/app/user_query.go` (90% confidence)
- `src/main/java/io/spring/api/TagsApi.java` → `internal/httpapi/tags.go` (92% confidence)
- `src/main/java/io/spring/application/data/CommentData.java` → `internal/app/comment_data.go` (85% confidence)
- `src/main/java/io/spring/graphql/TagDatafetcher.java` → `internal/graphql/tags.go` (83% confidence) ⚠️ needs review
- `src/main/java/io/spring/api/security/JwtTokenFilter.java` → `internal/httpapi/middleware.go` (89% confidence)
- `src/main/java/io/spring/infrastructure/service/DefaultJwtService.java` → `internal/auth/jwt.go` (87% confidence)
- `src/main/java/io/spring/application/CursorPageParameter.java` → `internal/app/paging.go` (72% confidence) ⚠️ needs review
- `src/main/java/io/spring/infrastructure/repository/MyBatisUserRepository.java` → `internal/store/user_repo.go` (84% confidence) ⚠️ needs review
- `src/main/java/io/spring/application/data/ArticleDataList.java` → `internal/app/article_data_list.go` (91% confidence)
- `src/main/java/io/spring/graphql/ProfileDatafetcher.java` → `internal/graphql/profile.go` (83% confidence) ⚠️ needs review
- `src/main/java/io/spring/api/ProfileApi.java` → `internal/httpapi/profiles.go` (86% confidence)
- `src/main/java/io/spring/infrastructure/repository/MyBatisArticleRepository.java` → `internal/store/article_repo.go` (88% confidence)
- `src/main/java/io/spring/graphql/RelationMutation.java` → `internal/graphql/relation_mutation.go` (84% confidence) ⚠️ needs review
- `src/main/java/io/spring/application/article/NewArticleParam.java` → `internal/app/article_command.go` (72% confidence) ⚠️ needs review
- `src/main/java/io/spring/api/security/WebSecurityConfig.java` → `internal/httpapi/middleware.go` (65% confidence) ⚠️ needs review
- `src/main/java/io/spring/graphql/MeDatafetcher.java` → `internal/graphql/user.go` (83% confidence) ⚠️ needs review
- `src/main/java/io/spring/application/ArticleQueryService.java` → `internal/app/article_query.go` (78% confidence) ⚠️ needs review
- `src/main/java/io/spring/application/article/DuplicatedArticleValidator.java` → `internal/app/validate_article.go` (86% confidence)
- `src/main/java/io/spring/api/ArticleFavoriteApi.java` → `internal/httpapi/favorites.go` (88% confidence)
- `src/main/java/io/spring/application/CommentQueryService.java` → `internal/app/comment_query.go` (86% confidence)
- `src/main/java/io/spring/graphql/ArticleDatafetcher.java` → `internal/graphql/article.go` (78% confidence) ⚠️ needs review
- `src/main/java/io/spring/graphql/CommentDatafetcher.java` → `internal/graphql/comment.go` (84% confidence) ⚠️ needs review
- `src/main/java/io/spring/api/CommentsApi.java` → `internal/httpapi/comments.go` (78% confidence) ⚠️ needs review
- `src/main/java/io/spring/application/user/RegisterParam.java` → `internal/app/user_service.go` (81% confidence) ⚠️ needs review
- `src/main/java/io/spring/application/article/ArticleCommandService.java` → `internal/app/article_command.go` (65% confidence) ⚠️ needs review
- `src/main/java/io/spring/graphql/CommentMutation.java` → `internal/graphql/comment_mutation.go` (85% confidence)
- `src/main/java/io/spring/application/user/DuplicatedEmailValidator.java` → `internal/app/validate_user.go` (91% confidence)
- `src/main/java/io/spring/graphql/ArticleMutation.java` → `internal/graphql/article_mutation.go` (83% confidence) ⚠️ needs review
- `src/main/java/io/spring/api/exception/CustomizeExceptionHandler.java` → `internal/httpapi/errors.go` (69% confidence) ⚠️ needs review
- `src/main/java/io/spring/application/user/DuplicatedUsernameValidator.java` → `internal/app/validate_user.go` (73% confidence) ⚠️ needs review
- `src/main/java/io/spring/application/user/UserService.java` → `internal/app/user_service.go` (68% confidence) ⚠️ needs review
- `src/main/java/io/spring/graphql/UserMutation.java` → `internal/graphql/user_mutation.go` (78% confidence) ⚠️ needs review
- `src/main/java/io/spring/api/UsersApi.java` → `internal/httpapi/users.go` (76% confidence) ⚠️ needs review
- `src/main/java/io/spring/api/CurrentUserApi.java` → `internal/httpapi/current_user.go` (83% confidence) ⚠️ needs review
- `src/main/java/io/spring/api/ArticlesApi.java` → `internal/httpapi/articles.go` (81% confidence) ⚠️ needs review
- `src/main/java/io/spring/api/ArticleApi.java` → `internal/httpapi/article.go` (80% confidence) ⚠️ needs review

## Not translated, and why

**Not migrated, per the agreed plan: All mapper/*.xml files are not carried over; their SQL is ported into internal/store.**
- `src/main/resources/mapper/UserMapper.xml`
- `src/main/resources/mapper/ArticleMapper.xml`
- `src/main/resources/mapper/CommentMapper.xml`
- `src/main/resources/mapper/ArticleFavoriteMapper.xml`
- `src/main/resources/mapper/UserReadService.xml`
- `src/main/resources/mapper/UserRelationshipQueryService.xml`
- `src/main/resources/mapper/ArticleFavoritesReadService.xml`
- `src/main/resources/mapper/TagReadService.xml`
- `src/main/resources/mapper/ArticleReadService.xml`
- `src/main/resources/mapper/CommentReadService.xml`

**Not migrated, per the agreed plan: Annotation type with no Go meaning; only the validator logic survives.**
- `src/main/java/io/spring/application/article/DuplicatedArticleConstraint.java`
- `src/main/java/io/spring/application/user/DuplicatedEmailConstraint.java`
- `src/main/java/io/spring/application/user/DuplicatedUsernameConstraint.java`

**Not migrated, per the agreed plan: No serializer framework needed; its behaviour moves into JSONTime in app/jsontime.go.**
- `src/main/java/io/spring/JacksonCustomizations.java`

**Not migrated, per the agreed plan: Abstract hierarchy collapsed into the Node interface plus FormatCursor.**
- `src/main/java/io/spring/application/PageCursor.java`

**Not migrated, per the agreed plan: Replaced by map[string]int.**
- `src/main/java/io/spring/application/data/ArticleFavoriteCount.java`

**Not migrated, per the agreed plan: MyBatis read-service interfaces are not carried over as files; their SQL is ported into internal/store (relation_read.go).**
- `src/main/java/io/spring/infrastructure/mybatis/readservice/UserRelationshipQueryService.java`

**Not migrated, per the agreed plan: MyBatis *ReadService interfaces are not carried over as files; their SQL is ported into internal/store (tag_read.go).**
- `src/main/java/io/spring/infrastructure/mybatis/readservice/TagReadService.java`

**Not migrated, per the agreed plan: Not needed as a class; its behaviour moves into httpapi/errors.go and ValidationError.**
- `src/main/java/io/spring/api/exception/FieldErrorResource.java`

**Not migrated, per the agreed plan: MyBatis mapper interfaces are not carried over as files; their SQL is ported into internal/store (comment_repo.go).**
- `src/main/java/io/spring/infrastructure/mybatis/mapper/CommentMapper.java`

**Not migrated, per the agreed plan: Abstract hierarchy collapsed into a single Node interface (Cursor() time.Time).**
- `src/main/java/io/spring/application/Node.java`

**Not migrated, per the agreed plan: MyBatis mapper interfaces are not carried over as files; their SQL is ported into internal/store (favorite_repo.go).**
- `src/main/java/io/spring/infrastructure/mybatis/mapper/ArticleFavoriteMapper.java`

**Not migrated, per the agreed plan: MyBatis *ReadService interfaces are not carried over as files; their SQL is ported into internal/store (user_read.go).**
- `src/main/java/io/spring/infrastructure/mybatis/readservice/UserReadService.java`

**Not migrated, per the agreed plan: MyBatis *ReadService interfaces are not carried over as files; their SQL is ported into internal/store (favorite_read.go).**
- `src/main/java/io/spring/infrastructure/mybatis/readservice/ArticleFavoritesReadService.java`

**Not migrated, per the agreed plan: MyBatis mapper interfaces are not carried over as files; their SQL is ported into internal/store (user_repo.go).**
- `src/main/java/io/spring/infrastructure/mybatis/mapper/UserMapper.java`

**Not migrated, per the agreed plan: MyBatis mapper interfaces are not carried over as files; their SQL is ported into internal/store (article_repo.go).**
- `src/main/java/io/spring/infrastructure/mybatis/mapper/ArticleMapper.java`

**Not migrated, per the agreed plan: All mapper/*.xml files are not carried over; its result maps are ported as row scanning in internal/store.**
- `src/main/resources/mapper/TransferData.xml`

**Not migrated, per the agreed plan: MyBatis *ReadService interfaces are not carried over as files; their SQL is ported into internal/store (article_read.go).**
- `src/main/java/io/spring/infrastructure/mybatis/readservice/ArticleReadService.java`

**Not migrated, per the agreed plan: MyBatis *ReadService interfaces are not carried over as files; their SQL is ported into internal/store (comment_read.go).**
- `src/main/java/io/spring/infrastructure/mybatis/readservice/CommentReadService.java`

**Not migrated, per the agreed plan: Not needed as a class; its behaviour moves into httpapi/errors.go.**
- `src/main/java/io/spring/api/exception/ErrorResource.java`

**Not migrated, per the agreed plan: No serializer framework needed; its behaviour moves into httpapi/errors.go.**
- `src/main/java/io/spring/api/exception/ErrorResourceSerializer.java`


## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `onException (DGS TypedGraphQLError / DefaultDataFetcherExceptionHandler integration)` in `src/main/java/io/spring/graphql/exception/GraphQLCustomizeExceptionHandler.java`
**Reason:** This is tied to the Netflix DGS and graphql-java exception-handling SPI and its typed-error extension format, which have no direct equivalent outside the JVM.
**Suggestion:** Manually rewrite it as the target GraphQL server's error formatter or middleware, and reproduce the extensions keys (errorType/classification plus the field->messages map) by hand.

### `ConstraintViolation handling` in `src/main/java/io/spring/graphql/exception/GraphQLCustomizeExceptionHandler.java`
**Reason:** It depends on the Bean Validation runtime model (ConstraintViolation, propertyPath, ConstraintDescriptor annotation type), which does not exist outside Java.
**Suggestion:** Map it from the target validation library's error objects (e.g. pydantic ValidationError.errors(), zod issues) to the same {field: [messages]} structure.

### `SecurityContextHolder / UsernamePasswordAuthenticationToken` in `src/main/java/io/spring/api/security/JwtTokenFilter.java`
**Reason:** Spring Security's thread-local security context and Authentication object model have no Go equivalent.
**Suggestion:** Manual rewrite: store the authenticated *User in the request context.Context under a private key type, and expose a CurrentUser(ctx) accessor. Replace Spring's authorization rules with an explicit RequireAuth middleware on protected routes.

### `OncePerRequestFilter / FilterChain registration` in `src/main/java/io/spring/api/security/JwtTokenFilter.java`
**Reason:** Servlet filter lifecycle and Spring Security filter chain ordering (addFilterBefore) are framework mechanics.
**Suggestion:** Rewrite as standard Go middleware wrapping the router, with ordering defined explicitly when the middleware stack is composed.

### `WebAuthenticationDetailsSource` in `src/main/java/io/spring/api/security/JwtTokenFilter.java`
**Reason:** Spring-specific details object (remote address, session id); it is not consumed by business logic.
**Suggestion:** Drop it, or optionally store r.RemoteAddr in the context if auditing is needed.

### `DGS annotations (@DgsComponent, @DgsData, @InputArgument)` in `src/main/java/io/spring/graphql/ProfileDatafetcher.java`
**Reason:** These are Netflix DGS framework-specific registration mechanisms with no direct equivalent outside the JVM DGS ecosystem.
**Suggestion:** Re-express them as a resolver map in the target GraphQL library, for example:
- Query.profile
- User.profile
- Article.author
- Comment.author

Suitable libraries include Apollo Server/graphql-js, Strawberry/Ariadne, gqlgen, async-graphql and Hot Chocolate.

### `DataFetchingEnvironment.getLocalContext usage` in `src/main/java/io/spring/graphql/ProfileDatafetcher.java`
**Reason:** graphql-java's localContext propagation from parent DataFetcherResult is a graphql-java-specific feature, and most other GraphQL servers lack an identical concept.
**Suggestion:** Manually rewrite. Either embed the needed data, such as the author username or ArticleData/CommentData, in the parent resolved object, or use a per-request context/DataLoader keyed by slug or comment id.

### `SecurityUtil.getCurrentUser` in `src/main/java/io/spring/graphql/ProfileDatafetcher.java`
**Reason:** It depends on Spring Security's thread-local SecurityContextHolder.
**Suggestion:** Populate the authenticated user (from the JWT) in the GraphQL request context in the target, and read it from the resolver's context argument.

### `@Transactional on save` in `src/main/java/io/spring/infrastructure/repository/MyBatisArticleRepository.java`
**Reason:** Spring's AOP proxy-based declarative transactions (propagation, automatic rollback rules, thread-bound SqlSession) have no Go equivalent.
**Suggestion:** Manual rewrite: explicit BeginTx/Commit/Rollback with defer, or a small helper `WithTx(ctx, db, func(tx DBTX) error) error`. Pass tx explicitly or via context if outer services need to share the transaction.

### `ArticleMapper (MyBatis mapper + XML result maps)` in `src/main/java/io/spring/infrastructure/repository/MyBatisArticleRepository.java`
**Reason:** MyBatis generates the mapper implementation at runtime from XML. The nested-collection result mapping and dynamic SQL are framework magic that does not exist in Go.
**Suggestion:** Manual rewrite: port each SQL statement from ArticleMapper.xml into hand-written queries with database/sql, sqlx, or sqlc (sqlc fits a fixed query set well). Implement tag aggregation by grouping join rows by article id.

### `@Repository exception translation` in `src/main/java/io/spring/infrastructure/repository/MyBatisArticleRepository.java`
**Reason:** Spring's PersistenceExceptionTranslationPostProcessor converts driver exceptions to DataAccessException hierarchy; Go has no such mechanism.
**Suggestion:** Wrap errors with %w and add explicit checks for driver error codes (e.g. pq/pgx unique_violation 23505, sqlite3 ErrConstraintUnique) where callers need to distinguish them.

### `WebSecurityConfig (WebSecurityConfigurerAdapter + HttpSecurity DSL)` in `src/main/java/io/spring/api/security/WebSecurityConfig.java`
**Reason:** The Spring Security filter chain, anonymous-authentication defaults, and the antMatchers first-match semantics are framework-specific. There is no 1:1 equivalent in other ecosystems.
**Suggestion:** Manually rewrite as target-framework middleware: a global CORS middleware with the exact settings above, a global optional-JWT middleware that sets the current user without rejecting, and either a central ordered rule table or per-route 'require auth' guards matching the rule list. Return a bare 401 when a guard fails.

### `getMe (SecurityContextHolder usage)` in `src/main/java/io/spring/graphql/MeDatafetcher.java`
**Reason:** Relies on Spring Security's thread-local SecurityContext populated by a servlet filter. This has no direct equivalent outside Spring.
**Suggestion:** Manually rewrite. Use auth middleware to decode the JWT and attach the user to the GraphQL request context, then read context.currentUser in the resolver.

### `DataFetcherResult.localContext propagation` in `src/main/java/io/spring/graphql/MeDatafetcher.java`
**Reason:** This is a graphql-java/DGS-specific mechanism for passing data from parent to child resolvers, and most other GraphQL server libraries do not offer it.
**Suggestion:** Return the domain user (or a wrapper containing it) as the parent object from mutation resolvers, and resolve UserPayload.user from the parent argument.

### `DuplicatedArticleValidator (Spring/Hibernate Validator integration via @Autowired field injection and SpringConstraintValidatorFactory)` in `src/main/java/io/spring/application/article/DuplicatedArticleValidator.java`
**Reason:** Go has no container that instantiates validators and injects beans into them, and no annotation-triggered validation on @Valid request bodies. The framework wiring cannot be translated directly; only the business rule can.
**Suggestion:** Manually rewrite as a plain struct with constructor-injected ArticleQueryService (or a narrower SlugExister interface). Either register it as a go-playground/validator custom tag via a closure using RegisterValidationCtx, or call it explicitly in the create-article service before insert. Back it with a DB UNIQUE constraint on slug and map violations to the RealWorld 422 error format.

### `DataFetcherResult.localContext usage (all resolvers)` in `src/main/java/io/spring/graphql/ArticleDatafetcher.java`
**Reason:** Relies on graphql-java's localContext propagation to child fields, which has no direct equivalent in most non-Java GraphQL libraries.
**Suggestion:** Manual rewrite: return internal parent objects carrying ArticleData/CommentData/domain Article, so child resolvers read from parent (root/obj) instead of localContext.

### `SecurityUtil.getCurrentUser()` in `src/main/java/io/spring/graphql/ArticleDatafetcher.java`
**Reason:** Depends on Spring Security's thread-local SecurityContextHolder.
**Suggestion:** Inject the authenticated user into the GraphQL request context in the target's auth middleware and read it from info.context/ctx.

### `DGS annotations (@DgsComponent/@DgsQuery/@DgsData/@InputArgument)` in `src/main/java/io/spring/graphql/ArticleDatafetcher.java`
**Reason:** Framework-specific resolver registration and argument binding.
**Suggestion:** Map to the target library's resolver registration (e.g. resolver maps keyed by Type.field), using the same SDL schema.

### `DataFetcherResult.localContext / DgsDataFetchingEnvironment.getLocalContext()` in `src/main/java/io/spring/graphql/CommentDatafetcher.java`
**Reason:** graphql-java-specific mechanism for passing hidden, per-subtree context from parent to child resolvers. Most other GraphQL server libraries have no direct equivalent.
**Suggestion:** Manual rewrite. Have parent resolvers return internal model objects that carry the hidden DTO (e.g., {slug, ..., _articleData}) for child resolvers to read from the parent/root argument. Alternatively use a request-scoped context cache or DataLoader keyed by ID. Apply the same approach across all related fetchers.

### `SecurityUtil.getCurrentUser()` in `src/main/java/io/spring/graphql/CommentDatafetcher.java`
**Reason:** Relies on Spring Security's thread-local SecurityContextHolder populated by a JWT filter. This does not carry over to async or non-Spring runtimes.
**Suggestion:** Populate the current user in the GraphQL request context via auth middleware and read it from the resolver's context argument.

### `@DgsComponent / @DgsData / @InputArgument annotations` in `src/main/java/io/spring/graphql/CommentDatafetcher.java`
**Reason:** Annotation-driven resolver registration and argument binding specific to Netflix DGS.
**Suggestion:** Register resolvers explicitly in the target GraphQL library's resolver map or schema definition, with arguments taken from the resolver's args parameter.

### `DataFetcherResult.localContext usage in createComment` in `src/main/java/io/spring/graphql/CommentMutation.java`
**Reason:** This is a graphql-java/DGS-specific mechanism for passing context to child data fetchers, and most other GraphQL libraries have no direct equivalent.
**Suggestion:** Manual rewrite: return a payload object that already contains the comment data (e.g. `{comment: commentData}`), or attach it to the parent object so the CommentPayload.comment resolver can read it.

### `SecurityUtil.getCurrentUser (thread-local SecurityContext)` in `src/main/java/io/spring/graphql/CommentMutation.java`
**Reason:** It relies on the Spring Security filter chain and SecurityContextHolder thread-local, which are not portable.
**Suggestion:** Manual rewrite: authenticate the JWT in request middleware, put the user into the GraphQL execution context, and read it from context in each resolver.

### `DuplicatedEmailValidator (annotation-driven invocation)` in `src/main/java/io/spring/application/user/DuplicatedEmailValidator.java`
**Reason:** The automatic invocation via the Bean Validation framework and Spring's SpringConstraintValidatorFactory autowiring has no Go equivalent. Validation cannot be triggered by an annotation that implicitly injects dependencies.
**Suggestion:** Rewrite manually. Use an explicit validator struct with the repository injected through the constructor, called from the registration handler or service. Alternatively, use go-playground/validator's RegisterValidation with a closure capturing the repository. The business logic itself (an empty email is valid, otherwise the email must not exist) translates directly.

### `DataFetcherResult.localContext propagation` in `src/main/java/io/spring/graphql/ArticleMutation.java`
**Reason:** This graphql-java/DGS-specific mechanism passes per-field context to child resolvers; most other GraphQL libraries have no identically named feature.
**Suggestion:** Manually rewrite: return the domain Article (or a wrapper holding it) as the parent object of ArticlePayload so the nested 'article' resolver receives it, or resolve the article eagerly in the mutation.

### `@DgsComponent/@DgsMutation/@InputArgument wiring` in `src/main/java/io/spring/graphql/ArticleMutation.java`
**Reason:** This annotation-based resolver registration and argument binding is DGS-specific framework magic.
**Suggestion:** Use the target GraphQL library's resolver registration (decorators, resolver maps, or codegen) with explicit argument parsing for 'input', 'slug' and 'changes'.

### `SecurityUtil.getCurrentUser` in `src/main/java/io/spring/graphql/ArticleMutation.java`
**Reason:** It depends on Spring Security's thread-local SecurityContextHolder.
**Suggestion:** Inject the authenticated user into the GraphQL execution context via JWT middleware and read it from the resolver's context/info argument.

### `CustomizeExceptionHandler (inherited ResponseEntityExceptionHandler defaults)` in `src/main/java/io/spring/api/exception/CustomizeExceptionHandler.java`
**Reason:** Behavior for ~15 standard Spring MVC exceptions is inherited implicitly and has no direct equivalent outside Spring. The target framework will produce its own status codes and bodies for malformed JSON, unsupported methods, etc.
**Suggestion:** Manual rewrite: explicitly add handlers in the target's global error middleware for malformed body (400), method not allowed (405), and unsupported media type (415) if parity is required. Otherwise document the accepted differences.

### `handleConstraintViolation / getParam` in `src/main/java/io/spring/api/exception/CustomizeExceptionHandler.java`
**Reason:** Relies on javax.validation ConstraintViolation metadata (propertyPath format 'method.param.field', constraint annotation type names) specific to Hibernate Validator method validation.
**Suggestion:** Use the target validation library (pydantic, class-validator, zod, go-playground/validator, etc.). Map its error objects directly to {field, message} pairs, then render as {"errors": {field: [msg]}} with status 422.

### `DuplicatedUsernameValidator (javax.validation ConstraintValidator integration + @Autowired field injection)` in `src/main/java/io/spring/application/user/DuplicatedUsernameValidator.java`
**Reason:** Go has no runtime that discovers annotation-bound validators and autowires dependencies into them. The implicit trigger via @Valid and the SpringConstraintValidatorFactory injection have no direct equivalent.
**Suggestion:** Manually rewrite as an explicit struct with constructor injection. Either register it as a closure-based custom validation in go-playground/validator, or invoke it explicitly in the registration service before persistence. Back it with a DB unique constraint.

### `@Validated / @Valid method-parameter validation (AOP proxy)` in `src/main/java/io/spring/application/user/UserService.java`
**Reason:** Go has no annotation-driven proxies that intercept method calls to validate arguments automatically.
**Suggestion:** Manual rewrite: call explicit Validate functions at the start of each service method, using go-playground/validator for field rules plus custom functions for DB-backed rules, and return a structured ValidationError.

### `UpdateUserConstraint / UpdateUserValidator (JSR-380 custom constraint with Spring-injected repository)` in `src/main/java/io/spring/application/user/UserService.java`
**Reason:** Relies on the Bean Validation runtime and Spring's SpringConstraintValidatorFactory to instantiate the validator and autowire fields. There is no direct Go equivalent.
**Suggestion:** Manual rewrite as a plain function, e.g. validateUpdateUser(ctx, repo, target *User, param UpdateUserParam) map[string][]string, invoked by UserService.UpdateUser. Alternatively, register a struct-level validation with go-playground/validator via RegisterStructValidation, using a closure that captures the repo.

### `DataFetcherResult.localContext usage` in `src/main/java/io/spring/graphql/UserMutation.java`
**Reason:** graphql-java's localContext propagation to child data fetchers has no direct gqlgen equivalent.
**Suggestion:** Manual rewrite: embed the domain User in the Go payload model (a field ignored by the schema) and implement the UserPayload.user/token field resolvers to read from it.

### `SecurityContextHolder / AnonymousAuthenticationToken checks` in `src/main/java/io/spring/graphql/UserMutation.java`
**Reason:** This relies on Spring Security's thread-local context and filter chain, which do not exist in Go.
**Suggestion:** Manual rewrite: JWT middleware stores *User in context.Context, and resolvers fetch it with a typed context-key helper. Treat absence as anonymous.

### `ConstraintViolationException-based validation (implicit via @Validated service and custom constraint annotations)` in `src/main/java/io/spring/graphql/UserMutation.java`
**Reason:** Annotation-driven method validation, including repository-backed uniqueness constraints, is framework magic with no Go equivalent.
**Suggestion:** Manual rewrite: explicit validation in the Go service using go-playground/validator plus manual uniqueness queries, returning a typed ValidationError(field→[]messages) that the resolver maps to the Error union (createUser) or a GraphQL error (updateUser).

### `LoginParam / RegisterParam Bean Validation annotations` in `src/main/java/io/spring/api/UsersApi.java`
**Reason:** Declarative javax.validation annotations, plus custom constraint validators that hit the DB, run implicitly before the handler. Go has no equivalent automatic interception.
**Suggestion:** Manual rewrite: an explicit Validate() method or go-playground/validator struct tags, with custom error messages and explicit uniqueness queries against the DB, returning a map[string][]string for the 422 body.

### `Global exception translation (CustomizeExceptionHandler, referenced implicitly)` in `src/main/java/io/spring/api/UsersApi.java`
**Reason:** @RestControllerAdvice maps thrown exceptions to HTTP responses outside this file. Go has no exceptions.
**Suggestion:** Define typed errors (ErrInvalidCredentials, ValidationError) and a shared writeError(w, err) helper or middleware that produces identical status codes and bodies.

### `@JsonRootName root unwrapping` in `src/main/java/io/spring/api/UsersApi.java`
**Reason:** Depends on global Jackson configuration (UNWRAP_ROOT_VALUE) and has no encoding/json equivalent.
**Suggestion:** Use explicit envelope structs with a `json:"user"` field for both request and response.

## Observer agent findings

The Observer agent monitored the migration and identified these patterns:

- **After 3 modules:** Nullable Java String semantics are silently collapsed into Go zero-value strings without a documented decision, so the Validator raises the same null-vs-empty finding module after module and confidence stays capped around 0.9.
- **After 6 modules:** Java null semantics are flattened into Go zero values (empty strings, zero ints) without telling apart internal fields, where this is harmless, from serialization and persistence boundaries, where it changes observable output.
- **After 9 modules:** Java null versus Go zero-value semantics is not resolved up front, so every module gets docked for null/empty divergences, most of which are harmless but are still recorded as failed specs.
- **After 12 modules:** Framework-provided semantics (nullable fields, Jackson root unwrapping, MyBatis mapping, the zero-config datasource) are not carried over explicitly, so the validator cannot verify equivalence and either hedges or raises no-effect findings.
- **After 15 modules:** The target expert handles cross-cutting concerns (error-to-status mapping, JSON root wrapping, null handling, DB config) separately in each file instead of in one shared place, which causes inconsistent signatures, missing mappings and a compile break.
- **After 18 modules:** The shared error-handling code is migrated piecemeal per exception file, with no single authoritative errors.go contract, which causes signature mismatches and missing status mappings.
- **After 21 modules:** The target expert makes undocumented edge-case semantic changes and calibrates confidence loosely, so its scores drift low (around 0.72–0.76) even on trivial modules the validator rates 0.85–0.92.
- **After 24 modules:** There is no up-front, project-wide policy for null/empty semantics or for replacing framework auto-registration, so the target expert improvises per module and the validator can't judge the deviations firmly.
- **After 27 modules:** Behavior that Spring/DGS provided implicitly through annotations and auto-wiring is deferred to modules that haven't been migrated yet or simply left unwired, so specs can't be verified and confidence drops even for correct low-complexity files.
- **After 30 modules:** Modules are migrated in an order that leaves cross-module dependencies unresolved: interfaces come before implementations, and validators before services. Unresolved dependencies (ErrNotFound, jwt.go, the uniqueness check) are neither stubbed nor recorded, so scores drop even though the migrated code itself is correct.
- **After 33 modules:** Confidence drops mainly because migrated files reference shared definitions that don't exist yet (above all core.ErrNotFound in internal/core/errors.go), so per-file contracts can't be verified.
- **After 36 modules:** The target expert references shared infrastructure (sentinel errors) and changes semantics without creating the shared code or recording the decision, so its self-assessed confidence stays low even where the validator finds no failures.
- **After 39 modules:** Implicit Java framework behavior (Jackson serialization, Lombok equality, Spring routing defaults) is neither documented by the Source Expert nor explicitly replicated by the Target Expert, which leaves both sides uncertain on otherwise trivial modules.
- **After 42 modules:** Confidence is pulled down mostly by implicit framework or library behavior that is not ported (auto-registration, routing defaults, jjwt validation, exception propagation) and by deviations left undocumented, made worse by speculative info-level findings from the validator.
- **After 45 modules:** Undocumented semantic divergences, mainly Java null becoming a Go zero value and exceptions becoming logged errors, are left for the validator to find instead of being declared up front, which lowers target self-confidence and generates repeated info-level findings.
- **After 48 modules:** Java null semantics are being lossily mapped to Go zero values, and the validator then reports these mostly unobservable differences as spec failures, which pulls down confidence on modules that are otherwise correct.
- **After 51 modules:** Confidence is lowest where the target relies on unverified cross-module dependencies (bcrypt in an unwritten password.go) or quietly changes null and conditional semantics, instead of implementing or explicitly documenting them.
- **After 54 modules:** Behavior is moved to other modules or framework wiring that is unverified or not yet migrated, so both agents carry unresolved cross-module uncertainty instead of local, verified implementations.
- **After 57 modules:** Confidence is low mainly because behavior depends on things outside the module (SQL ordering, framework auto-registration, exact error types) that neither the source spec captures nor the target expert verifies.
- **After 60 modules:** The target expert writes Go code against assumed, rather than actual, signatures of already-migrated internal packages, so build-breaking mismatches appear, and it gives no notes explaining its lower confidence.
- **After 63 modules:** The target expert writes code against assumed signatures and optional wiring for cross-module dependencies instead of the actual already-migrated Go APIs, which causes compile errors and dropped mandatory behavior.
- **After 66 modules:** Spring guarantees things implicitly, such as dependency injection that is never null and validation triggered by annotations, and the target expert does not turn these into explicit Go constructor rules, which leaves its own output uncertain and gives the validator gaps to flag.
- **After 69 modules:** Spring's implicit behaviors (exception-to-status mapping, error message extraction) are not specified or reproduced explicitly, and intentional deviations go undocumented, so the target self-scores low and the validator flags uncertain or deliberate differences.
- **After 72 modules:** Implicit Spring framework behavior (error-to-HTTP-status mapping, Bean Validation semantics) is not captured in specs or reproduced explicitly, and intentional deviations go undocumented, so both agents hedge their confidence.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `src/main/java/io/spring/application/data/ProfileData.java`
Confidence: 84%
Issues:
  - [warning] Bio and Image are plain Go strings, and the read store turns NULL columns into "". The source sends JSON null in that case. The migration documents this with a MIGRATION_NOTE.

### `src/main/resources/application.properties`
Confidence: 78%
Issues:
  - [warning] The SQLite dev.db datasource was replaced with a required PostgreSQL DATABASE_URL. Startup fails if it is not set, where the source started with zero configuration. The MyBatis cache, naming and generated-key settings belong to the query layer, not this config file.
  - [info] This config file does not cover it. Each handler has to unwrap the root key itself, and this file can't confirm that it does.

### `src/main/java/io/spring/api/exception/NoAuthorizationException.java`
Confidence: 77%
Issues:
  - [warning] The 403 mapping only works through the separate ForbiddenStatusFor, WriteAuthzError and AuthzHandlerFunc. The base StatusFor, WriteError and HandlerFunc in errors.go don't know about ErrNoAuthorization. So any handler wrapped in the plain HandlerFunc that returns ErrNoAuthorization will answer 500 instead of 403.

### `src/main/java/io/spring/api/exception/InvalidRequestException.java`
Confidence: 72%
Issues:
  - [info] If nil is passed in, it is replaced with an empty map, so Errors() never returns nil. The source returned null in that case. The handler renders the same output either way.

### `src/main/java/io/spring/core/user/User.java`
Confidence: 76%

### `src/main/java/io/spring/core/article/Article.java`
Confidence: 75%
Issues:
  - [info] A nil tagList now produces an empty tag list. In the source it threw a NullPointerException. Passing a zero createdAt now means 'use the current time'.

### `src/main/java/io/spring/core/comment/CommentRepository.java`
Confidence: 75%

### `src/main/java/io/spring/graphql/exception/GraphQLCustomizeExceptionHandler.java`
Confidence: 80%
Issues:
  - [warning] Nothing in the target codebase calls NewCustomizeExceptionHandler or ErrorsAsData, so the handler may never be hooked into the GraphQL server. DGS registers it automatically; Go does not.
  - [info] For auth errors the message comes from the ErrInvalidAuthentication constant, not the error that was actually returned. If that error was wrapped, the extra wrapping text is dropped. The result is still the same as the source, because the source exception always carries the same message.

### `src/main/java/io/spring/application/user/UpdateUserCommand.java`
Confidence: 66%
Issues:
  - [warning] The class-level UpdateUserConstraint check (email or username already used by another user) is moved to the service layer, but no such check exists anywhere in the target yet. Searching the target for 'already exist' found only an unrelated comment, and 'UpdateUserCommand' had no hits, so the user service hasn't been migrated.

### `src/main/java/io/spring/core/user/UserRepository.java`
Confidence: 68%
Issues:
  - [warning] The interface docs say a missing row comes back as ErrNotFound (internal/core/errors.go), but ErrNotFound isn't defined anywhere in the target codebase yet. This file still compiles because only comments mention it. Implementations and callers will need it.

### `src/main/java/io/spring/core/article/ArticleRepository.java`
Confidence: 66%
Issues:
  - [warning] The doc comments say a missing article is reported as the sentinel ErrNotFound from internal/core/errors.go. Searching the target codebase finds no definition of ErrNotFound and no errors.go. Only comments mention it. Implementations and callers therefore can't run the errors.Is check the contract depends on.

### `src/main/java/io/spring/application/CursorPager.java`
Confidence: 74%

### `src/main/java/io/spring/application/data/ArticleData.java`
Confidence: 72%
Issues:
  - [info] Equal treats a nil tag list and an empty one as equal. Lombok's equals did not, and Go has no hashCode equivalent.

### `src/main/java/io/spring/graphql/TagDatafetcher.java`
Confidence: 83%
Issues:
  - [warning] Nothing in the target codebase calls NewTagDatafetcher yet, so the resolver isn't hooked up to a GraphQL endpoint.

### `src/main/java/io/spring/application/CursorPageParameter.java`
Confidence: 72%
Issues:
  - [info] Go has no null for Direction, so a missing direction becomes DirectionPrev. Equal treats an instance with no direction and one with PREV as equal (Java treats them as different), and String prints PREV where Java printed null.

### `src/main/java/io/spring/infrastructure/repository/MyBatisUserRepository.java`
Confidence: 84%
Issues:
  - [info] Insert stores NULL for empty bio/image whereas source stored ''; reads normalize back to "", so not observable via repository.

### `src/main/java/io/spring/graphql/ProfileDatafetcher.java`
Confidence: 83%

### `src/main/java/io/spring/graphql/RelationMutation.java`
Confidence: 84%

### `src/main/java/io/spring/application/article/NewArticleParam.java`
Confidence: 72%
Issues:
  - [info] When the title is missing, the Go version looks up slug "" because Go strings can't be null. In the Java source a null title would make the validator throw an NPE. Normal requests are not affected.

### `src/main/java/io/spring/api/security/WebSecurityConfig.java`
Confidence: 65%
Issues:
  - [warning] The code defers BCrypt to internal/auth/password.go, but a search of the target finds no bcrypt usage anywhere. It may simply not be migrated yet.

### `src/main/java/io/spring/graphql/MeDatafetcher.java`
Confidence: 83%
Issues:
  - [info] The resolver is not registered with any GraphQL executor yet. This is documented as a MIGRATION_NOTE. Also, an empty or missing header now fails with a malformed-header error. Java instead fails with a missing-required-header error. Both cases still produce an error.

### `src/main/java/io/spring/application/ArticleQueryService.java`
Confidence: 78%
Issues:
  - [info] Unconfirmed: the ArticleReader interface comment says FindArticles keeps the order of the ids it is given. The Java result order depends on how the original findArticles SQL orders rows, so the reversed order on a previous-page request may differ. The original SQL and the Go FindArticles code were not checked.

### `src/main/java/io/spring/graphql/ArticleDatafetcher.java`
Confidence: 78%
Issues:
  - [info] When profile is nil, returns a missing-context error instead of NPE; this is just a defensive guard, so behavior is effectively equivalent.

### `src/main/java/io/spring/graphql/CommentDatafetcher.java`
Confidence: 84%

### `src/main/java/io/spring/api/CommentsApi.java`
Confidence: 78%

### `src/main/java/io/spring/application/user/RegisterParam.java`
Confidence: 81%

### `src/main/java/io/spring/application/article/ArticleCommandService.java`
Confidence: 65%
Issues:
  - [warning] Validation is skipped if the validator is nil. Nothing in the codebase calls the constructor yet, so it isn't clear the validator will always be provided.

### `src/main/java/io/spring/graphql/ArticleMutation.java`
Confidence: 83%

### `src/main/java/io/spring/api/exception/CustomizeExceptionHandler.java`
Confidence: 69%
Issues:
  - [info] The message body is built from err.Error(). If a caller wraps ErrInvalidAuthentication (for example with fmt.Errorf("login: %w", ...)), the wrapping prefix ends up in the message, while the source returns only the exception's own message.

### `src/main/java/io/spring/application/user/DuplicatedUsernameValidator.java`
Confidence: 73%

### `src/main/java/io/spring/application/user/UserService.java`
Confidence: 68%
Issues:
  - [warning] The new password is now hashed before User.Update is called. The spec says update values are passed through as-is and not encoded. This is a deliberate, documented fix (the Java code stored the plaintext password), but it is a behavior change.

### `src/main/java/io/spring/graphql/UserMutation.java`
Confidence: 78%

### `src/main/java/io/spring/api/UsersApi.java`
Confidence: 76%
Issues:
  - [warning] Badly formed JSON comes back as a plain wrapped error and will likely become a 500, where Spring returns 400. Not confirmed: the default path in the error mapper wasn't checked.
  - [info] The email pattern is only a rough copy of Hibernate's @Email rule, so unusual addresses may be accepted or rejected differently.

### `src/main/java/io/spring/api/CurrentUserApi.java`
Confidence: 83%

### `src/main/java/io/spring/api/ArticlesApi.java`
Confidence: 81%

### `src/main/java/io/spring/api/ArticleApi.java`
Confidence: 80%
