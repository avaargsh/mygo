package xerj

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestDiscoverCatalog(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/_cat/indices" || r.URL.Query().Get("format") != "json" || r.URL.Query().Get("h") != "index" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(400)
			return
		}
		fmt.Fprint(w, `[{"index":"demo-notes"},{"index":"other-code"},{"index":"demo-code"},{"index":"demo2-code"},{"index":"autoindex-catalog"}]`)
	}))
	defer s.Close()
	got, err := DiscoverIndices(context.Background(), s.URL, "demo")
	if err != nil || !reflect.DeepEqual(got, []string{"demo-code", "demo-notes"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if err := MatchIndexSet(got, []string{"demo-notes", "demo-code"}); err != nil {
		t.Fatal(err)
	}
	if err := MatchIndexSet(got, []string{"demo-code"}); err == nil {
		t.Fatal("unexpected dataset accepted")
	}
}

func TestCatalogRejectsMalformed(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `[{"index":"demo-code"},{"index":"demo-code"}]`, `[{"index":"demo-*"}]`, `[{"index":""}]`, `[] trailing`} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		_, err := DiscoverIndices(context.Background(), s.URL, "demo")
		s.Close()
		if err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestMatchIndexSetRejectsInvalid(t *testing.T) {
	for _, expected := range [][]string{nil, {"demo-*"}, {"demo-code", "demo-code"}, {"other-code"}} {
		if err := MatchIndexSet([]string{"demo-code"}, expected); err == nil {
			t.Errorf("accepted %v", expected)
		}
	}
}

func TestCatalogEmptyAndInvalidScope(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `[]`) }))
	defer s.Close()
	got, err := DiscoverIndices(context.Background(), s.URL, "demo")
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	if _, err = DiscoverIndices(context.Background(), s.URL, "*"); err == nil {
		t.Fatal("accepted wildcard prefix")
	}
	if _, err = DiscoverIndices(context.Background(), "http://example.org:9200", "demo"); err == nil {
		t.Fatal("accepted remote endpoint")
	}
}
