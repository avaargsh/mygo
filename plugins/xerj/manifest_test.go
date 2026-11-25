package xerj

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentRestartFencesAndReserves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	endpoint := "http://127.0.0.1:9200"
	m, err := CreatePersistentManager(endpoint, path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := m.Begin("a", "a-v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(first); err != nil {
		t.Fatal(err)
	}
	second, err := m.Begin("a", "a-v2")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(second); err != nil {
		t.Fatal(err)
	}
	_, err = m.Begin("b", "b-v1")
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := OpenPersistentManager(endpoint, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"a", "b"} {
		if got := recovered.Status(w); got.State != IndexFailed {
			t.Fatalf("%s: %+v", w, got)
		}
		if _, err := recovered.Search(context.Background(), w, "needle", 1); !errors.Is(err, ErrIndexNotReady) {
			t.Fatalf("search %s: %v", w, err)
		}
	}
	for _, idx := range []string{"a-v1", "a-v2", "b-v1"} {
		if _, err := recovered.Begin("c", idx); err == nil {
			t.Fatalf("reused %s", idx)
		}
	}
	if err := recovered.Publish(second); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("old ticket: %v", err)
	}
	third, err := recovered.Begin("a", "a-v3")
	if err != nil {
		t.Fatal(err)
	}
	if err := recovered.Publish(third); err != nil {
		t.Fatal(err)
	}
	if got := recovered.Status("a"); got.Generation != 3 || got.State != IndexReady {
		t.Fatalf("%+v", got)
	}
	again, err := OpenPersistentManager(endpoint, path)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Status("a"); got.Generation != 3 || got.State != IndexFailed {
		t.Fatalf("%+v", got)
	}
}

func TestManifestRejectsMissingCorruptAndWrongNode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	endpoint := "http://127.0.0.1:9200"
	if _, err := OpenPersistentManager(endpoint, path); err == nil {
		t.Fatal("accepted missing manifest")
	}
	if _, err := CreatePersistentManager(endpoint, path); err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePersistentManager(endpoint, path); err == nil {
		t.Fatal("overwrote manifest")
	}
	if _, err := OpenPersistentManager("http://127.0.0.1:9201", path); err == nil {
		t.Fatal("accepted different node")
	}
	for _, data := range []string{
		`{`,
		`{"version":2,"endpoint":"http://127.0.0.1:9200","reserved":[],"bindings":[]}`,
		`{"version":1,"endpoint":"http://127.0.0.1:9200","reserved":["a-v1"],"bindings":[{"workspace":"a","index":"missing","generation":1,"state":"READY"}]}`,
		`{"version":1,"endpoint":"http://127.0.0.1:9200","reserved":["a-v1"],"bindings":[{"workspace":"a","index":"a-v1","generation":1,"state":"READY"},{"workspace":"b","index":"a-v1","generation":1,"state":"READY"}]}`,
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenPersistentManager(endpoint, path); err == nil {
			t.Fatalf("accepted invalid manifest %s", data)
		}
	}
}

func TestManifestWriteFailureStopsSearch(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "manifest.json")
	m, err := CreatePersistentManager("http://127.0.0.1:9200", path)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := m.Begin("a", "a-v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir, dir+"-moved"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Begin("a", "a-v2"); !errors.Is(err, ErrManifestUnavailable) {
		t.Fatalf("write: %v", err)
	}
	if _, err := m.Search(context.Background(), "a", "needle", 1); !errors.Is(err, ErrManifestUnavailable) {
		t.Fatalf("search: %v", err)
	}
	if _, err := m.Begin("b", "b-v1"); !errors.Is(err, ErrManifestUnavailable) {
		t.Fatalf("retry: %v", err)
	}
}
