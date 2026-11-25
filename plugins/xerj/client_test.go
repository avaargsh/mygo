package xerj

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearch(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/workspace-docs/_search" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var q struct {
			Size  int            `json:"size"`
			Query map[string]any `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil || q.Size != 3 || q.Query["match"] == nil {
			t.Errorf("bad query: %+v %v", q, err)
		}
		w.Write([]byte(`{"hits":{"hits":[{"_source":{"ax_path":"src/a.go","start_line":12,"body":"hello"}}]}}`))
	}))
	defer s.Close()
	c, err := New(s.URL, "workspace-docs")
	if err != nil {
		t.Fatal(err)
	}
	hits, err := c.Search(context.Background(), "hello", 3)
	if err != nil || len(hits) != 1 || hits[0].Path != "src/a.go" || hits[0].Line != 12 || hits[0].Text != "hello" {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
}
func TestRejectScope(t *testing.T) {
	for _, idx := range []string{"*", "ax-*", "a,b", "../x", ""} {
		if _, err := New("http://127.0.0.1:9200", idx); err == nil {
			t.Errorf("accepted %q", idx)
		}
	}
	if _, err := New("http://example.org:9200", "docs"); err == nil {
		t.Fatal("accepted remote endpoint")
	}
}
func TestFailures(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{503, `{}`}, {200, `{}`}, {200, `{"hits":{"hits":[{"_source":{"body":"no source"}}]}}`}} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) }))
		c, err := New(s.URL, "docs")
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Search(context.Background(), "hello", 1)
		s.Close()
		if err == nil {
			t.Errorf("accepted %d %s", tc.status, tc.body)
		}
	}
}
