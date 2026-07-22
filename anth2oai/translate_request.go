package anth2oai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// emptyObject is the default Anthropic tool input schema / tool_use input.
var emptyObject = json.RawMessage(`{}`)

// defaultInputSchema is used when an OpenAI tool omits its parameters.
var defaultInputSchema = json.RawMessage(`{"type":"object"}`)

// translateRequest converts a decoded OpenAI Chat Completions request into an
// Anthropic Messages request. It is pure: no I/O, no globals mutated.
func translateRequest(in *oaiRequest, defaultMaxTokens int64) (*anthRequest, error) {
	out := &anthRequest{
		Model:  in.Model,
		Stream: in.Stream,
	}

	// max_tokens: Anthropic requires it.
	switch {
	case in.MaxTokens != nil && *in.MaxTokens > 0:
		out.MaxTokens = *in.MaxTokens
	case in.MaxCompletionTokens != nil && *in.MaxCompletionTokens > 0:
		out.MaxTokens = *in.MaxCompletionTokens
	default:
		out.MaxTokens = defaultMaxTokens
	}

	// temperature: OpenAI 0..2, Anthropic 0..1 — clamp.
	if in.Temperature != nil {
		t := *in.Temperature
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		out.Temperature = &t
	}
	if in.TopP != nil {
		out.TopP = in.TopP
	}

	// stop → stop_sequences.
	stops, err := normalizeStop(in.Stop)
	if err != nil {
		return nil, err
	}
	out.StopSequences = stops

	// messages.
	if err := translateMessages(in.Messages, out); err != nil {
		return nil, err
	}

	// tools + tool_choice.
	dropTools, err := translateToolChoice(in.ToolChoice, out)
	if err != nil {
		return nil, err
	}
	if !dropTools {
		out.Tools = translateTools(in.Tools)
	}

	return out, nil
}

func normalizeStop(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		if one == "" {
			return nil, nil
		}
		return []string{one}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many, nil
	}
	return nil, fmt.Errorf("anth2oai: invalid stop value: %s", string(raw))
}

// translateMessages maps OpenAI messages onto Anthropic system text + a message
// list, merging consecutive same-role blocks into one MessageParam (Anthropic
// rejects adjacent same-role messages, and tool results must live in a user
// message).
func translateMessages(msgs []oaiMessage, out *anthRequest) error {
	for _, m := range msgs {
		switch m.Role {
		case "system", "developer":
			text, err := messageText(m.Content)
			if err != nil {
				return err
			}
			if text != "" {
				out.System = append(out.System, anthTextBlock{Type: "text", Text: text})
			}

		case "user":
			blocks, err := userContentBlocks(m.Content)
			if err != nil {
				return err
			}
			appendBlocks(&out.Messages, "user", blocks)

		case "tool":
			// Normalize to a JSON string: OpenAI tool content may be a plain
			// string or an array of content parts; Anthropic's tool_result takes
			// a string (or blocks), so we flatten to text either way.
			text, err := messageText(m.Content)
			if err != nil {
				return err
			}
			content, err := json.Marshal(text)
			if err != nil {
				return fmt.Errorf("anth2oai: encoding tool result: %w", err)
			}
			block := anthContentBlock{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   content,
			}
			appendBlocks(&out.Messages, "user", []anthContentBlock{block})

		case "assistant":
			var blocks []anthContentBlock
			text, err := messageText(m.Content)
			if err != nil {
				return err
			}
			if text != "" {
				blocks = append(blocks, anthContentBlock{Type: "text", Text: text})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(strings.TrimSpace(tc.Function.Arguments))
				if len(input) == 0 {
					input = emptyObject
				}
				blocks = append(blocks, anthContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: input,
				})
			}
			appendBlocks(&out.Messages, "assistant", blocks)

		default:
			return fmt.Errorf("anth2oai: unsupported message role: %q", m.Role)
		}
	}
	return nil
}

// appendBlocks appends blocks to the message list, merging into the previous
// message when it shares the role.
func appendBlocks(messages *[]anthMessage, role string, blocks []anthContentBlock) {
	if len(blocks) == 0 {
		return
	}
	n := len(*messages)
	if n > 0 && (*messages)[n-1].Role == role {
		(*messages)[n-1].Content = append((*messages)[n-1].Content, blocks...)
		return
	}
	*messages = append(*messages, anthMessage{Role: role, Content: blocks})
}

// messageText extracts the plain text of an OpenAI message content that is
// either a JSON string or an array of parts (text parts concatenated).
func messageText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var parts []oaiContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("anth2oai: invalid message content: %w", err)
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "text" || p.Text != "" {
			b.WriteString(p.Text)
		}
	}
	return b.String(), nil
}

// userContentBlocks maps an OpenAI user message content to Anthropic blocks,
// handling text and image_url parts.
func userContentBlocks(raw json.RawMessage) ([]anthContentBlock, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return nil, nil
		}
		return []anthContentBlock{{Type: "text", Text: s}}, nil
	}
	var parts []oaiContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("anth2oai: invalid user content: %w", err)
	}
	var blocks []anthContentBlock
	for _, p := range parts {
		switch {
		case p.Type == "image_url" && p.ImageURL != nil:
			blocks = append(blocks, imageBlock(p.ImageURL.URL))
		case p.Type == "text" || p.Text != "":
			blocks = append(blocks, anthContentBlock{Type: "text", Text: p.Text})
		}
	}
	return blocks, nil
}

// imageBlock converts an OpenAI image_url into an Anthropic image block. A
// data: URI becomes a base64 source; any other URL becomes a URL source.
func imageBlock(u string) anthContentBlock {
	if strings.HasPrefix(u, "data:") {
		if mediaType, data, ok := parseDataURI(u); ok {
			return anthContentBlock{
				Type: "image",
				Source: &anthImageSource{
					Type:      "base64",
					MediaType: mediaType,
					Data:      data,
				},
			}
		}
	}
	return anthContentBlock{
		Type:   "image",
		Source: &anthImageSource{Type: "url", URL: u},
	}
}

// parseDataURI splits a "data:<mediatype>;base64,<data>" URI.
func parseDataURI(u string) (mediaType, data string, ok bool) {
	rest := strings.TrimPrefix(u, "data:")
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return "", "", false
	}
	meta, payload := rest[:comma], rest[comma+1:]
	if !strings.Contains(meta, "base64") {
		return "", "", false
	}
	mediaType = strings.TrimSuffix(meta, ";base64")
	mediaType = strings.TrimSuffix(mediaType, "base64")
	mediaType = strings.TrimSuffix(mediaType, ";")
	if mediaType == "" {
		mediaType = "image/png"
	}
	return mediaType, payload, true
}

func translateTools(tools []oaiTool) []anthTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthTool, 0, len(tools))
	for _, t := range tools {
		if t.Type != "" && t.Type != "function" {
			continue
		}
		schema := t.Function.Parameters
		if len(schema) == 0 || string(schema) == "null" {
			schema = defaultInputSchema
		}
		out = append(out, anthTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: schema,
		})
	}
	return out
}

// translateToolChoice maps OpenAI tool_choice onto Anthropic tool_choice.
// It returns dropTools=true when tools should be omitted entirely ("none").
func translateToolChoice(raw json.RawMessage, out *anthRequest) (dropTools bool, err error) {
	if len(raw) == 0 || string(raw) == "null" {
		return false, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto":
			out.ToolChoice = &anthToolChoice{Type: "auto"}
		case "required":
			out.ToolChoice = &anthToolChoice{Type: "any"}
		case "none":
			return true, nil
		}
		return false, nil
	}
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return false, fmt.Errorf("anth2oai: invalid tool_choice: %w", err)
	}
	if obj.Function.Name != "" {
		out.ToolChoice = &anthToolChoice{Type: "tool", Name: obj.Function.Name}
	}
	return false, nil
}
