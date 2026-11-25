package xerj

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

// TextIndexVerifier verifies a small, single-dataset text/code pilot. Expected
// is a trusted immutable map of slash-relative source paths to SHA-256 of the
// exact UTF-8 source bytes. Accept either a single body record or the narrow
// rc.103 file+single-section plain-text pair. Reject deduplicated, split,
// transformed and multi-dataset corpora.
// It checks a bounded snapshot, not engine identity or future index freshness.
type TextIndexVerifier struct {
	Expected map[string]string
}

// Verify can be used as Manager.Rebuild's verifier callback. Unlike normal
// passage retrieval, it reads full bodies for this bounded acceptance check.
func (v TextIndexVerifier) Verify(ctx context.Context, target BuildTarget) error {
	if ctx == nil || target.Generation == 0 || strings.TrimSpace(target.Workspace) == "" || len(v.Expected) == 0 || len(v.Expected) > 32 {
		return errors.New("invalid text verification scope")
	}
	expected := make(map[string][32]byte, len(v.Expected))
	for p, digest := range v.Expected {
		if p == "." || !fs.ValidPath(p) || strings.ContainsAny(p, "\\:\x00") || len(p) > 4096 {
			return errors.New("invalid expected relative source path")
		}
		b, err := hex.DecodeString(digest)
		if err != nil || len(b) != 32 {
			return errors.New("invalid expected SHA-256")
		}
		var h [32]byte
		copy(h[:], b)
		expected[p] = h
	}
	c, err := New(target.Endpoint, target.Index)
	if err != nil {
		return err
	}
	defer c.http.CloseIdleConnections()
	body, err := json.Marshal(map[string]any{"size": 2 * len(expected), "track_total_hits": true, "query": map[string]any{"match_all": map[string]any{}}, "_source": []string{"ax_path", "body", "ax_paths", "ax_file", "ax_run", "ax_format", "ax_locator", "start_line", "end_line"}})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("index verification HTTP %d", resp.StatusCode)
	}
	const maxBytes = 2 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxBytes {
		return errors.New("verification response exceeds budget")
	}
	var result struct {
		TimedOut bool `json:"timed_out"`
		Shards   struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
		Hits struct {
			Total struct {
				Value    int    `json:"value"`
				Relation string `json:"relation"`
			} `json:"total"`
			Hits []struct {
				Index  string `json:"_index"`
				Source struct {
					Path string `json:"ax_path"`
					Body *string `json:"body"`
					Paths []string `json:"ax_paths"`
					File string `json:"ax_file"`
					Run string `json:"ax_run"`
					Format string `json:"ax_format"`
					Locator string `json:"ax_locator"`
					StartLine int `json:"start_line"`
					EndLine int `json:"end_line"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	n := result.Hits.Total.Value
	if result.TimedOut || result.Shards.Failed != 0 || result.Hits.Total.Relation != "eq" || n != len(result.Hits.Hits) || (n != len(expected) && n != 2*len(expected)) {
		return errors.New("incomplete or unexpected index cardinality")
	}
	if n == len(expected) {
		// Legacy single-record shape, without autoindex pairing metadata.
		for _, h := range result.Hits.Hits {
			src := h.Source
			want, ok := expected[src.Path]
			if h.Index != target.Index || !ok || src.Body == nil || sha256.Sum256([]byte(*src.Body)) != want ||
				src.File != "" || src.Locator != "" || src.Run != "" || len(src.Paths) != 0 {
				return errors.New("index source identity or content mismatch")
			}
			delete(expected, src.Path)
		}
		if len(expected) != 0 {
			return errors.New("missing expected source")
		}
		return ctx.Err()
	}
	// rc.103 plain-text extraction emits a metadata record and one section.
	// Require a bijection with approved paths, identical file/run identity,
	// and the exact original bytes. In particular, trimmed newlines fail.
	type pair struct {
		file, run string
		header, section bool
	}
	pairs := make(map[string]pair, len(expected))
	owners := make(map[string]string, len(expected))
	for _, h := range result.Hits.Hits {
		src := h.Source
		want, ok := expected[src.Path]
		if h.Index != target.Index || !ok || src.File == "" || src.Run == "" || src.Format != "txt-prose" ||
			len(src.Paths) != 1 || src.Paths[0] != src.Path {
			return errors.New("index file identity or alias mismatch")
		}
		if owner, ok := owners[src.File]; ok && owner != src.Path {
			return errors.New("deduplicated file identity")
		}
		owners[src.File] = src.Path
		p := pairs[src.Path]
		if p.file != "" && (p.file != src.File || p.run != src.Run) {
			return errors.New("file-section identity mismatch")
		}
		p.file, p.run = src.File, src.Run
		switch src.Locator {
		case "file":
			if p.header || src.Body != nil {
				return errors.New("invalid file metadata record")
			}
			p.header = true
		case "s0":
			if p.section || src.StartLine != 1 || src.EndLine != 1 || src.Body == nil ||
				sha256.Sum256([]byte(*src.Body)) != want {
				return errors.New("section content mismatch")
			}
			p.section = true
		default:
			return errors.New("unexpected file section")
		}
		pairs[src.Path] = p
	}
	for path := range expected {
		p := pairs[path]
		if !p.header || !p.section {
			return errors.New("missing file or section record")
		}
	}
	return ctx.Err()
}
