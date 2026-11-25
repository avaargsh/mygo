package main

import (
	"fmt"
	"strings"
	"unicode"
)

func searchStatusText(state string, count int) string {
	switch state {
	case "IDLE":
		return "Enter a query to search the fixture."
	case "LOADING":
		return "Searching…"
	case "DISABLED":
		return "Search is disabled."
	case "INVALID_REQUEST":
		return "Enter a non-empty query of at most 4096 bytes."
	case "NOT_READY":
		return "Index is not ready."
	case "STALE":
		return "Index changed. Search again."
	case "CANCELED":
		return "Search canceled."
	case "TIMEOUT":
		return "Search timed out."
	case "EMPTY":
		return "No matching results."
	case "MATCHES":
		if count == 1 {
			return "1 match"
		}
		if count > 1 {
			return fmt.Sprintf("%d matches", count)
		}
	}
	return "Search service is unavailable."
}

// sourceLabel is a bounded display label, never a filesystem path to open.
func sourceLabel(path string, line int) string {
	path = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return '�'
		}
		return r
	}, path)
	if strings.TrimSpace(path) == "" {
		path = "Unknown source"
	}
	if rs := []rune(path); len(rs) > 256 {
		path = string(rs[:256]) + "…"
	}
	if line > 0 {
		return fmt.Sprintf("%s:%d", path, line)
	}
	return path
}
