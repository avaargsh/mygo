package main

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/apps/workspace/adapters/model"
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

// chatViewSession owns UI drafts; only model.ChatSession crosses threads.
// Connection details are archived only explicitly; credentials and drafts never are.
type chatViewSession struct {
	sessionID         string
	chat              model.ChatSession
	endpoint          string
	modelName         string
	apiKey            string
	draft             string
	boundEndpoint     string
	boundModel        string
	archiveConfirm    bool
	confirmConnection bool
	searchExcerpt     *chatSearchExcerpt
	searchExcerptOpen bool
}

func (s *shell) chatSession() *chatViewSession {
	if v := s.content.Session("chat"); v != nil {
		chat, _ := v.Value.(*chatViewSession)
		return chat
	}
	return nil
}

func (s *shell) sendChat(ch *chatViewSession) {
	if ch.confirmConnection {
		s.status = "Confirm the restored model before sending"
		return
	}
	ch.archiveConfirm = false
	if ch.boundEndpoint != "" && (ch.endpoint != ch.boundEndpoint || ch.modelName != ch.boundModel) {
		s.status = "Clear conversation before changing its model connection"
		return
	}
	client, err := model.NewClient(ch.endpoint, ch.modelName)
	if err != nil {
		s.status = err.Error()
		return
	}
	stream := func(ctx context.Context, key string, messages []model.Message, emit func(string) error) (model.Completion, error) {
		defer client.Close()
		return client.Stream(ctx, key, messages, emit)
	}
	var excerpt string
	if ch.searchExcerpt != nil {
		excerpt = ch.searchExcerpt.text
	}
	run, err := ch.chat.StartRun(context.Background(), stream, ch.apiKey, ch.draft, excerpt, nil)
	if err != nil {
		client.Close()
		s.status = err.Error()
		return
	}
	s.recordChatRun(chatIdentity(ch), run)
	ch.boundEndpoint, ch.boundModel = ch.endpoint, ch.modelName
	ch.apiKey, ch.draft = "", ""
	ch.searchExcerpt, ch.searchExcerptOpen = nil, false
	s.status = ""
}

func (s *shell) clearChat(ch *chatViewSession) {
	if err := ch.chat.Clear(); err != nil {
		s.status = err.Error()
		return
	}
	ch.boundEndpoint, ch.boundModel, ch.apiKey = "", "", ""
	ch.draft = ""
	ch.searchExcerpt, ch.searchExcerptOpen = nil, false
	ch.archiveConfirm, ch.confirmConnection = false, false
	s.status = ""
}

func (s *shell) renderChat(c *ui.Context, snapshot core.WorkspaceSnapshot, paneID string, ch *chatViewSession) {
	if core.SessionOwner(snapshot, chatIdentity(ch)) != paneID {
		ui.Text(c, "Chat is displayed in another pane.")
		ui.Button(c, "Show chat here").OnClick(func() { s.apply(s.workspace.Focus(paneID)) })
		return
	}
	state := ch.chat.Snapshot()
	ui.Text(c, "Model chat").Bold()
	if ch.boundEndpoint == "" {
		ui.TextInput(c.Key("chat-endpoint"), &ch.endpoint).Label("Chat completions endpoint").Placeholder("https://host/v1/chat/completions")
		ui.TextInput(c.Key("chat-model"), &ch.modelName).Label("Model name")
	} else {
		ui.Text(c, "Endpoint: "+ch.boundEndpoint)
		ui.Text(c, "Model: "+ch.boundModel)
	}
	// Keep input and send controls ahead of the flexible transcript in a split pane.
	if state.Active {
		c.After(100 * time.Millisecond)
		ui.Text(c, "Generation active")
		ui.Button(c, "Stop generation").OnClick(func() { ch.chat.Cancel() })
	} else {
		if ch.confirmConnection {
			ui.Text(c, "Review the restored endpoint and model. Confirm before allowing future sends.")
			ui.Button(c, "Confirm restored model").OnClick(func() { ch.confirmConnection = false; s.status = "" })
		}
		ui.TextArea(c.Key("chat-message"), &ch.draft).Label("Message").Placeholder("Write a message…").Height(64).MinHeight(48).MaxLength(8192)
		ui.TextInput(c.Key("chat-credential"), &ch.apiKey).Password().MaxLength(8192).Label("API key (optional; cleared on send)")
		sendLabel := "Send message"
		if ch.searchExcerpt != nil {
			sendLabel = "Send message with excerpt"
		}
		ui.Row(c).Gap(spaceControl).Wrap().Children(func() {
			ui.Button(c, sendLabel).Disabled(ch.confirmConnection || strings.TrimSpace(ch.draft) == "").OnClick(func() { s.sendChat(ch) })
			ui.Button(c, "Clear conversation").Disabled(len(state.Turns) == 0).OnClick(func() { s.clearChat(ch) })
		})
		ui.Row(c).Gap(spaceControl).Wrap().Children(func() {
			if ch.archiveConfirm {
				ui.Button(c, "Confirm archive save").OnClick(func() { s.saveChatArchive(ch) })
				ui.Button(c, "Cancel archive save").OnClick(func() { ch.archiveConfirm = false })
			} else {
				ui.Button(c, "Save chat archive").Disabled(len(state.Turns) == 0 || s.statePath == "").OnClick(func() {
					ch.archiveConfirm = true
					s.status = "Save this chat locally in plaintext? This replaces this session’s previous archive."
				})
			}
			ui.Button(c, "Load chat archive").Disabled(!canLoadChatArchive(ch) || s.statePath == "").OnClick(func() { s.loadChatArchive(ch) })
		})
		s.renderChatSearchExcerpt(c, ch)
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Text(c, "Send includes your prompt, completed chat history, and any attached excerpt. Chat archives are explicit local plaintext and exclude credentials and unsent drafts.").FontSize(fontSmall)
		ui.Column(c).Gap(spaceSection).Children(func() {
			for i, turn := range state.Turns {
				ui.Column(c.Key("turn-"+strconv.Itoa(i))).Gap(4).Children(func() {
					ui.Text(c, "You").Bold()
					ui.Text(c, turn.User)
					ui.Text(c, "Assistant · "+string(turn.State)).Bold()
					if turn.Assistant != "" {
						ui.Text(c, turn.Assistant)
					}
					if turn.FinishReason != "" {
						ui.Text(c, "Finish: "+turn.FinishReason).FontSize(fontSmall)
					}
					if turn.State == model.ChatFailed || turn.State == model.ChatCanceled {
						ui.Text(c, "Incomplete reply; this turn is excluded from future requests.").FontSize(fontSmall)
					}
					ui.Divider(c)
				})
			}
		})
	})
}

// Poll only while the worker is active. The UI owns its drafts and never
// receives synchronous UI calls from the network worker.
func (s *shell) renderChatActivity(c *ui.Context, snapshot core.WorkspaceSnapshot) {
	s.renderHiddenChats(c, snapshot)
}

func canLoadChatArchive(ch *chatViewSession) bool {
	snapshot := ch.chat.Snapshot()
	return !snapshot.Active && len(snapshot.Turns) == 0 && ch.draft == "" && ch.apiKey == "" && ch.searchExcerpt == nil
}

func (s *shell) saveChatArchive(ch *chatViewSession) {
	if !ch.archiveConfirm || s.statePath == "" {
		return
	}
	ch.archiveConfirm = false
	path := s.chatArchivePath(ch)
	if err := model.SaveChatArchive(path, ch.boundEndpoint, ch.boundModel, &ch.chat); err != nil {
		s.status = "Chat archive save failed: " + err.Error()
		return
	}
	s.status = "Chat archive saved locally"
}

func (s *shell) loadChatArchive(ch *chatViewSession) {
	session := s.content.Session(chatIdentity(ch))
	if session == nil || session.Value != ch || !canLoadChatArchive(ch) || s.statePath == "" {
		s.status = "Clear the conversation, draft, credential and excerpt before loading"
		return
	}
	endpoint, modelName, err := ch.chat.RestoreArchive(s.chatArchivePath(ch))
	if err != nil {
		s.status = "Chat archive load failed: " + err.Error()
		return
	}
	ch.endpoint, ch.modelName = endpoint, modelName
	ch.boundEndpoint, ch.boundModel = endpoint, modelName
	ch.archiveConfirm, ch.confirmConnection = false, true
	s.status = "Chat archive loaded; confirm the model connection before sending"
}

// The default chat retains its original archive path; named sessions never share it.
func (s *shell) chatArchivePath(ch *chatViewSession) string {
	id := chatIdentity(ch)
	if id == "chat" {
		return filepath.Join(filepath.Dir(s.statePath), "chat.json")
	}
	return filepath.Join(filepath.Dir(s.statePath), id+".json")
}
