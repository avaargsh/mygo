// Package model provides an explicitly invoked text-only model transport.
package model

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
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxResponseBytes = 2 << 20
const maxRequestBytes = 1 << 20

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Completion struct {
	Text         string
	FinishReason string
}

// Client has a fixed user-configured endpoint and model. Credentials are
// supplied per call, not persisted. No automatic requests or retries occur.
type Client struct {
	endpoint string
	model    string
	http     *http.Client
}

func NewClient(endpoint, model string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || !strings.HasSuffix(u.Path, "/chat/completions") || strings.ContainsAny(endpoint, "\\\r\n\t ") {
		return nil, errors.New("invalid chat completions endpoint")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return nil, errors.New("HTTPS required except literal loopback HTTP")
	}
	if strings.TrimSpace(model) == "" || len(model) > 256 || strings.ContainsFunc(model, unicode.IsControl) || !utf8.ValidString(model) {
		return nil, errors.New("invalid model name")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{endpoint: u.String(), model: model, http: &http.Client{Transport: transport, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Close releases idle HTTP connections. Active calls belong to their context.
func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) Complete(ctx context.Context, apiKey string, messages []Message) (Completion, error) {
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	if strings.ContainsFunc(apiKey, func(r rune) bool { return r < 33 || r > 126 }) || len(apiKey) > 8192 {
		return Completion{}, errors.New("invalid model credential")
	}
	if len(messages) == 0 || len(messages) > 256 {
		return Completion{}, errors.New("invalid message count")
	}
	total := 0
	for _, m := range messages {
		if m.Role != "user" && m.Role != "assistant" && m.Role != "system" {
			return Completion{}, errors.New("unsupported message role")
		}
		if !utf8.ValidString(m.Content) || len(m.Content) > maxRequestBytes-total {
			return Completion{}, errors.New("invalid or oversized message content")
		}
		total += len(m.Content)
	}
	body, err := json.Marshal(struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Stream   bool      `json:"stream"`
	}{c.model, messages, false})
	if err != nil || len(body) > maxRequestBytes {
		return Completion{}, errors.New("invalid or oversized model request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Completion{}, errors.New("could not create model request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Completion{}, ctx.Err()
		}
		return Completion{}, errors.New("model request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Completion{}, fmt.Errorf("model HTTP status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return Completion{}, ctx.Err()
		}
		return Completion{}, errors.New("could not read model response")
	}
	if len(data) > maxResponseBytes || !utf8.Valid(data) {
		return Completion{}, errors.New("invalid or oversized model response")
	}
	var result struct {
		Choices []struct {
			Message struct {
				Role      string
				Content   *string
				ToolCalls []json.RawMessage `json:"tool_calls"`
			}
			FinishReason string `json:"finish_reason"`
		}
	}
	if json.Unmarshal(data, &result) != nil || len(result.Choices) != 1 {
		return Completion{}, errors.New("invalid model response")
	}
	choice := result.Choices[0]
	if choice.Message.Role != "assistant" || choice.Message.Content == nil || len(choice.Message.ToolCalls) > 0 || choice.FinishReason == "tool_calls" || choice.FinishReason == "function_call" {
		return Completion{}, errors.New("unsupported model response")
	}
	return Completion{Text: *choice.Message.Content, FinishReason: choice.FinishReason}, nil
}
