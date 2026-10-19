package app

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestUpdateArticleParam_ZeroValue(t *testing.T) {
	var p UpdateArticleParam
	if p.Title() != "" || p.Body() != "" || p.Description() != "" {
		t.Fatalf("zero value not empty: %+v", p)
	}
}

func TestNewUpdateArticleParam(t *testing.T) {
	tests := []struct {
		name           string
		title, body, d string
	}{
		{"all set", "t", "b", "d"},
		{"all empty", "", "", ""},
		{"no trimming", "  t ", "\nb", " d\t"},
		{"partial", "t", "", "d"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewUpdateArticleParam(tc.title, tc.body, tc.d)
			if p.Title() != tc.title {
				t.Errorf("Title=%q want %q", p.Title(), tc.title)
			}
			if p.Body() != tc.body {
				t.Errorf("Body=%q want %q", p.Body(), tc.body)
			}
			if p.Description() != tc.d {
				t.Errorf("Description=%q want %q", p.Description(), tc.d)
			}
			if p.Title() != tc.title || p.Body() != tc.body || p.Description() != tc.d {
				t.Errorf("getters mutated state")
			}
		})
	}
}

func TestUpdateArticleParamCommand_DecodeJSON(t *testing.T) {
	tests := []struct {
		name          string
		in            string
		wantT, wantB  string
		wantD         string
		wantErr       bool
		wantErrIsRoot bool
	}{
		{name: "all fields", in: `{"article":{"title":"t","body":"b","description":"d"}}`, wantT: "t", wantB: "b", wantD: "d"},
		{name: "only title", in: `{"article":{"title":"t"}}`, wantT: "t"},
		{name: "empty article", in: `{"article":{}}`},
		{name: "null field becomes empty", in: `{"article":{"title":null,"body":"b"}}`, wantB: "b"},
		{name: "unknown fields ignored", in: `{"article":{"title":"t","x":1},"other":2}`, wantT: "t"},
		{name: "missing root", in: `{"title":"t"}`, wantErr: true, wantErrIsRoot: true},
		{name: "null root", in: `{"article": null }`, wantErr: true, wantErrIsRoot: true},
		{name: "invalid json", in: `{`, wantErr: true},
		{name: "non-object envelope", in: `[]`, wantErr: true},
		{name: "article not object", in: `{"article":"x"}`, wantErr: true},
		{name: "wrong field type", in: `{"article":{"title":5}}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var p UpdateArticleParam
			err := json.Unmarshal([]byte(tc.in), &p)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tc.wantErrIsRoot && !errors.Is(err, ErrMissingArticleRoot) {
					t.Fatalf("err=%v want ErrMissingArticleRoot", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.Title() != tc.wantT || p.Body() != tc.wantB || p.Description() != tc.wantD {
				t.Fatalf("got (%q,%q,%q) want (%q,%q,%q)", p.Title(), p.Body(), p.Description(), tc.wantT, tc.wantB, tc.wantD)
			}
		})
	}
}