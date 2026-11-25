package core

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestFilesBrowseAndParent(t *testing.T) {
	root := fstest.MapFS{"docs/a.md": {Data: []byte("a")}, "readme.txt": {Data: []byte("hello")}}
	b, err := NewFileBrowser(root)
	if err != nil {
		t.Fatal(err)
	}
	if b.Path() != "." || len(b.Entries()) != 2 || b.Entries()[0].Name != "docs" {
		t.Fatal(b.Entries())
	}
	if err := b.Enter("docs"); err != nil {
		t.Fatal(err)
	}
	if b.Path() != "docs" || len(b.Entries()) != 1 || b.Entries()[0].Name != "a.md" {
		t.Fatal(b.Entries())
	}
	if err := b.Up(); err != nil || b.Path() != "." {
		t.Fatal(err)
	}
	if err := b.Up(); err != nil || b.Path() != "." {
		t.Fatal(err)
	}
}

func TestFilesRejectInvalidNavigation(t *testing.T) {
	root := fstest.MapFS{"docs/a": {}, "link": {Mode: fs.ModeSymlink}, "file": {}}
	b, err := NewFileBrowser(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"..", "/", "docs/a", "docs\\a", "file", "link", "missing", "."} {
		if err := b.Enter(name); err == nil || b.Path() != "." {
			t.Fatalf("accepted %q", name)
		}
	}
	copy := b.Entries()
	copy[0].Name = "modified"
	if b.Entries()[0].Name == "modified" {
		t.Fatal("entry alias")
	}
}

func TestFilesRefreshFailurePreservesSnapshot(t *testing.T) {
	root := fstest.MapFS{"docs/a": {}}
	b, err := NewFileBrowser(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Enter("docs"); err != nil {
		t.Fatal(err)
	}
	delete(root, "docs/a")
	if err := b.Refresh(); err == nil || b.Path() != "docs" || len(b.Entries()) != 1 {
		t.Fatal("failed refresh lost snapshot")
	}
}
