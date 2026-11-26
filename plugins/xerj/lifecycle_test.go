package xerj

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkspaceLifecycle(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		index := strings.Split(r.URL.Path, "/")[1]
		fmt.Fprintf(w, `{"hits":{"hits":[{"_index":%q,"_source":{"ax_path":"notes.txt"},"fields":{"_passage":{"start_offset":0,"text":"needle"}}}]}}`, index)
	}))
	defer s.Close()
	m := NewManager(s.URL)
	if _, err := m.Search(context.Background(), "a", "needle", 1); !errors.Is(err, ErrIndexNotReady) {
		t.Fatalf("unknown: %v", err)
	}
	first, err := m.Begin("a", "a-v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Search(context.Background(), "a", "needle", 1); !errors.Is(err, ErrIndexNotReady) {
		t.Fatalf("building: %v", err)
	}
	if err := m.Publish(first); err != nil {
		t.Fatal(err)
	}
	h, err := m.Search(context.Background(), "a", "needle", 1)
	if err != nil || len(h) != 1 {
		t.Fatalf("ready: %+v %v", h, err)
	}
	if _, err := m.Begin("b", "a-v1"); err == nil {
		t.Fatal("reused index across workspaces")
	}
	next, err := m.Begin("a", "a-v2")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(first); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("old publish: %v", err)
	}
	if _, err := m.Search(context.Background(), "a", "needle", 1); !errors.Is(err, ErrIndexNotReady) {
		t.Fatalf("refresh: %v", err)
	}
	if err := m.Fail(next); err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(next); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("failed publish: %v", err)
	}
	if _, err := m.Begin("a", "a-v1"); err == nil {
		t.Fatal("reused retired index")
	}
	third, err := m.Begin("a", "a-v3")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(third); err != nil {
		t.Fatal(err)
	}
	if got := m.Status("a"); got.State != IndexReady || got.Generation != 3 {
		t.Fatalf("status %+v", got)
	}
}

func TestSearchDiscardsSupersededResult(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		fmt.Fprint(w, `{"hits":{"hits":[]}}`)
	}))
	defer s.Close()
	m := NewManager(s.URL)
	ticket, err := m.Begin("a", "a-v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := m.Search(context.Background(), "a", "needle", 1); result <- err }()
	<-entered
	_, err = m.Begin("a", "a-v2")
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("returned superseded result: %v", err)
	}
}

func TestRejectedBeginPreservesReadyBinding(t *testing.T) {
	m := NewManager("http://127.0.0.1:9200")
	ticket, err := m.Begin("a", "a-v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Begin("a", "*"); err == nil {
		t.Fatal("accepted wildcard")
	}
	if got := m.Status("a"); got.State != IndexReady || got.Generation != 1 {
		t.Fatalf("changed valid binding: %+v", got)
	}
	other := NewManager("http://127.0.0.1:9200")
	if _, err := other.Begin("a", "a-v1"); err != nil {
		t.Fatal(err)
	}
	if err := other.Publish(ticket); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("accepted foreign ticket: %v", err)
	}
}
