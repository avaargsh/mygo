package core

import "testing"

func TestCloseViewPreservesSession(t *testing.T) {
	var pane Pane
	if err := pane.Open(Tab{ID: "one", SessionID: "shared"}); err != nil {
		t.Fatal(err)
	}
	if err := pane.Open(Tab{ID: "two", SessionID: "shared"}); err != nil {
		t.Fatal(err)
	}
	if !pane.Close("two") || pane.ActiveID() != "one" {
		t.Fatal("closing active tab must select remaining view")
	}
	if tabs := pane.Tabs(); len(tabs) != 1 || tabs[0].SessionID != "shared" {
		t.Fatalf("remaining views: %+v", tabs)
	}
}

func TestInvalidTabDoesNotChangePane(t *testing.T) {
	var pane Pane
	if err := pane.Open(Tab{ID: "one", SessionID: "s"}); err != nil {
		t.Fatal(err)
	}
	for _, tab := range []Tab{{ID: "one", SessionID: "other"}, {ID: "two"}, {SessionID: "s"}} {
		if pane.Open(tab) == nil {
			t.Fatalf("accepted invalid tab: %+v", tab)
		}
	}
	if pane.Activate("missing") || pane.Close("missing") || pane.ActiveID() != "one" || len(pane.Tabs()) != 1 {
		t.Fatal("invalid operation changed pane")
	}
}

func TestCloseInactiveAndLast(t *testing.T) {
	var pane Pane
	for _, id := range []string{"a", "b", "c"} {
		if err := pane.Open(Tab{ID: id, SessionID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if !pane.Activate("b") || !pane.Close("a") || pane.ActiveID() != "b" {
		t.Fatal("closing inactive tab changed selection")
	}
	if !pane.Close("b") || pane.ActiveID() != "c" {
		t.Fatal("next tab not selected")
	}
	if !pane.Close("c") || pane.ActiveID() != "" || len(pane.Tabs()) != 0 {
		t.Fatal("empty pane is invalid")
	}
}

func TestTabsReturnsCopy(t *testing.T) {
	var pane Pane
	if err := pane.Open(Tab{ID: "a", SessionID: "s"}); err != nil {
		t.Fatal(err)
	}
	pane.Tabs()[0].ID = "modified"
	if pane.Tabs()[0].ID != "a" {
		t.Fatal("caller mutated pane")
	}
}
