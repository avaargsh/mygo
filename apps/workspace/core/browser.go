package core

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// BrowserAddress accepts explicit web URLs only. It is a navigation input
// check, not a network sandbox or a restriction on subresource requests.
func BrowserAddress(input string) (string, error) {
	input = strings.TrimSpace(input)
	if strings.ContainsRune(input, '\\') || strings.ContainsFunc(input, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", errors.New("invalid whitespace or backslash in address")
	}
	u, err := url.Parse(input)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return "", errors.New("enter an explicit HTTP or HTTPS URL without credentials")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("invalid port")
		}
	}
	return u.String(), nil
}

// SessionOwner chooses one visible pane for a singleton native resource.
// Prefer the focused pane, then the first visible view in layout order.
func SessionOwner(s WorkspaceSnapshot, sessionID string) string {
	visible := func(id string) bool {
		p := s.Panes[id]
		for _, t := range p.Tabs {
			if t.ID == p.ActiveTab && t.SessionID == sessionID {
				return true
			}
		}
		return false
	}
	if visible(s.ActivePane) {
		return s.ActivePane
	}
	var ids []string
	leafIDs(s.Layout, &ids)
	for _, id := range ids {
		if visible(id) {
			return id
		}
	}
	return ""
}
