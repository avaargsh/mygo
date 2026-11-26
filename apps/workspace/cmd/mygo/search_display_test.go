package main

import (
	"testing"

	"github.com/egoist/mygo/plugins/xerj"
)

func TestSearchHitLocation(t *testing.T) {
	tests := []struct {
		name string
		hit  xerj.Hit
		want string
	}{
		{name: "line known", hit: xerj.Hit{Path: "src/main.go", Line: 42}, want: "src/main.go:42"},
		{name: "line unknown", hit: xerj.Hit{Path: "docs/guide.md"}, want: "docs/guide.md"},
		{name: "line not positive", hit: xerj.Hit{Path: "docs/guide.md", Line: -1}, want: "docs/guide.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchHitLocation(tt.hit); got != tt.want {
				t.Fatalf("searchHitLocation() = %q, want %q", got, tt.want)
			}
		})
	}
}
