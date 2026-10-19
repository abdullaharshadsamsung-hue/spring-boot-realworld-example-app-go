package graphql

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"migrated-app/internal/httpapi"
)

type migStubFallbackHandler struct {
	called  bool
	gotErr  error
	gotPath []any
	result  HandlerResult
}

func (s *migStubFallbackHandler) HandleError(err error, path []any) HandlerResult {
	s.called = true
	s.gotErr = err
	s.gotPath = path
	return s.result
}

func TestMigCustomizeHandlerAuthErrors(t *testing.T) {
	path := []any{"login"}
	tests := []struct {
		name string
		err  error
	}{
		{"direct", httpapi.ErrInvalidAuthentication},
		{"wrapped", fmt.Errorf("ctx: %w", httpapi.ErrInvalidAuthentication)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := &migStubFallbackHandler{}
			res := NewCustomizeExceptionHandler(fb).HandleError(tt.err, path)
			if fb.called {
				t.Fatal("fallback should not be called")
			}
			if len(res.Errors) != 1 {
				t.Fatalf("want 1 error, got %d", len(res.Errors))
			}
			e := res.Errors[0]
			if e.Message != "invalid email or password" {
				t.Errorf("message = %q", e.Message)
			}
			if !reflect.DeepEqual(e.Path, path) {
				t.Errorf("path = %v", e.Path)
			}
			if e.Extensions["errorType"] != "UNAUTHENTICATED" || e.Extensions["classification"] != "UNAUTHENTICATED" {
				t.Errorf("extensions = %v", e.Extensions)
			}
		})
	}
}

func TestMigCustomizeHandlerConstraintViolations(t *testing.T) {
	path := []any{"createArticle"}
	tests := []struct {
		name    string
		vs      []Violation
		wrap    bool
		wantExt map[string][]string
		wantMsg string
	}{
		{
			name: "two fields",
			vs: []Violation{
				{PropertyPath: "createArticle.param.title", Message: "t msg"},
				{PropertyPath: "createArticle.param.body", Message: "b msg"},
			},
			wantExt: map[string][]string{"title": {"t msg"}, "body": {"b msg"}},
			wantMsg: "createArticle.param.title: t msg, createArticle.param.body: b msg",
		},
		{
			name: "same field multiple",
			vs: []Violation{
				{PropertyPath: "m.p.title", Message: "a"},
				{PropertyPath: "m.p.title", Message: "b"},
			},
			wantExt: map[string][]string{"title": {"a", "b"}},
			wantMsg: "m.p.title: a, m.p.title: b",
		},
		{
			name:    "no dots",
			vs:      []Violation{{PropertyPath: "email", Message: "x"}},
			wantExt: map[string][]string{"email": {"x"}},
			wantMsg: "email: x",
		},
		{
			name:    "two segments",
			vs:      []Violation{{PropertyPath: "method.arg", Message: "x"}},
			wantExt: map[string][]string{"": {"x"}},
			wantMsg: "method.arg: x",
		},
		{
			name:    "four segments",
			vs:      []Violation{{PropertyPath: "m.p.a.b", Message: "x"}},
			wantExt: map[string][]string{"a.b": {"x"}},
			wantMsg: "m.p.a.b: x",
		},
		{
			name:    "no violations",
			vs:      nil,
			wantExt: map[string][]string{},
			wantMsg: "",
		},
		{
			name:    "wrapped",
			vs:      []Violation{{PropertyPath: "m.p.title", Message: "x"}},
			wrap:    true,
			wantExt: map[string][]string{"title": {"x"}},
			wantMsg: "m.p.title: x",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error = NewConstraintViolationError(tt.vs...)
			if tt.wrap {
				err = fmt.Errorf("wrap: %w", err)
			}
			fb := &migStubFallbackHandler{}
			res := NewCustomizeExceptionHandler(fb).HandleError(err, path)
			if fb.called {
				t.Fatal("fallback called")
			}
			if len(res.Errors) != 1 {
				t.Fatalf("want 1 error, got %d", len(res.Errors))
			}
			e := res.Errors[0]
			if e.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", e.Message, tt.wantMsg)
			}
			if !reflect.DeepEqual(e.Path, path) {
				t.Errorf("path = %v", e.Path)
			}
			if e.Extensions["errorType"] != "BAD_REQUEST" || e.Extensions["classification"] != "BAD_REQUEST" {
				t.Errorf("type ext = %v", e.Extensions)
			}
			got := map[string][]string{}
			for k, v := range e.Extensions {
				if k == "errorType" || k == "classification" {
					continue
				}
				s, ok := v.([]string)
				if !ok {
					t.Fatalf("ext %q type %T", k, v)
				}
				got[k] = s
			}
			if !reflect.DeepEqual(got, tt.wantExt) {
				t.Errorf("ext = %v, want %v", got, tt.wantExt)
			}
		})
	}
}

func TestMigCustomizeHandlerFallbackInvoked(t *testing.T) {
	path := []any{"articles", 0}
	other := errors.New("boom")
	want := HandlerResult{Errors: []GraphQLError{{Message: "custom"}}}
	fb := &migStubFallbackHandler{result: want}
	res := NewCustomizeExceptionHandler(fb).HandleError(other, path)
	if !fb.called || fb.gotErr != other || !reflect.DeepEqual(fb.gotPath, path) {
		t.Fatalf("fallback not invoked correctly: %+v", fb)
	}
	if !reflect.DeepEqual(res, want) {
		t.Errorf("res = %+v", res)
	}
}

func TestMigCustomizeHandlerNilFallbackDefault(t *testing.T) {
	path := []any{"x"}
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"error", errors.New("boom"), "boom"},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := NewCustomizeExceptionHandler(nil).HandleError(tt.err, path)
			def := DefaultExceptionHandler{}.HandleError(tt.err, path)
			if !reflect.DeepEqual(res, def) {
				t.Errorf("res %+v != default %+v", res, def)
			}
			if len(res.Errors) != 1 {
				t.Fatalf("len = %d", len(res.Errors))
			}
			e := res.Errors[0]
			if e.Message != tt.wantMsg || e.Extensions["errorType"] != "INTERNAL" || e.Extensions["classification"] != "INTERNAL" {
				t.Errorf("err = %+v", e)
			}
			if !reflect.DeepEqual(e.Path, path) {
				t.Errorf("path = %v", e.Path)
			}
		})
	}
}

func TestMigErrorsAsDataPayload(t *testing.T) {
	tests := []struct {
		name string
		cve  *ConstraintViolationError
		want []ErrorItem
	}{
		{
			name: "title and body",
			cve: NewConstraintViolationError(
				Violation{PropertyPath: "m.a.title", Message: "t1"},
				Violation{PropertyPath: "m.a.body", Message: "b1"},
				Violation{PropertyPath: "m.a.body", Message: "b2"},
			),
			want: []ErrorItem{{Key: "body", Value: []string{"b1", "b2"}}, {Key: "title", Value: []string{"t1"}}},
		},
		{name: "no violations", cve: NewConstraintViolationError(), want: []ErrorItem{}},
		{name: "nil", cve: nil, want: []ErrorItem{}},
		{
			name: "no dots",
			cve:  NewConstraintViolationError(Violation{PropertyPath: "email", Message: "x"}),
			want: []ErrorItem{{Key: "email", Value: []string{"x"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ErrorsAsData(tt.cve)
			if got.Message != "BAD_REQUEST" {
				t.Errorf("message = %q", got.Message)
			}
			if got.Errors == nil {
				t.Fatal("errors nil")
			}
			if !reflect.DeepEqual(got.Errors, tt.want) {
				t.Errorf("errors = %+v, want %+v", got.Errors, tt.want)
			}
			total := 0
			for _, it := range got.Errors {
				total += len(it.Value)
			}
			n := 0
			if tt.cve != nil {
				n = len(tt.cve.Violations)
			}
			if total != n {
				t.Errorf("total %d != %d", total, n)
			}
		})
	}
}

func TestMigErrorsAsDataJSONEmptyList(t *testing.T) {
	b, err := json.Marshal(ErrorsAsData(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"message":"BAD_REQUEST","errors":[]}` {
		t.Errorf("json = %s", b)
	}
}

func TestMigFieldParamSegments(t *testing.T) {
	tests := []struct{ in, want string }{
		{"email", "email"},
		{"method.arg", ""},
		{"m.p.title", "title"},
		{"m.p.a.b", "a.b"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := fieldParam(tt.in); got != tt.want {
				t.Errorf("fieldParam(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMigConstraintViolationErrorFieldErrorer(t *testing.T) {
	var fe httpapi.FieldErrorer = NewConstraintViolationError(Violation{PropertyPath: "m.p.x", Message: "bad"})
	if !reflect.DeepEqual(fe.FieldErrors(), map[string][]string{"x": {"bad"}}) {
		t.Errorf("got %v", fe.FieldErrors())
	}
}