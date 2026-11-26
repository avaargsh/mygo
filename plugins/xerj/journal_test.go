package xerj

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestJournalRecoveryDoesNotReplay(t *testing.T) {
	dir := t.TempDir()
	m, err := CreatePersistentManager("http://127.0.0.1:9200", filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "jobs.json")
	q, err := CreateJournaledBuildQueue(m, 3, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"one", "two", "three"} {
		if _, err := q.submit(prefix, prefix, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := q.RunNext(context.Background()); !ok || err != nil {
		t.Fatalf("run %v %v", ok, err)
	}
	recovered, err := OpenJournaledBuildQueue(m, 3, path)
	if err != nil {
		t.Fatal(err)
	}
	history := recovered.History()
	if len(history) != 3 || history[0].State != BuildSucceeded || history[1].State != BuildInterrupted || history[2].State != BuildInterrupted {
		t.Fatalf("history %+v", history)
	}
	if ok, err := recovered.RunNext(context.Background()); ok || err != nil {
		t.Fatalf("replayed %v %v", ok, err)
	}
	if _, err := recovered.submit("new", "two", func(context.Context) error { return nil }); !errors.Is(err, ErrBuildDuplicate) {
		t.Fatalf("prefix reused %v", err)
	}
	if _, err := recovered.submit("new", "four", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestJournalWriteFailurePreventsExecution(t *testing.T) {
	dir := t.TempDir()
	m, err := CreatePersistentManager("http://127.0.0.1:9200", filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "state")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	q, err := CreateJournaledBuildQueue(m, 2, filepath.Join(state, "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	j, err := q.submit("a", "demo", func(context.Context) error { t.Error("unexpected execution"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(state, state+"-moved"); err != nil {
		t.Fatal(err)
	}
	if ok, err := q.RunNext(context.Background()); ok || !errors.Is(err, ErrBuildJournalUnavailable) {
		t.Fatalf("run %v %v", ok, err)
	}
	if j.Status() != BuildInterrupted || !errors.Is(j.Wait(context.Background()), ErrBuildJournalUnavailable) {
		t.Fatal(j.Status())
	}
}

func TestJournalRejectsCorruptAndExisting(t *testing.T) {
	dir := t.TempDir()
	m, err := CreatePersistentManager("http://127.0.0.1:9200", filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "jobs.json")
	if _, err := CreateJournaledBuildQueue(m, 1, path); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateJournaledBuildQueue(m, 1, path); err == nil {
		t.Fatal("overwrote journal")
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenJournaledBuildQueue(m, 1, path); err == nil {
		t.Fatal("opened corrupt journal")
	}
}
