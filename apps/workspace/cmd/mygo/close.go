package main

import (
	"fmt"

	"github.com/egoist/mygo"
)

const (
	closeSave = iota
	closeDiscard
	closeCancel
)

// confirmClose also checks Notes when its tab is closed. A note session and
// the layout have independent files, and a failure must leave the window open.
func (s *shell) confirmClose(dialog func(mygo.MessageOptions) (mygo.MessageResult, error)) bool {
	if s.sessionEnding {
		return true
	}
	if s.closePromptOpen {
		return false
	}
	note := s.note()
	if !s.dirty && (note == nil || !note.Dirty()) {
		return true
	}
	s.closePromptOpen = true
	defer func() { s.closePromptOpen = false }()
	message := "Save your changes before closing?"
	if s.profile.Name != "" {
		message = fmt.Sprintf("Save changes to %q before closing?", s.profile.Name)
	}
	result, err := dialog(mygo.MessageOptions{
		Parent:             s.window,
		Type:               mygo.MessageWarning,
		Title:              "MyGo Workspace",
		Message:            message,
		Buttons:            []string{"Save and Close", "Close Without Saving", "Cancel"},
		DefaultButton:      closeCancel,
		CancelButton:       closeCancel,
		DestructiveButtons: []int{closeDiscard},
	})
	if err != nil {
		s.status = "Could not confirm close: " + err.Error()
		return false
	}
	switch result.Button {
	case closeDiscard:
		return true
	case closeSave:
		if note := s.note(); note != nil && note.Dirty() {
			s.saveNote()
			if note.Dirty() {
				return false
			}
		}
		if s.dirty {
			s.save()
		}
		return !s.dirty
	default:
		return false
	}
}
