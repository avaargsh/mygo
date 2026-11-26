package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/plugins/xerj"
)

func TestFixtureServicePreflight(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("desktop fixture only")
	}
	var calls atomic.Int32
	var mode atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/fixture-text/_search" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var req map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, ok := req["track_total_hits"]; ok {
			body := "mygoxerjfixturemarker"
			switch mode.Load() {
			case 1:
				body = "wrong fixture"
			case 2:
				body += "\n"
			case 3:
				fmt.Fprintf(w, `{"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_index":"fixture-text","_source":{"ax_path":"mygo-xerj-fixture.txt","body":%q}}]}}`, body)
				return
			}
			fmt.Fprintf(w, `{"hits":{"total":{"value":2,"relation":"eq"},"hits":[{"_index":"fixture-text","_source":{"ax_path":"mygo-xerj-fixture.txt","ax_paths":["mygo-xerj-fixture.txt"],"ax_file":"fixture-id","ax_run":"fixture-run","ax_format":"txt-prose","ax_locator":"file"}},{"_index":"fixture-text","_source":{"ax_path":"mygo-xerj-fixture.txt","ax_paths":["mygo-xerj-fixture.txt"],"ax_file":"fixture-id","ax_run":"fixture-run","ax_format":"txt-prose","ax_locator":"s0","start_line":1,"end_line":1,"body":%q}}]}}`, body)
			return
		}
		fmt.Fprint(w, `{"hits":{"hits":[{"_index":"fixture-text","_source":{"ax_path":"mygo-xerj-fixture.txt"},"fields":{"_passage":{"text":"mygoxerjfixturemarker","start_offset":0}}}]}}`)
	}))
	defer server.Close()
	t.Setenv("MYGO_XERJ_ENDPOINT", server.URL)
	t.Setenv("MYGO_XERJ_INDEX", "fixture-text")
	t.Run("disabled by default", func(t *testing.T) {
		t.Setenv("MYGO_XERJ_PANEL", "")
		service, _ := fixtureService()
		got := service.Search(context.Background(), "mygoxerjfixturemarker", 5)
		if got.State != xerj.SearchDisabled || len(got.Hits) != 0 || calls.Load() != 0 {
			t.Fatalf("unexpected result or request: %+v calls=%d", got, calls.Load())
		}
	})
	t.Run("invalid index does not contact engine", func(t *testing.T) {
		t.Setenv("MYGO_XERJ_PANEL", "1")
		t.Setenv("MYGO_XERJ_INDEX", "*")
		service, _ := fixtureService()
		if got := service.Search(context.Background(), "needle", 5); got.State != xerj.SearchDisabled || calls.Load() != 0 {
			t.Fatalf("unexpected result or request: %+v calls=%d", got, calls.Load())
		}
	})
	t.Run("digest mismatch fails closed", func(t *testing.T) {
		t.Setenv("MYGO_XERJ_PANEL", "1")
		mode.Store(1)
		before := calls.Load()
		service, _ := fixtureService()
		got := service.Search(context.Background(), "needle", 5)
		if got.State != xerj.SearchDisabled || len(got.Hits) != 0 || calls.Load() != before+1 {
			t.Fatalf("unexpected result or request: %+v calls=%d", got, calls.Load())
		}
	})
	t.Run("trailing newline fails exact byte check", func(t *testing.T) {
		t.Setenv("MYGO_XERJ_PANEL", "1")
		mode.Store(2)
		before := calls.Load()
		service, _ := fixtureService()
		got := service.Search(context.Background(), "needle", 5)
		if got.State != xerj.SearchDisabled || len(got.Hits) != 0 || calls.Load() != before+1 {
			t.Fatalf("unexpected result or request: %+v calls=%d", got, calls.Load())
		}
	})
	t.Run("verified legacy record permits search", func(t *testing.T) {
		t.Setenv("MYGO_XERJ_PANEL", "1")
		mode.Store(3)
		before := calls.Load()
		service, _ := fixtureService()
		got := service.Search(context.Background(), "mygoxerjfixturemarker", 5)
		if got.State != xerj.SearchMatches || len(got.Hits) != 1 || calls.Load() != before+2 {
			t.Fatalf("unexpected result or request: %+v calls=%d", got, calls.Load())
		}
	})
	t.Run("verified rc.103 file and section permit search", func(t *testing.T) {
		t.Setenv("MYGO_XERJ_PANEL", "1")
		mode.Store(0)
		before := calls.Load()
		service, _ := fixtureService()
		got := service.Search(context.Background(), "mygoxerjfixturemarker", 5)
		if got.State != xerj.SearchMatches || len(got.Hits) != 1 || got.Hits[0].Path != "mygo-xerj-fixture.txt" || calls.Load() != before+2 {
			t.Fatalf("unexpected result or request: %+v calls=%d", got, calls.Load())
		}
	})
}
