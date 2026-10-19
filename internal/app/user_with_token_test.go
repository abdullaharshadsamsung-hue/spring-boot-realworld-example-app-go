package app

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNewUserWithToken(t *testing.T) {
	tests := []struct {
		name  string
		data  UserData
		token string
		want  [5]string // email, username, bio, image, token
	}{
		{"full", NewUserData("id1", "a@b.com", "alice", "hi", "img.png"), "jwt123",
			[5]string{"a@b.com", "alice", "hi", "img.png", "jwt123"}},
		// MIGRATION: null bio/image map to "" in Go.
		{"null-like bio image", UserData{ID: "1", Email: "a@b.com", Username: "alice"}, "t",
			[5]string{"a@b.com", "alice", "", "", "t"}},
		{"empty bio image", NewUserData("1", "a@b.com", "alice", "", ""), "t",
			[5]string{"a@b.com", "alice", "", "", "t"}},
		// MIGRATION: null token maps to "".
		{"empty token", NewUserData("1", "a@b.com", "alice", "hi", "img"), "",
			[5]string{"a@b.com", "alice", "hi", "img", ""}},
		// MIGRATION: null userData cannot occur (value type); zero value yields empty fields.
		{"zero user data", UserData{}, "tok", [5]string{"", "", "", "", "tok"}},
		{"no transformation", NewUserData("1", " A@B.com ", " alice ", " bio ", " img "), " tok ",
			[5]string{" A@B.com ", " alice ", " bio ", " img ", " tok "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := NewUserWithToken(tt.data, tt.token)
			for i := 0; i < 3; i++ {
				got := [5]string{u.Email(), u.Username(), u.Bio(), u.Image(), u.Token()}
				if got != tt.want {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestUserWithTokenIndependentOfSource(t *testing.T) {
	d := NewUserData("1", "a@b.com", "alice", "hi", "img.png")
	u := NewUserWithToken(d, "jwt")
	d.Email, d.Username, d.Bio, d.Image = "x", "y", "z", "w"
	if u.Email() != "a@b.com" || u.Username() != "alice" || u.Bio() != "hi" || u.Image() != "img.png" || u.Token() != "jwt" {
		t.Fatalf("values changed after source mutation: %+v", u)
	}
}

func TestUserWithTokenShape(t *testing.T) {
	typ := reflect.TypeOf(UserWithToken{})
	if typ.NumField() != 5 {
		t.Fatalf("want 5 fields, got %d", typ.NumField())
	}
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).IsExported() {
			t.Errorf("field %s is exported", typ.Field(i).Name)
		}
	}
	for _, name := range []string{"SetEmail", "SetUsername", "SetBio", "SetImage", "SetToken", "ID", "Id"} {
		if _, ok := reflect.PointerTo(typ).MethodByName(name); ok {
			t.Errorf("unexpected method %s", name)
		}
	}
}

func TestUserWithTokenMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		u    UserWithToken
		want map[string]string
	}{
		{"full", NewUserWithToken(NewUserData("id1", "a@b.com", "alice", "hi", "img.png"), "jwt123"),
			map[string]string{"email": "a@b.com", "username": "alice", "bio": "hi", "image": "img.png", "token": "jwt123"}},
		{"empty", NewUserWithToken(UserData{ID: "id1"}, ""),
			map[string]string{"email": "", "username": "", "bio": "", "image": "", "token": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.u)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]interface{}
			if err := json.Unmarshal(b, &raw); err != nil {
				t.Fatal(err)
			}
			if len(raw) != 5 {
				t.Fatalf("want 5 keys, got %v", raw)
			}
			for _, k := range []string{"id", "password"} {
				if _, ok := raw[k]; ok {
					t.Errorf("unexpected key %q", k)
				}
			}
			for k, v := range tt.want {
				if raw[k] != v {
					t.Errorf("key %s: got %v want %q", k, raw[k], v)
				}
			}
		})
	}
}