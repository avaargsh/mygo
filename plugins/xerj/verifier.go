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
// exact UTF-8 source bytes. One record per path is required. This deliberately
// rejects deduplicated, split, transformed and multi-dataset corpora.
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
	body, err := json.Marshal(map[string]any{"size": len(expected), "track_total_hits": true, "query": map[string]any{"match_all": map[string]any{}}, "_source": []string{"ax_path", "body"}})
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
					Path string  `json:"ax_path"`
					Body *string `json:"body"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if result.TimedOut || result.Shards.Failed != 0 || result.Hits.Total.Relation != "eq" || result.Hits.Total.Value != len(expected) || len(result.Hits.Hits) != len(expected) {
		return errors.New("incomplete or unexpected index cardinality")
	}
	for _, h := range result.Hits.Hits {
		want, ok := expected[h.Source.Path]
		if h.Index != target.Index || !ok || h.Source.Body == nil || sha256.Sum256([]byte(*h.Source.Body)) != want {
			return errors.New("index source identity or content mismatch")
		}
		delete(expected, h.Source.Path)
	}
	if len(expected) != 0 {
		return errors.New("missing expected source")
	}
	return ctx.Err()
}
