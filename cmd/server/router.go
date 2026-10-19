package main

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lib/pq"
	"github.com/rs/zerolog/log"

	"migrated-app/internal/app"
	"migrated-app/internal/auth"
	"migrated-app/internal/config"
	"migrated-app/internal/core"
	"migrated-app/internal/httpapi"
	"migrated-app/internal/store"
)

// buildRouter creates the chi router with logging/recovery middleware, the
// health check, and every REST handler from internal/httpapi mounted behind
// the security chain (CORS -> JWT authentication -> authorization rules).
func buildRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.Recoverer)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	api, err := buildAPI()
	if err != nil {
		log.Fatal().Err(err).Msg("build api routes")
	}
	r.Handle("/*", api)
	return r
}

// buildAPI constructs the dependency graph and registers all httpapi routes.
func buildAPI() (http.Handler, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := db.PingContext(pingCtx); err != nil {
		log.Warn().Err(err).Msg("database ping failed; continuing")
	}
	cancel()

	schemaCtx, schemaCancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = store.EnsureSchema(schemaCtx, db)
	schemaCancel()
	if err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	jwt, err := auth.NewJWTService(cfg.JWTSecret, int(cfg.JWTSessionTime/time.Second))
	if err != nil {
		return nil, fmt.Errorf("jwt service: %w", err)
	}
	encoder := passwordHasher{}

	// Write-side repositories.
	userRepo := store.NewUserRepository(db)
	articleRepo := store.NewArticleRepository(db)
	favoriteRepo := store.NewFavoriteRepo(db)
	commentRepo := commentStore{repo: store.NewCommentRepo(db)}

	// Read-side adapters.
	users := userReads{db: db}
	relations := relationReads{db: db}
	favorites := favoriteReads{db: db}
	articles := articleReads{db: db}
	comments := commentReads{db: db}
	tags := tagReads{db: db}

	tagsSvc, err := app.NewTagsQueryService(tags)
	if err != nil {
		return nil, err
	}
	userQuery, err := app.NewUserQueryService(users)
	if err != nil {
		return nil, err
	}
	profileQuery, err := app.NewProfileQueryService(users, relations)
	if err != nil {
		return nil, err
	}
	userSvc, err := app.NewUserService(userRepo, cfg.DefaultImage, encoder)
	if err != nil {
		return nil, err
	}
	articleQuery := app.NewArticleQueryService(articles, relations, favorites)
	articleCmd := app.NewArticleCommandService(articleRepo, nil)
	slugs, err := app.NewDuplicatedArticleValidator(articleQuery)
	if err != nil {
		return nil, err
	}
	commentQuery := app.NewCommentQueryService(comments, relations)

	tagsH, err := httpapi.NewTagsHandler(tagsSvc)
	if err != nil {
		return nil, err
	}
	usersH, err := httpapi.NewUsersHandler(userSvc, userRepo, userQuery, encoder, jwt)
	if err != nil {
		return nil, err
	}
	currentH, err := httpapi.NewCurrentUserHandler(userSvc, userQuery, auth.UserFrom)
	if err != nil {
		return nil, err
	}
	profilesH, err := httpapi.NewProfilesHandler(profileQuery, userRepo)
	if err != nil {
		return nil, err
	}
	articlesH, err := httpapi.NewArticlesHandler(articleCmd, articleQuery, slugs)
	if err != nil {
		return nil, err
	}
	articleH, err := httpapi.NewArticleHandler(articleQuery, articleRepo, articleCmd, core.CanWriteArticle)
	if err != nil {
		return nil, err
	}
	favoritesH, err := httpapi.NewFavoritesHandler(favoriteRepo, articleRepo, articleQuery)
	if err != nil {
		return nil, err
	}
	commentsH, err := httpapi.NewCommentsHandler(articleRepo, commentRepo, commentQuery)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	tagsH.Register(mux)
	usersH.Register(mux)
	currentH.Register(mux)
	profilesH.Register(mux)
	articlesH.Register(mux)
	articleH.Register(mux)
	favoritesH.Register(mux)
	commentsH.Register(mux)

	return httpapi.SecurityChain(mux, subjectResolver{jwt: jwt}, userRepo), nil
}

// subjectResolver adapts auth.JWTService to httpapi.SubjectResolver.
type subjectResolver struct{ jwt *auth.JWTService }

func (s subjectResolver) GetSubFromToken(token string) (string, bool) {
	return s.jwt.SubjectFromToken(token)
}

// commentStore adapts store.CommentRepo to httpapi.CommentStore.
type commentStore struct{ repo *store.CommentRepo }

func (c commentStore) Save(ctx context.Context, cm *core.Comment) error { return c.repo.Save(ctx, cm) }
func (c commentStore) Remove(ctx context.Context, cm *core.Comment) error {
	return c.repo.Remove(ctx, cm)
}
func (c commentStore) FindByID(ctx context.Context, articleID, id string) (*core.Comment, error) {
	cm, found, err := c.repo.FindByID(ctx, articleID, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, core.ErrNotFound
	}
	return cm, nil
}

// passwordHasher is an iterated, salted SHA-512 password encoder.
// MIGRATION_NOTE: bcrypt (golang.org/x/crypto) is not a module dependency;
// hashes produced by the Java source's BCrypt encoder will not match.
type passwordHasher struct{}

const pwIterations = 10000

func derivePassword(raw string, salt []byte, n int) []byte {
	buf := make([]byte, 0, len(salt)+len(raw))
	buf = append(buf, salt...)
	buf = append(buf, raw...)
	h := sha512.Sum512(buf)
	for i := 1; i < n; i++ {
		next := make([]byte, 0, len(h)+len(salt))
		next = append(next, h[:]...)
		next = append(next, salt...)
		h = sha512.Sum512(next)
	}
	return h[:]
}

func (passwordHasher) Encode(raw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	sum := derivePassword(raw, salt, pwIterations)
	return fmt.Sprintf("sha512$%d$%s$%s", pwIterations, hex.EncodeToString(salt), hex.EncodeToString(sum)), nil
}

func (passwordHasher) Matches(raw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "sha512" {
		return false
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n <= 0 {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(derivePassword(raw, salt, n), want) == 1
}

// ---------------------------------------------------------------------------
// Read-side PostgreSQL adapters
// ---------------------------------------------------------------------------

func queryStrings(ctx context.Context, db *sql.DB, q string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func whereSQL(conds []string) string {
	if len(conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conds, " AND ")
}

type tagReads struct{ db *sql.DB }

func (t tagReads) All(ctx context.Context) ([]string, error) {
	return queryStrings(ctx, t.db, `SELECT name FROM tags`)
}

type userReads struct{ db *sql.DB }

const userDataSelect = `SELECT id, COALESCE(email,''), COALESCE(username,''), COALESCE(bio,''), COALESCE(image,'') FROM users`

func (u userReads) one(ctx context.Context, q, arg string) (app.UserData, bool, error) {
	var d app.UserData
	err := u.db.QueryRowContext(ctx, q, arg).Scan(&d.ID, &d.Email, &d.Username, &d.Bio, &d.Image)
	if err == sql.ErrNoRows {
		return app.UserData{}, false, nil
	}
	if err != nil {
		return app.UserData{}, false, err
	}
	return d, true, nil
}

func (u userReads) FindByUsername(ctx context.Context, username string) (app.UserData, bool, error) {
	return u.one(ctx, userDataSelect+` WHERE username = $1`, username)
}

func (u userReads) FindByID(ctx context.Context, id string) (app.UserData, bool, error) {
	return u.one(ctx, userDataSelect+` WHERE id = $1`, id)
}

type relationReads struct{ db *sql.DB }

func (r relationReads) IsUserFollowing(ctx context.Context, userID, anotherUserID string) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM follows WHERE user_id = $1 AND follow_id = $2)`,
		userID, anotherUserID).Scan(&ok)
	return ok, err
}

func (r relationReads) FollowingAuthors(ctx context.Context, userID string, authorIDs []string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	if len(authorIDs) == 0 {
		return out, nil
	}
	ids, err := queryStrings(ctx, r.db,
		`SELECT follow_id FROM follows WHERE user_id = $1 AND follow_id = ANY($2)`,
		userID, pq.Array(authorIDs))
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}

func (r relationReads) FollowedUsers(ctx context.Context, userID string) ([]string, error) {
	return queryStrings(ctx, r.db, `SELECT follow_id FROM follows WHERE user_id = $1`, userID)
}

type favoriteReads struct{ db *sql.DB }

func (f favoriteReads) IsUserFavorite(ctx context.Context, userID, articleID string) (bool, error) {
	var ok bool
	err := f.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM article_favorites WHERE user_id = $1 AND article_id = $2)`,
		userID, articleID).Scan(&ok)
	return ok, err
}

func (f favoriteReads) ArticleFavoriteCount(ctx context.Context, articleID string) (int, error) {
	var n int
	err := f.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM article_favorites WHERE article_id = $1`, articleID).Scan(&n)
	return n, err
}

func (f favoriteReads) ArticlesFavoriteCount(ctx context.Context, articleIDs []string) (map[string]int, error) {
	out := map[string]int{}
	if len(articleIDs) == 0 {
		return out, nil
	}
	rows, err := f.db.QueryContext(ctx,
		`SELECT article_id, COUNT(*) FROM article_favorites WHERE article_id = ANY($1) GROUP BY article_id`,
		pq.Array(articleIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

func (f favoriteReads) UserFavorites(ctx context.Context, articleIDs []string, user *core.User) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	if user == nil || len(articleIDs) == 0 {
		return out, nil
	}
	ids, err := queryStrings(ctx, f.db,
		`SELECT article_id FROM article_favorites WHERE user_id = $1 AND article_id = ANY($2)`,
		user.ID, pq.Array(articleIDs))
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}

type articleReads struct{ db *sql.DB }

const articleSelect = `SELECT a.id, COALESCE(a.slug,''), COALESCE(a.title,''), COALESCE(a.description,''), COALESCE(a.body,''),
a.created_at, a.updated_at, u.id, u.username, COALESCE(u.bio,''), COALESCE(u.image,'')
FROM articles a LEFT JOIN users u ON u.id = a.user_id`

func (a articleReads) list(ctx context.Context, q string, args ...any) ([]app.ArticleData, error) {
	rows, err := a.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := []app.ArticleData{}
	for rows.Next() {
		var d app.ArticleData
		var created, updated store.UTCTime
		var uid, uname sql.NullString
		var bio, image string
		if err := rows.Scan(&d.ID, &d.Slug, &d.Title, &d.Description, &d.Body,
			&created, &updated, &uid, &uname, &bio, &image); err != nil {
			rows.Close()
			return nil, err
		}
		d.CreatedAt = created.Time
		d.UpdatedAt = updated.Time
		if uid.Valid {
			d.Author = &app.ProfileData{ID: uid.String, Username: uname.String, Bio: bio, Image: image}
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := a.fillTags(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a articleReads) fillTags(ctx context.Context, list []app.ArticleData) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]string, 0, len(list))
	for _, d := range list {
		ids = append(ids, d.ID)
	}
	rows, err := a.db.QueryContext(ctx,
		`SELECT at.article_id, t.name FROM article_tags at JOIN tags t ON t.id = at.tag_id WHERE at.article_id = ANY($1)`,
		pq.Array(ids))
	if err != nil {
		return err
	}
	defer rows.Close()
	tags := map[string][]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		tags[id] = append(tags[id], name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range list {
		list[i].TagList = tags[list[i].ID]
		if list[i].TagList == nil {
			list[i].TagList = []string{}
		}
	}
	return nil
}

func (a articleReads) single(ctx context.Context, q, arg string) (app.ArticleData, bool, error) {
	list, err := a.list(ctx, q, arg)
	if err != nil {
		return app.ArticleData{}, false, err
	}
	if len(list) == 0 {
		return app.ArticleData{}, false, nil
	}
	return list[0], true, nil
}

func (a articleReads) FindByID(ctx context.Context, id string) (app.ArticleData, bool, error) {
	return a.single(ctx, articleSelect+` WHERE a.id = $1`, id)
}

func (a articleReads) FindBySlug(ctx context.Context, slug string) (app.ArticleData, bool, error) {
	return a.single(ctx, articleSelect+` WHERE a.slug = $1`, slug)
}

func articleFilterConds(tag, author, favoritedBy string, args *[]any) []string {
	var conds []string
	if tag != "" {
		*args = append(*args, tag)
		conds = append(conds, fmt.Sprintf(
			"a.id IN (SELECT at.article_id FROM article_tags at JOIN tags t ON t.id = at.tag_id WHERE t.name = $%d)", len(*args)))
	}
	if author != "" {
		*args = append(*args, author)
		conds = append(conds, fmt.Sprintf("a.user_id IN (SELECT id FROM users WHERE username = $%d)", len(*args)))
	}
	if favoritedBy != "" {
		*args = append(*args, favoritedBy)
		conds = append(conds, fmt.Sprintf(
			"a.id IN (SELECT af.article_id FROM article_favorites af JOIN users fu ON fu.id = af.user_id WHERE fu.username = $%d)", len(*args)))
	}
	return conds
}

func (a articleReads) QueryArticles(ctx context.Context, tag, author, favoritedBy string, page app.Page) ([]string, error) {
	args := []any{}
	conds := articleFilterConds(tag, author, favoritedBy, &args)
	args = append(args, page.Limit(), page.Offset())
	q := `SELECT a.id FROM articles a` + whereSQL(conds) +
		fmt.Sprintf(" ORDER BY a.created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	return queryStrings(ctx, a.db, q, args...)
}

func (a articleReads) CountArticle(ctx context.Context, tag, author, favoritedBy string) (int, error) {
	args := []any{}
	conds := articleFilterConds(tag, author, favoritedBy, &args)
	var n int
	err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM articles a`+whereSQL(conds), args...).Scan(&n)
	return n, err
}

func (a articleReads) FindArticles(ctx context.Context, ids []string) ([]app.ArticleData, error) {
	if len(ids) == 0 {
		return []app.ArticleData{}, nil
	}
	list, err := a.list(ctx, articleSelect+` WHERE a.id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	byID := make(map[string]app.ArticleData, len(list))
	for _, d := range list {
		byID[d.ID] = d
	}
	out := make([]app.ArticleData, 0, len(ids))
	for _, id := range ids {
		if d, ok := byID[id]; ok {
			out = append(out, d)
		}
	}
	return out, nil
}

func (a articleReads) FindArticlesOfAuthors(ctx context.Context, authorIDs []string, page app.Page) ([]app.ArticleData, error) {
	if len(authorIDs) == 0 {
		return []app.ArticleData{}, nil
	}
	return a.list(ctx, articleSelect+` WHERE a.user_id = ANY($1) ORDER BY a.created_at DESC LIMIT $2 OFFSET $3`,
		pq.Array(authorIDs), page.Limit(), page.Offset())
}

func (a articleReads) CountFeedSize(ctx context.Context, authorIDs []string) (int, error) {
	if len(authorIDs) == 0 {
		return 0, nil
	}
	var n int
	err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM articles WHERE user_id = ANY($1)`,
		pq.Array(authorIDs)).Scan(&n)
	return n, err
}

func cursorClause(column string, page app.CursorPageParameter, conds []string, args *[]any) ([]string, string) {
	if c := page.Cursor(); c != nil {
		*args = append(*args, store.ToDB(*c))
		op := ">"
		if page.IsNext() {
			op = "<"
		}
		conds = append(conds, fmt.Sprintf("%s %s $%d", column, op, len(*args)))
	}
	order := " ORDER BY " + column + " ASC"
	if page.IsNext() {
		order = " ORDER BY " + column + " DESC"
	}
	*args = append(*args, page.QueryLimit())
	return conds, order + fmt.Sprintf(" LIMIT $%d", len(*args))
}

func (a articleReads) FindArticlesWithCursor(ctx context.Context, tag, author, favoritedBy string, page app.CursorPageParameter) ([]string, error) {
	args := []any{}
	conds := articleFilterConds(tag, author, favoritedBy, &args)
	conds, tail := cursorClause("a.updated_at", page, conds, &args)
	return queryStrings(ctx, a.db, `SELECT a.id FROM articles a`+whereSQL(conds)+tail, args...)
}

func (a articleReads) FindArticlesOfAuthorsWithCursor(ctx context.Context, authorIDs []string, page app.CursorPageParameter) ([]app.ArticleData, error) {
	if len(authorIDs) == 0 {
		return []app.ArticleData{}, nil
	}
	args := []any{pq.Array(authorIDs)}
	conds := []string{"a.user_id = ANY($1)"}
	conds, tail := cursorClause("a.updated_at", page, conds, &args)
	return a.list(ctx, articleSelect+whereSQL(conds)+tail, args...)
}

type commentReads struct{ db *sql.DB }

const commentSelect = `SELECT c.id, COALESCE(c.body,''), COALESCE(c.article_id,''), c.created_at, c.updated_at,
u.id, u.username, COALESCE(u.bio,''), COALESCE(u.image,'')
FROM comments c LEFT JOIN users u ON u.id = c.user_id`

func (c commentReads) list(ctx context.Context, q string, args ...any) ([]app.CommentData, error) {
	rows, err := c.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []app.CommentData{}
	for rows.Next() {
		var d app.CommentData
		var created, updated store.UTCTime
		var uid, uname sql.NullString
		var bio, image string
		if err := rows.Scan(&d.ID, &d.Body, &d.ArticleID, &created, &updated,
			&uid, &uname, &bio, &image); err != nil {
			return nil, err
		}
		d.CreatedAt = created.Time
		d.UpdatedAt = updated.Time
		if uid.Valid {
			d.Author = &app.ProfileData{ID: uid.String, Username: uname.String, Bio: bio, Image: image}
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (c commentReads) FindByID(ctx context.Context, id string) (app.CommentData, bool, error) {
	list, err := c.list(ctx, commentSelect+` WHERE c.id = $1`, id)
	if err != nil {
		return app.CommentData{}, false, err
	}
	if len(list) == 0 {
		return app.CommentData{}, false, nil
	}
	return list[0], true, nil
}

func (c commentReads) FindByArticleID(ctx context.Context, articleID string) ([]app.CommentData, error) {
	return c.list(ctx, commentSelect+` WHERE c.article_id = $1 ORDER BY c.created_at`, articleID)
}

func (c commentReads) FindByArticleIDWithCursor(ctx context.Context, articleID string, page app.CursorPageParameter) ([]app.CommentData, error) {
	args := []any{articleID}
	conds := []string{"c.article_id = $1"}
	conds, tail := cursorClause("c.created_at", page, conds, &args)
	return c.list(ctx, commentSelect+whereSQL(conds)+tail, args...)
}