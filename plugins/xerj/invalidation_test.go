package xerj

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestInvalidateReadyAndBuilding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	endpoint := "http://127.0.0.1:9200"
	m, err := CreatePersistentManager(endpoint, path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := m.Begin("a", "first-code")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Publish(first); err != nil {
		t.Fatal(err)
	}
	if err = m.Invalidate("a"); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Search(context.Background(), "a", "needle", 1); !errors.Is(err, ErrIndexNotReady) {
		t.Fatal(err)
	}
	second, err := m.Begin("a", "second-code")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Invalidate("a"); err != nil {
		t.Fatal(err)
	}
	if err = m.Publish(second); !errors.Is(err, ErrStaleGeneration) {
		t.Fatal(err)
	}
	recovered, err := OpenPersistentManager(endpoint, path)
	if err != nil {
		t.Fatal(err)
	}
	if got := recovered.Status("a"); got.State != IndexFailed || got.Generation != 2 {
		t.Fatal(got)
	}
	if _, err = recovered.Begin("b", "second-code"); err == nil {
		t.Fatal("lost reservation")
	}
}

func TestInvalidateQueuedUnbound(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, `[]`) }))
	defer s.Close()
	m := NewManager(s.URL)
	q, _ := NewBuildQueue(m, 1)
	job, err := q.Submit("a", AutoindexConfig{Prefix: "demo"}, map[string]TextIndexVerifier{"demo-code": {Expected: map[string]string{"a.txt": fmt.Sprintf("%064x", 0)}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Invalidate("a"); err != nil {
		t.Fatal(err)
	}
	if ok, err := q.RunNext(context.Background()); !ok || !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("run %v %v", ok, err)
	}
	if calls.Load() != 0 || job.Status() != BuildFailed || m.Status("a").State != IndexUnbound {
		t.Fatal("stale job dispatched")
	}
}

func TestInvalidateDuringCatalogPreflight(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; fmt.Fprint(w, `[]`) }))
	defer s.Close()
	m := NewManager(s.URL)
	done := make(chan error, 1)
	go func() {
		done <- m.buildTextSet(context.Background(), "a", "demo", map[string]TextIndexVerifier{"demo-code": {Expected: map[string]string{"a.txt": fmt.Sprintf("%064x", 0)}}}, func(context.Context, BuildTarget) error { t.Error("stale build executed"); return nil })
	}()
	<-entered
	err := m.Invalidate("a")
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, ErrStaleGeneration) {
		t.Fatal(err)
	}
	if m.Status("a").State != IndexUnbound {
		t.Fatal("stale preflight reserved binding")
	}
}

func TestInvalidateStagedJobReleasesCorpus(t *testing.T) {
	source, staging, state := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("needle"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewManager("http://127.0.0.1:1")
	q, _ := NewBuildQueue(m, 1)
	job, err := q.SubmitApprovedText(context.Background(), "a", AutoindexConfig{Prefix: "demo", StateRoot: state}, source, staging, map[string][]string{"demo-code": {"a.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Invalidate("a"); err != nil {
		t.Fatal(err)
	}
	if ok, err := q.RunNext(context.Background()); !ok || !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("%v %v", ok, err)
	}
	entries, err := os.ReadDir(staging)
	if err != nil || len(entries) != 0 || job.CleanupError() != nil {
		t.Fatalf("stage %v %v %v", entries, err, job.CleanupError())
	}
}

func TestInvalidateDiscardsInFlightSearch(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		fmt.Fprint(w, `{"hits":{"hits":[]}}`)
	}))
	defer s.Close()
	m := NewManager(s.URL)
	ticket, err := m.Begin("a", "demo-code")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := m.Search(context.Background(), "a", "needle", 1); done <- err }()
	<-entered
	err = m.Invalidate("a")
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, ErrStaleGeneration) {
		t.Fatal(err)
	}
}
