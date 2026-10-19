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

func fixClock(t *testing.T) time.Time {
	t.Helper()
	fixed := time.Date(2024, 1, 2, 3, 4, 5, 6000000, time.UTC)
	old := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = old })
	return fixed
}

func TestErrResourceNotFound_Basics(t *testing.T) {
	if ErrResourceNotFound == nil {
		t.Fatal("ErrResourceNotFound must not be nil")
	}
	if errors.Unwrap(ErrResourceNotFound) != nil {
		t.Error("ErrResourceNotFound should carry no cause")
	}
	if ErrResourceNotFound.Error() != "resource not found" {
		t.Errorf("unexpected message %q", ErrResourceNotFound.Error())
	}
}

func TestStatusFor(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, http.StatusOK},
		{"direct not found", ErrResourceNotFound, http.StatusNotFound},
		{"wrapped article", fmt.Errorf("article %q: %w", "slug", ErrResourceNotFound), http.StatusNotFound},
		{"wrapped comment", fmt.Errorf("comment 1: %w", ErrResourceNotFound), http.StatusNotFound},
		{"wrapped profile", fmt.Errorf("profile: %w", ErrResourceNotFound), http.StatusNotFound},
		{"double wrapped user", fmt.Errorf("outer: %w", fmt.Errorf("user: %w", ErrResourceNotFound)), http.StatusNotFound},
		{"joined", errors.Join(errors.New("x"), ErrResourceNotFound), http.StatusNotFound},
		{"other error", errors.New("boom"), http.StatusInternalServerError},
		{"same text different error", errors.New("resource not found"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := StatusFor(tc.err); got != tc.want {
				t.Errorf("StatusFor = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	fixed := fixClock(t)
	tests := []struct {
		name string
		err  error
		path string
		want int
	}{
		{"not found", ErrResourceNotFound, "/articles/unknown", http.StatusNotFound},
		{"wrapped not found", fmt.Errorf("x: %w", ErrResourceNotFound), "/profiles/bob", http.StatusNotFound},
		{"generic", errors.New("boom"), "/x", http.StatusInternalServerError},
		{"nil coerced to 500", nil, "/y", http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			WriteError(rec, req, tc.err)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("content-type = %q", ct)
			}
			var body ErrorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Status != tc.want || body.Error != http.StatusText(tc.want) {
				t.Errorf("body status/error = %d/%q", body.Status, body.Error)
			}
			if body.Message != "" {
				t.Errorf("message = %q, want empty", body.Message)
			}
			if body.Path != tc.path {
				t.Errorf("path = %q", body.Path)
			}
			if body.Timestamp != fixed.Format(timestampLayout) {
				t.Errorf("timestamp = %q", body.Timestamp)
			}
		})
	}
}

func TestHandlerFunc_ServeHTTP(t *testing.T) {
	fixClock(t)
	var _ http.Handler = HandlerFunc(func(w http.ResponseWriter, r *http.Request) error { return nil })
	tests := []struct {
		name     string
		h        HandlerFunc
		wantCode int
		wantBody string
		sideEff  int
	}{
		{
			name: "success passes through",
			h: func(w http.ResponseWriter, r *http.Request) error {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
				return nil
			},
			wantCode: http.StatusOK, wantBody: "ok",
		},
		{
			name: "not found aborts with 404 and no success body",
			h: func(w http.ResponseWriter, r *http.Request) error {
				return fmt.Errorf("article %q: %w", "unknown", ErrResourceNotFound)
			},
			wantCode: http.StatusNotFound,
		},
		{
			name: "state change before not found still 404",
			h: func(w http.ResponseWriter, r *http.Request) error {
				return ErrResourceNotFound
			},
			wantCode: http.StatusNotFound, sideEff: 1,
		},
		{
			name: "generic error 500",
			h: func(w http.ResponseWriter, r *http.Request) error {
				return errors.New("db down")
			},
			wantCode: http.StatusInternalServerError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			counter := 0
			h := tc.h
			if tc.sideEff > 0 {
				inner := h
				h = func(w http.ResponseWriter, r *http.Request) error {
					counter++
					return inner(w, r)
				}
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/articles/unknown", nil)
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if counter != tc.sideEff {
				t.Errorf("side effects = %d, want %d", counter, tc.sideEff)
			}
			if tc.wantBody != "" {
				if rec.Body.String() != tc.wantBody {
					t.Errorf("body = %q", rec.Body.String())
				}
				return
			}
			var body ErrorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("expected error JSON: %v", err)
			}
			if body.Status != tc.wantCode {
				t.Errorf("body status = %d", body.Status)
			}
			if body.Path != "/articles/unknown" {
				t.Errorf("path = %q", body.Path)
			}
		})
	}
}