package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxChatArchiveBytes = 4 << 20

// chatArchive records a settled conversation and its original model binding.
// It contains neither credential fields nor a pending input draft.
type chatArchive struct {
	Version  int        `json:"version"`
	Endpoint string     `json:"endpoint"`
	Model    string     `json:"model"`
	Turns    []ChatTurn `json:"turns"`
}

func (a chatArchive) validate() error {
	if a.Version != 1 || len(a.Turns) == 0 || len(a.Turns) > maxChatTurns {
		return errors.New("unsupported chat archive version or turn count")
	}
	// Construction validates the connection without making a network request.
	client, err := NewClient(a.Endpoint, a.Model)
	if err != nil {
		return err
	}
	client.Close()
	for _, t := range a.Turns {
		if strings.TrimSpace(t.User) == "" || !utf8.ValidString(t.User) || len(t.User) > maxChatPromptBytes || !utf8.ValidString(t.Assistant) || len(t.Assistant) > maxChatReplyBytes {
			return errors.New("invalid chat archive text")
		}
		switch t.State {
		case ChatCompleted:
			if t.FinishReason != "stop" && t.FinishReason != "length" && t.FinishReason != "content_filter" {
				return errors.New("invalid archived finish reason")
			}
		case ChatCanceled, ChatFailed:
			if t.FinishReason != "" {
				return errors.New("incomplete archived turn has finish reason")
			}
		default:
			return errors.New("chat archive requires settled turns")
		}
	}
	return nil
}

// SaveChatArchive writes an explicit snapshot to a caller-selected path.
// Chat text and endpoint/model are plaintext; credentials are not accepted.
// The caller must serialize writes to the same path. Active and empty chats
// are rejected. A successful save replaces the file using a private temp file.
func SaveChatArchive(path, endpoint, modelName string, s *ChatSession) error {
	if s == nil {
		return errors.New("missing chat session")
	}
	snapshot := s.Snapshot()
	if snapshot.Active {
		return errors.New("cannot archive an active chat")
	}
	a := chatArchive{Version: 1, Endpoint: endpoint, Model: modelName, Turns: snapshot.Turns}
	if err := a.validate(); err != nil {
		return err
	}
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if len(data) > maxChatArchiveBytes {
		return errors.New("chat archive exceeds size limit")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".chat-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// LoadChatArchive validates the entire archive before returning a fresh idle
// session and the saved model binding. It never resumes or sends a request.
// The caller must explicitly confirm the restored binding before sending.
func LoadChatArchive(path string) (*ChatSession, string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxChatArchiveBytes+1))
	if err != nil {
		return nil, "", "", err
	}
	if len(data) > maxChatArchiveBytes || !utf8.Valid(data) {
		return nil, "", "", errors.New("invalid or oversized chat archive")
	}
	var a chatArchive
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&a); err != nil {
		return nil, "", "", errors.New("invalid chat archive")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, "", "", errors.New("trailing chat archive data")
	}
	if err := a.validate(); err != nil {
		return nil, "", "", err
	}
	return &ChatSession{turns: a.Turns}, a.Endpoint, a.Model, nil
}

// RestoreArchive loads a validated archive into an empty, idle session.
// It leaves an existing conversation unchanged on every error.
func (s *ChatSession) RestoreArchive(path string) (string, string, error) {
	restored, endpoint, modelName, err := LoadChatArchive(path)
	if err != nil {
		return "", "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil || len(s.turns) != 0 {
		return "", "", errors.New("restore requires an empty idle chat")
	}
	s.turns = restored.turns
	return endpoint, modelName, nil
}
