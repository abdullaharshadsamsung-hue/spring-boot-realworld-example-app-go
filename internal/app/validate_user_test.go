package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"migrated-app/internal/core"
)

type fakeEmailFinder struct {
	users map[string]*core.User
	err   error
	calls []string
}

func (f *fakeEmailFinder) FindByEmail(ctx context.Context, email string) (*core.User, error) {
	f.calls = append(f.calls, email)
	if f.err != nil {
		return nil, f.err
	}
	if u, ok := f.users[email]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("find by email: %w", core.ErrNotFound)
}

func TestNewDuplicatedEmailValidator_Nil(t *testing.T) {
	v, err := NewDuplicatedEmailValidator(nil)
	if !errors.Is(err, ErrNilUserByEmailFinder) {
		t.Fatalf("expected ErrNilUserByEmailFinder, got %v", err)
	}
	if v != nil {
		t.Fatalf("expected nil validator")
	}
}

func TestDuplicatedEmailValidator_IsValid(t *testing.T) {
	registered := map[string]*core.User{
		"taken@example.com": {ID: "1", Email: "taken@example.com"},
		"   ":               {ID: "2", Email: "   "},
	}
	repoErr := errors.New("db down")

	tests := []struct {
		name      string
		users     map[string]*core.User
		findErr   error
		email     string
		want      bool
		wantErr   error
		wantCalls []string
	}{
		{name: "empty email skips repository", users: registered, email: "", want: true, wantCalls: nil},
		{name: "unregistered email", users: registered, email: "new@example.com", want: true, wantCalls: []string{"new@example.com"}},
		{name: "registered email", users: registered, email: "taken@example.com", want: false, wantCalls: []string{"taken@example.com"}},
		{name: "whitespace registered", users: registered, email: "   ", want: false, wantCalls: []string{"   "}},
		{name: "whitespace not registered", users: map[string]*core.User{}, email: "   ", want: true, wantCalls: []string{"   "}},
		{name: "case differs no normalization", users: registered, email: "TAKEN@example.com", want: true, wantCalls: []string{"TAKEN@example.com"}},
		{name: "surrounding whitespace not trimmed", users: registered, email: " taken@example.com ", want: true, wantCalls: []string{" taken@example.com "}},
		{name: "repository error propagates", findErr: repoErr, email: "x@example.com", want: false, wantErr: repoErr, wantCalls: []string{"x@example.com"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeEmailFinder{users: tc.users, err: tc.findErr}
			v, err := NewDuplicatedEmailValidator(f)
			if err != nil {
				t.Fatalf("constructor: %v", err)
			}
			got, err := v.IsValid(context.Background(), tc.email)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("IsValid = %v, want %v", got, tc.want)
			}
			if len(f.calls) != len(tc.wantCalls) {
				t.Fatalf("calls = %q, want %q", f.calls, tc.wantCalls)
			}
			for i := range f.calls {
				if f.calls[i] != tc.wantCalls[i] {
					t.Errorf("call %d = %q, want %q", i, f.calls[i], tc.wantCalls[i])
				}
			}
		})
	}
}