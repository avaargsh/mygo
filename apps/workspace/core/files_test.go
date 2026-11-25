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

func TestFilesReadBoundedRegularText(t *testing.T) {
	root := fstest.MapFS{
		"docs/readme.md": {Data: []byte("# MyGo")},
		"docs/large.txt": {Data: []byte("123456")},
	}
	b, err := NewFileBrowser(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Enter("docs"); err != nil {
		t.Fatal(err)
	}
	got, err := b.ReadFile("readme.md", 16)
	if err != nil || string(got) != "# MyGo" {
		t.Fatalf("read = %q, err = %v", got, err)
	}
	if _, err := b.ReadFile("large.txt", 5); err == nil {
		t.Fatal("oversized file was read")
	}
	if _, err := b.ReadFile("readme.md", 0); err == nil {
		t.Fatal("zero byte limit was accepted")
	}
}

func TestFilesReadRejectsUnlistedPathsDirectoriesAndSymlinks(t *testing.T) {
	root := fstest.MapFS{
		"docs/readme.md": {Data: []byte("safe")},
		"docs/link": {Mode: fs.ModeSymlink},
	}
	b, err := NewFileBrowser(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Enter("docs"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"..", "/", "docs/readme.md", "link", "missing"} {
		if _, err := b.ReadFile(name, 64); err == nil {
			t.Fatalf("read accepted %q", name)
		}
	}
	if err := b.Up(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ReadFile("docs", 64); err == nil {
		t.Fatal("directory was read as a file")
	}
}

func TestFilesReadRejectsChangedNonRegularEntry(t *testing.T) {
	root := fstest.MapFS{"file.txt": {Data: []byte("before")}}
	b, err := NewFileBrowser(root)
	if err != nil {
		t.Fatal(err)
	}
	root["file.txt"] = &fstest.MapFile{Mode: fs.ModeDir}
	if _, err := b.ReadFile("file.txt", 64); err == nil {
		t.Fatal("entry changed to directory but was still read")
	}
}
