package core

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestLayoutNestedSplitAndCollapse(t *testing.T) {
	l, err := NewLayout("a")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Split("a", "s1", "b", Horizontal, .6); err != nil {
		t.Fatal(err)
	}
	if err := l.Split("b", "s2", "c", Vertical, .4); err != nil {
		t.Fatal(err)
	}
	want := LayoutNode{ID: "s1", Direction: Horizontal, Ratio: .6, Children: []LayoutNode{{ID: "a"}, {ID: "s2", Direction: Vertical, Ratio: .4, Children: []LayoutNode{{ID: "b"}, {ID: "c"}}}}}
	if !reflect.DeepEqual(l.Snapshot(), want) {
		t.Fatalf("got %+v", l.Snapshot())
	}
	if err := l.Resize("s2", .7); err != nil {
		t.Fatal(err)
	}
	if err := l.RemovePane("b"); err != nil {
		t.Fatal(err)
	}
	if got := l.Snapshot(); got.Children[1].ID != "c" || len(got.Children[1].Children) != 0 {
		t.Fatalf("collapse: %+v", got)
	}
	if err := l.RemovePane("a"); err != nil {
		t.Fatal(err)
	}
	if l.Snapshot().ID != "c" || l.RemovePane("c") == nil {
		t.Fatal("root pane must survive")
	}
}

func TestLayoutRejectsInvalidMutations(t *testing.T) {
	l, _ := NewLayout("a")
	before := l.Snapshot()
	for _, err := range []error{l.Split("missing", "s", "b", Horizontal, .5), l.Split("a", "s", "a", Horizontal, .5), l.Split("a", "s", "s", Horizontal, .5), l.Split("a", "s", "b", "diagonal", .5), l.Split("a", "s", "b", Horizontal, math.NaN()), l.Resize("a", .5), l.RemovePane("missing")} {
		if err == nil {
			t.Fatal("accepted invalid mutation")
		}
	}
	if !reflect.DeepEqual(before, l.Snapshot()) {
		t.Fatal("invalid mutation altered layout")
	}
}

func TestLayoutRestoreAndIsolation(t *testing.T) {
	l, _ := NewLayout("a")
	if err := l.Split("a", "s", "b", Horizontal, .5); err != nil {
		t.Fatal(err)
	}
	snapshot := l.Snapshot()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded LayoutNode
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreLayout(decoded)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Children[0].ID = "changed"
	decoded.Children[0].ID = "also changed"
	if !reflect.DeepEqual(restored.Snapshot(), l.Snapshot()) {
		t.Fatal("snapshot aliases live state")
	}
}

func TestLayoutRejectsMalformedSnapshots(t *testing.T) {
	for _, n := range []LayoutNode{{}, {ID: "a", Ratio: .5}, {ID: "s", Direction: Horizontal, Ratio: .5, Children: []LayoutNode{{ID: "a"}}}, {ID: "s", Direction: Horizontal, Ratio: .5, Children: []LayoutNode{{ID: "a"}, {ID: "a"}}}, {ID: "s", Direction: Vertical, Ratio: 1, Children: []LayoutNode{{ID: "a"}, {ID: "b"}}}} {
		if _, err := RestoreLayout(n); err == nil {
			t.Fatalf("accepted %+v", n)
		}
	}
	cyclic := LayoutNode{ID: "s", Direction: Horizontal, Ratio: .5, Children: make([]LayoutNode, 2)}
	cyclic.Children[0] = cyclic
	cyclic.Children[1] = LayoutNode{ID: "b"}
	if _, err := RestoreLayout(cyclic); err == nil {
		t.Fatal("accepted cyclic snapshot")
	}
}
