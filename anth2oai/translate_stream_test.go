package anth2oai

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseAnthropicSSE extracts the JSON payloads of an Anthropic SSE stream.
func parseAnthropicSSE(t *testing.T, raw string) []*anthStreamEvent {
	t.Helper()
	var events []*anthStreamEvent
	sc := bufio.NewScanner(strings.NewReader(raw))
	var data strings.Builder
	flush := func() {
		if data.Len() == 0 {
			return
		}
		var ev anthStreamEvent
		require.NoError(t, json.Unmarshal([]byte(data.String()), &ev))
		events = append(events, &ev)
		data.Reset()
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	flush()
	return events
}

// runTransformer feeds parsed events through the state machine and returns the
// decoded OpenAI chunks plus whether [DONE] was seen.
func runTransformer(t *testing.T, events []*anthStreamEvent, includeUsage bool) (chunks []oaiChunk, sawDone bool) {
	t.Helper()
	tr := newStreamTransformer(999, includeUsage)
	for _, ev := range events {
		frames, done, err := tr.handle(ev)
		require.NoError(t, err)
		for _, f := range frames {
			s := string(f)
			require.True(t, strings.HasPrefix(s, "data: "), "frame must start with 'data: ': %q", s)
			payload := strings.TrimSuffix(strings.TrimPrefix(s, "data: "), "\n\n")
			if payload == "[DONE]" {
				sawDone = true
				continue
			}
			var ch oaiChunk
			require.NoError(t, json.Unmarshal([]byte(payload), &ch))
			chunks = append(chunks, ch)
		}
		if done {
			break
		}
	}
	return chunks, sawDone
}

func TestStream_Text(t *testing.T) {
	raw, err := os.ReadFile("testdata/text_stream.sse")
	require.NoError(t, err)

	events := parseAnthropicSSE(t, string(raw))
	chunks, done := runTransformer(t, events, false)

	require.True(t, done, "stream terminated with [DONE]")
	require.NotEmpty(t, chunks)

	// First chunk carries the assistant role.
	assert.Equal(t, "assistant", chunks[0].Choices[0].Delta.Role)
	assert.Equal(t, "chat.completion.chunk", chunks[0].Object)
	assert.Equal(t, "chatcmpl-msg_text_1", chunks[0].ID)
	assert.Equal(t, "glm-4.5-air", chunks[0].Model)

	// Content accumulates to "OK".
	var content strings.Builder
	var finish string
	for _, c := range chunks {
		require.Len(t, c.Choices, 1)
		if c.Choices[0].Delta.Content != nil {
			content.WriteString(*c.Choices[0].Delta.Content)
		}
		if c.Choices[0].FinishReason != nil {
			finish = *c.Choices[0].FinishReason
		}
	}
	assert.Equal(t, "OK", content.String())
	assert.Equal(t, "stop", finish)
}

func TestStream_Tool(t *testing.T) {
	raw, err := os.ReadFile("testdata/tool_stream.sse")
	require.NoError(t, err)

	events := parseAnthropicSSE(t, string(raw))
	chunks, done := runTransformer(t, events, false)
	require.True(t, done)

	var name, args string
	var finish string
	var toolIndex = -1
	for _, c := range chunks {
		if len(c.Choices) == 0 {
			continue
		}
		for _, tcd := range c.Choices[0].Delta.ToolCalls {
			if tcd.Index != nil {
				toolIndex = *tcd.Index
			}
			if tcd.Function.Name != "" {
				name = tcd.Function.Name
			}
			args += tcd.Function.Arguments
		}
		if c.Choices[0].FinishReason != nil {
			finish = *c.Choices[0].FinishReason
		}
	}
	assert.Equal(t, 0, toolIndex)
	assert.Equal(t, "get_weather", name)
	assert.JSONEq(t, `{"location":"Paris"}`, args)
	assert.Equal(t, "tool_calls", finish)
}

func TestStream_IncludeUsage(t *testing.T) {
	raw, err := os.ReadFile("testdata/text_stream.sse")
	require.NoError(t, err)
	events := parseAnthropicSSE(t, string(raw))
	chunks, done := runTransformer(t, events, true)
	require.True(t, done)

	// The final non-DONE chunk is usage-only: empty choices, populated usage.
	last := chunks[len(chunks)-1]
	assert.Empty(t, last.Choices)
	require.NotNil(t, last.Usage)
	assert.Equal(t, int64(12), last.Usage.PromptTokens)
	assert.Equal(t, int64(5), last.Usage.CompletionTokens)
	assert.Equal(t, int64(17), last.Usage.TotalTokens)
}

func TestStream_ErrorEvent(t *testing.T) {
	tr := newStreamTransformer(1, false)
	ev := &anthStreamEvent{Type: "error", Error: &anthErrorBody{Type: "overloaded_error", Message: "boom"}}
	_, done, err := tr.handle(ev)
	require.Error(t, err)
	assert.True(t, done)
	assert.Contains(t, err.Error(), "boom")
}
