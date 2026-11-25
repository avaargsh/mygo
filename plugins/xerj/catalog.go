package xerj

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// DiscoverIndices lists exact index names belonging to a trusted prefix on a
// dedicated local node. It does not establish ownership, freshness or corpus
// completeness. Do not expose this operation to untrusted callers.
func DiscoverIndices(ctx context.Context, endpoint, prefix string) ([]string, error) {
	if ctx == nil || !autoindexPrefix.MatchString(prefix) {
		return nil, errors.New("invalid catalog scope")
	}
	c, err := New(endpoint, "catalog-probe")
	if err != nil {
		return nil, err
	}
	defer c.http.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(c.endpoint, "/catalog-probe/_search")+"/_cat/indices?format=json&h=index", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog HTTP %d", resp.StatusCode)
	}
	const maxBytes = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxBytes {
		return nil, errors.New("catalog exceeds budget")
	}
	var rows []struct {
		Index string `json:"index"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, errors.New("missing catalog rows")
	}
	seen := make(map[string]bool)
	out := make([]string, 0)
	for _, row := range rows {
		name := row.Index
		if name == "" || seen[name] {
			return nil, errors.New("invalid or duplicate catalog index")
		}
		seen[name] = true
		if !strings.HasPrefix(name, prefix+"-") {
			continue
		}
		if !indexName.MatchString(name) || name == prefix+"-" {
			return nil, errors.New("invalid scoped index")
		}
		out = append(out, name)
	}
	sort.Strings(out)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// MatchIndexSet checks a nonempty, independently supplied set of expected exact
// index names. Discovery alone must never authorize an unexpected dataset.
func MatchIndexSet(discovered, expected []string) error {
	if len(expected) == 0 || len(discovered) != len(expected) {
		return errors.New("unexpected dataset count")
	}
	remaining := make(map[string]bool, len(expected))
	for _, name := range expected {
		if !indexName.MatchString(name) || remaining[name] {
			return errors.New("invalid expected index set")
		}
		remaining[name] = true
	}
	for _, name := range discovered {
		if !remaining[name] {
			return errors.New("unexpected or duplicate dataset")
		}
		delete(remaining, name)
	}
	return nil
}
