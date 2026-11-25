package core

import (
	"reflect"
	"testing"
)

func TestCommandRegistrySearchIsOrderedAndSideEffectFree(t *testing.T) {
	r := NewCommandRegistry()
	calls := 0
	for _, c := range []Command{
		{ID: "workspace.save", Title: "Save workspace", Run: func() { calls++ }},
		{ID: "content.notes", Title: "Open Notes", Keywords: "markdown document", Run: func() { calls++ }},
		{ID: "content.browser", Title: "Open Browser", Keywords: "web", Run: func() { calls++ }},
	} {
		if err := r.Register(c); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query string
		ids   []string
	}{
		{"", []string{"workspace.save", "content.notes", "content.browser"}},
		{"  OPEN  ", []string{"content.notes", "content.browser"}},
		{"markdown notes", []string{"content.notes"}},
		{"missing", nil},
	} {
		var ids []string
		for _, result := range r.Search(tc.query) {
			ids = append(ids, result.ID)
		}
		if !reflect.DeepEqual(ids, tc.ids) {
			t.Fatalf("search %q = %v, want %v", tc.query, ids, tc.ids)
		}
	}
	if calls != 0 {
		t.Fatal("search executed commands")
	}
}

func TestCommandRegistryRejectsInvalidAndDuplicateRegistration(t *testing.T) {
	r := NewCommandRegistry()
	for _, c := range []Command{{}, {ID: "missing.title", Run: func() {}}, {ID: "missing.run", Title: "Missing run"}, {ID: " invalid ", Title: "Invalid ID", Run: func() {}}} {
		if err := r.Register(c); err == nil {
			t.Fatalf("accepted invalid command: %+v", c)
		}
	}
	calls := 0
	if err := r.Register(Command{ID: "save", Title: "Save", Run: func() { calls++ }}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(Command{ID: "save", Title: "Replacement", Run: func() { calls += 10 }}); err == nil {
		t.Fatal("duplicate registration accepted")
	}
	if err := r.Execute("save"); err != nil || calls != 1 {
		t.Fatalf("original command replaced: %d, %v", calls, err)
	}
}

func TestCommandRegistryRechecksAvailabilityAtExecution(t *testing.T) {
	r := NewCommandRegistry()
	allowed, calls := true, 0
	if err := r.Register(Command{ID: "move", Title: "Move tab", Enabled: func() bool { return allowed }, Run: func() { calls++ }}); err != nil {
		t.Fatal(err)
	}
	if got := r.Search("move"); len(got) != 1 || !got[0].Enabled {
		t.Fatal("available command not listed")
	}
	allowed = false
	if err := r.Execute("move"); err == nil || calls != 0 {
		t.Fatal("stale availability allowed command")
	}
	if r.Available("move") || r.Available("unknown") || r.Execute("unknown") == nil {
		t.Fatal("unavailable command was accepted")
	}
	allowed = true
	if err := r.Execute("move"); err != nil || calls != 1 {
		t.Fatalf("available command did not execute: %v", err)
	}
}
