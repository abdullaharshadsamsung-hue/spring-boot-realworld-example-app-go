package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type fakeTagsQuerier struct {
	tags  []string
	err   error
	calls int
}

func (f *fakeTagsQuerier) AllTags(ctx context.Context) ([]string, error) {
	f.calls++
	return f.tags, f.err
}

func newTagsTestMux(t *testing.T, q TagsQuerier) *http.ServeMux {
	t.Helper()
	h, err := NewTagsHandler(q)
	if err != nil {
		t.Fatalf("NewTagsHandler: %v", err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func TestNewTagsHandler_Nil(t *testing.T) {
	h, err := NewTagsHandler(nil)
	if !errors.Is(err, ErrNilTagsQuerier) {
		t.Fatalf("expected ErrNilTagsQuerier, got %v", err)
	}
	if h != nil {
		t.Fatalf("expected nil handler")
	}
}

func TestGetTags_Success(t *testing.T) {
	tests := []struct {
		name    string
		tags    []string
		target  string
		headers map[string]string
		want    []string
	}{
		{"tags exist", []string{"java", "spring", "dragons"}, "/tags", nil, []string{"java", "spring", "dragons"}},
		{"nil tags become empty", nil, "/tags", nil, []string{}},
		{"empty tags", []string{}, "/tags", nil, []string{}},
		{"no auth header", []string{"a", "b"}, "/tags", nil, []string{"a", "b"}},
		{"with auth token", []string{"a", "b"}, "/tags", map[string]string{"Authorization": "Token abc.def.ghi"}, []string{"a", "b"}},
		{"query params ignored", []string{"x", "y", "z"}, "/tags?limit=1&offset=2", nil, []string{"x", "y", "z"}},
		{"no transformation", []string{"b", "a", "a", " spaced "}, "/tags", nil, []string{"b", "a", "a", " spaced "}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeTagsQuerier{tags: tc.tags}
			mux := newTagsTestMux(t, q)
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("content-type = %q", ct)
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatalf("decode: %v body=%s", err, rec.Body.String())
			}
			if len(raw) != 1 {
				t.Fatalf("expected exactly one key, got %v", raw)
			}
			tagsRaw, ok := raw["tags"]
			if !ok {
				t.Fatalf("missing tags key")
			}
			if bytes.Equal(bytes.TrimSpace(tagsRaw), []byte("null")) {
				t.Fatalf("tags is null")
			}
			var got []string
			if err := json.Unmarshal(tagsRaw, &got); err != nil {
				t.Fatalf("tags not string array: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("tags = %#v, want %#v", got, tc.want)
			}
			if q.calls != 1 {
				t.Errorf("calls = %d, want 1", q.calls)
			}
		})
	}
}

func TestTags_UnsupportedMethods(t *testing.T) {
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(m, func(t *testing.T) {
			q := &fakeTagsQuerier{tags: []string{"a"}}
			mux := newTagsTestMux(t, q)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(m, "/tags", strings.NewReader(`{"tags":["x"]}`)))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want 405", rec.Code)
			}
			if q.calls != 0 {
				t.Errorf("querier called %d times", q.calls)
			}
		})
	}
}