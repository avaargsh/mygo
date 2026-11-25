package xerj

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMultiBindingAndSearch(t *testing.T) {
	var fail atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		index := strings.Split(r.URL.Path, "/")[1]
		if fail.Load() && index == "demo-notes" {
			w.WriteHeader(503)
			return
		}
		fmt.Fprintf(w, `{"hits":{"hits":[{"_index":%q,"_source":{"ax_path":"same.txt"},"fields":{"_passage":{"start_offset":0,"text":%q}}}]}}`, index, index)
	}))
	defer s.Close()
	m := NewManager(s.URL)
	ticket, err := m.BeginSet("a", []string{"demo-notes", "demo-code"})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	h, err := m.Search(context.Background(), "a", "needle", 2)
	if err != nil || len(h) != 2 || h[0].Index != "demo-code" || h[1].Index != "demo-notes" {
		t.Fatalf("hits=%+v err=%v", h, err)
	}
	if _, err = m.BeginSet("b", []string{"b-new", "demo-code"}); err == nil {
		t.Fatal("accepted reserved index")
	}
	if _, err = m.Begin("b", "b-new"); err != nil {
		t.Fatalf("partial reservation: %v", err)
	}
	fail.Store(true)
	h, err = m.Search(context.Background(), "a", "needle", 1)
	if err == nil || h != nil {
		t.Fatalf("partial results: %+v %v", h, err)
	}
}

func TestMultiManifestRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	endpoint := "http://127.0.0.1:9200"
	m, err := CreatePersistentManager(endpoint, path)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := m.BeginSet("a", []string{"demo-code", "demo-notes"})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	m, err = OpenPersistentManager(endpoint, path)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Status("a"); got.State != IndexFailed || got.Generation != 1 {
		t.Fatalf("status %+v", got)
	}
	for _, idx := range []string{"demo-code", "demo-notes"} {
		if _, err = m.Begin("b", idx); err == nil {
			t.Fatalf("lost reservation %s", idx)
		}
	}
	if _, err = m.Search(context.Background(), "a", "needle", 1); !errors.Is(err, ErrIndexNotReady) {
		t.Fatalf("search %v", err)
	}
}
