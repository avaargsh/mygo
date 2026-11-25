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
		w.Write([]byte(`{"hits":{"hits":[{"_index":"workspace-docs","_source":{"ax_path":"src/a.go","start_line":12},"fields":{"_passage":{"start_offset":0,"text":"hello"}}}]}}`))
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

func TestPassageSearch(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Fields []string `json:"fields"`
			Source []string `json:"_source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		if len(q.Fields) != 1 || q.Fields[0] != "_passage" {
			t.Errorf("missing passage projection: %+v", q)
		}
		for _, f := range q.Source {
			if f == "body" || f == "code" {
				t.Errorf("whole text requested: %s", f)
			}
		}
		w.Write([]byte(`{"hits":{"hits":[{"_index":"docs","_source":{"ax_path":"notes.txt"},"fields":{"_passage":[{"field":"body","start_offset":80,"text":"needle"}]}}]}}`))
	}))
	defer s.Close()
	c, err := New(s.URL, "docs")
	if err != nil {
		t.Fatal(err)
	}
	hits, err := c.Search(context.Background(), "needle", 1)
	if err != nil || len(hits) != 1 || hits[0].Text != "needle" || hits[0].Line != 0 {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
}

func TestRejectIncompleteOrForeignHit(t *testing.T) {
	for _, body := range []string{
		`{"timed_out":true,"hits":{"hits":[]}}`,
		`{"_shards":{"failed":1},"hits":{"hits":[]}}`,
		`{"hits":{"hits":[{"_index":"other","_source":{"ax_path":"notes.txt"},"fields":{"_passage":{"start_offset":0,"text":"needle"}}}]}}`,
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		c, err := New(s.URL, "docs")
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Search(context.Background(), "needle", 1)
		s.Close()
		if err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestEmptySearch(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"timed_out":false,"hits":{"hits":[]}}`))
	}))
	defer s.Close()
	c, err := New(s.URL, "docs")
	if err != nil {
		t.Fatal(err)
	}
	hits, err := c.Search(context.Background(), "needle", 1)
	if err != nil || hits == nil || len(hits) != 0 {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
}
