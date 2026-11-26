package xerj

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var autoindexPrefix = regexp.MustCompile(`^[a-z0-9]{1,64}$`)

// AutoindexConfig is trusted application configuration, not Agent input.
// Corpus must be an approved staged snapshot, StateRoot a private directory
// outside the corpus, and Binary an immutable executable with a trusted digest.
// The caller must reserve a fresh prefix on a dedicated local development node.
type AutoindexConfig struct {
	Binary    string
	SHA256    string
	Corpus    string
	StateRoot string
	Prefix    string
}

// Run is a build callback for Manager.Rebuild. Exit zero only means the
// subprocess reported success; it does not establish corpus completeness or
// index ownership. Independent verification is required before publication.
// Each invocation leaves a private journal directory for later reconciliation.
func (c AutoindexConfig) Run(ctx context.Context, target BuildTarget) error {
	if ctx == nil {
		return errors.New("context required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !autoindexPrefix.MatchString(c.Prefix) || !strings.HasPrefix(target.Index, c.Prefix+"-") || target.Index == c.Prefix+"-" || target.Generation == 0 || strings.TrimSpace(target.Workspace) == "" {
		return errors.New("invalid generation or prefix")
	}
	if _, err := New(target.Endpoint, target.Index); err != nil {
		return err
	}
	corpus, err := resolveDirectory(c.Corpus)
	if err != nil {
		return err
	}
	state, err := resolveDirectory(c.StateRoot)
	if err != nil {
		return err
	}
	if containsPath(corpus, state) || containsPath(state, corpus) {
		return errors.New("corpus and state directories must be disjoint")
	}
	binary, err := verifyExecutable(c.Binary, c.SHA256)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	journal, err := os.MkdirTemp(state, "autoindex-")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "autoindex", corpus, "--url", target.Endpoint, "--prefix", c.Prefix, "--state-dir", journal, "--workers", "1", "--no-semantic", "--progress", "none", "--disable-feedback")
	cmd.Dir = journal
	// Do not inherit proxy, API-key, or other application environment variables.
	cmd.Env = []string{"XERJ_DISABLE_FEEDBACK=true"}
	cmd.WaitDelay = time.Second
	// Discard output; it may contain sensitive source paths or content.
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	err = cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		return nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return fmt.Errorf("autoindex exit %d: generation not verified", exit.ExitCode())
	}
	return fmt.Errorf("autoindex launch failed: %w", err)
}

func resolveDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("directory path must be absolute")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("expected directory")
	}
	return resolved, nil
}

func containsPath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func verifyExecutable(path, digest string) (string, error) {
	if !filepath.IsAbs(path) || len(digest) != 64 {
		return "", errors.New("absolute binary path and SHA-256 required")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return "", errors.New("invalid SHA-256")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("binary must be a regular file")
	}
	f, err := os.Open(resolved)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), digest) {
		return "", errors.New("binary SHA-256 mismatch")
	}
	return resolved, nil
}
