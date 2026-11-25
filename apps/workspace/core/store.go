package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const maxStateBytes = 4 << 20

type desktopState struct {
	Version   int               `json:"version"`
	Theme     string            `json:"theme"`
	Workspace WorkspaceSnapshot `json:"workspace"`
}

func validTheme(mode string) bool { return mode == "system" || mode == "light" || mode == "dark" }

// LoadState validates the entire saved state before exposing a workspace.
func LoadState(path string) (*Workspace, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxStateBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxStateBytes {
		return nil, "", errors.New("workspace state exceeds size limit")
	}
	var s desktopState
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return nil, "", err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, "", errors.New("trailing workspace state data")
	}
	if s.Version != 1 || !validTheme(s.Theme) {
		return nil, "", errors.New("unsupported desktop state version or theme")
	}
	w, err := RestoreWorkspace(s.Workspace)
	if err != nil {
		return nil, "", err
	}
	return w, s.Theme, nil
}

var stateSaveMu sync.Mutex

// SaveState writes a complete validated snapshot through a same-directory
// temporary file. Saves in this process are serialized; callers still own
// Workspace mutations. Concurrent independent snapshots are last-writer-wins,
// not merged. Cross-process coordination is outside this contract.
func SaveState(path string, w *Workspace, theme string) error {
	stateSaveMu.Lock()
	defer stateSaveMu.Unlock()
	return saveState(path, w, theme, os.Rename, syncStateDirectory)
}

// replace and syncDir allow deterministic fault injection at the commit boundary.
func saveState(path string, w *Workspace, theme string, replace func(string, string) error, syncDir func(string) error) error {
	if w == nil || !validTheme(theme) {
		return errors.New("invalid workspace or theme")
	}
	s := desktopState{Version: 1, Theme: theme, Workspace: w.Snapshot()}
	if _, err := RestoreWorkspace(s.Workspace); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if len(data) > maxStateBytes {
		return errors.New("workspace state exceeds size limit")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".workspace-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err := replace(f.Name(), path); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		// The new snapshot may already be visible; do not claim rollback.
		return fmt.Errorf("workspace replaced but directory sync failed: %w", err)
	}
	return nil
}

// Directory fsync is supported on the targeted Linux and macOS filesystems.
// Other platforms retain the existing replacement behavior; this is not a
// cross-platform power-loss guarantee.
func syncStateDirectory(path string) error {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
