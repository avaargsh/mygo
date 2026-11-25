// Package xerj provides opt-in, read-only local workspace search.
package xerj

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var indexName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)

// Hit is untrusted retrieved content, not an instruction or a verified citation.
type Hit struct {
	Index string `json:"index"`
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
	Text string `json:"text"`
}

type Client struct {
	endpoint string
	index    string
	http     *http.Client
}

// New connects to an existing loopback-only XERJ node and one exact index.
// Index selection must come from trusted workspace configuration, not an Agent.
func New(endpoint, index string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Port() == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return nil, errors.New("expected local HTTP origin with explicit port")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, errors.New("XERJ endpoint must be a numeric loopback address")
	}
	if !indexName.MatchString(index) {
		return nil, errors.New("expected an exact index name")
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	return &Client{index: index, endpoint: strings.TrimSuffix(u.String(), "/") + "/" + index + "/_search", http: &http.Client{Transport: tr, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Search submits a bounded lexical query. An empty hit list is not an error.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Hit, error) {
	if strings.TrimSpace(query) == "" || len(query) > 4096 || limit < 1 || limit > 20 {
		return nil, errors.New("invalid query or limit")
	}
	body, err := json.Marshal(map[string]any{"size": limit, "query": map[string]any{"match": map[string]string{"body": query}}, "_source": []string{"ax_path", "path", "start_line", "line"}, "fields": []string{"_passage"}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("XERJ search returned HTTP %d", resp.StatusCode)
	}
	const maxResponse = 2 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponse {
		return nil, errors.New("XERJ response exceeds budget")
	}
	var result struct {
		TimedOut bool `json:"timed_out"`
		Shards   struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
		Hits *struct {
			Hits *[]struct {
				Index  string `json:"_index"`
				Fields struct {
					Passage json.RawMessage `json:"_passage"`
				} `json:"fields"`
				Source struct {
					Path      string `json:"path"`
					AXPath    string `json:"ax_path"`
					StartLine int    `json:"start_line"`
					Line      int    `json:"line"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result.TimedOut || result.Shards.Failed != 0 {
		return nil, errors.New("incomplete XERJ search")
	}
	if result.Hits == nil || result.Hits.Hits == nil || len(*result.Hits.Hits) > limit {
		return nil, errors.New("invalid XERJ hits")
	}
	out := make([]Hit, 0, len(*result.Hits.Hits))
	for _, h := range *result.Hits.Hits {
		if h.Index != c.index {
			return nil, errors.New("XERJ hit index mismatch")
		}
		s := h.Source
		path := s.AXPath
		if path == "" {
			path = s.Path
		}
		line := s.StartLine
		if line == 0 {
			line = s.Line
		}
		snippet := ""
		if len(h.Fields.Passage) != 0 {
			var passages []struct {
				Text        string `json:"text"`
				StartOffset *int   `json:"start_offset"`
			}
			raw := h.Fields.Passage
			if len(raw) > 0 && raw[0] == '{' {
				raw = append(append([]byte{'['}, raw...), ']')
			}
			if err := json.Unmarshal(raw, &passages); err != nil || len(passages) == 0 || passages[0].StartOffset == nil || *passages[0].StartOffset < 0 {
				return nil, errors.New("invalid XERJ passage")
			}
			snippet = passages[0].Text
			// A character offset alone does not establish a source line.
			if *passages[0].StartOffset != 0 {
				line = 0
			}
		}
		if strings.TrimSpace(path) == "" || line < 0 || strings.TrimSpace(snippet) == "" {
			return nil, errors.New("hit lacks source or content")
		}
		r := []rune(snippet)
		if len(r) > 4096 {
			snippet = string(r[:4096])
		}
		out = append(out, Hit{Index: c.index, Path: path, Line: line, Text: snippet})
	}
	return out, nil
}
