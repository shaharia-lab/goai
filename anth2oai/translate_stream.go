package anth2oai

import (
	"encoding/json"
	"fmt"
)

// streamTransformer is the state machine that turns a stream of Anthropic SSE
// events into OpenAI chat.completion.chunk SSE frames. It is keyed by
// content-block index so interleaved text and tool_use blocks are tracked
// independently, and it is incremental — one call per Anthropic event.
type streamTransformer struct {
	id           string
	model        string
	created      int64
	includeUsage bool

	inputTokens  int64
	outputTokens int64

	roleSent      bool
	toolCallCount int          // number of tool_use blocks started so far
	blockToolIdx  map[int]int  // Anthropic block index -> OpenAI tool_call index
	toolArgsSeen  map[int]bool // Anthropic block index -> received any input_json_delta
}

func newStreamTransformer(created int64, includeUsage bool) *streamTransformer {
	return &streamTransformer{
		created:      created,
		includeUsage: includeUsage,
		blockToolIdx: map[int]int{},
		toolArgsSeen: map[int]bool{},
	}
}

// handle consumes one Anthropic event and returns the SSE frames to write
// (each already terminated with "\n\n", the [DONE] sentinel included when the
// stream ends). done reports whether the stream is complete.
func (s *streamTransformer) handle(ev *anthStreamEvent) (frames [][]byte, done bool, err error) {
	switch ev.Type {
	case "message_start":
		if ev.Message != nil {
			s.id = chatCmplPrefix + ev.Message.ID
			s.model = ev.Message.Model
			s.inputTokens = ev.Message.Usage.InputTokens
		}
		s.roleSent = true
		role := "assistant"
		empty := ""
		return s.frame(oaiDelta{Role: role, Content: &empty}, nil), false, nil

	case "content_block_start":
		if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
			idx := s.toolCallCount
			s.toolCallCount++
			s.blockToolIdx[ev.Index] = idx
			args := ""
			delta := oaiDelta{ToolCalls: []oaiToolCall{{
				Index: &idx,
				ID:    ev.ContentBlock.ID,
				Type:  "function",
				Function: oaiFunctionCall{
					Name:      ev.ContentBlock.Name,
					Arguments: args,
				},
			}}}
			return s.frame(delta, nil), false, nil
		}
		return nil, false, nil

	case "content_block_delta":
		if ev.Delta == nil {
			return nil, false, nil
		}
		switch ev.Delta.Type {
		case "text_delta":
			text := ev.Delta.Text
			return s.frame(oaiDelta{Content: &text}, nil), false, nil
		case "input_json_delta":
			idx, ok := s.blockToolIdx[ev.Index]
			if !ok {
				return nil, false, nil
			}
			s.toolArgsSeen[ev.Index] = true
			delta := oaiDelta{ToolCalls: []oaiToolCall{{
				Index:    &idx,
				Function: oaiFunctionCall{Arguments: ev.Delta.PartialJSON},
			}}}
			return s.frame(delta, nil), false, nil
		}
		return nil, false, nil

	case "content_block_stop":
		// A tool_use block that produced no input_json_delta would otherwise
		// leave arguments as "" — emit "{}" so strict clients can json.Unmarshal,
		// matching the non-streaming path's inputToArguments default.
		if idx, ok := s.blockToolIdx[ev.Index]; ok && !s.toolArgsSeen[ev.Index] {
			args := "{}"
			delta := oaiDelta{ToolCalls: []oaiToolCall{{
				Index:    &idx,
				Function: oaiFunctionCall{Arguments: args},
			}}}
			return s.frame(delta, nil), false, nil
		}
		return nil, false, nil

	case "message_delta":
		if ev.Usage != nil {
			s.outputTokens = ev.Usage.OutputTokens
		}
		if ev.Delta != nil && ev.Delta.StopReason != "" {
			fr := mapStopReason(ev.Delta.StopReason)
			return s.frame(oaiDelta{}, &fr), false, nil
		}
		return nil, false, nil

	case "message_stop":
		var out [][]byte
		if s.includeUsage {
			out = append(out, s.usageFrame())
		}
		out = append(out, []byte("data: [DONE]\n\n"))
		return out, true, nil

	case "error":
		msg := "upstream stream error"
		if ev.Error != nil && ev.Error.Message != "" {
			msg = ev.Error.Message
		}
		// The [DONE] terminator is written by the transport's finishStream on error.
		return nil, true, fmt.Errorf("anth2oai: %s", msg)

	case "ping":
		return nil, false, nil

	default:
		return nil, false, nil
	}
}

// frame marshals a single chunk into an SSE data frame.
func (s *streamTransformer) frame(delta oaiDelta, finish *string) [][]byte {
	chunk := oaiChunk{
		ID:      s.id,
		Object:  "chat.completion.chunk",
		Created: s.created,
		Model:   s.model,
		Choices: []oaiChunkChoice{{
			Index:        0,
			Delta:        delta,
			FinishReason: finish,
		}},
	}
	return [][]byte{encodeSSE(chunk)}
}

// usageFrame emits a final usage-only chunk (choices: []) before [DONE].
func (s *streamTransformer) usageFrame() []byte {
	chunk := oaiChunk{
		ID:      s.id,
		Object:  "chat.completion.chunk",
		Created: s.created,
		Model:   s.model,
		Choices: []oaiChunkChoice{},
		Usage: &oaiUsage{
			PromptTokens:     s.inputTokens,
			CompletionTokens: s.outputTokens,
			TotalTokens:      s.inputTokens + s.outputTokens,
		},
	}
	return encodeSSE(chunk)
}

func encodeSSE(chunk oaiChunk) []byte {
	b, err := json.Marshal(chunk)
	if err != nil {
		// oaiChunk is composed of JSON-safe types; marshal cannot realistically fail.
		return []byte("data: {}\n\n")
	}
	out := make([]byte, 0, len(b)+8)
	out = append(out, "data: "...)
	out = append(out, b...)
	out = append(out, '\n', '\n')
	return out
}
