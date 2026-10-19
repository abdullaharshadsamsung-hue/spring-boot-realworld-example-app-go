package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeChecker struct {
	existing map[string]bool
	err      error
	calls    []string
}

func (f *fakeChecker) SlugExists(_ context.Context, slug string) (bool, error) {
	f.calls = append(f.calls, slug)
	if f.err != nil {
		return false, f.err
	}
	return f.existing[slug], nil
}

func TestDecodeNewArticleParam(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    NewArticleParam
		wantErr error
		anyErr  bool
	}{
		{
			name: "full",
			body: `{"article":{"title":"t","description":"d","body":"b","tagList":["x","y"]}}`,
			want: NewArticleParam{Title: "t", Description: "d", Body: "b", TagList: []string{"x", "y"}},
		},
		{
			name: "omits tagList",
			body: `{"article":{"title":"t","description":"d","body":"b"}}`,
			want: NewArticleParam{Title: "t", Description: "d", Body: "b"},
		},
		{
			name: "omits required fields",
			body: `{"article":{"tagList":[]}}`,
			want: NewArticleParam{TagList: []string{}},
		},
		{
			name: "unknown fields ignored",
			body: `{"article":{"title":"t","extra":1},"other":2}`,
			want: NewArticleParam{Title: "t"},
		},
		{name: "missing root", body: `{"title":"t"}`, wantErr: ErrMissingArticleRoot},
		{name: "invalid json", body: `{`, anyErr: true},
		{name: "wrong inner type", body: `{"article":{"title":5}}`, anyErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeNewArticleParam(strings.NewReader(tc.body))
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if tc.anyErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			if tc.name == "omits tagList" && got.TagList != nil {
				t.Fatal("tagList should be nil")
			}
		})
	}
}

func TestConstructorsAndBuilder(t *testing.T) {
	if got := NewEmptyNewArticleParam(); !reflect.DeepEqual(got, NewArticleParam{}) || got.TagList != nil {
		t.Fatalf("empty = %+v", got)
	}
	tags := []string{"a"}
	want := NewArticleParam{Title: "t", Description: "d", Body: "b", TagList: []string{"a"}}
	if got := NewNewArticleParam("t", "d", "b", tags); !reflect.DeepEqual(got, want) {
		t.Fatalf("all-args = %+v", got)
	}
	// construction never validates
	if got := NewNewArticleParam("", "", "", nil); !reflect.DeepEqual(got, NewArticleParam{}) {
		t.Fatalf("blank all-args = %+v", got)
	}
	src := []string{"a"}
	b := NewArticleParamBuilderFor().Title("t").Description("d").Body("b").TagList(src)
	got := b.Build()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("builder = %+v", got)
	}
	src[0] = "changed"
	if got.TagList[0] != "a" {
		t.Fatal("Build should copy tag slice")
	}
	if NewArticleParamBuilderFor().Build().TagList != nil {
		t.Fatal("nil tagList should stay nil")
	}
}

func TestValidateNewArticleParam(t *testing.T) {
	valid := NewNewArticleParam("Hello World", "d", "b", nil)
	tests := []struct {
		name     string
		p        NewArticleParam
		existing map[string]bool
		want     map[string][]string
	}{
		{name: "valid nil tags", p: valid},
		{name: "valid empty tags", p: NewNewArticleParam("x", "d", "b", []string{})},
		{name: "empty title", p: NewNewArticleParam("", "d", "b", nil), want: map[string][]string{"title": {MsgCantBeEmpty}}},
		{name: "whitespace description", p: NewNewArticleParam("t", " \t\n", "b", nil), want: map[string][]string{"description": {MsgCantBeEmpty}}},
		{name: "empty body", p: NewNewArticleParam("t", "d", "", nil), want: map[string][]string{"body": {MsgCantBeEmpty}}},
		{name: "all blank", p: NewArticleParam{}, want: map[string][]string{
			"title": {MsgCantBeEmpty}, "description": {MsgCantBeEmpty}, "body": {MsgCantBeEmpty}}},
		{name: "duplicate", p: valid, existing: map[string]bool{"hello-world": true},
			want: map[string][]string{"title": {MsgArticleNameExists}}},
		{name: "blank and duplicate", p: NewNewArticleParam(" ", "d", "b", nil), existing: map[string]bool{"-": true},
			want: map[string][]string{"title": {MsgCantBeEmpty, MsgArticleNameExists}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &fakeChecker{existing: tc.existing}
			err := ValidateNewArticleParam(context.Background(), tc.p, c)
			if len(c.calls) != 1 || c.calls[0] != ArticleTitleToSlug(tc.p.Title) {
				t.Fatalf("checker calls = %v", c.calls)
			}
			if tc.want == nil {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				return
			}
			var verr *NewArticleValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("err = %v, want validation error", err)
			}
			if !reflect.DeepEqual(verr.Fields, tc.want) {
				t.Fatalf("fields = %v, want %v", verr.Fields, tc.want)
			}
			if _, ok := verr.Fields["tagList"]; ok {
				t.Fatal("tagList must not be validated")
			}
		})
	}
}

func TestValidateNewArticleParamErrors(t *testing.T) {
	p := NewNewArticleParam("t", "d", "b", nil)
	if err := ValidateNewArticleParam(context.Background(), p, nil); !errors.Is(err, ErrNilArticleSlugChecker) {
		t.Fatalf("err = %v", err)
	}
	boom := errors.New("db down")
	err := ValidateNewArticleParam(context.Background(), p, ArticleSlugCheckerFunc(func(context.Context, string) (bool, error) {
		return false, boom
	}))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	var verr *NewArticleValidationError
	if errors.As(err, &verr) {
		t.Fatal("checker error must not be validation error")
	}
}

func TestValidationErrorMessage(t *testing.T) {
	e := &NewArticleValidationError{Fields: map[string][]string{
		"title": {"a", "b"}, "body": {"c"},
	}}
	want := "validation failed: body: c; title: a, b"
	if got := e.Error(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestArticleTitleToSlug(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Hello World", "hello-world"},
		{"How to train, your dragon?", "how-to-train-your-dragon-"},
		{"A & B | C", "a-b-c"},
		{"it’s “ok”.", "it-s-“ok”-"},
		{"plain", "plain"},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := ArticleTitleToSlug(tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}