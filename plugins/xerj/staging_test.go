package xerj

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStageApprovedText(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"src/a.txt": "approved", "private.txt": "not approved"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	stage, err := StageTextCorpus(context.Background(), root, state, []string{"src/a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !containsPath(state, stage.Root()) || stage.Root() == state {
		t.Fatal("not a dedicated stage")
	}
	if err := os.WriteFile(filepath.Join(root, "src/a.txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(stage.Root(), "src/a.txt"))
	if err != nil || string(data) != "approved" {
		t.Fatalf("staged bytes %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(stage.Root(), "private.txt")); !os.IsNotExist(err) {
		t.Fatal("unapproved file staged")
	}
	digests := stage.Digests()
	if len(digests) != 1 || digests["src/a.txt"] != fmt.Sprintf("%x", sha256.Sum256(data)) {
		t.Fatal(digests)
	}
	delete(digests, "src/a.txt")
	if len(stage.Digests()) != 1 {
		t.Fatal("mutable digest map")
	}
}

func TestStageRejectsAndRemovesPartialCopy(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][]string{nil, {}, {"ok.txt", "missing.txt"}, {"ok.txt", "../bad"}, {"ok.txt", "ok.txt"}} {
		if _, err := StageTextCorpus(context.Background(), root, state, paths); err == nil {
			t.Fatalf("accepted %v", paths)
		}
		entries, err := os.ReadDir(state)
		if err != nil || len(entries) != 0 {
			t.Fatalf("partial stage retained %v %v", entries, err)
		}
	}
	if _, err := StageTextCorpus(context.Background(), root, root, []string{"ok.txt"}); err == nil {
		t.Fatal("overlapping roots accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := StageTextCorpus(ctx, root, state, []string{"ok.txt"}); err != context.Canceled {
		t.Fatal(err)
	}
}
