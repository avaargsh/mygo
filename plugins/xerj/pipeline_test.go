package xerj

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTextPipeline(t *testing.T) {
	for _, mode := range []string{"ok", "occupied", "extra", "wrong-body", "changed-catalog", "build-error"} {
		t.Run(mode, func(t *testing.T) {
			var built atomic.Bool
			var catalogs, checked atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/_cat/indices" {
					count := catalogs.Add(1)
					if !built.Load() && mode != "occupied" {
						fmt.Fprint(w, `[]`)
						return
					}
					if mode == "extra" || (mode == "changed-catalog" && count >= 3) {
						fmt.Fprint(w, `[{"index":"demo-code"},{"index":"demo-notes"},{"index":"demo-extra"}]`)
						return
					}
					fmt.Fprint(w, `[{"index":"demo-code"},{"index":"demo-notes"}]`)
					return
				}
				index := strings.Split(r.URL.Path, "/")[1]
				checked.Add(1)
				body := "needle"
				if mode == "wrong-body" && index == "demo-notes" {
					body = "wrong"
				}
				fmt.Fprintf(w, `{"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_index":%q,"_source":{"ax_path":"a.txt","body":%q}}]}}`, index, body)
			}))
			defer s.Close()
			m := NewManager(s.URL)
			expected := map[string]TextIndexVerifier{"demo-code": {Expected: map[string]string{"a.txt": fmt.Sprintf("%x", sha256.Sum256([]byte("needle")))}}, "demo-notes": {Expected: map[string]string{"a.txt": fmt.Sprintf("%x", sha256.Sum256([]byte("needle")))}}}
			err := m.buildTextSet(context.Background(), "a", "demo", expected, func(_ context.Context, target BuildTarget) error {
				if target.Index != "demo-code" || m.Status("a").State != IndexBuilding {
					t.Errorf("target %+v", target)
				}
				built.Store(true)
				if mode == "build-error" {
					return fmt.Errorf("fixture build failed")
				}
				return nil
			})
			switch mode {
			case "ok":
				if err != nil || m.Status("a").State != IndexReady || checked.Load() != 2 || catalogs.Load() != 3 {
					t.Fatalf("%v %v checked=%d catalogs=%d", err, m.Status("a"), checked.Load(), catalogs.Load())
				}
			case "occupied":
				if err == nil || built.Load() || m.Status("a").State != IndexUnbound {
					t.Fatalf("occupied: %v %v built=%t", err, m.Status("a"), built.Load())
				}
			default:
				if err == nil || m.Status("a").State != IndexFailed {
					t.Fatalf("%v %v", err, m.Status("a"))
				}
			}
		})
	}
}

func TestPipelineRejectsHistoricalPrefix(t *testing.T) {
	m := NewManager("http://127.0.0.1:1")
	if _, err := m.Begin("a", "demo-old"); err != nil {
		t.Fatal(err)
	}
	err := m.buildTextSet(context.Background(), "b", "demo", map[string]TextIndexVerifier{"demo-new": {Expected: map[string]string{"a.txt": fmt.Sprintf("%064x", 0)}}}, func(context.Context, BuildTarget) error { t.Fatal("unexpected build"); return nil })
	if err == nil || !strings.Contains(err.Error(), "prefix already reserved") {
		t.Fatalf("%v", err)
	}
}

func TestPipelineConcurrentPrefixReservation(t *testing.T) {
	m := NewManager("http://127.0.0.1:9200")
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, index := range []string{"demo-code", "demo-notes"} {
		go func(index string) {
			<-start
			_, err := m.beginSet(index, []string{index}, "demo")
			results <- err
		}(index)
	}
	close(start)
	a, b := <-results, <-results
	if (a == nil) == (b == nil) {
		t.Fatalf("expected exactly one reservation: %v, %v", a, b)
	}
}
