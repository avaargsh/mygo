package xerj

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyTextIndex(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		ok             bool
	}{
		{"exact", `{"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_index":"demo-code","_source":{"ax_path":"a.go","body":"package a\n"}}]}}`, true},
		{"wrong body", `{"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_index":"demo-code","_source":{"ax_path":"a.go","body":"other"}}]}}`, false},
		{"wrong path", `{"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_index":"demo-code","_source":{"ax_path":"b.go","body":"package a\n"}}]}}`, false},
		{"wrong index", `{"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_index":"other","_source":{"ax_path":"a.go","body":"package a\n"}}]}}`, false},
		{"partial", `{"timed_out":true,"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_index":"demo-code","_source":{"ax_path":"a.go","body":"package a\n"}}]}}`, false},
		{"lower bound", `{"hits":{"total":{"value":1,"relation":"gte"},"hits":[{"_index":"demo-code","_source":{"ax_path":"a.go","body":"package a\n"}}]}}`, false},
		{"extra", `{"hits":{"total":{"value":2,"relation":"eq"},"hits":[{"_index":"demo-code","_source":{"ax_path":"a.go","body":"package a\n"}}]}}`, false},
		{"shard failure", `{"_shards":{"failed":1},"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_index":"demo-code","_source":{"ax_path":"a.go","body":"package a\n"}}]}}`, false},
		{"missing body", `{"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_index":"demo-code","_source":{"ax_path":"a.go"}}]}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/demo-code/_search" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(400)
					return
				}
				fmt.Fprint(w, tc.response)
			}))
			defer s.Close()
			hash := sha256.Sum256([]byte("package a\n"))
			v := TextIndexVerifier{Expected: map[string]string{"a.go": hex.EncodeToString(hash[:])}}
			err := v.Verify(context.Background(), BuildTarget{Workspace: "a", Index: "demo-code", Endpoint: s.URL, Generation: 1})
			if (err == nil) != tc.ok {
				t.Fatalf("ok=%v err=%v", tc.ok, err)
			}
		})
	}
}

func TestVerifierRejectsInvalidManifest(t *testing.T) {
	for _, expected := range []map[string]string{nil, {"../a.go": "bad"}, {"a.go": "bad"}, {"/a.go": fmt.Sprintf("%064x", 0)}} {
		v := TextIndexVerifier{Expected: expected}
		if err := v.Verify(context.Background(), BuildTarget{Workspace: "a", Index: "demo-code", Endpoint: "http://127.0.0.1:1", Generation: 1}); err == nil {
			t.Fatal("accepted invalid manifest")
		}
	}
}
