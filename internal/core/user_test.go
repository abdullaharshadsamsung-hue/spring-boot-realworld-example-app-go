package core

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
)

var userTestUUIDV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// userTestErrNotFound stands in for the repository "not found" sentinel.
var userTestErrNotFound = errors.New("test: not found")

var userTestErrConflict = errors.New("test: unique constraint violation")

// userTestMemRepo is an in-memory UserRepository used to validate the port contract.
type userTestMemRepo struct {
	mu        sync.Mutex
	users     map[string]User
	relations map[[2]string]FollowRelation
}

var _ UserRepository = (*userTestMemRepo)(nil)

func newUserTestMemRepo() *userTestMemRepo {
	return &userTestMemRepo{users: map[string]User{}, relations: map[[2]string]FollowRelation{}}
}

func (m *userTestMemRepo) Save(_ context.Context, u *User) error {
	if u == nil {
		return ErrNilUser
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, other := range m.users {
		if id == u.ID {
			continue
		}
		if other.Username == u.Username || other.Email == u.Email {
			return fmt.Errorf("save user: %w", userTestErrConflict)
		}
	}
	m.users[u.ID] = *u
	return nil
}

func (m *userTestMemRepo) find(pred func(User) bool) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if pred(u) {
			c := u
			return &c, nil
		}
	}
	return nil, userTestErrNotFound
}

func (m *userTestMemRepo) FindByID(_ context.Context, id string) (*User, error) {
	return m.find(func(u User) bool { return u.ID == id })
}

func (m *userTestMemRepo) FindByUsername(_ context.Context, username string) (*User, error) {
	return m.find(func(u User) bool { return u.Username == username })
}

func (m *userTestMemRepo) FindByEmail(_ context.Context, email string) (*User, error) {
	return m.find(func(u User) bool { return u.Email == email })
}

func (m *userTestMemRepo) SaveRelation(_ context.Context, r *FollowRelation) error {
	if r == nil {
		return ErrNilFollowRelation
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.relations[[2]string{r.UserID, r.TargetID}] = *r
	return nil
}

func (m *userTestMemRepo) FindRelation(_ context.Context, userID, targetID string) (*FollowRelation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.relations[[2]string{userID, targetID}]
	if !ok {
		return nil, userTestErrNotFound
	}
	return &r, nil
}

func (m *userTestMemRepo) RemoveRelation(_ context.Context, r *FollowRelation) error {
	if r == nil {
		return ErrNilFollowRelation
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.relations, [2]string{r.UserID, r.TargetID})
	return nil
}

func userTestMustUser(t *testing.T, email, username, password, bio, image string) *User {
	t.Helper()
	u, err := NewUser(email, username, password, bio, image)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	return u
}

func TestUserAggregateSpec(t *testing.T) {
	t.Run("NewUser", userAggNewUser)
	t.Run("UniqueIDs", userAggUniqueIDs)
	t.Run("NewUUID", userAggNewUUID)
	t.Run("Update", userAggUpdate)
	t.Run("UpdateNilReceiver", userAggUpdateNilReceiver)
	t.Run("Equal", userAggEqual)
	t.Run("String", userAggString)
	t.Run("NewFollowRelation", userAggNewFollowRelation)
	t.Run("FollowRelationEqual", userAggFollowRelationEqual)
	t.Run("FollowRelationString", userAggFollowRelationString)
	t.Run("SentinelErrors", userAggSentinelErrors)
}

func TestUserPortSpec(t *testing.T) {
	t.Run("SaveAndFind", userPortSaveAndFind)
	t.Run("FindMissing", userPortFindMissing)
	t.Run("SaveUpdatesExisting", userPortSaveUpdatesExisting)
	t.Run("SaveIdempotent", userPortSaveIdempotent)
	t.Run("SaveErrors", userPortSaveErrors)
	t.Run("Relations", userPortRelations)
	t.Run("RemoveRelationKeepsReverseAndUsers", userPortRemoveRelationKeepsReverseAndUsers)
	t.Run("NilRelationErrors", userPortNilRelationErrors)
}

// ---------- User domain ----------

func userAggNewUser(t *testing.T) {
	tests := []struct {
		name                                  string
		email, username, password, bio, image string
	}{
		{"all fields", "a@b.c", "alice", "hash", "bio", "http://img"},
		{"empty bio and image", "x@y.z", "bob", "hash2", "", ""},
		{"all empty", "", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := NewUser(tt.email, tt.username, tt.password, tt.bio, tt.image)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if u == nil {
				t.Fatal("nil user")
			}
			if !userTestUUIDV4Re.MatchString(u.ID) {
				t.Errorf("ID %q is not a v4 UUID", u.ID)
			}
			if u.Email != tt.email || u.Username != tt.username || u.Password != tt.password ||
				u.Bio != tt.bio || u.Image != tt.image {
				t.Errorf("fields mismatch: %+v", *u)
			}
		})
	}
}

func userAggUniqueIDs(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		u := userTestMustUser(t, "e", "u", "p", "", "")
		if seen[u.ID] {
			t.Fatalf("duplicate id %s", u.ID)
		}
		seen[u.ID] = true
	}
}

func userAggNewUUID(t *testing.T) {
	for i := 0; i < 100; i++ {
		id, err := newUUID()
		if err != nil {
			t.Fatalf("newUUID: %v", err)
		}
		if len(id) != 36 || !userTestUUIDV4Re.MatchString(id) {
			t.Fatalf("bad uuid %q", id)
		}
	}
}

func userAggUpdate(t *testing.T) {
	base := User{ID: "id1", Email: "old@e", Username: "old", Password: "oldpw", Bio: "oldbio", Image: "oldimg"}
	tests := []struct {
		name                                  string
		email, username, password, bio, image string
		want                                  User
	}{
		{"all empty leaves unchanged", "", "", "", "", "", base},
		{"all set", "n@e", "new", "newpw", "newbio", "newimg",
			User{ID: "id1", Email: "n@e", Username: "new", Password: "newpw", Bio: "newbio", Image: "newimg"}},
		{"only email", "n@e", "", "", "", "",
			User{ID: "id1", Email: "n@e", Username: "old", Password: "oldpw", Bio: "oldbio", Image: "oldimg"}},
		{"only username", "", "new", "", "", "",
			User{ID: "id1", Email: "old@e", Username: "new", Password: "oldpw", Bio: "oldbio", Image: "oldimg"}},
		{"only password", "", "", "newpw", "", "",
			User{ID: "id1", Email: "old@e", Username: "old", Password: "newpw", Bio: "oldbio", Image: "oldimg"}},
		{"only bio", "", "", "", "newbio", "",
			User{ID: "id1", Email: "old@e", Username: "old", Password: "oldpw", Bio: "newbio", Image: "oldimg"}},
		{"only image", "", "", "", "", "newimg",
			User{ID: "id1", Email: "old@e", Username: "old", Password: "oldpw", Bio: "oldbio", Image: "newimg"}},
		{"whitespace is not empty", " ", "", "", " ", "",
			User{ID: "id1", Email: " ", Username: "old", Password: "oldpw", Bio: " ", Image: "oldimg"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := base
			u.Update(tt.email, tt.username, tt.password, tt.bio, tt.image)
			if u != tt.want {
				t.Errorf("got %+v, want %+v", u, tt.want)
			}
		})
	}
}

func userAggUpdateNilReceiver(t *testing.T) {
	var u *User
	u.Update("a", "b", "c", "d", "e") // must not panic
}

func userAggEqual(t *testing.T) {
	a := &User{ID: "1", Email: "a"}
	tests := []struct {
		name string
		u, o *User
		want bool
	}{
		{"same id different fields", a, &User{ID: "1", Email: "other"}, true},
		{"different id", a, &User{ID: "2", Email: "a"}, false},
		{"same pointer", a, a, true},
		{"both nil", nil, nil, true},
		{"nil receiver", nil, a, false},
		{"nil other", a, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.u.Equal(tt.o); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func userAggString(t *testing.T) {
	tests := []struct {
		name string
		u    *User
		want string
	}{
		{"nil", nil, "User<nil>"},
		{"full", &User{ID: "1", Email: "e", Username: "u", Password: "SECRETPW", Bio: "b", Image: "i"},
			"User{id=1, email=e, username=u, bio=b, image=i}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.u.String()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if strings.Contains(got, "SECRETPW") {
				t.Error("password leaked in String()")
			}
		})
	}
}

// ---------- FollowRelation domain ----------

func userAggNewFollowRelation(t *testing.T) {
	r := NewFollowRelation("u", "t")
	if r.UserID != "u" || r.TargetID != "t" {
		t.Errorf("got %+v", *r)
	}
}

func userAggFollowRelationEqual(t *testing.T) {
	r := NewFollowRelation("a", "b")
	tests := []struct {
		name string
		r, o *FollowRelation
		want bool
	}{
		{"same values", r, NewFollowRelation("a", "b"), true},
		{"reversed", r, NewFollowRelation("b", "a"), false},
		{"different user", r, NewFollowRelation("x", "b"), false},
		{"different target", r, NewFollowRelation("a", "x"), false},
		{"both nil", nil, nil, true},
		{"nil receiver", nil, r, false},
		{"nil other", r, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.Equal(tt.o); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func userAggFollowRelationString(t *testing.T) {
	var nilR *FollowRelation
	if got := nilR.String(); got != "FollowRelation<nil>" {
		t.Errorf("got %q", got)
	}
	if got := NewFollowRelation("a", "b").String(); got != "FollowRelation{userId=a, targetId=b}" {
		t.Errorf("got %q", got)
	}
}

func userAggSentinelErrors(t *testing.T) {
	if ErrNilUser == nil || ErrNilFollowRelation == nil || errors.Is(ErrNilUser, ErrNilFollowRelation) {
		t.Fatal("sentinels must be distinct non-nil errors")
	}
}

// ---------- UserRepository contract ----------

func userPortSaveAndFind(t *testing.T) {
	ctx := context.Background()
	repo := newUserTestMemRepo()
	u := userTestMustUser(t, "a@e", "alice", "pw", "bio", "img")
	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("save: %v", err)
	}
	lookups := []struct {
		name string
		fn   func() (*User, error)
	}{
		{"by id", func() (*User, error) { return repo.FindByID(ctx, u.ID) }},
		{"by username", func() (*User, error) { return repo.FindByUsername(ctx, "alice") }},
		{"by email", func() (*User, error) { return repo.FindByEmail(ctx, "a@e") }},
	}
	for _, l := range lookups {
		t.Run(l.name, func(t *testing.T) {
			got, err := l.fn()
			if err != nil || got == nil {
				t.Fatalf("got %v, %v", got, err)
			}
			if !got.Equal(u) || *got != *u {
				t.Errorf("got %+v, want %+v", *got, *u)
			}
		})
	}
}

func userPortFindMissing(t *testing.T) {
	ctx := context.Background()
	repo := newUserTestMemRepo()
	_ = repo.Save(ctx, userTestMustUser(t, "a@e", "alice", "pw", "", ""))
	tests := []struct {
		name string
		fn   func() (*User, error)
	}{
		{"unknown id", func() (*User, error) { return repo.FindByID(ctx, "nope") }},
		{"unknown username", func() (*User, error) { return repo.FindByUsername(ctx, "nope") }},
		{"unknown email", func() (*User, error) { return repo.FindByEmail(ctx, "nope@e") }},
		{"case differs username", func() (*User, error) { return repo.FindByUsername(ctx, "ALICE") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn()
			if !errors.Is(err, userTestErrNotFound) {
				t.Errorf("err = %v, want not found", err)
			}
			if got != nil {
				t.Errorf("expected nil user, got %+v", got)
			}
		})
	}
}

func userPortSaveUpdatesExisting(t *testing.T) {
	ctx := context.Background()
	repo := newUserTestMemRepo()
	u := userTestMustUser(t, "a@e", "alice", "pw", "", "")
	if err := repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}
	origID := u.ID
	u.Update("new@e", "alice2", "pw2", "newbio", "newimg")
	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("update save: %v", err)
	}
	if u.ID != origID {
		t.Fatal("id changed on update")
	}
	got, err := repo.FindByID(ctx, origID)
	if err != nil || *got != *u {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := repo.FindByUsername(ctx, "alice"); !errors.Is(err, userTestErrNotFound) {
		t.Errorf("old username still found: %v", err)
	}
	if _, err := repo.FindByEmail(ctx, "a@e"); !errors.Is(err, userTestErrNotFound) {
		t.Errorf("old email still found: %v", err)
	}
	if g, err := repo.FindByUsername(ctx, "alice2"); err != nil || g.ID != origID {
		t.Errorf("new username lookup: %v %v", g, err)
	}
	if g, err := repo.FindByEmail(ctx, "new@e"); err != nil || g.ID != origID {
		t.Errorf("new email lookup: %v %v", g, err)
	}
}

func userPortSaveIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := newUserTestMemRepo()
	u := userTestMustUser(t, "a@e", "alice", "pw", "b", "i")
	for i := 0; i < 3; i++ {
		if err := repo.Save(ctx, u); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	if len(repo.users) != 1 {
		t.Errorf("want 1 user, got %d", len(repo.users))
	}
	got, _ := repo.FindByID(ctx, u.ID)
	if *got != *u {
		t.Errorf("got %+v", *got)
	}
}

func userPortSaveErrors(t *testing.T) {
	ctx := context.Background()
	repo := newUserTestMemRepo()
	_ = repo.Save(ctx, userTestMustUser(t, "a@e", "alice", "pw", "", ""))
	tests := []struct {
		name string
		u    *User
		want error
	}{
		{"nil user", nil, ErrNilUser},
		{"duplicate username", userTestMustUser(t, "other@e", "alice", "pw", "", ""), userTestErrConflict},
		{"duplicate email", userTestMustUser(t, "a@e", "other", "pw", "", ""), userTestErrConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := repo.Save(ctx, tt.u); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
	if len(repo.users) != 1 {
		t.Errorf("failed saves must not persist; have %d users", len(repo.users))
	}
}

func userPortRelations(t *testing.T) {
	ctx := context.Background()
	repo := newUserTestMemRepo()
	ab := NewFollowRelation("A", "B")

	if err := repo.SaveRelation(ctx, ab); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveRelation(ctx, NewFollowRelation("A", "B")); err != nil {
		t.Fatalf("duplicate save should be no-op: %v", err)
	}
	if len(repo.relations) != 1 {
		t.Errorf("duplicate relation created: %d", len(repo.relations))
	}
	_ = repo.SaveRelation(ctx, NewFollowRelation("A", "C"))

	finds := []struct {
		name     string
		u, t     string
		wantFind bool
	}{
		{"saved", "A", "B", true},
		{"reverse not present", "B", "A", false},
		{"unknown pair", "X", "Y", false},
		{"other relation", "A", "C", true},
	}
	for _, f := range finds {
		t.Run("find "+f.name, func(t *testing.T) {
			r, err := repo.FindRelation(ctx, f.u, f.t)
			if f.wantFind {
				if err != nil || r == nil || r.UserID != f.u || r.TargetID != f.t {
					t.Fatalf("got %v, %v", r, err)
				}
			} else if !errors.Is(err, userTestErrNotFound) || r != nil {
				t.Fatalf("expected not found, got %v, %v", r, err)
			}
		})
	}

	if err := repo.RemoveRelation(ctx, NewFollowRelation("A", "B")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindRelation(ctx, "A", "B"); !errors.Is(err, userTestErrNotFound) {
		t.Errorf("relation still present: %v", err)
	}
	if _, err := repo.FindRelation(ctx, "A", "C"); err != nil {
		t.Errorf("unrelated relation removed: %v", err)
	}
	if err := repo.RemoveRelation(ctx, NewFollowRelation("A", "B")); err != nil {
		t.Errorf("removing missing relation must not error: %v", err)
	}
	if err := repo.RemoveRelation(ctx, NewFollowRelation("Q", "R")); err != nil {
		t.Errorf("removing unknown relation must not error: %v", err)
	}
}

func userPortRemoveRelationKeepsReverseAndUsers(t *testing.T) {
	ctx := context.Background()
	repo := newUserTestMemRepo()
	a := userTestMustUser(t, "a@e", "a", "pw", "", "")
	b := userTestMustUser(t, "b@e", "b", "pw", "", "")
	_ = repo.Save(ctx, a)
	_ = repo.Save(ctx, b)
	_ = repo.SaveRelation(ctx, NewFollowRelation(a.ID, b.ID))
	_ = repo.SaveRelation(ctx, NewFollowRelation(b.ID, a.ID))
	if err := repo.RemoveRelation(ctx, NewFollowRelation(a.ID, b.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindRelation(ctx, b.ID, a.ID); err != nil {
		t.Errorf("reverse relation removed: %v", err)
	}
	for _, id := range []string{a.ID, b.ID} {
		if _, err := repo.FindByID(ctx, id); err != nil {
			t.Errorf("user %s affected: %v", id, err)
		}
	}
}

func userPortNilRelationErrors(t *testing.T) {
	ctx := context.Background()
	repo := newUserTestMemRepo()
	tests := []struct {
		name string
		fn   func() error
	}{
		{"save nil", func() error { return repo.SaveRelation(ctx, nil) }},
		{"remove nil", func() error { return repo.RemoveRelation(ctx, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); !errors.Is(err, ErrNilFollowRelation) {
				t.Errorf("err = %v", err)
			}
		})
	}
}