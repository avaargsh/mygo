package core

import (
	"errors"
	"io"
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
	Regular   bool
	Symlink   bool
}

// MaxFileReadBytes bounds a single in-memory preview.
const MaxFileReadBytes int64 = 1 << 20

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

// ReadFile reads a listed regular file within a strict caller-supplied limit.
// It does not follow entries identified as symlinks by the directory snapshot.
func (b *FileBrowser) ReadFile(name string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 || maxBytes > MaxFileReadBytes {
		return nil, errors.New("file preview limit must be between 1 byte and 1 MiB")
	}
	if name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "/\\") {
		return nil, errors.New("invalid file name")
	}
	found := false
	for _, e := range b.entries {
		if e.Name == name && e.Regular && !e.Symlink {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("file is not a listed regular file")
	}
	file, err := b.root.Open(path.Join(b.current, name))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("file is no longer a regular file")
	}
	if info.Size() < 0 || info.Size() > maxBytes {
		return nil, errors.New("file exceeds preview size limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("file exceeds preview size limit")
	}
	return data, nil
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
		info, err := e.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		next = append(next, FileEntry{
			Name: e.Name(), Directory: e.IsDir(),
			Regular: mode.IsRegular(), Symlink: mode&fs.ModeSymlink != 0,
		})
	}
	b.current, b.entries = p, next
	return nil
}
