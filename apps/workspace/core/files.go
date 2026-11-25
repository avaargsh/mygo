package core

import (
	"errors"
	"io/fs"
	"path"
	"strings"
)

// FileBrowser lists directories in a caller-supplied rooted filesystem.
// The caller must provide actual OS containment, not merely os.DirFS.
type FileBrowser struct {
	root    fs.FS
	current string
	entries []FileEntry
}

type FileEntry struct {
	Name      string
	Directory bool
}

func NewFileBrowser(root fs.FS) (*FileBrowser, error) {
	if root == nil {
		return nil, errors.New("directory not authorized")
	}
	b := &FileBrowser{root: root, current: "."}
	if err := b.Refresh(); err != nil {
		return nil, err
	}
	return b, nil
}
func (b *FileBrowser) Path() string         { return b.current }
func (b *FileBrowser) Entries() []FileEntry { return append([]FileEntry(nil), b.entries...) }
func (b *FileBrowser) Refresh() error       { return b.load(b.current) }
func (b *FileBrowser) Up() error {
	if b.current == "." {
		return nil
	}
	return b.load(path.Dir(b.current))
}
func (b *FileBrowser) Enter(name string) error {
	if name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "/\\") {
		return errors.New("invalid directory name")
	}
	for _, e := range b.entries {
		if e.Name == name && e.Directory {
			return b.load(path.Join(b.current, name))
		}
	}
	return errors.New("not a listed directory")
}
func (b *FileBrowser) load(p string) error {
	entries, err := fs.ReadDir(b.root, p)
	if err != nil {
		return err
	}
	if len(entries) > 5000 {
		return errors.New("directory exceeds 5000-entry listing limit")
	}
	next := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		next = append(next, FileEntry{Name: e.Name(), Directory: e.IsDir()})
	}
	b.current, b.entries = p, next
	return nil
}
