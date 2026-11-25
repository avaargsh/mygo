package core

import (
	"fmt"
	"strings"
)

// Command is a UI-owned, explicitly invoked operation, not a shell command.
// Enabled must be side-effect free. Run handles its own operation result.
type Command struct {
	ID       string
	Title    string
	Keywords string
	Enabled  func() bool
	Run      func()
}

type CommandMatch struct {
	ID      string
	Title   string
	Enabled bool
}

// CommandRegistry is owned by the UI thread; search never runs commands.
type CommandRegistry struct {
	commands map[string]Command
	order    []string
}

func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{commands: make(map[string]Command)}
}

func (r *CommandRegistry) Register(c Command) error {
	if c.ID == "" || strings.Join(strings.Fields(c.ID), "") != c.ID || strings.TrimSpace(c.Title) == "" || c.Run == nil {
		return fmt.Errorf("invalid command registration: %q", c.ID)
	}
	if _, exists := r.commands[c.ID]; exists {
		return fmt.Errorf("duplicate command: %s", c.ID)
	}
	r.commands[c.ID] = c
	r.order = append(r.order, c.ID)
	return nil
}

func (r *CommandRegistry) Available(id string) bool {
	c, ok := r.commands[id]
	return ok && (c.Enabled == nil || c.Enabled())
}

func (r *CommandRegistry) Execute(id string) error {
	if !r.Available(id) {
		return fmt.Errorf("command unavailable: %s", id)
	}
	r.commands[id].Run()
	return nil
}

func (r *CommandRegistry) Search(query string) []CommandMatch {
	tokens := strings.Fields(strings.ToLower(query))
	var matches []CommandMatch
	for _, id := range r.order {
		c := r.commands[id]
		haystack := strings.ToLower(c.ID + " " + c.Title + " " + c.Keywords)
		matched := true
		for _, token := range tokens {
			if !strings.Contains(haystack, token) {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, CommandMatch{ID: id, Title: c.Title, Enabled: r.Available(id)})
		}
	}
	return matches
}
