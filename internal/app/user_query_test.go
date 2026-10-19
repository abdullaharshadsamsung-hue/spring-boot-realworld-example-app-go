package app

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeUserByIDReader struct {
	user    UserData
	found   bool
	err     error
	calls   int
	gotIDs  []string
	gotCtxs []context.Context
}

func (f *fakeUserByIDReader) FindByID(ctx context.Context, id string) (UserData, bool, error) {
	f.calls++
	f.gotIDs = append(f.gotIDs, id)
	f.gotCtxs = append(f.gotCtxs, ctx)
	return f.user, f.found, f.err
}

func TestNewUserQueryService(t *testing.T) {
	tests := []struct {
		name    string
		reader  UserByIDReader
		wantErr error
	}{
		{"nil reader", nil, ErrNilUserByIDReader},
		{"valid reader", &fakeUserByIDReader{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewUserQueryService(tt.reader)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && svc != nil {
				t.Fatalf("expected nil service on error")
			}
			if tt.wantErr == nil && svc == nil {
				t.Fatalf("expected non-nil service")
			}
		})
	}
}

type ctxKey struct{}

func TestUserQueryService_FindByID(t *testing.T) {
	dbErr := errors.New("db failure")
	existing := NewUserData("u1", "a@b.c", "alice", "bio", "img.png")

	tests := []struct {
		name      string
		id        string
		reader    *fakeUserByIDReader
		wantUser  UserData
		wantFound bool
		wantErr   error
	}{
		{"existing user", "u1", &fakeUserByIDReader{user: existing, found: true}, existing, true, nil},
		{"existing user with empty bio/image", "u2",
			&fakeUserByIDReader{user: NewUserData("u2", "x@y.z", "bob", "", ""), found: true},
			NewUserData("u2", "x@y.z", "bob", "", ""), true, nil},
		{"no matching user", "missing", &fakeUserByIDReader{}, UserData{}, false, nil},
		{"not found discards reader data", "missing", &fakeUserByIDReader{user: existing, found: false}, UserData{}, false, nil},
		{"empty id passed through", "", &fakeUserByIDReader{}, UserData{}, false, nil},
		{"reader error propagates", "u1", &fakeUserByIDReader{user: existing, found: true, err: dbErr}, UserData{}, false, dbErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewUserQueryService(tt.reader)
			if err != nil {
				t.Fatalf("constructor: %v", err)
			}
			ctx := context.WithValue(context.Background(), ctxKey{}, "v")
			got, found, err := svc.FindByID(ctx, tt.id)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want wrapping %v", err, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.id) {
					t.Errorf("error %q should mention id %q", err, tt.id)
				}
			} else if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
			if !got.Equal(tt.wantUser) {
				t.Errorf("user = %v, want %v", got, tt.wantUser)
			}
			if tt.reader.calls != 1 {
				t.Errorf("reader calls = %d, want 1", tt.reader.calls)
			}
			if len(tt.reader.gotIDs) == 1 && tt.reader.gotIDs[0] != tt.id {
				t.Errorf("reader got id %q, want %q", tt.reader.gotIDs[0], tt.id)
			}
			if len(tt.reader.gotCtxs) == 1 && tt.reader.gotCtxs[0].Value(ctxKey{}) != "v" {
				t.Errorf("context not propagated to reader")
			}
		})
	}
}