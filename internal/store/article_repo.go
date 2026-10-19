package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"migrated-app/internal/core"
)

// articleQuerier is the subset of *sql.DB / *sql.Tx used for article
// statements, so the same helpers work both inside and outside a transaction.
type articleQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ArticleRepo is the PostgreSQL implementation of core.ArticleRepository. It
// replaces the source's MyBatisArticleRepository + ArticleMapper pair.
type ArticleRepo struct {
	db *sql.DB
}

// NewArticleRepository returns an ArticleRepo backed by db.
//
// MIGRATION_NOTE: register this in the composition root (cmd/server/main.go)
// wherever core.ArticleRepository is needed.
func NewArticleRepository(db *sql.DB) *ArticleRepo {
	return &ArticleRepo{db: db}
}

// Compile-time check that ArticleRepo satisfies the domain port.
var _ core.ArticleRepository = (*ArticleRepo)(nil)

// errNilArticle is returned when a nil article is passed to Save or Remove.
var errNilArticle = errors.New("store: article is nil")

const (
	insertArticleSQL = `INSERT INTO articles (id, slug, title, description, body, user_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	findTagByNameSQL = `SELECT id, name FROM tags WHERE name = $1 LIMIT 1`

	insertTagSQL = `INSERT INTO tags (id, name) VALUES ($1, $2)`

	insertArticleTagSQL = `INSERT INTO article_tags (article_id, tag_id) VALUES ($1, $2)`

	deleteArticleSQL = `DELETE FROM articles WHERE id = $1`

	selectArticleWithTagsSQL = `SELECT a.id, a.slug, a.title, a.description, a.body, a.user_id,
       a.created_at, a.updated_at, t.id, t.name
FROM articles a
LEFT JOIN article_tags art ON art.article_id = a.id
LEFT JOIN tags t ON t.id = art.tag_id`

	findArticleByIDSQL   = selectArticleWithTagsSQL + ` WHERE a.id = $1`
	findArticleBySlugSQL = selectArticleWithTagsSQL + ` WHERE a.slug = $1`
)

// Save persists a in a single transaction. If no article with a.ID exists,
// each tag is looked up by name and created if missing, the article_tags rows
// are inserted, and then the article itself is inserted. Otherwise the
// existing row gets title, slug, description and body updated (only the
// non-empty ones); tags are not touched and updated_at is not written.
// Any failure rolls back the whole save, including tags created during it.
func (r *ArticleRepo) Save(ctx context.Context, a *core.Article) (err error) {
	if a == nil {
		return errNilArticle
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save article %s: begin tx: %w", a.ID, err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	_, findErr := findArticle(ctx, tx, findArticleByIDSQL, a.ID, "id")
	switch {
	case errors.Is(findErr, core.ErrNotFound):
		if err = createArticle(ctx, tx, a); err != nil {
			return err
		}
	case findErr != nil:
		err = fmt.Errorf("store: save article %s: %w", a.ID, findErr)
		return err
	default:
		if err = updateArticle(ctx, tx, a); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("store: save article %s: commit: %w", a.ID, err)
	}
	return nil
}

// createArticle is the find-or-create tags + relation + insert path.
func createArticle(ctx context.Context, q articleQuerier, a *core.Article) error {
	for _, tag := range a.Tags {
		target, err := findTagByName(ctx, q, tag.Name)
		switch {
		case errors.Is(err, core.ErrNotFound):
			if _, err := q.ExecContext(ctx, insertTagSQL, tag.ID, tag.Name); err != nil {
				return fmt.Errorf("store: insert tag %q: %w", tag.Name, err)
			}
			target = tag
		case err != nil:
			return err
		}
		if _, err := q.ExecContext(ctx, insertArticleTagSQL, a.ID, target.ID); err != nil {
			return fmt.Errorf("store: link article %s to tag %s: %w", a.ID, target.ID, err)
		}
	}
	if _, err := q.ExecContext(ctx, insertArticleSQL,
		a.ID, a.Slug, a.Title, a.Description, a.Body, a.UserID,
		ToDB(a.CreatedAt), ToDB(a.UpdatedAt)); err != nil {
		return fmt.Errorf("store: insert article %s: %w", a.ID, err)
	}
	return nil
}

// updateArticle mirrors the mapper's dynamic <set>: only non-empty title,
// slug, description and body are written. updated_at is intentionally not
// written (source quirk Q2).
func updateArticle(ctx context.Context, q articleQuerier, a *core.Article) error {
	sets := make([]string, 0, 4)
	args := make([]any, 0, 5)
	add := func(column, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		sets = append(sets, column+" = $"+strconv.Itoa(len(args)))
	}
	add("title", a.Title)
	add("slug", a.Slug)
	add("description", a.Description)
	add("body", a.Body)

	// MIGRATION_NOTE: with every field empty MyBatis would emit invalid SQL;
	// here that case is a no-op.
	if len(sets) == 0 {
		return nil
	}
	args = append(args, a.ID)
	query := "UPDATE articles SET " + strings.Join(sets, ", ") + " WHERE id = $" + strconv.Itoa(len(args))
	if _, err := q.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("store: update article %s: %w", a.ID, err)
	}
	return nil
}

func findTagByName(ctx context.Context, q articleQuerier, name string) (core.Tag, error) {
	var t core.Tag
	err := q.QueryRowContext(ctx, findTagByNameSQL, name).Scan(&t.ID, &t.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Tag{}, fmt.Errorf("store: find tag %q: %w", name, core.ErrNotFound)
	}
	if err != nil {
		return core.Tag{}, fmt.Errorf("store: find tag %q: %w", name, err)
	}
	return t, nil
}

// FindByID returns the article with the given id and its tags, or
// core.ErrNotFound.
func (r *ArticleRepo) FindByID(ctx context.Context, id string) (*core.Article, error) {
	return findArticle(ctx, r.db, findArticleByIDSQL, id, "id")
}

// FindBySlug returns the article with the given slug and its tags, or
// core.ErrNotFound.
func (r *ArticleRepo) FindBySlug(ctx context.Context, slug string) (*core.Article, error) {
	return findArticle(ctx, r.db, findArticleBySlugSQL, slug, "slug")
}

// findArticle runs the left-join query and collapses the rows into a single
// article with its Tags collection (the MyBatis nested result map).
func findArticle(ctx context.Context, q articleQuerier, query, arg, by string) (*core.Article, error) {
	rows, err := q.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, fmt.Errorf("store: find article by %s %q: %w", by, arg, err)
	}
	defer rows.Close()

	var a *core.Article
	for rows.Next() {
		var (
			id                                    string
			slug, title, description, body, userID sql.NullString
			createdAt, updatedAt                  UTCTime
			tagID, tagName                        sql.NullString
		)
		if err := rows.Scan(&id, &slug, &title, &description, &body, &userID,
			&createdAt, &updatedAt, &tagID, &tagName); err != nil {
			return nil, fmt.Errorf("store: find article by %s %q: scan: %w", by, arg, err)
		}
		if a == nil {
			a = &core.Article{
				ID:          id,
				Slug:        slug.String,
				Title:       title.String,
				Description: description.String,
				Body:        body.String,
				UserID:      userID.String,
				CreatedAt:   createdAt.Time,
				UpdatedAt:   updatedAt.Time,
				Tags:        []core.Tag{},
			}
		}
		if tagID.Valid {
			a.Tags = append(a.Tags, core.Tag{ID: tagID.String, Name: tagName.String})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: find article by %s %q: %w", by, arg, err)
	}
	if a == nil {
		return nil, fmt.Errorf("store: find article by %s %q: %w", by, arg, core.ErrNotFound)
	}
	return a, nil
}

// Remove deletes the article row with a.ID.
func (r *ArticleRepo) Remove(ctx context.Context, a *core.Article) error {
	if a == nil {
		return errNilArticle
	}
	if _, err := r.db.ExecContext(ctx, deleteArticleSQL, a.ID); err != nil {
		return fmt.Errorf("store: remove article %s: %w", a.ID, err)
	}
	return nil
}
