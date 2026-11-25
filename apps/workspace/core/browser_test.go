package core

import "testing"

func TestBrowserAddressAllowsWebURLs(t *testing.T) {
	for _, input := range []string{"https://example.org/path?q=1", " http://localhost:8080/ ", "https://[::1]:8443/"} {
		if _, err := BrowserAddress(input); err != nil {
			t.Fatalf("%q: %v", input, err)
		}
	}
}

func TestBrowserAddressRejectsOtherInputs(t *testing.T) {
	for _, input := range []string{"", "example.org", "file:///tmp/a", "javascript:alert(1)", "data:text/html,hello", "https://", "https://user:pass@example.org", "https://example.org/a b", "https://example.org\\x", "https://example.org:99999"} {
		if _, err := BrowserAddress(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestSessionOwnerPrefersFocusAndFallsBack(t *testing.T) {
	w, _ := NewWorkspace("left")
	if err := w.Open("left", Tab{ID: "a", SessionID: "browser"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Split("left", "split", "right", Horizontal, .5); err != nil {
		t.Fatal(err)
	}
	if err := w.Open("right", Tab{ID: "b", SessionID: "browser"}); err != nil {
		t.Fatal(err)
	}
	if got := SessionOwner(w.Snapshot(), "browser"); got != "right" {
		t.Fatal(got)
	}
	if err := w.Open("right", Tab{ID: "c", SessionID: "notes"}); err != nil {
		t.Fatal(err)
	}
	if got := SessionOwner(w.Snapshot(), "browser"); got != "left" {
		t.Fatal(got)
	}
	if err := w.Close("left", "a"); err != nil {
		t.Fatal(err)
	}
	if got := SessionOwner(w.Snapshot(), "browser"); got != "" {
		t.Fatal(got)
	}
}
