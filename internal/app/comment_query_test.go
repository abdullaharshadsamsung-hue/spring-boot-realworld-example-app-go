package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"migrated-app/internal/core"
)

type fakeCommentReader struct {
	byID       map[string]CommentData
	byArticle  []CommentData
	cursorRows []CommentData
	err        error
	gotPage    CursorPageParameter
}

func (f *fakeCommentReader) FindByID(ctx context.Context, id string) (CommentData, bool, error) {
	if f.err != nil {
		return CommentData{}, false, f.err
	}
	c, ok := f.byID[id]
	return c, ok, nil
}

func (f *fakeCommentReader) FindByArticleID(ctx context.Context, articleID string) ([]CommentData, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byArticle, nil
}

func (f *fakeCommentReader) FindByArticleIDWithCursor(ctx context.Context, articleID string, page CursorPageParameter) ([]CommentData, error) {
	f.gotPage = page
	if f.err != nil {
		return nil, f.err
	}
	return f.cursorRows, nil
}

type fakeRelations struct {
	follows       map[string]map[string]bool
	err           error
	isCalls       int
	followingCall int
}

var baseTime = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

func mkComment(id, authorID, username string, offset int) CommentData {
	return NewCommentData(id, "content", "123", baseTime.Add(time.Duration(offset)*time.Minute), baseTime,
		&ProfileData{ID: authorID, Username: username})
}

func TestCommentQueryService_FindByID(t *testing.T) {
	me := &core.User{ID: "u1", Username: "aisensiy"}
	readErr := errors.New("db down")
	tests := []struct {
		name          string
		reader        *fakeCommentReader
		rel           *fakeRelations
		id            string
		user          *core.User
		wantOK        bool
		wantErr       error
		wantFollowing bool
		wantUsername  string
		wantIsCalls   int
	}{
		{"own comment", &fakeCommentReader{byID: map[string]CommentData{"c1": mkComment("c1", "u1", "aisensiy", 0)}},
			&fakeRelations{}, "c1", me, true, nil, false, "aisensiy", 1},
		{"not found", &fakeCommentReader{byID: map[string]CommentData{}}, &fakeRelations{}, "x", me, false, nil, false, "", 0},
		{"follows author", &fakeCommentReader{byID: map[string]CommentData{"c1": mkComment("c1", "u2", "user2", 0)}},
			&fakeRelations{follows: map[string]map[string]bool{"u1": {"u2": true}}}, "c1", me, true, nil, true, "user2", 1},
		{"not following", &fakeCommentReader{byID: map[string]CommentData{"c1": mkComment("c1", "u2", "user2", 0)}},
			&fakeRelations{}, "c1", me, true, nil, false, "user2", 1},
		{"nil user skips lookup", &fakeCommentReader{byID: map[string]CommentData{"c1": mkComment("c1", "u2", "user2", 0)}},
			&fakeRelations{}, "c1", nil, true, nil, false, "user2", 0},
		{"reader error", &fakeCommentReader{err: readErr}, &fakeRelations{}, "c1", me, false, readErr, false, "", 0},
		{"relation error", &fakeCommentReader{byID: map[string]CommentData{"c1": mkComment("c1", "u2", "user2", 0)}},
			&fakeRelations{err: readErr}, "c1", me, false, readErr, false, "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewCommentQueryService(tt.reader, tt.rel)
			got, ok, err := s.FindByID(context.Background(), tt.id, tt.user)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v", ok)
			}
			if tt.rel.isCalls != tt.wantIsCalls {
				t.Errorf("isCalls = %d, want %d", tt.rel.isCalls, tt.wantIsCalls)
			}
			if !ok {
				return
			}
			if got.Author.Username != tt.wantUsername || got.Author.Following != tt.wantFollowing {
				t.Errorf("author = %+v", got.Author)
			}
		})
	}
}