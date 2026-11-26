package xerj

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTextSnapshotChanges(t *testing.T) {
	root := t.TempDir()
	put := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("a.txt", "one")
	put("b.txt", "two")
	put("ignored.txt", "not approved")
	ctx := context.Background()
	first, err := CaptureTextSnapshot(ctx, root, []string{"b.txt", "a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got := first.Digests(); len(got) != 2 || got["a.txt"] != fmt.Sprintf("%x", sha256.Sum256([]byte("one"))) {
		t.Fatalf("digests: %v", got)
	}
	first.Digests()["a.txt"] = "changed outside"
	same, err := CaptureTextSnapshot(ctx, root, []string{"a.txt", "b.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if d := first.Diff(same); d.Changed() {
		t.Fatalf("unchanged: %+v", d)
	}
	put("a.txt", "ONE")
	put("c.txt", "three")
	if err := os.Remove(filepath.Join(root, "b.txt")); err != nil {
		t.Fatal(err)
	}
	next, err := CaptureTextSnapshot(ctx, root, []string{"a.txt", "c.txt"})
	if err != nil {
		t.Fatal(err)
	}
	d := first.Diff(next)
	if !d.Changed() || !reflect.DeepEqual(d.Added, []string{"c.txt"}) || !reflect.DeepEqual(d.Removed, []string{"b.txt"}) || !reflect.DeepEqual(d.Modified, []string{"a.txt"}) {
		t.Fatalf("diff: %+v", d)
	}
}

func TestTextSnapshotRejectsUnsafeOrIncompleteInputs(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "binary"), []byte{0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "large"), make([]byte, maxSnapshotFileBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][]string{nil, {"ok.txt", "ok.txt"}, {"../secret.txt"}, {"missing.txt"}, {"binary"}, {"large"}, {"."}, {"/ok.txt"}, {"dir\\file"}} {
		if _, err := CaptureTextSnapshot(context.Background(), root, paths); err == nil {
			t.Errorf("accepted %q", paths)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err == nil {
		if _, err := CaptureTextSnapshot(context.Background(), root, []string{"linked/secret.txt"}); err == nil {
			t.Fatal("accepted symlink directory")
		}
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "link.txt")); err == nil {
		if _, err := CaptureTextSnapshot(context.Background(), root, []string{"link.txt"}); err == nil {
			t.Fatal("accepted symlink file")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CaptureTextSnapshot(ctx, root, []string{"ok.txt"}); err != context.Canceled {
		t.Fatalf("cancel: %v", err)
	}
}
