package xerj

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestInstalledFlowFixture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	script := []byte("#!/bin/sh\n[ -f \"$2/mygo-xerj-fixture.txt\" ] && [ ! -e \"$2/unapproved.txt\" ] || exit 9\nexit 0\n")
	binary := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(binary, script, 0700); err != nil {
		t.Fatal(err)
	}
	var catalogs atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/_cat/indices":
			n := catalogs.Add(1)
			rows := []map[string]string{}
			if n >= 2 {
				rows = append(rows, map[string]string{"index": "fixturea-text"})
			}
			if n >= 5 {
				rows = append(rows, map[string]string{"index": "fixtureb-text"})
			}
			_ = json.NewEncoder(w).Encode(rows)
		case "/fixturea-text/_search", "/fixtureb-text/_search":
			index := strings.Split(r.URL.Path, "/")[1]
			marker := installedMarkerA
			if index == "fixtureb-text" {
				marker = installedMarkerB
			}
			var req map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(400)
				return
			}
			body := marker + "\n"
			if _, ok := req["track_total_hits"]; ok {
				fmt.Fprintf(w, `{"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_index":%q,"_source":{"ax_path":"mygo-xerj-fixture.txt","body":%q}}]}}`, index, body)
			} else {
				fmt.Fprintf(w, `{"hits":{"hits":[{"_index":%q,"_source":{"ax_path":"mygo-xerj-fixture.txt"},"fields":{"_passage":{"text":%q,"start_offset":0}}}]}}`, index, body)
			}
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	exerciseInstalledFlow(t, server.URL, binary, fmt.Sprintf("%x", sha256.Sum256(script)), "text", "fixture")
	if catalogs.Load() != 6 {
		t.Fatalf("expected six catalog requests, got %d", catalogs.Load())
	}
}
