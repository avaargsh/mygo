package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/xerj"
)

func xerjFixtureServer(body string) (*httptest.Server, *atomic.Int32, *atomic.Int32) {
	var verifyCalls, searchCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/fixture-text/_search" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var query map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			http.Error(w, "invalid query", http.StatusBadRequest)
			return
		}
		if _, verify := query["track_total_hits"]; verify {
			verifyCalls.Add(1)
			fmt.Fprintf(w, `{"hits":{"total":{"value":2,"relation":"eq"},"hits":[{"_index":"fixture-text","_source":{"ax_path":"mygo-xerj-fixture.txt","ax_paths":["mygo-xerj-fixture.txt"],"ax_file":"fixture-id","ax_run":"fixture-run","ax_format":"txt-prose","ax_locator":"file"}},{"_index":"fixture-text","_source":{"ax_path":"mygo-xerj-fixture.txt","ax_paths":["mygo-xerj-fixture.txt"],"ax_file":"fixture-id","ax_run":"fixture-run","ax_format":"txt-prose","ax_locator":"s0","start_line":1,"end_line":1,"body":%q}}]}}`, body)
			return
		}
		searchCalls.Add(1)
		fmt.Fprint(w, `{"hits":{"hits":[{"_index":"fixture-text","_source":{"ax_path":"mygo-xerj-fixture.txt","start_line":1},"fields":{"_passage":{"text":"mygoxerjfixturemarker","start_offset":0}}}]}}`)
	}))
	return server, &verifyCalls, &searchCalls
}

func TestConnectSearchFixtureVerifiesBeforeSearch(t *testing.T) {
	server, verifyCalls, searchCalls := xerjFixtureServer("mygoxerjfixturemarker")
	defer server.Close()
	runner, err := connectSearchFixture(context.Background(), server.URL, "fixture-text")
	if err != nil {
		t.Fatal(err)
	}
	controller, ok := runner.(*xerj.SearchController)
	if !ok {
		t.Fatalf("unexpected runner type %T", runner)
	}
	if verifyCalls.Load() != 1 || searchCalls.Load() != 0 {
		t.Fatalf("connection must verify before query: verify=%d search=%d", verifyCalls.Load(), searchCalls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	notify := make(chan struct{}, 1)
	go func() {
		done <- controller.Serve(ctx, func() {
			select {
			case notify <- struct{}{}:
			default:
			}
		})
	}()
	if !controller.Submit("mygoxerjfixturemarker", 5) {
		t.Fatal("verified controller rejected query")
	}
	select {
	case <-notify:
	case <-time.After(3 * time.Second):
		t.Fatal("search result notification timed out")
	}
	got := controller.Snapshot()
	if got.State != xerj.SearchMatches || len(got.Hits) != 1 || got.Hits[0].Path != "mygo-xerj-fixture.txt" || got.Hits[0].Line != 1 {
		t.Fatalf("unexpected verified result: %+v", got)
	}
	if searchCalls.Load() != 1 {
		t.Fatalf("expected one search request, got %d", searchCalls.Load())
	}
	controller.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("search worker did not stop")
	}
}

func TestConnectSearchFixtureRejectsChangedBytes(t *testing.T) {
	server, verifyCalls, searchCalls := xerjFixtureServer("mygoxerjfixturemarker\n")
	defer server.Close()
	runner, err := connectSearchFixture(context.Background(), server.URL, "fixture-text")
	if err == nil {
		if runner != nil {
			runner.Close()
		}
		t.Fatal("changed fixture bytes were accepted")
	}
	if verifyCalls.Load() != 1 || searchCalls.Load() != 0 {
		t.Fatalf("rejected corpus must not become searchable: verify=%d search=%d", verifyCalls.Load(), searchCalls.Load())
	}
}

func TestConnectSearchFixtureRejectsNonLoopbackEndpoint(t *testing.T) {
	if runner, err := connectSearchFixture(context.Background(), "http://example.com:9200", "fixture-text"); err == nil {
		if runner != nil {
			runner.Close()
		}
		t.Fatal("non-loopback endpoint was accepted")
	}
}
