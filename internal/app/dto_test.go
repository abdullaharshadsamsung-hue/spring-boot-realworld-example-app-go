package app

import (
	"encoding/json"
	"testing"
)

func TestNewProfileData(t *testing.T) {
	tests := []struct {
		name                         string
		id, username, bio, image     string
		following                    bool
	}{
		{"user fixture following false", "u1", "johnjacob", "I like to skateboard", "https://i.stack.imgur.com/xHWG8.jpg", false},
		{"empty bio and image", "u2", "jane", "", "", false},
		{"following true", "u3", "bob", "bio", "img", true},
		{"all empty", "", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProfileData(tt.id, tt.username, tt.bio, tt.image, tt.following)
			if p.ID != tt.id || p.Username != tt.username || p.Bio != tt.bio || p.Image != tt.image || p.Following != tt.following {
				t.Fatalf("got %+v, want positional values %q %q %q %q %v", p, tt.id, tt.username, tt.bio, tt.image, tt.following)
			}
		})
	}
}

func TestProfileDataZeroValueAndSetters(t *testing.T) {
	var p ProfileData
	if p.ID != "" || p.Username != "" || p.Bio != "" || p.Image != "" || p.Following {
		t.Fatalf("zero value not empty: %+v", p)
	}

	tests := []struct {
		name  string
		set   func(*ProfileData)
		check func(ProfileData) bool
	}{
		{"id", func(p *ProfileData) { p.ID = "id1" }, func(p ProfileData) bool { return p.ID == "id1" }},
		{"username", func(p *ProfileData) { p.Username = "u" }, func(p ProfileData) bool { return p.Username == "u" }},
		{"bio", func(p *ProfileData) { p.Bio = "b" }, func(p ProfileData) bool { return p.Bio == "b" }},
		{"image", func(p *ProfileData) { p.Image = "i" }, func(p ProfileData) bool { return p.Image == "i" }},
		{"following", func(p *ProfileData) { p.Following = true }, func(p ProfileData) bool { return p.Following }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p ProfileData
			tt.set(&p)
			if !tt.check(p) {
				t.Fatalf("setter not reflected: %+v", p)
			}
		})
	}
}

func TestProfileDataJSON(t *testing.T) {
	tests := []struct {
		name string
		in   ProfileData
		want map[string]interface{}
	}{
		{"populated", NewProfileData("secret-id", "jake", "bio", "img", true),
			map[string]interface{}{"username": "jake", "bio": "bio", "image": "img", "following": true}},
		{"zero value", ProfileData{},
			map[string]interface{}{"username": "", "bio": "", "image": "", "following": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]interface{}
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("keys mismatch: %s", b)
			}
			for k, v := range tt.want {
				gv, ok := got[k]
				if !ok || gv != v {
					t.Errorf("key %q: got %v, want %v", k, gv, v)
				}
			}
			if _, ok := got["following"].(bool); !ok {
				t.Errorf("following not bool: %s", b)
			}
			for _, k := range []string{"id", "ID", "Id"} {
				if _, ok := got[k]; ok {
					t.Errorf("id exposed: %s", b)
				}
			}
		})
	}

	t.Run("id not deserialized", func(t *testing.T) {
		var p ProfileData
		if err := json.Unmarshal([]byte(`{"id":"x","ID":"x","username":"u"}`), &p); err != nil {
			t.Fatal(err)
		}
		if p.ID != "" || p.Username != "u" {
			t.Fatalf("got %+v", p)
		}
	})
}

func TestProfileDataEqual(t *testing.T) {
	base := NewProfileData("id", "u", "b", "i", false)
	tests := []struct {
		name  string
		other ProfileData
		want  bool
	}{
		{"identical", NewProfileData("id", "u", "b", "i", false), true},
		{"diff id", NewProfileData("id2", "u", "b", "i", false), false},
		{"diff username", NewProfileData("id", "u2", "b", "i", false), false},
		{"diff bio", NewProfileData("id", "u", "b2", "i", false), false},
		{"diff image", NewProfileData("id", "u", "b", "i2", false), false},
		{"diff following", NewProfileData("id", "u", "b", "i", true), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base.Equal(tt.other); got != tt.want {
				t.Errorf("Equal = %v, want %v", got, tt.want)
			}
			if got := tt.other.Equal(base); got != tt.want {
				t.Errorf("symmetric Equal = %v, want %v", got, tt.want)
			}
			if got := base == tt.other; got != tt.want {
				t.Errorf("== = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("usable as map key (hash consistency)", func(t *testing.T) {
		m := map[ProfileData]int{base: 1}
		if m[NewProfileData("id", "u", "b", "i", false)] != 1 {
			t.Fatal("equal values should hash identically")
		}
	})
}