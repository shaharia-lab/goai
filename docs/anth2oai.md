# anth2oai — an OpenAI-compatible façade over Anthropic endpoints

`anth2oai` (subpackage `github.com/shaharia-lab/goai/anth2oai`) lets you talk to
an **Anthropic-format** inference endpoint — the Anthropic Messages API,
`POST /v1/messages` — using the **OpenAI Chat Completions** shape and SDK.

Many providers ship an Anthropic-compatible endpoint so that Claude Code can use
them (Claude Code speaks the Anthropic Messages API): Z.ai/GLM, Kimi, DeepSeek,
and Claude itself. `anth2oai` turns any of them into an OpenAI-compatible target,
so existing OpenAI tooling and code can reach them unchanged.

It translates OpenAI requests → Anthropic requests on the way out, and Anthropic
responses → OpenAI responses on the way back — for both **non-streaming** and
**streaming** calls.

## Install

```go
import "github.com/shaharia-lab/goai/anth2oai"
```

## Quick start

```go
client := anth2oai.New("https://api.z.ai/api/anthropic", token)

resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
    Model: openai.F("glm-4.5-air"),
    Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
        openai.UserMessage("Reply with exactly: OK"),
    }),
})
fmt.Println(resp.Choices[0].Message.Content)
```

`New` returns a plain `*openai.Client`; use the normal OpenAI SDK surface,
including `Chat.Completions.NewStreaming(...)`.

## Architecture — four layers

```
L3  anth2oai.New(...)      → *openai.Client preloaded with the L2 transport
L4  anth2oai.NewHandler()  → OpenAI-compatible POST /v1/chat/completions proxy
L2  anth2oai.NewRoundTripper(...) → http.RoundTripper (plug into any http.Client)
L1  pure translators       OpenAI ⇄ Anthropic (request, response, SSE stream)
```

- **L1** — pure, deterministic translators with no I/O.
- **L2** — an `http.RoundTripper` that reads the OpenAI request, rewrites the URL
  from `…/chat/completions` to `{baseURL}/v1/messages`, sets upstream auth and
  `anthropic-version`, and translates the body and response. Streaming is lazy
  (an `io.Pipe` + goroutine) so first-token latency is preserved.
- **L3** — `New(...)`, a convenience `*openai.Client`.
- **L4** — `NewHandler(...)`, an `http.Handler` so any OpenAI HTTP client (in any
  language) can reach the upstream.

## Public API

```go
// L3 convenience client.
func New(baseURL, apiKey string, opts ...Option) *openai.Client

// L2 transport — plug into any http.Client or the OpenAI SDK.
func NewRoundTripper(baseURL, apiKey string, opts ...Option) http.RoundTripper

// L4 proxy handler — OpenAI-compatible POST /v1/chat/completions → Anthropic.
func NewHandler(baseURL, apiKey string, opts ...Option) http.Handler

// Options.
func WithDefaultMaxTokens(n int) Option        // default 4096
func WithAnthropicVersion(v string) Option      // default "2023-06-01"
func WithAuthHeader(kind AuthKind) Option        // AuthBearer (default) | AuthXAPIKey
func WithHTTPClient(c *http.Client) Option        // underlying transport for upstream calls
func WithLogger(l goai.Logger) Option             // null logger by default
func WithBaseTransport(rt http.RoundTripper) Option // for testing / composition
```

## Auth & headers

By default the caller's key is forwarded as `Authorization: Bearer <key>`
(validated against GLM/Z.ai, which Claude Code reaches via
`ANTHROPIC_AUTH_TOKEN` as a bearer token), plus an `anthropic-version:
2023-06-01` header. For endpoints that require the native Anthropic scheme, use
`WithAuthHeader(anth2oai.AuthXAPIKey)` to send `x-api-key: <key>` instead.

## Field mapping

### Request (OpenAI → Anthropic)

| OpenAI | Anthropic | Notes |
|---|---|---|
| `model` | `model` | Passthrough, verbatim (no aliasing). |
| `max_tokens` / `max_completion_tokens` | `max_tokens` | Anthropic requires it; defaults to `WithDefaultMaxTokens` (4096). |
| `system` / `developer` messages | `system` | Concatenated, order-preserving. |
| `user` messages | `user` message | Text → `text` block; `image_url` → `image` block (best-effort). |
| `assistant` messages | `assistant` message | Text → `text`; `tool_calls` → `tool_use`. |
| `tool` messages | `user` message | `tool_call_id` → `tool_use_id`, content → `tool_result`. |
| `temperature` | `temperature` | **Clamped to [0,1]** (OpenAI allows 0–2). |
| `top_p` | `top_p` | Passthrough. |
| `stop` | `stop_sequences` | String or array normalised to `[]string`. |
| `tools[].function` | `tools[]` | `parameters` → `input_schema`. |
| `tool_choice` | `tool_choice` | `auto`→`auto`, `required`→`any`, `{name}`→`{type:tool,name}`, `none`→ tools omitted. |
| `stream`, `stream_options.include_usage` | `stream` | `include_usage` emits a final usage-only chunk. |

Images: a `data:` URI becomes a base64 image block; any other URL becomes a URL
image block. Support varies by upstream.

### Response (Anthropic → OpenAI)

- `content` text blocks are concatenated into `choices[0].message.content`
  (null when only tool calls are present).
- `tool_use` blocks become `tool_calls` with JSON `arguments`.
- `stop_reason` → `finish_reason`: `end_turn`/`stop_sequence`→`stop`,
  `max_tokens`→`length`, `tool_use`→`tool_calls`.
- `usage.input_tokens`/`output_tokens` → `prompt_tokens`/`completion_tokens`.

### Streaming (Anthropic SSE → OpenAI chunk SSE)

The Anthropic typed events (`message_start`, `content_block_start`,
`content_block_delta`, `message_delta`, `message_stop`, …) are reconstructed
into `chat.completion.chunk` frames, terminated by `data: [DONE]`.

## Unsupported (ignored in v1)

`n > 1` (treated as 1), `logit_bias`, `presence_penalty`, `frequency_penalty`,
`seed`, `response_format`, `logprobs`. No model aliasing. Only Chat Completions
is proxied — not `/v1/models`, embeddings, or other OpenAI endpoints.

## Errors

Upstream Anthropic errors are mapped to the OpenAI error shape
(`{"error": {"message", "type", "code"}}`), preserving the HTTP status, so they
surface through the OpenAI SDK as a normal `*openai.Error`.

## Example

See [`_examples/anth2oai_glm`](../_examples/anth2oai_glm) for a runnable
non-streaming + streaming demo driven entirely by environment variables.

## Live integration tests

`anth2oai/integration_test.go` (build tag `integration`) exercises a real
endpoint. It is skipped unless both env vars are set:

```bash
export ANTHROPIC_BASE_URL=https://api.z.ai/api/anthropic
export ANTHROPIC_AUTH_TOKEN=<your token>
go test -tags=integration ./anth2oai/...
```
