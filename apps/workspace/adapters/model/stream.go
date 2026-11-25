package model

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"
	"unicode/utf8"
)

const maxStreamEventBytes = 256 << 10
const maxStreamWireBytes = 8 << 20

// Stream calls onText synchronously for each nonempty text delta. The caller
// owns partial output on failure; a Completion is returned only after a
// finish reason and [DONE]. Callbacks must return promptly. No tools execute.
func (c *Client) Stream(ctx context.Context, apiKey string, messages []Message, onText func(string) error) (Completion, error) {
	if onText == nil {
		return Completion{}, errors.New("stream callback required")
	}
	resp, err := c.request(ctx, apiKey, messages, true)
	if err != nil {
		return Completion{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Completion{}, fmt.Errorf("model HTTP status %d", resp.StatusCode)
	}
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "text/event-stream" {
		return Completion{}, errors.New("expected SSE model response")
	}
	limited := &io.LimitedReader{R: resp.Body, N: maxStreamWireBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), maxStreamEventBytes)
	var event, text strings.Builder
	finish := ""
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return Completion{}, err
		}
		if limited.N <= 0 {
			return Completion{}, errors.New("model stream exceeds wire limit")
		}
		line := scanner.Text()
		if line != "" {
			if line == "data" || strings.HasPrefix(line, "data:") {
				value := strings.TrimPrefix(line, "data")
				value = strings.TrimPrefix(value, ":")
				value = strings.TrimPrefix(value, " ")
				if event.Len()+len(value)+1 > maxStreamEventBytes {
					return Completion{}, errors.New("model stream event exceeds limit")
				}
				event.WriteString(value)
				event.WriteByte('\n')
			}
			continue
		}
		if event.Len() == 0 {
			continue
		}
		payload := strings.TrimSuffix(event.String(), "\n")
		event.Reset()
		if payload == "[DONE]" {
			if finish == "" {
				return Completion{}, errors.New("model stream missing finish reason")
			}
			return Completion{Text: text.String(), FinishReason: finish}, nil
		}
		var chunk struct {
			Choices []struct {
				Index *int
				Delta struct {
					Role         string
					Content      *string
					ToolCalls    []json.RawMessage `json:"tool_calls"`
					FunctionCall json.RawMessage   `json:"function_call"`
				}
				FinishReason *string `json:"finish_reason"`
			}
			Usage json.RawMessage
			Error json.RawMessage
		}
		if !utf8.ValidString(payload) || json.Unmarshal([]byte(payload), &chunk) != nil || len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return Completion{}, errors.New("invalid model stream event")
		}
		// Usage-only events are allowed after the final choice.
		if len(chunk.Choices) == 0 && finish != "" && len(chunk.Usage) > 0 && string(chunk.Usage) != "null" {
			continue
		}
		if len(chunk.Choices) != 1 || finish != "" {
			return Completion{}, errors.New("unexpected model stream choice")
		}
		choice := chunk.Choices[0]
		delta := choice.Delta
		if choice.Index == nil || *choice.Index != 0 || (delta.Role != "" && delta.Role != "assistant") || len(delta.ToolCalls) > 0 || (len(delta.FunctionCall) > 0 && string(delta.FunctionCall) != "null") {
			return Completion{}, errors.New("unsupported model stream response")
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			switch *choice.FinishReason {
			case "stop", "length", "content_filter":
				finish = *choice.FinishReason
			default:
				return Completion{}, errors.New("unsupported model stream finish reason")
			}
		}
		if delta.Content != nil && *delta.Content != "" {
			if len(*delta.Content) > maxResponseBytes-text.Len() {
				return Completion{}, errors.New("model stream exceeds text limit")
			}
			text.WriteString(*delta.Content)
			if err := onText(*delta.Content); err != nil {
				return Completion{}, err
			}
			if err := ctx.Err(); err != nil {
				return Completion{}, err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	if scanner.Err() != nil || limited.N <= 0 {
		return Completion{}, errors.New("could not read bounded model stream")
	}
	return Completion{}, errors.New("model stream ended before completion")
}
