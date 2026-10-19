package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

type customFieldErr struct{ f map[string][]string }

func (c customFieldErr) Error() string                    { return "custom" }
func (c customFieldErr) FieldErrors() map[string][]string { return c.f }

func TestNewInvalidRequestError(t *testing.T) {
	tests := []struct {
		name string
		in   FieldErrors
		want FieldErrors
	}{
		{"with errors", FieldErrors{"email": {"duplicated email"}}, FieldErrors{"email": {"duplicated email"}}},
		{"empty", FieldErrors{}, FieldErrors{}},
		{"nil", nil, FieldErrors{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewInvalidRequestError(tt.in)
			if e.Error() != "" {
				t.Errorf("Error() = %q, want empty", e.Error())
			}
			if e.Errors() == nil {
				t.Fatal("Errors() returned nil")
			}
			if !reflect.DeepEqual(e.Errors(), tt.want) {
				t.Errorf("Errors() = %v, want %v", e.Errors(), tt.want)
			}
			if errors.Unwrap(e) != nil {
				t.Error("expected no cause")
			}
			if tt.in != nil {
				// same reference: mutation visible
				tt.in.Add("x", "y")
				if len(e.Errors()["x"]) != 1 {
					t.Error("Errors() not same reference as input")
				}
			}
			a, b := e.Errors(), e.Errors()
			if reflect.ValueOf(a).Pointer() != reflect.ValueOf(b).Pointer() {
				t.Error("Errors() returns different references")
			}
			if reflect.ValueOf(e.FieldErrors()).Pointer() != reflect.ValueOf(a).Pointer() {
				t.Error("FieldErrors() differs from Errors()")
			}
		})
	}
}

func TestFieldErrors(t *testing.T) {
	f := FieldErrors{}
	if f.HasErrors() {
		t.Error("empty should have no errors")
	}
	f.Add("name", "a")
	f.Add("name", "b")
	if !f.HasErrors() {
		t.Error("expected errors")
	}
	if !reflect.DeepEqual(f["name"], []string{"a", "b"}) {
		t.Errorf("got %v", f["name"])
	}
}

func TestWriteError(t *testing.T) {
	fixed := time.Date(2024, 1, 2, 3, 4, 5, 6000000, time.UTC)
	old := now
	now = func() time.Time { return fixed }
	defer func() { now = old }()

	tests := []struct {
		name   string
		err    error
		status int
		check  func(t *testing.T, body []byte)
	}{
		{"invalid request", NewInvalidRequestError(FieldErrors{"username": {"duplicated username"}}), 422,
			func(t *testing.T, b []byte) {
				var v struct{ Errors map[string][]string }
				json.Unmarshal(b, &v)
				if !reflect.DeepEqual(v.Errors, map[string][]string{"username": {"duplicated username"}}) {
					t.Errorf("body %s", b)
				}
			}},
		{"invalid request nil", NewInvalidRequestError(nil), 422,
			func(t *testing.T, b []byte) {
				var v map[string]map[string][]string
				json.Unmarshal(b, &v)
				if v["errors"] == nil {
					t.Errorf("body %s", b)
				}
			}},
		{"wrapped invalid request", fmt.Errorf("w: %w", NewInvalidRequestError(FieldErrors{"a": {"b"}})), 422, nil},
		{"custom fielderrorer nil", customFieldErr{}, 422,
			func(t *testing.T, b []byte) {
				var v map[string]map[string][]string
				json.Unmarshal(b, &v)
				if v["errors"] == nil {
					t.Errorf("body %s", b)
				}
			}},
		{"invalid auth", fmt.Errorf("login: %w", ErrInvalidAuthentication), 422,
			func(t *testing.T, b []byte) {
				var v messageBody
				json.Unmarshal(b, &v)
				if v.Message != "invalid email or password" {
					t.Errorf("body %s", b)
				}
			}},
		{"not found", ErrResourceNotFound, 404,
			func(t *testing.T, b []byte) {
				var v ErrorBody
				json.Unmarshal(b, &v)
				if v.Status != 404 || v.Error != "Not Found" || v.Path != "/p" || v.Message != "resource not found" ||
					v.Timestamp != fixed.Format(timestampLayout) {
					t.Errorf("body %+v", v)
				}
			}},
		{"internal", errors.New("secret db"), 500,
			func(t *testing.T, b []byte) {
				var v ErrorBody
				json.Unmarshal(b, &v)
				if v.Message != "Internal Server Error" {
					t.Errorf("leaked: %+v", v)
				}
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			WriteError(w, httptest.NewRequest(http.MethodGet, "/p", nil), tt.err)
			if w.Code != tt.status {
				t.Errorf("status %d want %d", w.Code, tt.status)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("content-type %q", ct)
			}
			if tt.check != nil {
				tt.check(t, w.Body.Bytes())
			}
		})
	}
}

func TestWriteErrorNilAndNilRequest(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, nil, nil)
	if w.Body.Len() != 0 {
		t.Error("nil err should write nothing")
	}
	w = httptest.NewRecorder()
	WriteError(w, nil, ErrResourceNotFound)
	var v ErrorBody
	json.Unmarshal(w.Body.Bytes(), &v)
	if w.Code != 404 || v.Path != "" {
		t.Errorf("got %d %+v", w.Code, v)
	}
}

func TestErrorHandlerFunc(t *testing.T) {
	tests := []struct {
		name   string
		h      ErrorHandlerFunc
		status int
	}{
		{"ok", func(w http.ResponseWriter, r *http.Request) error { w.WriteHeader(204); return nil }, 204},
		{"validation", func(w http.ResponseWriter, r *http.Request) error {
			return NewInvalidRequestError(FieldErrors{"title": {"can't be empty"}})
		}, 422},
		{"not found", func(w http.ResponseWriter, r *http.Request) error { return ErrResourceNotFound }, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tt.h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/x", nil))
			if w.Code != tt.status {
				t.Errorf("status %d want %d", w.Code, tt.status)
			}
		})
	}
}