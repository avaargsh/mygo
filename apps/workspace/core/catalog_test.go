package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/internal/wait"
)

func TestWorkspaceCatalogKeepsIndependentCreations(t *testing.T) {
	root := t.TempDir()
	first, err := OpenWorkspaceCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenWorkspaceCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	type outcome struct {
		profile WorkspaceProfile
		err     error
	}
	results := make(chan outcome, 2)
	for _, spec := range []struct {
		catalog *WorkspaceCatalog
		name    string
	}{{first, "Research"}, {second, "Development"}} {
		go func() {
			<-start
			profile, err := spec.catalog.Create(spec.name)
			results <- outcome{profile, err}
		}()
	}
	close(start)

	ids := map[string]bool{DefaultWorkspaceID: true}
	for range 2 {
		result := wait.For(t, "workspace creation to finish", results)
		if result.err != nil || ids[result.profile.ID] {
			t.Fatalf("workspace creation did not return a distinct ID: %+v", result)
		}
		ids[result.profile.ID] = true
	}

	reopened, err := OpenWorkspaceCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	profiles := reopened.Profiles()
	if len(profiles) != 3 || reopened.Active().ID != DefaultWorkspaceID {
		t.Fatalf("unexpected catalog after concurrent creation: %+v", profiles)
	}
	for _, profile := range profiles {
		dir, err := reopened.Directory(profile.ID)
		if err != nil || !ids[profile.ID] || !filepath.IsLocal(mustRel(t, root, dir)) {
			t.Fatalf("workspace did not resolve inside the config root: %q, %v", dir, err)
		}
	}
	selected := profiles[1]
	if err := reopened.Select(selected.ID); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenWorkspaceCatalog(root)
	if err != nil || restarted.Active() != selected {
		t.Fatalf("selected workspace was not restored: %v", err)
	}
	if _, err := restarted.Create("  " + selected.Name + " "); err == nil {
		t.Fatal("duplicate workspace name was accepted")
	}
}

func mustRel(t *testing.T, root, dir string) string {
	t.Helper()
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}

func TestWorkspaceSelectionFailureKeepsCurrentWorkspace(t *testing.T) {
	root := t.TempDir()
	catalog, err := OpenWorkspaceCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := catalog.Create("Research")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "active-workspace.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Select(profile.ID); err == nil || catalog.Active().ID != DefaultWorkspaceID {
		t.Fatal("failed selection exposed a new active workspace")
	}
}

func TestWorkspaceCatalogRejectsOutsideSelection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "active-workspace.json"), []byte(`{"version":1,"id":"../outside"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWorkspaceCatalog(root); err == nil {
		t.Fatal("invalid workspace path was accepted")
	}
}

func TestWorkspaceCatalogDoesNotTreatMalformedProfileAsMissing(t *testing.T) {
	root := t.TempDir()
	catalog, err := OpenWorkspaceCatalog(root)
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
	dir, err := catalog.Directory(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWorkspaceCatalog(root); err == nil {
		t.Fatal("startup hid the error in an existing workspace profile")
	}
}
