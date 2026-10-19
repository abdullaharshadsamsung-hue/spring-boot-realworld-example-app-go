package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func fixAuthzClock(t *testing.T) time.Time {
	t.Helper()
	fixed := time.Date(2024, 1, 2, 3, 4, 5, 6000000, time.UTC)
	old := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = old })
	return fixed
}

func TestErrNoAuthorization_Identity(t *testing.T) {
	if ErrNoAuthorization == nil {
		t.Fatal("ErrNoAuthorization is nil")
	}
	if ErrNoAuthorization.Error() != "no authorization" {
		t.Errorf("message = %q", ErrNoAuthorization.Error())
	}
	if errors.Unwrap(ErrNoAuthorization) != nil {
		t.Error("expected no cause")
	}
	if errors.Is(ErrNoAuthorization, ErrResourceNotFound) {
		t.Error("must be distinct from ErrResourceNotFound")
	}
}

func TestForbiddenStatusFor(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, http.StatusOK},
		{"direct", ErrNoAuthorization, http.StatusForbidden},
		{"wrapped", fmt.Errorf("delete comment %s: %w", "1", ErrNoAuthorization), http.StatusForbidden},
		{"double wrapped", fmt.Errorf("a: %w", fmt.Errorf("b: %w", ErrNoAuthorization)), http.StatusForbidden},
		{"not found", ErrResourceNotFound, http.StatusNotFound},
		{"other", errors.New("boom"), http.StatusInternalServerError},
		{"same text different error", errors.New("no authorization"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ForbiddenStatusFor(tc.err); got != tc.want {
				t.Errorf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestWriteAuthzError(t *testing.T) {
	fixed := fixAuthzClock(t)
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"direct", ErrNoAuthorization, http.StatusForbidden},
		{"wrapped", fmt.Errorf("update article: %w", ErrNoAuthorization), http.StatusForbidden},
		{"not found delegates", ErrResourceNotFound, http.StatusNotFound},
		{"other delegates", errors.New("x"), http.StatusInternalServerError},
		{"nil delegates", nil, http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodDelete, "/articles/foo", nil)
			WriteAuthzError(rec, req, tc.err)
			if rec.Code != tc.want {
				t.Fatalf("status %d want %d", rec.Code, tc.want)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("content-type %q", ct)
			}
			var b ErrorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
				t.Fatal(err)
			}
			if b.Status != tc.want || b.Error != http.StatusText(tc.want) || b.Message != "" || b.Path != "/articles/foo" {
				t.Errorf("body %+v", b)
			}
			if b.Timestamp != fixed.Format(timestampLayout) {
				t.Errorf("timestamp %q", b.Timestamp)
			}
		})
	}
}

// authzMockStore simulates a persistence dependency.
type authzStore interface{ Delete(id string) error }

type authzMockStore struct{ deleted []string }

func (m *authzMockStore) Delete(id string) error { m.deleted = append(m.deleted, id); return nil }

func TestAuthzHandlerFunc(t *testing.T) {
	fixAuthzClock(t)
	tests := []struct {
		name        string
		owner       string
		user        string
		swallow     bool
		wantStatus  int
		wantDeleted int
	}{
		{"not owner forbidden", "alice", "bob", false, http.StatusForbidden, 0},
		{"owner ok", "alice", "alice", false, http.StatusNoContent, 1},
		{"handled in app code", "alice", "bob", true, http.StatusOK, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := &authzMockStore{}
			var s authzStore = ms
			h := AuthzHandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
				var err error
				if tc.owner != tc.user {
					err = fmt.Errorf("delete: %w", ErrNoAuthorization)
				}
				if err != nil {
					if tc.swallow && errors.Is(err, ErrNoAuthorization) {
						w.WriteHeader(http.StatusOK)
						return nil
					}
					return err
				}
				if e := s.Delete("1"); e != nil {
					return e
				}
				w.WriteHeader(http.StatusNoContent)
				return nil
			})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/articles/x/comments/1", nil))
			if rec.Code != tc.wantStatus {
				t.Errorf("status %d want %d", rec.Code, tc.wantStatus)
			}
			for _, bad := range []int{401, 404, 500} {
				if rec.Code == bad {
					t.Errorf("forbidden status %d", bad)
				}
			}
			if len(ms.deleted) != tc.wantDeleted {
				t.Errorf("deleted %d want %d", len(ms.deleted), tc.wantDeleted)
			}
		})
	}
}

func TestPlainHandlerFunc_DoesNotMapForbidden(t *testing.T) {
	fixAuthzClock(t)
	h := HandlerFunc(func(w http.ResponseWriter, r *http.Request) error { return ErrNoAuthorization })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d", rec.Code)
	}
}