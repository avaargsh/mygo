package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/apps/workspace/adapters/model"
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

// chatViewSession owns UI drafts; only model.ChatSession crosses threads.
// Connection details and credentials are never included in workspace storage.
type chatViewSession struct {
	sessionID string
	chat          model.ChatSession
	endpoint      string
	modelName     string
	apiKey        string
	draft         string
	boundEndpoint string
	boundModel    string
}

func (s *shell) chatSession() *chatViewSession {
	if v := s.content.Session("chat"); v != nil {
		chat, _ := v.Value.(*chatViewSession)
		return chat
	}
	return nil
}

func (s *shell) sendChat(ch *chatViewSession) {
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
	if _, err := ch.chat.Start(context.Background(), stream, ch.apiKey, ch.draft, nil); err != nil {
		client.Close()
		s.status = err.Error()
		return
	}
	ch.boundEndpoint, ch.boundModel = ch.endpoint, ch.modelName
	ch.apiKey, ch.draft = "", ""
	s.status = ""
}

func (s *shell) clearChat(ch *chatViewSession) {
	if err := ch.chat.Clear(); err != nil {
		s.status = err.Error()
		return
	}
	ch.boundEndpoint, ch.boundModel, ch.apiKey = "", "", ""
	ch.draft = ""
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
		ui.TextInput(c, &ch.endpoint).Label("Chat completions endpoint").Placeholder("https://host/v1/chat/completions")
		ui.TextInput(c, &ch.modelName).Label("Model name")
	} else {
		ui.Text(c, "Endpoint: "+ch.boundEndpoint)
		ui.Text(c, "Model: "+ch.boundModel)
	}
	ui.Text(c, "Send transmits your prompt and completed chat history to the selected model. No other workspace content is included. This conversation is not saved.")
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
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
	if state.Active {
		c.After(100 * time.Millisecond)
		ui.Text(c, "Generation active")
		ui.Button(c, "Stop generation").OnClick(func() { ch.chat.Cancel() })
		return
	}
	ui.TextInput(c, &ch.apiKey).Password().MaxLength(8192).Label("API key (optional; cleared on send)")
	ui.TextArea(c, &ch.draft).Label("Message").Height(80).MaxLength(8192)
	ui.Row(c).Gap(spaceControl).Children(func() {
		ui.Button(c, "Send message").Disabled(strings.TrimSpace(ch.draft) == "").OnClick(func() { s.sendChat(ch) })
		ui.Button(c, "Clear conversation").Disabled(len(state.Turns) == 0).OnClick(func() { s.clearChat(ch) })
	})
}

// Poll only while the worker is active. The UI owns its drafts and never
// receives synchronous UI calls from the network worker.
func (s *shell) renderChatActivity(c *ui.Context, snapshot core.WorkspaceSnapshot) {
	s.renderHiddenChats(c, snapshot)
}
