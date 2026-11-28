package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/apps/workspace/adapters/model"
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

func TestWorkspaceChooserIsolatesDataAndRestoresSelection(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("legacy note"), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := newWorkspaceHost(dir)
	if err != nil {
		t.Fatal(err)
	}
	original := h.current
	tt := ui.NewTester(h.view, 1200, 800)
	click := func(label string) {
		t.Helper()
		tt.Frame()
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %q: %v", label, err)
		}
	}
	click("Notes")
	if original.note().Text != "legacy note" {
		t.Fatal("default workspace lost the legacy note")
	}
	original.note().Text = "unsaved default note"
	original.open("chat")
	chat := original.chatSession()
	done, err := chat.chat.Start(t.Context(), func(ctx context.Context, _ string, _ []model.Message, _ func(string) error) (model.Completion, error) {
		<-ctx.Done()
		return model.Completion{}, ctx.Err()
	}, "", "pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer chat.chat.Cancel()
	click("Workspaces")
	click("Workspace name")
	tt.Type("Research")
	click("Create workspace")
	research := h.current
	if research == original || research.statePath == original.statePath || research.content == original.content {
		t.Fatal("the new workspace reused the default workspace")
	}
	if !chat.chat.Snapshot().Active || research.chatSession() != nil {
		t.Fatal("switching canceled or shared the default Chat")
	}
	click("Notes")
	if research.note().Text != "" {
		t.Fatal("the new workspace inherited a note")
	}
	research.note().Text = "research only"
	click("Save note")
	click("Split right")
	click("Save workspace")
	if len(research.workspace.Snapshot().Panes) != 2 || len(original.workspace.Snapshot().Panes) != 1 {
		t.Fatal("splitting the new workspace also changed the default layout")
	}

	click("Workspaces")
	click("Switch to Default")
	if h.current != original || h.current.note().Text != "unsaved default note" {
		t.Fatal("switching discarded the retained session")
	}
	click("Workspaces")
	click("Switch to Research")
	if h.current != research {
		t.Fatal("switching replaced the Research session")
	}
	restarted, err := newWorkspaceHost(dir)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.current.profile.Name != "Research" || len(restarted.current.workspace.Snapshot().Panes) != 2 ||
		restarted.current.note() == nil || restarted.current.note().Text != "research only" {
		t.Fatal("restart failed to restore the selected workspace and its own state")
	}
	if legacy, err := os.ReadFile(filepath.Join(dir, "notes.md")); err != nil || string(legacy) != "legacy note" {
		t.Fatalf("opening and switching rewrote the legacy note: %q, %v", legacy, err)
	}
	if h.confirmClose(func(opts mygo.MessageOptions) (mygo.MessageResult, error) {
		if !strings.Contains(opts.Message, "Default") {
			t.Fatalf("close prompt did not identify the hidden workspace: %q", opts.Message)
		}
		return mygo.MessageResult{Button: closeCancel}, nil
	}) {
		t.Fatal("closing ignored unsaved changes in a hidden workspace")
	}
	if note := original.note(); note.Text != "unsaved default note" || !note.Dirty() {
		t.Fatal("canceling close discarded the hidden note")
	}
	chat.chat.Cancel()
	wait.For(t, "default Chat to stop", done)
	if _, _, err := core.LoadState(research.statePath); err != nil {
		t.Fatalf("workspace did not write a valid snapshot at its own path: %v", err)
	}
}

func TestWorkspaceStartupUsesDefaultWhenSelectedWorkspaceIsMissing(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(notePath, []byte("default note"), 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := core.OpenWorkspaceCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := catalog.Create("Research")
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Select(profile.ID); err != nil {
		t.Fatal(err)
	}
	selectionPath := filepath.Join(dir, "active-workspace.json")
	selection, err := os.ReadFile(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	profileDir, err := catalog.Directory(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(profileDir, profileDir+".backup"); err != nil {
		t.Fatal(err)
	}

	h, err := newWorkspaceHost(dir)
	if err != nil {
		t.Fatalf("open workspace after selected directory went missing: %v", err)
	}
	if h.current.profile.ID != core.DefaultWorkspaceID || !strings.Contains(h.current.status, "missing") {
		t.Fatalf("missing selection was not reported on the default workspace: %+v, %q", h.current.profile, h.current.status)
	}
	tt := ui.NewTester(h.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("Notes"); err != nil {
		t.Fatal(err)
	}
	if h.current.note() == nil || h.current.note().Text != "default note" {
		t.Fatal("default workspace did not reopen its original note")
	}
	if got, err := os.ReadFile(selectionPath); err != nil || string(got) != string(selection) {
		t.Fatalf("startup rewrote the original selection: %q, %v", got, err)
	}

	if err := os.Rename(profileDir+".backup", profileDir); err != nil {
		t.Fatal(err)
	}
	restored, err := newWorkspaceHost(dir)
	if err != nil || restored.current.profile != profile {
		t.Fatalf("restored profile was not selected: %v", err)
	}

	// This time deliberately keep Default before restoring the old directory.
	if err := os.Rename(profileDir, profileDir+".backup"); err != nil {
		t.Fatal(err)
	}
	h, err = newWorkspaceHost(dir)
	if err != nil {
		t.Fatal(err)
	}
	tt = ui.NewTester(h.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("Workspaces"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Keep Default on startup"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(profileDir+".backup", profileDir); err != nil {
		t.Fatal(err)
	}
	kept, err := newWorkspaceHost(dir)
	if err != nil {
		t.Fatal(err)
	}
	if kept.current.profile.ID != core.DefaultWorkspaceID || kept.catalog.MissingSavedWorkspace() {
		t.Fatal("an explicit Default selection did not survive restart")
	}
}

func TestClosingChecksDirtyWorkspaceAfterItsDirectoryIsMoved(t *testing.T) {
	dir := t.TempDir()
	h, err := newWorkspaceHost(dir)
	if err != nil {
		t.Fatal(err)
	}
	defaultProfile := h.current.profile
	h.newName = "Research"
	h.createWorkspace()
	research := h.current
	if research.profile == defaultProfile || h.chooserErr != "" {
		t.Fatalf("create Research: %s", h.chooserErr)
	}
	research.open("notes")
	research.note().Text = "still in memory"
	h.selectWorkspace(defaultProfile)
	if h.current.profile != defaultProfile || h.chooserErr != "" {
		t.Fatalf("switch back to Default: %s", h.chooserErr)
	}
	researchDir, err := h.catalog.Directory(research.profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(researchDir, researchDir+".backup"); err != nil {
		t.Fatal(err)
	}

	h.showChooser()
	if h.chooserErr != "" || len(h.catalog.Profiles()) != 1 {
		t.Fatalf("expected only Default after refresh: %v, %s", h.catalog.Profiles(), h.chooserErr)
	}
	var dialogs int
	if h.confirmClose(func(opts mygo.MessageOptions) (mygo.MessageResult, error) {
		dialogs++
		if !strings.Contains(opts.Message, "Research") {
			t.Fatalf("wrong workspace in close prompt: %q", opts.Message)
		}
		return mygo.MessageResult{Button: closeCancel}, nil
	}) {
		t.Fatal("quit ignored the unsaved note in the loaded Research workspace")
	}
	if dialogs != 1 || !research.note().Dirty() || research.note().Text != "still in memory" {
		t.Fatal("cancel did not preserve the removed workspace's note in memory")
	}
	save := func(mygo.MessageOptions) (mygo.MessageResult, error) {
		return mygo.MessageResult{Button: closeSave}, nil
	}
	if h.confirmClose(save) || !research.note().Dirty() || !strings.Contains(h.current.status, "Research: Note save failed") {
		t.Fatal("a save failure should keep the window and unsaved edits")
	}
	if err := os.Rename(researchDir+".backup", researchDir); err != nil {
		t.Fatal(err)
	}
	if !h.confirmClose(save) || research.note().Dirty() {
		t.Fatal("restoring the directory should let the user save and exit")
	}
	if got, err := os.ReadFile(filepath.Join(researchDir, "notes.md")); err != nil || string(got) != "still in memory" {
		t.Fatalf("note missing after saving to the restored directory: %q, %v", got, err)
	}
}
