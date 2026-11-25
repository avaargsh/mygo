package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const streamText = `data: {"choices":[{"index":0,"delta":{"role":"assistant","content":"hello "}}]}` + "\r\n\r\n" + `data: {"choices":[{"index":0,"delta":{"content":"world"}}]}` + "\n\n" + `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n" + "data: [DONE]\n\n"

func TestStreamDeliversTextAndFinish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Stream   bool
			Messages []Message
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !body.Stream || len(body.Messages) != 1 || r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected stream request: %+v, %v", body, err)
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		fmt.Fprint(w, ": heartbeat\n\n"+streamText)
	}))
	defer server.Close()
	c, err := NewClient(server.URL+"/v1/chat/completions", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var chunks []string
	got, err := c.Stream(context.Background(), "test-key", []Message{{Role: "user", Content: "hello"}}, func(s string) error { chunks = append(chunks, s); return nil })
	if err != nil || got.Text != "hello world" || got.FinishReason != "stop" || len(chunks) != 2 {
		t.Fatalf("got %+v, chunks %q, err %v", got, chunks, err)
	}
}

func TestStreamRejectsIncompleteAndUnsupportedEvents(t *testing.T) {
	cases := []string{
		strings.TrimSuffix(streamText, "data: [DONE]\n\n"),
		"data: [DONE]\n\n",
		"data: not-json\n\n",
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{}]}}]}` + "\n\n",
		`data: {"choices":[{"index":0,"delta":{"function_call":{"name":"tool"}}}]}` + "\n\n",
		`data: {"choices":[{"index":1,"delta":{"content":"bad"}}]}` + "\n\n",
		strings.Repeat("x", maxStreamEventBytes+1),
	}
	for i, body := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			c, err := NewClient(server.URL+"/v1/chat/completions", "model")
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if _, err = c.Stream(context.Background(), "", []Message{{Role: "user", Content: "hello"}}, func(string) error { return nil }); err == nil {
				t.Fatal("invalid stream accepted")
			}
		})
	}
}

func TestStreamCancellationAndCallbackFailure(t *testing.T) {
	for _, cancelRequest := range []bool{true, false} {
		t.Run(fmt.Sprint(cancelRequest), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, streamText)
			}))
			defer server.Close()
			c, err := NewClient(server.URL+"/v1/chat/completions", "model")
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sentinel := errors.New("stop consumer")
			calls := 0
			_, err = c.Stream(ctx, "", []Message{{Role: "user", Content: "hello"}}, func(string) error {
				calls++
				if cancelRequest {
					cancel()
					return nil
				}
				return sentinel
			})
			want := sentinel
			if cancelRequest {
				want = context.Canceled
			}
			if !errors.Is(err, want) || calls != 1 {
				t.Fatalf("err %v, calls %d", err, calls)
			}
		})
	}
}

func TestStreamRejectsWrongContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, streamText)
	}))
	defer server.Close()
	c, err := NewClient(server.URL+"/v1/chat/completions", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.Stream(context.Background(), "", []Message{{Role: "user", Content: "hello"}}, func(string) error { return nil }); err == nil {
		t.Fatal("non-SSE response accepted")
	}
}
