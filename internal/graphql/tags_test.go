package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type fakeTagLister struct {
	mu    sync.Mutex
	tags  []string
	err   error
	calls int
	ctxs  []context.Context
}

func (f *fakeTagLister) AllTags(ctx context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.ctxs = append(f.ctxs, ctx)
	if f.err != nil {
		return nil, f.err
	}
	return f.tags, nil
}

func TestNewTagDatafetcher(t *testing.T) {
	tests := []struct {
		name    string
		lister  TagLister
		wantErr error
	}{
		{"nil lister", nil, ErrNilTagLister},
		{"valid lister", &fakeTagLister{}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := NewTagDatafetcher(tc.lister)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil && d != nil {
				t.Fatalf("expected nil datafetcher")
			}
			if tc.wantErr == nil && d == nil {
				t.Fatalf("expected non-nil datafetcher")
			}
		})
	}
}

func TestSchemaCoordinates(t *testing.T) {
	if QueryType != "Query" {
		t.Errorf("QueryType = %q", QueryType)
	}
	if QueryFieldTags != "tags" {
		t.Errorf("QueryFieldTags = %q", QueryFieldTags)
	}
}

func TestTagDatafetcher_Tags(t *testing.T) {
	svcErr := errors.New("database unavailable")
	tests := []struct {
		name    string
		tags    []string
		err     error
		want    []string
		wantErr bool
	}{
		{"tags exist preserve order", []string{"java", "spring", "dragons"}, nil, []string{"java", "spring", "dragons"}, false},
		{"no dedup or sorting", []string{"b", "a", "b"}, nil, []string{"b", "a", "b"}, false},
		{"nil becomes empty", nil, nil, []string{}, false},
		{"empty stays empty", []string{}, nil, []string{}, false},
		{"service failure", nil, svcErr, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeTagLister{tags: tc.tags, err: tc.err}
			d, err := NewTagDatafetcher(f)
			if err != nil {
				t.Fatal(err)
			}
			got, err := d.Tags(context.Background())
			if f.calls != 1 {
				t.Fatalf("calls = %d, want 1", f.calls)
			}
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !errors.Is(err, svcErr) {
					t.Errorf("error does not wrap cause: %v", err)
				}
				if !strings.Contains(err.Error(), "Query.tags") {
					t.Errorf("error lacks coordinates: %v", err)
				}
				if got != nil {
					t.Errorf("got = %v, want nil", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got == nil {
				t.Fatal("got nil slice, want non-nil")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// minimal GraphQL HTTP harness for Query.tags only.
func tagsHandler(d *TagDatafetcher) http.Handler {
	h := NewCustomizeExceptionHandler(nil)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		q := strings.Join(strings.Fields(req.Query), "")
		resp := map[string]any{}
		switch {
		case q == "{tags}" || q == "query{tags}":
			tags, err := d.Tags(r.Context())
			if err != nil {
				res := h.HandleError(err, []any{QueryFieldTags})
				resp["data"] = map[string]any{"tags": nil}
				resp["errors"] = res.Errors
			} else {
				resp["data"] = map[string]any{"tags": tags}
			}
		case strings.HasPrefix(strings.TrimPrefix(q, "query"), "{tags("):
			resp["errors"] = []GraphQLError{{Message: "Validation error: Unknown argument on field 'Query.tags'",
				Extensions: map[string]any{"classification": "ValidationError"}}}
		default:
			resp["errors"] = []GraphQLError{{Message: "unsupported"}}
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func TestGraphQLTagsEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		tags       []string
		err        error
		query      string
		token      string
		wantTags   []string
		wantNull   bool
		wantErrors bool
		wantCalls  int
	}{
		{"tags exist", []string{"java", "spring", "dragons"}, nil, "{ tags }", "", []string{"java", "spring", "dragons"}, false, false, 1},
		{"no tags", nil, nil, "{ tags }", "", []string{}, false, false, 1},
		{"anonymous", []string{"a"}, nil, "query { tags }", "", []string{"a"}, false, false, 1},
		{"authenticated", []string{"a"}, nil, "{ tags }", "Token abc", []string{"a"}, false, false, 1},
		{"unknown argument", []string{"a"}, nil, "{ tags(limit: 1) }", "", nil, false, true, 0},
		{"service failure", nil, errors.New("db down"), "{ tags }", "", nil, true, true, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeTagLister{tags: tc.tags, err: tc.err}
			d, _ := NewTagDatafetcher(f)
			srv := httptest.NewServer(tagsHandler(d))
			defer srv.Close()

			body, _ := json.Marshal(map[string]string{"query": tc.query})
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/graphql", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			if tc.token != "" {
				req.Header.Set("Authorization", tc.token)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			var out struct {
				Data   map[string]json.RawMessage `json:"data"`
				Errors []GraphQLError             `json:"errors"`
			}
			if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			calls := f.calls
			f.mu.Unlock()
			if calls != tc.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tc.wantCalls)
			}
			if tc.wantErrors != (len(out.Errors) > 0) {
				t.Fatalf("errors = %v, wantErrors %v", out.Errors, tc.wantErrors)
			}
			if tc.wantNull {
				if string(out.Data["tags"]) != "null" {
					t.Errorf("tags = %s, want null", out.Data["tags"])
				}
				if len(out.Errors[0].Path) != 1 || out.Errors[0].Path[0] != "tags" {
					t.Errorf("path = %v", out.Errors[0].Path)
				}
				if out.Errors[0].Extensions["errorType"] != string(ErrorTypeInternal) {
					t.Errorf("extensions = %v", out.Errors[0].Extensions)
				}
				return
			}
			if tc.wantTags != nil {
				raw := out.Data["tags"]
				if string(raw) == "null" {
					t.Fatal("tags serialized as null")
				}
				var got []string
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, tc.wantTags) {
					t.Errorf("got %v want %v", got, tc.wantTags)
				}
			}
		})
	}
}