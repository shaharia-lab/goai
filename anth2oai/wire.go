package anth2oai

import "encoding/json"

// This file defines plain JSON wire structs for the OpenAI Chat Completions and
// Anthropic Messages formats.
//
// Why not the SDK types? The openai-go / anthropic-sdk-go request ("params")
// types wrap every field in a param.Field helper that marshals but does NOT
// unmarshal — so an inbound OpenAI request body cannot be reliably decoded back
// into openai.ChatCompletionNewParams. The translators therefore operate on the
// wire JSON directly. The official SDK types remain the public boundary: New()
// returns a *openai.Client, and the OpenAI-shaped bytes these structs emit are
// exactly what that client (and any other OpenAI client) deserialises.

// ---- OpenAI request (inbound) ----

type oaiRequest struct {
	Model               string            `json:"model"`
	Messages            []oaiMessage      `json:"messages"`
	MaxTokens           *int64            `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int64            `json:"max_completion_tokens,omitempty"`
	Temperature         *float64          `json:"temperature,omitempty"`
	TopP                *float64          `json:"top_p,omitempty"`
	Stop                json.RawMessage   `json:"stop,omitempty"` // string or []string
	Tools               []oaiTool         `json:"tools,omitempty"`
	ToolChoice          json.RawMessage   `json:"tool_choice,omitempty"` // string or object
	Stream              bool              `json:"stream,omitempty"`
	StreamOptions       *oaiStreamOptions `json:"stream_options,omitempty"`
	N                   *int              `json:"n,omitempty"`
}

type oaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type oaiMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"` // string or []oaiContentPart (may be null)
	ToolCalls  []oaiToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

type oaiContentPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *oaiImageURL `json:"image_url,omitempty"`
}

type oaiImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type oaiTool struct {
	Type     string          `json:"type"`
	Function oaiToolFunction `json:"function"`
}

type oaiToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ---- OpenAI response (outbound) ----

type oaiResponse struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	Model   string      `json:"model"`
	Choices []oaiChoice `json:"choices"`
	Usage   oaiUsage    `json:"usage"`
}

type oaiChoice struct {
	Index        int            `json:"index"`
	Message      oaiRespMessage `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

type oaiRespMessage struct {
	Role      string        `json:"role"`
	Content   *string       `json:"content"`
	ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
}

type oaiToolCall struct {
	Index    *int            `json:"index,omitempty"`
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type,omitempty"`
	Function oaiFunctionCall `json:"function"`
}

type oaiFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"`
}

type oaiUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// ---- OpenAI streaming chunk (outbound) ----

type oaiChunk struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []oaiChunkChoice `json:"choices"`
	Usage   *oaiUsage        `json:"usage,omitempty"`
}

type oaiChunkChoice struct {
	Index        int      `json:"index"`
	Delta        oaiDelta `json:"delta"`
	FinishReason *string  `json:"finish_reason"`
}

type oaiDelta struct {
	Role      string        `json:"role,omitempty"`
	Content   *string       `json:"content,omitempty"`
	ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
}

// ---- Anthropic request (outbound) ----

type anthRequest struct {
	Model         string          `json:"model"`
	MaxTokens     int64           `json:"max_tokens"`
	System        []anthTextBlock `json:"system,omitempty"`
	Messages      []anthMessage   `json:"messages"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Tools         []anthTool      `json:"tools,omitempty"`
	ToolChoice    *anthToolChoice `json:"tool_choice,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
}

type anthTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthMessage struct {
	Role    string             `json:"role"`
	Content []anthContentBlock `json:"content"`
}

// anthContentBlock is a union over the Anthropic block types we emit:
// text, image, tool_use and tool_result. Unused fields are omitted.
type anthContentBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// image
	Source *anthImageSource `json:"source,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type anthImageSource struct {
	Type      string `json:"type"` // "base64" | "url"
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type anthTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthToolChoice struct {
	Type string `json:"type"` // "auto" | "any" | "tool"
	Name string `json:"name,omitempty"`
}

// ---- Anthropic response (inbound, non-streaming) ----

type anthResponse struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Role         string          `json:"role"`
	Model        string          `json:"model"`
	Content      []anthRespBlock `json:"content"`
	StopReason   string          `json:"stop_reason"`
	StopSequence *string         `json:"stop_sequence"`
	Usage        anthUsage       `json:"usage"`
}

type anthRespBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// ---- Anthropic streaming events (inbound) ----

type anthStreamEvent struct {
	Type         string          `json:"type"`
	Message      *anthResponse   `json:"message,omitempty"`       // message_start
	Index        int             `json:"index"`                   // content_block_*
	ContentBlock *anthRespBlock  `json:"content_block,omitempty"` // content_block_start
	Delta        *anthEventDelta `json:"delta,omitempty"`         // content_block_delta / message_delta
	Usage        *anthUsage      `json:"usage,omitempty"`         // message_delta
	Error        *anthErrorBody  `json:"error,omitempty"`         // error
}

type anthEventDelta struct {
	Type         string  `json:"type,omitempty"`          // text_delta | input_json_delta
	Text         string  `json:"text,omitempty"`          // text_delta
	PartialJSON  string  `json:"partial_json,omitempty"`  // input_json_delta
	StopReason   string  `json:"stop_reason,omitempty"`   // message_delta
	StopSequence *string `json:"stop_sequence,omitempty"` // message_delta
}

// ---- Anthropic / OpenAI error shapes ----

type anthErrorEnvelope struct {
	Type  string         `json:"type"`
	Error *anthErrorBody `json:"error"`
}

type anthErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
