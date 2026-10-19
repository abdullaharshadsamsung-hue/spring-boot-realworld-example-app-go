package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// NOTE: errors.go calls WriteError(w, err) and WriteAuthzError(w, err), but
// no_authorization.go declares WriteAuthzError(w, r, err) and calls
// WriteError(w, r, err). The package will not compile until those signatures
// agree. These tests use the signatures declared in errors.go. They avoid
// asserting on the details of the ErrNoAuthorization path.

func TestErrInvalidAuthentication_Message(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"sentinel", ErrInvalidAuthentication},
		{"constant", errors.New(invalidAuthenticationMessage)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := tc.err.Error()
			if msg != "invalid email or password" {
				t.Fatalf("message = %q", msg)
			}
			if msg != strings.ToLower(msg) {
				t.Errorf("message not lowercase: %q", msg)
			}
			if strings.HasSuffix(msg, ".") || strings.HasSuffix(msg, "!") {
				t.Errorf("message has trailing punctuation: %q", msg)
			}
		})
	}
}

func TestInvalidAuthenticationBody_NonMatching(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"not found", ErrResourceNotFound},
		{"other", errors.New("boom")},
		{"same text but different value", errors.New("invalid email or password")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if body, ok := invalidAuthenticationBody(tc.err); ok || body != nil {
				t.Fatalf("expected no match, got %v", body)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
		wantEmpty   bool
		forbidden   []string
	}{
		{
			name:        "invalid auth",
			err:         ErrInvalidAuthentication,
			wantStatus:  http.StatusUnprocessableEntity,
			wantMessage: "invalid email or password",
		},
		{
			name:        "wrapped invalid auth does not leak the email",
			err:         fmt.Errorf("login secret@x.com wrong password: %w", ErrInvalidAuthentication),
			wantStatus:  http.StatusUnprocessableEntity,
			wantMessage: "invalid email or password",
			forbidden:   []string{"secret@x.com", "wrong password"},
		},
		{
			name:       "not found",
			err:        ErrResourceNotFound,
			wantStatus: http.StatusNotFound,
			wantEmpty:  true,
		},
		{
			name:       "wrapped not found",
			err:        fmt.Errorf("article x: %w", ErrResourceNotFound),
			wantStatus: http.StatusNotFound,
			wantEmpty:  true,
		},
		{
			name:        "unknown error is generic 500",
			err:         errors.New("db password=hunter2"),
			wantStatus:  http.StatusInternalServerError,
			wantMessage: http.StatusText(http.StatusInternalServerError),
			forbidden:   []string{"hunter2"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(rec, tc.err)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			raw := rec.Body.String()
			if tc.wantEmpty {
				if raw != "" {
					t.Fatalf("expected empty body, got %q", raw)
				}
				return
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("content-type = %q", ct)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v (%q)", err, raw)
			}
			if body["message"] != tc.wantMessage {
				t.Errorf("message = %q, want %q", body["message"], tc.wantMessage)
			}
			for _, f := range tc.forbidden {
				if strings.Contains(raw, f) {
					t.Errorf("body leaks %q: %s", f, raw)
				}
			}
		})
	}
}

func TestErrorHandlerFunc(t *testing.T) {
	tests := []struct {
		name       string
		h          ErrorHandlerFunc
		wantStatus int
		wantBody   string
	}{
		{
			name: "success passes through",
			h: func(w http.ResponseWriter, r *http.Request) error {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte("ok"))
				return nil
			},
			wantStatus: http.StatusCreated,
			wantBody:   "ok",
		},
		{
			name: "invalid auth",
			h: func(w http.ResponseWriter, r *http.Request) error {
				return fmt.Errorf("login: %w", ErrInvalidAuthentication)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `"message":"invalid email or password"`,
		},
		{
			name: "not found",
			h: func(w http.ResponseWriter, r *http.Request) error {
				return ErrResourceNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "unknown",
			h: func(w http.ResponseWriter, r *http.Request) error {
				return errors.New("internal detail")
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   http.StatusText(http.StatusInternalServerError),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/users/login", nil)
			tc.h.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body %q missing %q", rec.Body.String(), tc.wantBody)
			}
			if strings.Contains(rec.Body.String(), "internal detail") {
				t.Error("internal detail leaked")
			}
		})
	}
}