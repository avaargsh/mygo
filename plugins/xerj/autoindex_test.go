package xerj

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAutoindexCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture; Windows requires native acceptance")
	}
	root := t.TempDir()
	corpus, state := filepath.Join(root, "approved corpus"), filepath.Join(root, "state")
	for _, dir := range []string{corpus, state} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	// The production adapter resolves symlinks before invoking the engine.
	// On macOS, TempDir may use /var while the physical path uses /private/var.
	canonicalCorpus, err := filepath.EvalSymlinks(corpus)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "xerj-fixture")
	// The fixture validates argv and the opt-out environment without contacting a node.
	for _, code := range []int{0, 3, 4} {
		script := fmt.Sprintf(`#!/bin/sh
[ "$#" = 14 ] && [ "$1" = autoindex ] && [ "$2" = %q ] && [ "$3" = --url ] && [ "$4" = http://127.0.0.1:9200 ] && [ "$5" = --prefix ] && [ "$6" = mygoabc ] && [ "$7" = --state-dir ] && [ "$8" = "$(pwd -P)" ] && [ "$9" = --workers ] && [ "${10}" = 1 ] && [ "${11}" = --no-semantic ] && [ "${12}" = --progress ] && [ "${13}" = none ] && [ "${14}" = --disable-feedback ] && [ "$XERJ_DISABLE_FEEDBACK" = true ] || exit 99
exit %d
`, canonicalCorpus, code)
		if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		cfg := AutoindexConfig{Binary: binary, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(script))), Corpus: corpus, StateRoot: state, Prefix: "mygoabc"}
		target := BuildTarget{Workspace: "a", Index: "mygoabc-docs", Endpoint: "http://127.0.0.1:9200", Generation: 1}
		err := cfg.Run(context.Background(), target)
		if code == 0 && err != nil {
			t.Fatal(err)
		}
		if code != 0 && (err == nil || !strings.Contains(err.Error(), fmt.Sprintf("exit %d", code))) {
			t.Fatalf("exit %d: %v", code, err)
		}
	}
	entries, err := os.ReadDir(state)
	if err != nil || len(entries) != 3 {
		t.Fatalf("state dirs: %v %v", entries, err)
	}
}

func TestAutoindexRejectsInvalidConfig(t *testing.T) {
	root := t.TempDir()
	corpus, state := filepath.Join(root, "corpus"), filepath.Join(root, "state")
	for _, dir := range []string{corpus, state} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(root, "fixture")
	if err := os.WriteFile(binary, []byte("not executable"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := AutoindexConfig{Binary: binary, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("not executable"))), Corpus: corpus, StateRoot: state, Prefix: "mygoabc"}
	target := BuildTarget{Workspace: "a", Index: "mygoabc-docs", Endpoint: "http://127.0.0.1:9200", Generation: 1}
	for _, change := range []func(*AutoindexConfig, *BuildTarget){
		func(c *AutoindexConfig, _ *BuildTarget) { c.SHA256 = strings.Repeat("0", 64) },
		func(c *AutoindexConfig, _ *BuildTarget) { c.StateRoot = corpus },
		func(c *AutoindexConfig, _ *BuildTarget) { c.Corpus = "." },
		func(c *AutoindexConfig, _ *BuildTarget) { c.Prefix = "*" },
		func(_ *AutoindexConfig, t *BuildTarget) { t.Endpoint = "http://example.org:9200" },
		func(_ *AutoindexConfig, t *BuildTarget) { t.Index = "other-docs" },
	} {
		c, b := cfg, target
		change(&c, &b)
		if err := c.Run(context.Background(), b); err == nil {
			t.Fatal("accepted invalid config")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cfg.Run(ctx, target); err != context.Canceled {
		t.Fatalf("cancel: %v", err)
	}
	entries, err := os.ReadDir(state)
	if err != nil || len(entries) != 0 {
		t.Fatalf("invalid config created state: %v %v", entries, err)
	}
}
