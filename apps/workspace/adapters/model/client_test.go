package model

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompleteSendsExplicitTextRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Model    string
			Messages []Message
			Stream   bool
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "local-model" || body.Stream || len(body.Messages) != 1 || body.Messages[0].Content != "hello" {
			t.Errorf("unexpected body: %+v", body)
		}
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello back"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	c, err := NewClient(server.URL+"/v1/chat/completions", "local-model")
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Complete(context.Background(), "test-key", []Message{{Role: "user", Content: "hello"}})
	if err != nil || got.Text != "hello back" || got.FinishReason != "stop" {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

func TestClientRejectsUnsafeConfig(t *testing.T) {
	for _, endpoint := range []string{"http://example.org/v1/chat/completions", "file:///tmp/model", "https://user:pass@example.org/v1/chat/completions", "https://example.org/v1/chat/completions?token=secret", "https://example.org/"} {
		if _, err := NewClient(endpoint, "model"); err == nil {
			t.Errorf("accepted invalid endpoint")
		}
	}
	if _, err := NewClient("https://example.org/v1/chat/completions", " "); err == nil {
		t.Fatal("empty model accepted")
	}
}

func TestClientRejectsRedirectAndRedactsRemoteErrors(t *testing.T) {
	calls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(200) }))
	defer target.Close()
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusUnauthorized} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", target.URL)
			w.WriteHeader(status)
			w.Write([]byte("private-token"))
		}))
		c, err := NewClient(server.URL+"/v1/chat/completions", "model")
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Complete(context.Background(), "private-token", []Message{{Role: "user", Content: "hello"}})
		server.Close()
		if err == nil || strings.Contains(err.Error(), "private-token") {
			t.Fatalf("unsafe error: %v", err)
		}
	}
	if calls != 0 {
		t.Fatal("redirect followed")
	}
}

func TestClientRejectsInvalidResponsesAndInput(t *testing.T) {
	for _, response := range []string{`{"choices":[]}`, `{"choices":[{"message":{"role":"assistant","content":null}}]}`, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]} trailing`, strings.Repeat("x", maxResponseBytes+1)} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(response)) }))
		c, err := NewClient(server.URL+"/v1/chat/completions", "model")
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Complete(context.Background(), "", []Message{{Role: "user", Content: "hello"}})
		server.Close()
		if err == nil {
			t.Fatal("invalid response accepted")
		}
	}
	c, err := NewClient("https://example.org/v1/chat/completions", "model")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Complete(context.Background(), "", []Message{{Role: "tool", Content: "bad"}}); err == nil {
		t.Fatal("unsupported message role")
	}
	if _, err := c.Complete(context.Background(), "key\r\nInjected: yes", []Message{{Role: "user", Content: "hello"}}); err == nil {
		t.Fatal("invalid key accepted")
	}
}

func TestClientHonorsCanceledContext(t *testing.T) {
	c, err := NewClient("https://example.org/v1/chat/completions", "model")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.Complete(ctx, "", []Message{{Role: "user", Content: "hello"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
