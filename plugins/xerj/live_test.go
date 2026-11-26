package xerj

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run only against an explicitly prepared disposable corpus and local node.
func TestLiveSearch(t *testing.T) {
	if os.Getenv("MYGO_XERJ_LIVE") != "1" {
		t.Skip("requires an explicitly prepared XERJ node and fixture")
	}
	endpoint, index := os.Getenv("MYGO_XERJ_ENDPOINT"), os.Getenv("MYGO_XERJ_INDEX")
	if endpoint == "" || index == "" {
		t.Fatal("MYGO_XERJ_ENDPOINT and MYGO_XERJ_INDEX are required")
	}
	c, err := New(endpoint, index)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := c.Search(context.Background(), "mygoxerjfixturemarker", 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if filepath.Base(h.Path) == "mygo-xerj-fixture.txt" && strings.Contains(h.Text, "mygoxerjfixturemarker") {
			return
		}
	}
	t.Fatal("expected fixture passage not found")
}
