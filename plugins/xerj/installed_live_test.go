package xerj

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	installedMarkerA = "mygoinstalledalpha"
	installedMarkerB = "mygoinstalledbeta"
)

// Opt-in only: the caller must start a disposable, exclusively owned local
// node using the same pinned executable. No auto-install or index deletion.
func TestLiveInstalledFlow(t *testing.T) {
	if os.Getenv("MYGO_XERJ_ACCEPTANCE") != "1" {
		t.Skip("requires a pinned XERJ executable and disposable local node")
	}
	endpoint, binary, digest, suffix := os.Getenv("MYGO_XERJ_ENDPOINT"), os.Getenv("MYGO_XERJ_BINARY"), os.Getenv("MYGO_XERJ_SHA256"), os.Getenv("MYGO_XERJ_DATASET_SUFFIX")
	if !validDatasetSuffix(suffix) {
		t.Fatal("MYGO_XERJ_DATASET_SUFFIX must be an independently specified exact fixture dataset suffix")
	}
	if _, err := New(endpoint, "acceptance-probe"); err != nil {
		t.Fatal(err)
	}
	verified, err := verifyExecutable(binary, digest)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	prefix := "mygoaccept" + hex.EncodeToString(nonce[:])
	t.Logf("fixture prefixes %sa and %sb; binary SHA-256 %s", prefix, prefix, digest)
	exerciseInstalledFlow(t, endpoint, verified, digest, suffix, prefix)
}

func exerciseInstalledFlow(t *testing.T, endpoint, binary, digest, suffix, prefix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	source, stage, cliState, state := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	manifestPath, journalPath := filepath.Join(state, "manifest.json"), filepath.Join(state, "jobs.json")
	m, err := CreatePersistentManager(endpoint, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	q, err := CreateJournaledBuildQueue(m, 1, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	const workspace = "installed-fixture"
	const name = "mygo-xerj-fixture.txt"
	if err := os.WriteFile(filepath.Join(source, "unapproved.txt"), []byte("unapprovedsyntheticfixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for i, marker := range []string{installedMarkerA, installedMarkerB} {
		p := prefix + []string{"a", "b"}[i]
		index := p + "-" + suffix
		if err := os.WriteFile(filepath.Join(source, name), []byte(marker), 0600); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			if err := m.Invalidate(workspace); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Search(ctx, workspace, installedMarkerA, 5); !errors.Is(err, ErrIndexNotReady) {
				t.Fatalf("invalidated search: %v", err)
			}
		}
		cfg := AutoindexConfig{Binary: binary, SHA256: digest, StateRoot: cliState, Prefix: p}
		job, err := q.SubmitApprovedText(ctx, workspace, cfg, source, stage, map[string][]string{index: {name}})
		if err != nil {
			t.Fatalf("generation %d submit: %v", i+1, err)
		}
		if ok, err := q.RunNext(ctx); !ok || err != nil {
			t.Fatalf("generation %d build/verify: processed=%t err=%v", i+1, ok, err)
		}
		if err := job.Wait(ctx); err != nil {
			t.Fatal(err)
		}
		if job.Status() != BuildSucceeded || job.CleanupError() != nil {
			t.Fatalf("job %v cleanup %v", job.Status(), job.CleanupError())
		}
		entries, err := os.ReadDir(stage)
		if err != nil || len(entries) != 0 {
			t.Fatalf("staging cleanup: %d entries err=%v", len(entries), err)
		}
		if got := m.Status(workspace); got.State != IndexReady || got.Generation != uint64(i+1) {
			t.Fatalf("unexpected binding %+v", got)
		}
		// The original source is not the indexed directory. This query runs
		// only after the indexed stage has been removed by the queue.
		hits, err := m.Search(ctx, workspace, marker, 5)
		if err != nil {
			t.Fatalf("post-cleanup search generation %d: %v", i+1, err)
		}
		found := false
		for _, h := range hits {
			if h.Index != index {
				t.Fatalf("unexpected hit index %q", h.Index)
			}
			if filepath.Base(h.Path) == name && strings.Contains(h.Text, marker) {
				found = true
			}
			if strings.Contains(h.Text, "unapprovedsyntheticfixture") || (i == 1 && strings.Contains(h.Text, installedMarkerA)) {
				t.Fatal("unexpected content in current generation")
			}
		}
		if !found {
			t.Fatalf("expected fixture passage absent for generation %d", i+1)
		}
		t.Logf("generation %d verified, stage removed and marker retrieved from %s", i+1, index)
	}
	// Logical owner reopen only; no concurrent operations or process crash.
	recovered, err := OpenPersistentManager(endpoint, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	history, err := OpenJournaledBuildQueue(recovered, 1, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	records := history.History()
	if len(records) != 2 || records[0].State != BuildSucceeded || records[1].State != BuildSucceeded {
		t.Fatalf("unexpected history %+v", records)
	}
	if got := recovered.Status(workspace); got.State != IndexFailed || got.Generation != 2 {
		t.Fatalf("reopen binding %+v", got)
	}
	if _, err := recovered.Search(ctx, workspace, installedMarkerB, 5); !errors.Is(err, ErrIndexNotReady) {
		t.Fatalf("reopened search: %v", err)
	}
	if _, err := recovered.Begin("other", prefix+"a-"+suffix); err == nil {
		t.Fatal("historical reservation lost")
	}
	t.Log("persistent reopen preserved history and fenced search")
}

// TestLiveInstalledNormalizedBytesRejected checks the separately built probe
 // whose original bytes include a newline. Never authorize transformed bytes.
func TestLiveInstalledNormalizedBytesRejected(t *testing.T) {
	if os.Getenv("MYGO_XERJ_ACCEPTANCE") != "1" || os.Getenv("MYGO_XERJ_NORMALIZED_PROBE") != "1" {
		t.Skip("requires the disposable acceptance probe")
	}
	digest := sha256.Sum256([]byte(installedMarkerA+"\n"))
	v := TextIndexVerifier{Expected: map[string]string{"mygo-xerj-fixture.txt": hex.EncodeToString(digest[:])}}
	err := v.Verify(context.Background(), BuildTarget{Workspace:"probe", Index:"mygoslugprobe-docs", Endpoint:os.Getenv("MYGO_XERJ_ENDPOINT"), Generation:1})
	if err == nil || err.Error() != "section content mismatch" {
		t.Fatalf("expected byte mismatch, got %v", err)
	}
}
