package anth2oai

import (
	"encoding/json"
	"strings"
)

// chatCmplPrefix mirrors OpenAI's completion id convention.
const chatCmplPrefix = "chatcmpl-"

// translateResponse converts a non-streaming Anthropic Message into an OpenAI
// ChatCompletion. It is pure; created is injected by the caller.
func translateResponse(in *anthResponse, created int64) *oaiResponse {
	var text strings.Builder
	var toolCalls []oaiToolCall
	for _, block := range in.Content {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			toolCalls = append(toolCalls, oaiToolCall{
				ID:   block.ID,
				Type: "function",
				Function: oaiFunctionCall{
					Name:      block.Name,
					Arguments: inputToArguments(block.Input),
				},
			})
		}
	}

	msg := oaiRespMessage{Role: "assistant", ToolCalls: toolCalls}
	// OpenAI represents "assistant made only tool calls" as content: null.
	if !(text.Len() == 0 && len(toolCalls) > 0) {
		s := text.String()
		msg.Content = &s
	}

	return &oaiResponse{
		ID:      chatCmplPrefix + in.ID,
		Object:  "chat.completion",
		Created: created,
		Model:   in.Model,
		Choices: []oaiChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: mapStopReason(in.StopReason),
		}},
		Usage: oaiUsage{
			PromptTokens:     in.Usage.InputTokens,
			CompletionTokens: in.Usage.OutputTokens,
			TotalTokens:      in.Usage.InputTokens + in.Usage.OutputTokens,
		},
	}
}

// inputToArguments renders an Anthropic tool_use input as an OpenAI arguments
// JSON string, defaulting to "{}".
func inputToArguments(input json.RawMessage) string {
	s := strings.TrimSpace(string(input))
	if s == "" || s == "null" {
		return "{}"
	}
	return s
}

// mapStopReason maps an Anthropic stop_reason to an OpenAI finish_reason.
func mapStopReason(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "":
		return ""
	default:
		return "stop"
	}
}
