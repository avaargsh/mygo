package xerj

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// StagedCorpus contains an independent copy of the approved source bytes and
// their digests. It is not immutable against other processes or a sandbox.
// The application owns the private directory and its lifetime after success.
type StagedCorpus struct {
	root     string
	snapshot TextSnapshot
}

func (s StagedCorpus) Root() string               { return s.root }
func (s StagedCorpus) Digests() map[string]string { return s.snapshot.Digests() }

// StageTextCorpus copies only explicitly approved files to a fresh private
// directory under a separate staging root. Both roots must be trusted and
// stable during the operation. The caller must keep the staged bytes unchanged
// until the build and verification finish. No indexing is performed here.
func StageTextCorpus(ctx context.Context, sourceRoot, stagingRoot string, paths []string) (result StagedCorpus, err error) {
	if ctx == nil || len(paths) == 0 || len(paths) > 32 {
		return StagedCorpus{}, errors.New("invalid staging scope")
	}
	if err := ctx.Err(); err != nil {
		return StagedCorpus{}, err
	}
	source, err := resolveDirectory(sourceRoot)
	if err != nil {
		return StagedCorpus{}, err
	}
	parent, err := resolveDirectory(stagingRoot)
	if err != nil {
		return StagedCorpus{}, err
	}
	if containsPath(source, parent) || containsPath(parent, source) {
		return StagedCorpus{}, errors.New("source and staging roots must be disjoint")
	}
	dir, err := os.MkdirTemp(parent, "xerj-corpus-")
	if err != nil {
		return StagedCorpus{}, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(dir))
		}
	}()
	snapshot, err := captureText(ctx, source, paths, func(name string, data []byte) error {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(data)
		return errors.Join(writeErr, f.Close())
	})
	if err != nil {
		return StagedCorpus{}, err
	}
	return StagedCorpus{root: dir, snapshot: snapshot}, nil
}
