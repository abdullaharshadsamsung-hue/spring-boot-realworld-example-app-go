package app

import (
	"reflect"
	"testing"

	"migrated-app/internal/core"
)

func TestNewUpdateUserCommand(t *testing.T) {
	user := &core.User{
		ID:       "id-1",
		Email:    "a@example.com",
		Username: "alice",
		Password: "secret",
		Bio:      "bio",
		Image:    "img",
	}
	emptyUser := &core.User{}

	tests := []struct {
		name  string
		user  *core.User
		param UpdateUserParam
	}{
		{name: "non-nil user and zero param", user: user, param: UpdateUserParam{}},
		{name: "zero-value user", user: emptyUser, param: UpdateUserParam{}},
		{name: "nil user", user: nil, param: UpdateUserParam{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var snapshot core.User
			if tt.user != nil {
				snapshot = *tt.user
			}

			cmd := NewUpdateUserCommand(tt.user, tt.param)

			if got := cmd.TargetUser(); got != tt.user {
				t.Fatalf("TargetUser() = %p, want same pointer %p", got, tt.user)
			}
			if got := cmd.Param(); !reflect.DeepEqual(got, tt.param) {
				t.Fatalf("Param() = %#v, want %#v", got, tt.param)
			}

			// Repeated calls return the same values.
			for i := 0; i < 3; i++ {
				if cmd.TargetUser() != tt.user {
					t.Fatalf("TargetUser() changed on call %d", i)
				}
				if !reflect.DeepEqual(cmd.Param(), tt.param) {
					t.Fatalf("Param() changed on call %d", i)
				}
			}

			// Constructor and getters must not modify the user.
			if tt.user != nil && *tt.user != snapshot {
				t.Fatalf("user mutated: got %+v, want %+v", *tt.user, snapshot)
			}
		})
	}
}

func TestUpdateUserCommand_ZeroValue(t *testing.T) {
	var cmd UpdateUserCommand
	if cmd.TargetUser() != nil {
		t.Fatalf("zero command TargetUser() = %v, want nil", cmd.TargetUser())
	}
	if !reflect.DeepEqual(cmd.Param(), UpdateUserParam{}) {
		t.Fatalf("zero command Param() = %#v, want zero", cmd.Param())
	}
}