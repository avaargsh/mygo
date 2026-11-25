package xerj

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
)

func TestSubmitApprovedTextCleanup(t *testing.T) {
	source, staging, state := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("needle"), 0600); err != nil {
		t.Fatal(err)
	}
	q, _ := NewBuildQueue(NewManager("http://127.0.0.1:9200"), 1)
	cfg := AutoindexConfig{Prefix: "demo", StateRoot: state}
	datasets := map[string][]string{"demo-code": {"a.txt"}}
	j, err := q.SubmitApprovedText(context.Background(), "a", cfg, source, staging, datasets)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(staging)
	if err != nil || len(entries) != 1 {
		t.Fatalf("stage %v %v", entries, err)
	}
	cfg.Prefix = "other"
	if _, err := q.SubmitApprovedText(context.Background(), "b", cfg, source, staging, map[string][]string{"other-code": {"a.txt"}}); !errors.Is(err, ErrBuildQueueFull) {
		t.Fatalf("full %v", err)
	}
	entries, err = os.ReadDir(staging)
	if err != nil || len(entries) != 1 {
		t.Fatalf("rejected stage %v %v", entries, err)
	}
	if !j.Cancel() || j.CleanupError() != nil {
		t.Fatalf("cancel %v", j.CleanupError())
	}
	entries, err = os.ReadDir(staging)
	if err != nil || len(entries) != 0 {
		t.Fatalf("cancel stage %v %v", entries, err)
	}
	if _, err := q.SubmitApprovedText(context.Background(), "a", cfg, source, staging, map[string][]string{"other-code": {"a.txt"}, "other-notes": {"a.txt"}}); err == nil {
		t.Fatal("accepted duplicate source assignment")
	}
}

func TestSubmitApprovedTextBuildAndRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	for _, bad := range []bool{false, true} {
		t.Run(fmt.Sprint(bad), func(t *testing.T) {
			source, staging, state := t.TempDir(), t.TempDir(), t.TempDir()
			for name, body := range map[string]string{"a.txt": "needle", "private.txt": "unapproved"} {
				if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			script := []byte("#!/bin/sh\n[ -f \"$2/a.txt\" ] && [ ! -e \"$2/private.txt\" ] || exit 9\nexit 0\n")
			binary := filepath.Join(t.TempDir(), "fixture")
			if err := os.WriteFile(binary, script, 0700); err != nil {
				t.Fatal(err)
			}
			var catalogs atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/_cat/indices" {
					if catalogs.Add(1) == 1 {
						fmt.Fprint(w, `[]`)
					} else {
						fmt.Fprint(w, `[{"index":"demo-code"}]`)
					}
					return
				}
				body := "needle"
				if bad {
					body = "wrong"
				}
				fmt.Fprintf(w, `{"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_index":"demo-code","_source":{"ax_path":"a.txt","body":%q}}]}}`, body)
			}))
			defer s.Close()
			m := NewManager(s.URL)
			q, _ := NewBuildQueue(m, 1)
			cfg := AutoindexConfig{Prefix: "demo", StateRoot: state, Binary: binary, SHA256: fmt.Sprintf("%x", sha256.Sum256(script))}
			job, err := q.SubmitApprovedText(context.Background(), "a", cfg, source, staging, map[string][]string{"demo-code": {"a.txt"}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("changed"), 0600); err != nil {
				t.Fatal(err)
			}
			ok, err := q.RunNext(context.Background())
			if !ok || (err != nil) != bad {
				t.Fatalf("run %v %v", ok, err)
			}
			if job.CleanupError() != nil {
				t.Fatal(job.CleanupError())
			}
			entries, err := os.ReadDir(staging)
			if err != nil || len(entries) != 0 {
				t.Fatalf("stage %v %v", entries, err)
			}
			want := IndexReady
			if bad {
				want = IndexFailed
			}
			if m.Status("a").State != want {
				t.Fatal(m.Status("a"))
			}
		})
	}
}

func TestApprovedStageReleasedOnJournalFault(t *testing.T) {
	root, staging, cliState, journalState := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("needle"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := CreatePersistentManager("http://127.0.0.1:9200", filepath.Join(t.TempDir(), "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	q, err := CreateJournaledBuildQueue(m, 1, filepath.Join(journalState, "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	job, err := q.SubmitApprovedText(context.Background(), "a", AutoindexConfig{Prefix: "demo", StateRoot: cliState}, root, staging, map[string][]string{"demo-code": {"a.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(journalState); err != nil {
		t.Fatal(err)
	}
	if ok, err := q.RunNext(context.Background()); ok || !errors.Is(err, ErrBuildJournalUnavailable) {
		t.Fatalf("dispatch %v %v", ok, err)
	}
	if job.Status() != BuildInterrupted || job.CleanupError() != nil {
		t.Fatalf("job %v cleanup %v", job.Status(), job.CleanupError())
	}
	entries, err := os.ReadDir(staging)
	if err != nil || len(entries) != 0 {
		t.Fatalf("stage %v %v", entries, err)
	}
}
