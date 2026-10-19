package app

import (
	"context"
	"errors"
	"testing"

	"migrated-app/internal/core"
)

type fakeUsernameFinder struct {
	users map[string]*core.User
	err   error
	calls []string
}

func (f *fakeUsernameFinder) FindByUsername(ctx context.Context, username string) (*core.User, error) {
	f.calls = append(f.calls, username)
	if f.err != nil {
		return nil, f.err
	}
	if u, ok := f.users[username]; ok {
		return u, nil
	}
	return nil, core.ErrNotFound
}

func TestNewDuplicatedUsernameValidator_Nil(t *testing.T) {
	v, err := NewDuplicatedUsernameValidator(nil)
	if v != nil || !errors.Is(err, ErrNilUserByUsernameFinder) {
		t.Fatalf("got %v, %v", v, err)
	}
}

func TestDuplicatedUsernameValidator_IsValid(t *testing.T) {
	dbErr := errors.New("db down")
	existing := map[string]*core.User{
		"alice": {ID: "1", Username: "alice"},
		"   ":   {ID: "2", Username: "   "},
	}
	tests := []struct {
		name      string
		users     map[string]*core.User
		finderErr error
		input     string
		want      bool
		wantErr   error
		wantCalls []string
	}{
		{"empty is valid without lookup", existing, nil, "", true, nil, nil},
		{"empty valid even if repo fails", nil, dbErr, "", true, nil, nil},
		{"unknown username valid", existing, nil, "bob", true, nil, []string{"bob"}},
		{"existing username invalid", existing, nil, "alice", false, nil, []string{"alice"}},
		{"case not normalized", existing, nil, "Alice", true, nil, []string{"Alice"}},
		{"whitespace existing invalid", existing, nil, "   ", false, nil, []string{"   "}},
		{"whitespace not existing valid", map[string]*core.User{}, nil, "  ", true, nil, []string{"  "}},
		{"untrimmed passed as is", existing, nil, " alice ", true, nil, []string{" alice "}},
		{"lookup error propagates", nil, dbErr, "alice", false, dbErr, []string{"alice"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeUsernameFinder{users: tc.users, err: tc.finderErr}
			v, err := NewDuplicatedUsernameValidator(f)
			if err != nil {
				t.Fatal(err)
			}
			got, err := v.IsValid(context.Background(), tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected err %v", err)
			} else if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if len(f.calls) != len(tc.wantCalls) {
				t.Fatalf("calls = %q, want %q", f.calls, tc.wantCalls)
			}
			for i := range f.calls {
				if f.calls[i] != tc.wantCalls[i] {
					t.Fatalf("calls = %q, want %q", f.calls, tc.wantCalls)
				}
			}
			// determinism
			if tc.wantErr == nil {
				again, _ := v.IsValid(context.Background(), tc.input)
				if again != got {
					t.Fatalf("non-deterministic result")
				}
			}
		})
	}
}