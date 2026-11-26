package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
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

// SaveState writes a complete, validated snapshot through a same-directory
// temporary file. Callers serialize saves. Cross-platform crash durability
// (including directory sync and replacement semantics) is not yet verified.
func SaveState(path string, w *Workspace, theme string) error {
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
	return writeWorkspaceFile(path, data)
}

func writeWorkspaceFile(path string, data []byte) error {
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
	return os.Rename(f.Name(), path)
}
