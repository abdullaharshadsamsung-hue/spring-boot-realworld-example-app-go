package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// constraintViolation simulates a ConstraintViolationException-like error.
type constraintViolation struct {
	paths []string
	msgs  []string
}

func (c constraintViolation) Error() string { return "constraint violation" }

func (c constraintViolation) FieldErrors() FieldErrors {
	fe := FieldErrors{}
	for i, p := range c.paths {
		fe.Add(ParamFromPropertyPath(p), c.msgs[i])
	}
	return fe
}

// nilFieldErrorer returns a nil map to test normalisation.
type nilFieldErrorer struct{}

func (nilFieldErrorer) Error() string            { return "nil fields" }
func (nilFieldErrorer) FieldErrors() FieldErrors { return nil }

type teapotErr struct{}

func (teapotErr) Error() string   { return "teapot" }
func (teapotErr) HTTPStatus() int { return http.StatusTeapot }

type unauthErr struct{}

func (unauthErr) Error() string   { return "unauthenticated" }
func (unauthErr) HTTPStatus() int { return http.StatusUnauthorized }

func decodeMap(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", body, err)
	}
	return m
}

func errorsField(t *testing.T, m map[string]any) map[string][]string {
	t.Helper()
	raw, ok := m["errors"].(map[string]any)
	if !ok {
		t.Fatalf("errors is not an object: %#v", m["errors"])
	}
	out := map[string][]string{}
	for k, v := range raw {
		arr, ok := v.([]any)
		if !ok {
			t.Fatalf("errors[%q] not array", k)
		}
		for _, s := range arr {
			out[k] = append(out[k], s.(string))
		}
	}
	return out
}

func TestWriteError_FieldErrors(t *testing.T) {
	multi := FieldErrors{}
	multi.Add("email", "can't be empty")
	multi.Add("email", "should be an email")
	multi.Add("username", "duplicated username")

	tests := []struct {
		name string
		err  error
		want map[string][]string
	}{
		{"invalid request grouped", NewInvalidRequestError(multi), map[string][]string{
			"email":    {"can't be empty", "should be an email"},
			"username": {"duplicated username"},
		}},
		{"invalid request empty", NewInvalidRequestError(FieldErrors{}), map[string][]string{}},
		{"invalid request nil map", NewInvalidRequestError(nil), map[string][]string{}},
		{"wrapped invalid request", fmt.Errorf("wrap: %w", NewInvalidRequestError(FieldErrors{"title": {"can't be empty"}})),
			map[string][]string{"title": {"can't be empty"}}},
		{"constraint multi segment", constraintViolation{[]string{"createArticle.param.title"}, []string{"can't be empty"}},
			map[string][]string{"title": {"can't be empty"}}},
		{"constraint single segment", constraintViolation{[]string{"email"}, []string{"bad"}},
			map[string][]string{"email": {"bad"}}},
		{"constraint two segments", constraintViolation{[]string{"method.param"}, []string{"bad"}},
			map[string][]string{"": {"bad"}}},
		{"constraint multiple grouped", constraintViolation{
			[]string{"m.p.title", "m.p.title", "m.p.body"}, []string{"a", "b", "c"}},
			map[string][]string{"title": {"a", "b"}, "body": {"c"}}},
		{"nil field errors normalised", nilFieldErrorer{}, map[string][]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(rec, httptest.NewRequest(http.MethodPost, "/x", nil), tt.err)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("content-type = %q", ct)
			}
			got := errorsField(t, decodeMap(t, rec.Body.String()))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("errors = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestStatusFor_Common(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, http.StatusOK},
		{"invalid auth", ErrInvalidAuthentication, http.StatusUnprocessableEntity},
		{"not found", ErrResourceNotFound, http.StatusNotFound},
		{"unknown", errors.New("x"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StatusFor(tt.err); got != tt.want {
				t.Fatalf("got %d want %d", got, tt.want)
			}
		})
	}
}

func TestParamFromPropertyPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"createArticle.newArticleParam.title", "title"},
		{"email", "email"},
		{"method.param", ""},
		{"a.b.c.d", "c.d"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ParamFromPropertyPath(tt.in); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestFieldErrorsAndInvalidRequestError(t *testing.T) {
	fe := FieldErrors{}
	if fe.HasErrors() {
		t.Fatal("empty should have no errors")
	}
	fe.Add("b", "y")
	fe.Add("a", "x1")
	fe.Add("a", "x2")
	if !fe.HasErrors() {
		t.Fatal("expected errors")
	}
	if !reflect.DeepEqual(fe["a"], []string{"x1", "x2"}) {
		t.Fatalf("order not preserved: %v", fe["a"])
	}

	tests := []struct {
		name string
		err  *InvalidRequestError
		want string
	}{
		{"nil receiver", nil, "invalid request"},
		{"empty", NewInvalidRequestError(nil), "invalid request"},
		{"sorted", NewInvalidRequestError(fe), "invalid request: a: x1, x2; b: y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
			if tt.err.Errors() == nil || tt.err.FieldErrors() == nil {
				t.Fatal("errors must be non-nil")
			}
		})
	}
}

func TestErrorHandlerFunc(t *testing.T) {
	tests := []struct {
		name   string
		h      ErrorHandlerFunc
		status int
		body   string
	}{
		{"success", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("ok"))
			return nil
		}, http.StatusCreated, "ok"},
		{"validation", func(w http.ResponseWriter, r *http.Request) error {
			return NewInvalidRequestError(FieldErrors{"body": {"can't be empty"}})
		}, http.StatusUnprocessableEntity, `{"errors":{"body":["can't be empty"]}}`},
		{"auth", func(w http.ResponseWriter, r *http.Request) error {
			return ErrInvalidAuthentication
		}, http.StatusUnprocessableEntity, `{"message":"invalid email or password"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			var h http.Handler = tt.h
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
			if rec.Code != tt.status {
				t.Fatalf("status = %d want %d", rec.Code, tt.status)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.body {
				t.Fatalf("body = %q want %q", got, tt.body)
			}
		})
	}
}