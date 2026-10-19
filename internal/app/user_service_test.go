package app

import (
	"encoding/json"
	"errors"
	"testing"
)

type userFields struct {
	email, password, username, bio, image string
}

func fieldsOf(p UpdateUserParam) userFields {
	return userFields{p.Email(), p.Password(), p.Username(), p.Bio(), p.Image()}
}

func TestUpdateUserParam_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    userFields
		wantErr error
		anyErr  bool
	}{
		{
			name: "all fields",
			body: `{"user":{"email":"a@b.com","username":"u","password":"p","bio":"b","image":"i"}}`,
			want: userFields{"a@b.com", "p", "u", "b", "i"},
		},
		{
			name: "partial fields",
			body: `{"user":{"bio":"x"}}`,
			want: userFields{bio: "x"},
		},
		{
			name: "empty user object",
			body: `{"user":{}}`,
			want: userFields{},
		},
		{
			name: "explicit null field becomes empty",
			body: `{"user":{"email":null,"bio":"x"}}`,
			want: userFields{bio: "x"},
		},
		{
			name: "unknown properties ignored",
			body: `{"user":{"username":"u","foo":"bar","nested":{"a":1}},"other":1}`,
			want: userFields{username: "u"},
		},
		{
			name:    "missing root",
			body:    `{"email":"a@b.com"}`,
			wantErr: ErrMissingUserRoot,
		},
		{
			name:    "null root",
			body:    `{"user": null }`,
			wantErr: ErrMissingUserRoot,
		},
		{
			name:   "invalid json",
			body:   `{"user":`,
			anyErr: true,
		},
		{
			name:   "wrong field type",
			body:   `{"user":{"email":123}}`,
			anyErr: true,
		},
		{
			name:   "user not object",
			body:   `{"user":"str"}`,
			anyErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p UpdateUserParam
			err := json.Unmarshal([]byte(tt.body), &p)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if tt.anyErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got := fieldsOf(p); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestUpdateUserParamBuilder(t *testing.T) {
	tests := []struct {
		name  string
		build func() UpdateUserParam
		want  userFields
	}{
		{"no setters", func() UpdateUserParam { return NewUpdateUserParamBuilder().Build() }, userFields{}},
		{"email only", func() UpdateUserParam { return NewUpdateUserParamBuilder().Email("a@b.com").Build() }, userFields{email: "a@b.com"}},
		{"all", func() UpdateUserParam {
			return NewUpdateUserParamBuilder().Email("e").Password("p").Username("u").Bio("b").Image("i").Build()
		}, userFields{"e", "p", "u", "b", "i"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fieldsOf(tt.build()); got != tt.want {
				t.Errorf("got %+v want %+v", got, tt.want)
			}
		})
	}
}