package xerj

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxSnapshotFileBytes  = 128 << 10
	maxSnapshotTotalBytes = 256 << 10
)

// TextSnapshot contains content digests for an explicitly approved small text
// corpus. It is not a filesystem snapshot or an authorization boundary.
type TextSnapshot struct{ digests map[string]string }

// CaptureTextSnapshot reads only the approved slash-relative paths. The trusted
// caller must control the root and its contents throughout capture; portable
// component checks are not protection against concurrent hostile path swaps.
// Missing approved files are errors. Use an explicitly refreshed allowlist for
// deletions. An explicit empty list is valid for change detection, not indexing.
func CaptureTextSnapshot(ctx context.Context, root string, paths []string) (TextSnapshot, error) {
	if ctx == nil || paths == nil || len(paths) > 32 {
		return TextSnapshot{}, errors.New("invalid snapshot scope")
	}
	if err := ctx.Err(); err != nil {
		return TextSnapshot{}, err
	}
	root, err := resolveDirectory(root)
	if err != nil {
		return TextSnapshot{}, err
	}
	digests := make(map[string]string, len(paths))
	total := 0
	for _, name := range paths {
		if err := ctx.Err(); err != nil {
			return TextSnapshot{}, err
		}
		if name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "\\:\x00") || len(name) > 4096 || !filepath.IsLocal(filepath.FromSlash(name)) {
			return TextSnapshot{}, errors.New("invalid approved source path")
		}
		if _, ok := digests[name]; ok {
			return TextSnapshot{}, errors.New("duplicate source path")
		}
		data, err := readSnapshotFile(root, name)
		if err != nil {
			return TextSnapshot{}, err
		}
		total += len(data)
		if total > maxSnapshotTotalBytes {
			return TextSnapshot{}, errors.New("snapshot exceeds total byte budget")
		}
		if !utf8.Valid(data) {
			return TextSnapshot{}, errors.New("source is not UTF-8")
		}
		digest := sha256.Sum256(data)
		digests[name] = hex.EncodeToString(digest[:])
	}
	if err := ctx.Err(); err != nil {
		return TextSnapshot{}, err
	}
	return TextSnapshot{digests: digests}, nil
}

func readSnapshotFile(root, name string) ([]byte, error) {
	path := root
	parts := strings.Split(name, "/")
	var info os.FileInfo
	for i, p := range parts {
		path = filepath.Join(path, p)
		st, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("symlink source not supported")
		}
		if i < len(parts)-1 && !st.IsDir() {
			return nil, errors.New("invalid source directory")
		}
		info = st
	}
	if !info.Mode().IsRegular() || info.Size() > maxSnapshotFileBytes {
		return nil, errors.New("source is not regular or exceeds file budget")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || !os.SameFile(info, before) || before.Size() != info.Size() || !before.ModTime().Equal(info.ModTime()) {
		return nil, errors.New("source changed before read")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxSnapshotFileBytes+1))
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if len(data) > maxSnapshotFileBytes || int64(len(data)) != after.Size() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, errors.New("source changed or exceeds file budget")
	}
	return data, nil
}

// Digests returns a copy suitable for a trusted TextIndexVerifier expectation.
func (s TextSnapshot) Digests() map[string]string {
	out := make(map[string]string, len(s.digests))
	for k, v := range s.digests {
		out[k] = v
	}
	return out
}

type SnapshotDiff struct{ Added, Removed, Modified []string }

func (d SnapshotDiff) Changed() bool { return len(d.Added)+len(d.Removed)+len(d.Modified) > 0 }

// Diff compares two successfully captured snapshots by path and exact bytes.
func (s TextSnapshot) Diff(next TextSnapshot) SnapshotDiff {
	var d SnapshotDiff
	for p, v := range s.digests {
		if n, ok := next.digests[p]; !ok {
			d.Removed = append(d.Removed, p)
		} else if n != v {
			d.Modified = append(d.Modified, p)
		}
	}
	for p := range next.digests {
		if _, ok := s.digests[p]; !ok {
			d.Added = append(d.Added, p)
		}
	}
	sort.Strings(d.Added)
	sort.Strings(d.Removed)
	sort.Strings(d.Modified)
	return d
}
