package core

import (
	"errors"
	"strings"
	"testing"
)

func TestContentInstancesAreIndependentAndIdempotent(t *testing.T) {
	m := NewContentManager()
	calls := 0
	if err := m.Register(ContentProvider{ID: "chat", Title: "Chat", Open: func() (any, error) { return new(int), nil }, NewInstance: func(id string) (any, error) {
		calls++
		return new(int), nil
	}}); err != nil {
		t.Fatal(err)
	}
	a, err := m.OpenInstance("chat", "chat-1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.OpenInstance("chat", "chat-2")
	if err != nil {
		t.Fatal(err)
	}
	*a.Value.(*int) = 42
	if a.ID != "chat-1" || a.ProviderID != "chat" || *b.Value.(*int) != 0 || a == b || calls != 2 {
		t.Fatal("instances share identity or state")
	}
	again, err := m.OpenInstance("chat", "chat-1")
	if err != nil || again != a || calls != 2 {
		t.Fatal("reopening created another instance")
	}
	legacy, err := m.Open("chat")
	if err != nil || legacy.ProviderID != "chat" || legacy == a {
		t.Fatal("default session no longer independent")
	}
	view, err := m.Open("chat-1")
	if err != nil || view != a {
		t.Fatal("tab could not resolve existing session")
	}
	if _, err := m.Open("chat-unknown"); err == nil || calls != 2 {
		t.Fatal("unknown session silently created")
	}
}

func TestContentInstancesRejectConflictsAndFailedFactories(t *testing.T) {
	m := NewContentManager()
	calls := 0
	factory := func(id string) (any, error) {
		calls++
		if id == "failed" {
			return nil, errors.New("factory failure")
		}
		return id, nil
	}
	for _, p := range []ContentProvider{
		{ID: "chat", Title: "Chat", Open: func() (any, error) { return nil, nil }, NewInstance: factory},
		{ID: "browser", Title: "Browser", Open: func() (any, error) { return nil, nil }, NewInstance: factory},
		{ID: "notes", Title: "Notes", Open: func() (any, error) { return nil, nil }},
	} {
		if err := m.Register(p); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"", "../bad", "x/y", "x y", strings.Repeat("a", 129), "notes"} {
		if _, err := m.OpenInstance("chat", id); err == nil {
			t.Fatalf("accepted invalid or reserved ID %q", id)
		}
	}
	if _, err := m.OpenInstance("notes", "note-1"); err == nil || calls != 0 {
		t.Fatal("singleton provider created an instance")
	}
	if _, err := m.OpenInstance("unknown", "new"); err == nil {
		t.Fatal("unknown provider accepted")
	}
	if _, err := m.OpenInstance("chat", "chat-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenInstance("browser", "chat-1"); err == nil || calls != 1 {
		t.Fatal("cross-provider ID reuse accepted")
	}
	if err := m.Register(ContentProvider{ID: "chat-1", Title: "Conflict", Open: func() (any, error) { return nil, nil }}); err == nil {
		t.Fatal("provider shadowed an existing instance")
	}
	if _, err := m.OpenInstance("chat", "failed"); err == nil || m.Session("failed") != nil || calls != 2 {
		t.Fatal("failed factory published a session")
	}
}
