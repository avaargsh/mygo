package xerj

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
)

func TestWorkerQueueCapacityAndFIFO(t *testing.T) {
	q, err := NewBuildQueue(NewManager("http://127.0.0.1:9200"), 2)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	submit := func(w, p string) (*BuildJob, error) {
		return q.submit(w, p, func(context.Context) error { order = append(order, w); return nil })
	}
	a, err := submit("a", "demoa")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = submit("a", "demob"); !errors.Is(err, ErrBuildDuplicate) {
		t.Fatalf("duplicate workspace: %v", err)
	}
	if _, err = submit("b", "demoa"); !errors.Is(err, ErrBuildDuplicate) {
		t.Fatalf("duplicate prefix: %v", err)
	}
	b, err := submit("b", "demob")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = submit("c", "democ"); !errors.Is(err, ErrBuildQueueFull) {
		t.Fatalf("full: %v", err)
	}
	if a.Status() != BuildQueued {
		t.Fatal(a.Status())
	}
	for i := 0; i < 2; i++ {
		if ok, err := q.RunNext(context.Background()); !ok || err != nil {
			t.Fatalf("run %v %v", ok, err)
		}
	}
	if a.Status() != BuildSucceeded || b.Status() != BuildSucceeded || len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("order %v state %v %v", order, a.Status(), b.Status())
	}
	if ok, err := q.RunNext(context.Background()); ok || err != nil {
		t.Fatalf("empty %v %v", ok, err)
	}
}

func TestWorkerCancellationAndBusy(t *testing.T) {
	q, err := NewBuildQueue(NewManager("http://127.0.0.1:9200"), 2)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	a, err := q.submit("a", "demoa", func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	b, err := q.submit("b", "demob", func(context.Context) error { t.Error("canceled queued job executed"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !b.Cancel() || b.Status() != BuildCanceled {
		t.Fatal("queued cancel failed")
	}
	done := make(chan error, 1)
	go func() { _, err := q.RunNext(context.Background()); done <- err }()
	<-entered
	if ok, err := q.RunNext(context.Background()); ok || !errors.Is(err, ErrBuildWorkerBusy) {
		t.Fatalf("busy %v %v", ok, err)
	}
	if !a.Cancel() {
		t.Fatal("cancel running")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if a.Status() != BuildCanceled || a.Cancel() {
		t.Fatal(a.Status())
	}
	if err := a.Wait(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := q.submit("a", "democ", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerFailureAndCanceledCaller(t *testing.T) {
	q, err := NewBuildQueue(NewManager("http://127.0.0.1:9200"), 1)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("fixture build failure")
	job, err := q.submit("a", "demoa", func(context.Context) error { return boom })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ok, err := q.RunNext(ctx); ok || !errors.Is(err, context.Canceled) || job.Status() != BuildQueued {
		t.Fatalf("pre-canceled %v %v %v", ok, err, job.Status())
	}
	if err := job.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if ok, err := q.RunNext(context.Background()); !ok || !errors.Is(err, boom) {
		t.Fatalf("failed %v %v", ok, err)
	}
	if job.Status() != BuildFailed || !errors.Is(job.Wait(context.Background()), boom) {
		t.Fatal("failure not retained")
	}
}

func TestWorkerSubmitBuildPipeline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	root := t.TempDir()
	corpus, state := filepath.Join(root, "corpus"), filepath.Join(root, "state")
	for _, dir := range []string{corpus, state} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	script := []byte("#!/bin/sh\nexit 0\n")
	binary := filepath.Join(root, "fixture")
	if err := os.WriteFile(binary, script, 0700); err != nil {
		t.Fatal(err)
	}
	var catalogs atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/_cat/indices":
			if catalogs.Add(1) == 1 {
				fmt.Fprint(w, `[]`)
			} else {
				fmt.Fprint(w, `[{"index":"demo-code"}]`)
			}
		case "/demo-code/_search":
			fmt.Fprint(w, `{"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_index":"demo-code","_source":{"ax_path":"a.txt","body":"needle"}}]}}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	m := NewManager(s.URL)
	q, err := NewBuildQueue(m, 1)
	if err != nil {
		t.Fatal(err)
	}
	cfg := AutoindexConfig{Binary: binary, SHA256: fmt.Sprintf("%x", sha256.Sum256(script)), Corpus: corpus, StateRoot: state, Prefix: "demo"}
	expected := map[string]TextIndexVerifier{"demo-code": {Expected: map[string]string{"a.txt": fmt.Sprintf("%x", sha256.Sum256([]byte("needle")))}}}
	job, err := q.Submit("a", cfg, expected)
	if err != nil {
		t.Fatal(err)
	}
	expected["demo-code"].Expected["a.txt"] = fmt.Sprintf("%064x", 0)
	delete(expected, "demo-code")
	if m.Status("a").State != IndexUnbound {
		t.Fatal("submission mutated binding")
	}
	if ok, err := q.RunNext(context.Background()); !ok || err != nil {
		t.Fatalf("run %v %v", ok, err)
	}
	if job.Status() != BuildSucceeded || job.Wait(context.Background()) != nil || m.Status("a").State != IndexReady || catalogs.Load() != 3 {
		t.Fatalf("job %v binding %v catalogs %d", job.Status(), m.Status("a"), catalogs.Load())
	}
}
