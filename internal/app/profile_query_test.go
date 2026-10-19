package app

import (
	"context"
	"errors"
	"testing"

	"migrated-app/internal/core"
)

type fakeUserReadService struct {
	users map[string]UserData
	err   error
	calls int
}

func (f *fakeUserReadService) FindByUsername(ctx context.Context, username string) (UserData, bool, error) {
	f.calls++
	if f.err != nil {
		return UserData{}, false, f.err
	}
	u, ok := f.users[username]
	return u, ok, nil
}

type fakeRelationService struct {
	follows map[[2]string]bool
	err     error
	calls   int
	lastU   string
	lastT   string
}

func (f *fakeRelationService) IsUserFollowing(ctx context.Context, userID, anotherUserID string) (bool, error) {
	f.calls++
	f.lastU, f.lastT = userID, anotherUserID
	if f.err != nil {
		return false, f.err
	}
	return f.follows[[2]string{userID, anotherUserID}], nil
}

func TestNewProfileQueryService(t *testing.T) {
	tests := []struct {
		name    string
		users   UserReadService
		rels    UserRelationshipQueryService
		wantErr error
	}{
		{"ok", &fakeUserReadService{}, &fakeRelationService{}, nil},
		{"nil users", nil, &fakeRelationService{}, ErrNilUserReadService},
		{"nil relations", &fakeUserReadService{}, nil, ErrNilUserRelationshipQueryService},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewProfileQueryService(tt.users, tt.rels)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && s == nil {
				t.Fatal("expected service")
			}
			if tt.wantErr != nil && s != nil {
				t.Fatal("expected nil service")
			}
		})
	}
}

func TestProfileQueryService_FindByUsername(t *testing.T) {
	stored := NewUserData("pid", "p@test.com", "p", "", "")
	stored2 := NewUserData("qid", "q@test.com", "q", "bio q", "img q")
	current := &core.User{ID: "aid", Email: "a@test.com", Username: "a", Password: "123"}
	dbErr := errors.New("db down")

	tests := []struct {
		name          string
		username      string
		current       *core.User
		follows       map[[2]string]bool
		userErr       error
		relErr        error
		wantFound     bool
		wantProfile   ProfileData
		wantErr       error
		wantRelCalls  int
	}{
		{name: "existing user unsaved current", username: "p", current: current, wantFound: true,
			wantProfile: NewProfileData("pid", "p", "", "", false), wantRelCalls: 1},
		{name: "unknown username", username: "nobody", current: current, wantFound: false, wantRelCalls: 0},
		{name: "nil current", username: "q", current: nil, wantFound: true,
			wantProfile: NewProfileData("qid", "q", "bio q", "img q", false), wantRelCalls: 0},
		{name: "following", username: "q", current: current, follows: map[[2]string]bool{{"aid", "qid"}: true},
			wantFound: true, wantProfile: NewProfileData("qid", "q", "bio q", "img q", true), wantRelCalls: 1},
		{name: "not following", username: "q", current: current, follows: map[[2]string]bool{{"qid", "aid"}: true},
			wantFound: true, wantProfile: NewProfileData("qid", "q", "bio q", "img q", false), wantRelCalls: 1},
		{name: "user lookup error", username: "q", current: current, userErr: dbErr, wantErr: dbErr},
		{name: "relation error", username: "q", current: current, relErr: dbErr, wantErr: dbErr, wantRelCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := &fakeUserReadService{users: map[string]UserData{"p": stored, "q": stored2}, err: tt.userErr}
			rels := &fakeRelationService{follows: tt.follows, err: tt.relErr}
			s, err := NewProfileQueryService(users, rels)
			if err != nil {
				t.Fatal(err)
			}
			got, found, err := s.FindByUsername(context.Background(), tt.username, tt.current)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want wrapping %v", err, tt.wantErr)
				}
				if found || got != (ProfileData{}) {
					t.Fatalf("expected empty result on error, got %+v %v", got, found)
				}
			} else if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if tt.wantErr == nil && !got.Equal(tt.wantProfile) {
				t.Fatalf("profile = %+v, want %+v", got, tt.wantProfile)
			}
			if rels.calls != tt.wantRelCalls {
				t.Fatalf("relation calls = %d, want %d", rels.calls, tt.wantRelCalls)
			}
			if rels.calls > 0 && tt.current != nil && (rels.lastU != tt.current.ID) {
				t.Fatalf("relation queried with user %q, want %q", rels.lastU, tt.current.ID)
			}
			if users.calls != 1 {
				t.Fatalf("user lookup calls = %d, want 1", users.calls)
			}
		})
	}
}