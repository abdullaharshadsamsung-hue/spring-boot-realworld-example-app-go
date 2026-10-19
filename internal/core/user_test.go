package core

import (
	"regexp"
	"strings"
	"testing"
)

var userUUIDv4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUser(t *testing.T) {
	tests := []struct {
		name                                  string
		email, username, password, bio, image string
	}{
		{"all valid", "a@b.com", "alice", "hashed", "hello", "http://img/x.png"},
		{"some empty", "", "bob", "", "", ""},
		{"all empty", "", "", "", "", ""},
		{"whitespace not trimmed", " a@b.com ", "  bob", "p ", " ", "\t"},
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
			if !userUUIDv4Re.MatchString(u.ID) || len(u.ID) != 36 {
				t.Errorf("ID %q not canonical uuid v4", u.ID)
			}
			if u.Email != tt.email || u.Username != tt.username || u.Password != tt.password ||
				u.Bio != tt.bio || u.Image != tt.image {
				t.Errorf("fields not stored as-is: %+v", *u)
			}
		})
	}
}

func TestNewUserUniqueIDs(t *testing.T) {
	seen := map[string]bool{}
	var prev *User
	for i := 0; i < 200; i++ {
		u, err := NewUser("a@b.com", "alice", "p", "b", "i")
		if err != nil {
			t.Fatal(err)
		}
		if seen[u.ID] {
			t.Fatalf("duplicate id %s", u.ID)
		}
		seen[u.ID] = true
		if prev != nil && prev.Equal(u) {
			t.Fatal("users with identical args should not be equal")
		}
		prev = u
	}
}

func TestZeroValueUser(t *testing.T) {
	var u User
	if u.ID != "" || u.Email != "" || u.Username != "" || u.Password != "" || u.Bio != "" || u.Image != "" {
		t.Errorf("zero value not empty: %+v", u)
	}
}

func TestUserUpdate(t *testing.T) {
	base := User{ID: "id-1", Email: "e", Username: "u", Password: "p", Bio: "b", Image: "i"}
	tests := []struct {
		name                                  string
		email, username, password, bio, image string
		want                                  User
	}{
		{"all non-empty", "e2", "u2", "p2", "b2", "i2", User{ID: "id-1", Email: "e2", Username: "u2", Password: "p2", Bio: "b2", Image: "i2"}},
		{"all empty", "", "", "", "", "", base},
		{"only email", "e2", "", "", "", "", User{ID: "id-1", Email: "e2", Username: "u", Password: "p", Bio: "b", Image: "i"}},
		{"only image and bio", "", "", "", "b2", "i2", User{ID: "id-1", Email: "e", Username: "u", Password: "p", Bio: "b2", Image: "i2"}},
		{"whitespace overwrites", " ", "", "", "\t", "", User{ID: "id-1", Email: " ", Username: "u", Password: "p", Bio: "\t", Image: "i"}},
		{"same values", "e", "u", "p", "b", "i", base},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := base
			u.Update(tt.email, tt.username, tt.password, tt.bio, tt.image)
			if u != tt.want {
				t.Errorf("got %+v want %+v", u, tt.want)
			}
		})
	}
}

func TestUserUpdateKeepsIdentity(t *testing.T) {
	u, err := NewUser("e", "u", "p", "b", "i")
	if err != nil {
		t.Fatal(err)
	}
	before := *u
	key := u.ID
	u.Update("e2", "u2", "p2", "b2", "i2")
	if u.ID != key || !u.Equal(&before) || !u.Equal(u) {
		t.Error("identity changed after update")
	}
}

func TestUserEqual(t *testing.T) {
	var nilUser *User
	tests := []struct {
		name string
		a, b *User
		want bool
	}{
		{"same id different fields", &User{ID: "x", Email: "a"}, &User{ID: "x", Email: "b"}, true},
		{"different id same fields", &User{ID: "x", Email: "a"}, &User{ID: "y", Email: "a"}, false},
		{"two zero values", &User{}, &User{}, true},
		{"non-nil vs nil", &User{ID: "x"}, nil, false},
		{"nil vs non-nil", nil, &User{ID: "x"}, false},
		{"nil vs nil", nilUser, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("got %v want %v", got, tt.want)
			}
			if tt.want && tt.a != nil && tt.b != nil {
				m := map[string]bool{tt.a.ID: true}
				if !m[tt.b.ID] {
					t.Error("equal users should share map key")
				}
			}
		})
	}
}

func TestUserString(t *testing.T) {
	var nilUser *User
	if got := nilUser.String(); got != "User<nil>" {
		t.Errorf("got %q", got)
	}
	u := &User{ID: "id", Email: "e@x", Username: "bob", Password: "supersecret", Bio: "b", Image: "i"}
	s := u.String()
	if strings.Contains(s, "supersecret") {
		t.Errorf("password leaked: %s", s)
	}
	for _, want := range []string{"id", "e@x", "bob"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

func TestNewUUIDv4(t *testing.T) {
	for i := 0; i < 100; i++ {
		id, err := newUUIDv4()
		if err != nil {
			t.Fatal(err)
		}
		if !userUUIDv4Re.MatchString(id) {
			t.Fatalf("bad uuid %q", id)
		}
	}
}