package core

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const DefaultWorkspaceID = "default"
const MaxWorkspaceNameRunes = 64
const maxWorkspaceProfiles = 32
const maxWorkspaceRecordBytes = 4 << 10

type WorkspaceProfile struct {
	ID   string
	Name string
}

type workspaceProfileRecord struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
}

type workspaceSelection struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
}

// WorkspaceCatalog owns local workspace identities and the last selection.
// The caller serializes changes. One record per workspace keeps concurrent
// creations from overwriting each other.
type WorkspaceCatalog struct {
	root                  string
	active                string
	profiles              []WorkspaceProfile
	missingSavedWorkspace bool
}

func OpenWorkspaceCatalog(root string) (*WorkspaceCatalog, error) {
	c := &WorkspaceCatalog{root: root, active: DefaultWorkspaceID}
	var saved workspaceSelection
	err := readWorkspaceRecord(c.selectionPath(), &saved)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	case saved.Version != 1 || !validWorkspaceID(saved.ID):
		return nil, errors.New("invalid workspace selection")
	}
	if err := c.Refresh(); err != nil {
		return nil, err
	}
	if saved.ID != "" {
		if slices.ContainsFunc(c.profiles, func(p WorkspaceProfile) bool { return p.ID == saved.ID }) {
			c.active = saved.ID
		} else {
			// Keep the saved selection intact so restoring the workspace directory
			// later makes the original selection usable again.
			c.missingSavedWorkspace = true
		}
	}
	return c, nil
}

func (c *WorkspaceCatalog) Refresh() error {
	profiles := []WorkspaceProfile{{ID: DefaultWorkspaceID, Name: "Default"}}
	parent := c.workspaceRoot()
	if err := realDirectory(parent); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(parent)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			id := entry.Name()
			if id == DefaultWorkspaceID || !validWorkspaceID(id) {
				continue
			}
			if !entry.IsDir() {
				return errors.New("workspace storage is not a directory")
			}
			var record workspaceProfileRecord
			if err := readWorkspaceRecord(filepath.Join(parent, id, "profile.json"), &record); err != nil {
				return err
			}
			if record.Version != 1 || !validWorkspaceName(record.Name) {
				return errors.New("invalid workspace profile")
			}
			profiles = append(profiles, WorkspaceProfile{ID: id, Name: record.Name})
		}
	}
	if !slices.ContainsFunc(profiles, func(p WorkspaceProfile) bool { return p.ID == c.active }) {
		return errors.New("selected workspace is missing")
	}
	slices.SortFunc(profiles[1:], func(a, b WorkspaceProfile) int {
		if cmp := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})
	c.profiles = profiles
	return nil
}

func readWorkspaceRecord(path string, record any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("workspace record is not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxWorkspaceRecordBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxWorkspaceRecordBytes {
		return errors.New("workspace record exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(record); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing workspace record data")
	}
	return nil
}

func validWorkspaceID(id string) bool {
	if id == DefaultWorkspaceID {
		return true
	}
	if len(id) != 32 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func validWorkspaceName(name string) bool {
	return name != "" && name == strings.TrimSpace(name) && utf8.ValidString(name) &&
		utf8.RuneCountInString(name) <= MaxWorkspaceNameRunes && !strings.ContainsFunc(name, unicode.IsControl)
}

func (c *WorkspaceCatalog) Profiles() []WorkspaceProfile { return slices.Clone(c.profiles) }

// MissingSavedWorkspace reports whether startup fell back to Default because
// the saved workspace directory was absent.
func (c *WorkspaceCatalog) MissingSavedWorkspace() bool { return c.missingSavedWorkspace }

func (c *WorkspaceCatalog) Active() WorkspaceProfile {
	index := slices.IndexFunc(c.profiles, func(p WorkspaceProfile) bool { return p.ID == c.active })
	return c.profiles[index]
}

// Directory resolves a registered ID under the app's private config root.
func (c *WorkspaceCatalog) Directory(id string) (string, error) {
	if !slices.ContainsFunc(c.profiles, func(p WorkspaceProfile) bool { return p.ID == id }) {
		return "", errors.New("unknown workspace")
	}
	if id == DefaultWorkspaceID {
		return c.root, nil
	}
	parent := c.workspaceRoot()
	if err := realDirectory(parent); err != nil {
		return "", err
	}
	dir := filepath.Join(parent, id)
	return dir, realDirectory(dir)
}

func realDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("workspace storage is not a directory")
	}
	return nil
}

func (c *WorkspaceCatalog) Create(name string) (WorkspaceProfile, error) {
	name = strings.TrimSpace(name)
	if !validWorkspaceName(name) {
		return WorkspaceProfile{}, errors.New("enter a workspace name of at most 64 characters")
	}
	if err := c.Refresh(); err != nil {
		return WorkspaceProfile{}, err
	}
	if slices.ContainsFunc(c.profiles, func(p WorkspaceProfile) bool { return strings.EqualFold(p.Name, name) }) {
		return WorkspaceProfile{}, errors.New("that workspace name is already in use")
	}
	if len(c.profiles) >= maxWorkspaceProfiles {
		return WorkspaceProfile{}, errors.New("workspace limit reached")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return WorkspaceProfile{}, err
	}
	profile := WorkspaceProfile{ID: hex.EncodeToString(id[:]), Name: name}

	parent := c.workspaceRoot()
	if err := os.Mkdir(parent, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return WorkspaceProfile{}, err
	}
	if err := realDirectory(parent); err != nil {
		return WorkspaceProfile{}, err
	}
	stage, err := os.MkdirTemp(parent, ".creating-*")
	if err != nil {
		return WorkspaceProfile{}, err
	}
	defer os.RemoveAll(stage)
	raw, err := json.Marshal(workspaceProfileRecord{Version: 1, Name: name})
	if err != nil {
		return WorkspaceProfile{}, err
	}
	if err := writeWorkspaceFile(filepath.Join(stage, "profile.json"), raw); err != nil {
		return WorkspaceProfile{}, err
	}
	if err := os.Rename(stage, filepath.Join(parent, profile.ID)); err != nil {
		return WorkspaceProfile{}, err
	}
	c.profiles = append(c.profiles, profile)
	return profile, nil
}

func (c *WorkspaceCatalog) Select(id string) error {
	if _, err := c.Directory(id); err != nil {
		return err
	}
	if c.active == id && !c.missingSavedWorkspace {
		return nil
	}
	raw, err := json.Marshal(workspaceSelection{Version: 1, ID: id})
	if err != nil {
		return err
	}
	if err := writeWorkspaceFile(c.selectionPath(), raw); err != nil {
		return err
	}
	c.active = id
	c.missingSavedWorkspace = false
	return nil
}

func (c *WorkspaceCatalog) selectionPath() string {
	return filepath.Join(c.root, "active-workspace.json")
}

func (c *WorkspaceCatalog) workspaceRoot() string {
	return filepath.Join(c.root, "workspaces")
}
