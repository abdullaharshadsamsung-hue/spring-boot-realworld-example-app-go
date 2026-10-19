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

// userQuerier is the subset of *sql.DB / *sql.Tx / *sql.Conn used by
// UserRepo. Accepting it lets the repository run inside a caller-owned
// transaction. This replaces the source's implicit Spring transaction
// participation.
type userQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// UserRepo is the PostgreSQL implementation of core.UserRepository. It
// replaces the source's MyBatisUserRepository + UserMapper pair.
type UserRepo struct {
	db userQuerier
}

// NewUserRepository returns a UserRepo backed by db, which can be a *sql.DB,
// *sql.Tx or *sql.Conn.
func NewUserRepository(db userQuerier) *UserRepo {
	return &UserRepo{db: db}
}

// Compile-time check that UserRepo satisfies the domain port.
var _ core.UserRepository = (*UserRepo)(nil)

const (
	insertUserSQL = `INSERT INTO users (id, username, email, password, bio, image)
VALUES ($1, $2, $3, $4, $5, $6)`

	selectUserColumns = `SELECT id, username, email, password, bio, image FROM users`

	findUserByIDSQL       = selectUserColumns + ` WHERE id = $1`
	findUserByUsernameSQL = selectUserColumns + ` WHERE username = $1`
	findUserByEmailSQL    = selectUserColumns + ` WHERE email = $1`

	insertFollowSQL = `INSERT INTO follows (user_id, follow_id) VALUES ($1, $2)`

	findFollowSQL = `SELECT user_id, follow_id FROM follows
WHERE user_id = $1 AND follow_id = $2`

	deleteFollowSQL = `DELETE FROM follows WHERE user_id = $1 AND follow_id = $2`
)

// Save stores u. If no user with u.ID exists it inserts u. Otherwise it
// updates the stored row, changing only the fields that are non-empty in u
// (username, email, password, bio, image), as the source mapper's dynamic
// <set> did.
//
// MIGRATION_NOTE: the source checks for the row and then inserts or updates,
// which can race under concurrent requests. That is kept so the conditional
// update stays the same. Run Save in a transaction if strict atomicity matters.
func (r *UserRepo) Save(ctx context.Context, u *core.User) error {
	if u == nil {
		return core.ErrNilUser
	}
	_, err := r.FindByID(ctx, u.ID)
	switch {
	case errors.Is(err, core.ErrNotFound):
		return r.insert(ctx, u)
	case err != nil:
		return fmt.Errorf("store: save user %s: %w", u.ID, err)
	default:
		return r.update(ctx, u)
	}
}

func (r *UserRepo) insert(ctx context.Context, u *core.User) error {
	if _, err := r.db.ExecContext(ctx, insertUserSQL,
		u.ID, u.Username, u.Email, u.Password, nullIfEmpty(u.Bio), nullIfEmpty(u.Image)); err != nil {
		return fmt.Errorf("store: insert user %s: %w", u.ID, err)
	}
	return nil
}

func (r *UserRepo) update(ctx context.Context, u *core.User) error {
	sets := make([]string, 0, 5)
	args := make([]any, 0, 6)
	add := func(column, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		sets = append(sets, column+" = $"+strconv.Itoa(len(args)))
	}
	add("username", u.Username)
	add("email", u.Email)
	add("password", u.Password)
	add("bio", u.Bio)
	add("image", u.Image)

	// MIGRATION_NOTE: with every field empty, MyBatis would have produced
	// invalid SQL ("UPDATE users WHERE ..."). Here that case does nothing.
	if len(sets) == 0 {
		return nil
	}
	args = append(args, u.ID)
	query := "UPDATE users SET " + strings.Join(sets, ", ") + " WHERE id = $" + strconv.Itoa(len(args))
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("store: update user %s: %w", u.ID, err)
	}
	return nil
}

// FindByID looks up a user by ID. It returns core.ErrNotFound if no user has
// that ID.
func (r *UserRepo) FindByID(ctx context.Context, id string) (*core.User, error) {
	return r.findOne(ctx, findUserByIDSQL, id, "id")
}

// FindByUsername looks up a user by exact username. It returns
// core.ErrNotFound if no user has that username.
func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*core.User, error) {
	return r.findOne(ctx, findUserByUsernameSQL, username, "username")
}

// FindByEmail looks up a user by exact email. It returns core.ErrNotFound if
// no user has that email.
func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*core.User, error) {
	return r.findOne(ctx, findUserByEmailSQL, email, "email")
}

func (r *UserRepo) findOne(ctx context.Context, query, arg, by string) (*core.User, error) {
	var (
		u                         core.User
		username, email, password sql.NullString
		bio, image                sql.NullString
	)
	err := r.db.QueryRowContext(ctx, query, arg).
		Scan(&u.ID, &username, &email, &password, &bio, &image)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: find user by %s %q: %w", by, arg, core.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: find user by %s %q: %w", by, arg, err)
	}
	u.Username = username.String
	u.Email = email.String
	u.Password = password.String
	u.Bio = bio.String
	u.Image = image.String
	return &u, nil
}

// SaveRelation records that rel.UserID follows rel.TargetID. If the relation
// already exists it does nothing.
//
// MIGRATION_NOTE: the follows table has no unique constraint, so ON CONFLICT
// can't be used. The source's check-then-insert is kept.
func (r *UserRepo) SaveRelation(ctx context.Context, rel *core.FollowRelation) error {
	if rel == nil {
		return core.ErrNilFollowRelation
	}
	_, err := r.FindRelation(ctx, rel.UserID, rel.TargetID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, core.ErrNotFound) {
		return fmt.Errorf("store: save follow relation: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, insertFollowSQL, rel.UserID, rel.TargetID); err != nil {
		return fmt.Errorf("store: save follow relation (%s -> %s): %w", rel.UserID, rel.TargetID, err)
	}
	return nil
}

// FindRelation finds the relation where userID follows targetID. It returns
// core.ErrNotFound if no such relation exists.
func (r *UserRepo) FindRelation(ctx context.Context, userID, targetID string) (*core.FollowRelation, error) {
	var rel core.FollowRelation
	err := r.db.QueryRowContext(ctx, findFollowSQL, userID, targetID).Scan(&rel.UserID, &rel.TargetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: find follow relation (%s -> %s): %w", userID, targetID, core.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: find follow relation (%s -> %s): %w", userID, targetID, err)
	}
	return &rel, nil
}

// RemoveRelation deletes the relation where rel.UserID follows rel.TargetID.
// Deleting a relation that does not exist is not an error.
func (r *UserRepo) RemoveRelation(ctx context.Context, rel *core.FollowRelation) error {
	if rel == nil {
		return core.ErrNilFollowRelation
	}
	if _, err := r.db.ExecContext(ctx, deleteFollowSQL, rel.UserID, rel.TargetID); err != nil {
		return fmt.Errorf("store: remove follow relation (%s -> %s): %w", rel.UserID, rel.TargetID, err)
	}
	return nil
}

// nullIfEmpty maps the core layer's "" (absent) to SQL NULL.
func nullIfEmpty(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
