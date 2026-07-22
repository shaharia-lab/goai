package anth2oai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslateResponse_Text(t *testing.T) {
	in := &anthResponse{
		ID:         "msg_123",
		Model:      "glm-4.5-air",
		Role:       "assistant",
		Content:    []anthRespBlock{{Type: "text", Text: "OK"}},
		StopReason: "end_turn",
		Usage:      anthUsage{InputTokens: 10, OutputTokens: 2},
	}
	out := translateResponse(in, 1700000000)

	assert.Equal(t, "chatcmpl-msg_123", out.ID)
	assert.Equal(t, "chat.completion", out.Object)
	assert.Equal(t, int64(1700000000), out.Created)
	assert.Equal(t, "glm-4.5-air", out.Model)

	require.Len(t, out.Choices, 1)
	c := out.Choices[0]
	assert.Equal(t, 0, c.Index)
	assert.Equal(t, "assistant", c.Message.Role)
	require.NotNil(t, c.Message.Content)
	assert.Equal(t, "OK", *c.Message.Content)
	assert.Equal(t, "stop", c.FinishReason)

	assert.Equal(t, int64(10), out.Usage.PromptTokens)
	assert.Equal(t, int64(2), out.Usage.CompletionTokens)
	assert.Equal(t, int64(12), out.Usage.TotalTokens)
}

func TestTranslateResponse_ToolUse(t *testing.T) {
	in := &anthResponse{
		ID:    "msg_9",
		Model: "m",
		Content: []anthRespBlock{
			{Type: "tool_use", ID: "toolu_1", Name: "get_weather", Input: json.RawMessage(`{"location":"Paris"}`)},
		},
		StopReason: "tool_use",
		Usage:      anthUsage{InputTokens: 5, OutputTokens: 7},
	}
	out := translateResponse(in, 1)

	c := out.Choices[0]
	assert.Nil(t, c.Message.Content, "content is null when only tool calls are present")
	require.Len(t, c.Message.ToolCalls, 1)
	tc := c.Message.ToolCalls[0]
	assert.Equal(t, "toolu_1", tc.ID)
	assert.Equal(t, "function", tc.Type)
	assert.Equal(t, "get_weather", tc.Function.Name)
	assert.JSONEq(t, `{"location":"Paris"}`, tc.Function.Arguments)
	assert.Equal(t, "tool_calls", c.FinishReason)
}

func TestTranslateResponse_TextAndToolUse(t *testing.T) {
	in := &anthResponse{
		ID:    "msg_x",
		Model: "m",
		Content: []anthRespBlock{
			{Type: "text", Text: "Let me check. "},
			{Type: "tool_use", ID: "t1", Name: "f", Input: json.RawMessage(`{}`)},
		},
		StopReason: "tool_use",
	}
	out := translateResponse(in, 1)
	c := out.Choices[0]
	require.NotNil(t, c.Message.Content)
	assert.Equal(t, "Let me check. ", *c.Message.Content)
	require.Len(t, c.Message.ToolCalls, 1)
}

func TestTranslateResponse_EmptyToolInput(t *testing.T) {
	in := &anthResponse{
		ID:      "m",
		Content: []anthRespBlock{{Type: "tool_use", ID: "t", Name: "f"}},
	}
	out := translateResponse(in, 1)
	assert.Equal(t, "{}", out.Choices[0].Message.ToolCalls[0].Function.Arguments)
}

func TestMapStopReason(t *testing.T) {
	cases := map[string]string{
		"end_turn":      "stop",
		"stop_sequence": "stop",
		"max_tokens":    "length",
		"tool_use":      "tool_calls",
		"":              "",
		"weird":         "stop",
	}
	for in, want := range cases {
		assert.Equal(t, want, mapStopReason(in), "stop_reason=%q", in)
	}
}

// TestTranslateResponse_Marshals confirms the emitted JSON has the OpenAI shape
// that an OpenAI SDK client deserialises.
func TestTranslateResponse_Marshals(t *testing.T) {
	in := &anthResponse{ID: "msg_1", Model: "m", Content: []anthRespBlock{{Type: "text", Text: "hi"}}, StopReason: "end_turn"}
	out := translateResponse(in, 42)
	b, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"object":"chat.completion"`)
	assert.Contains(t, string(b), `"finish_reason":"stop"`)
	assert.Contains(t, string(b), `"role":"assistant"`)
}
