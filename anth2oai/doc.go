// Package anth2oai turns an Anthropic-format inference endpoint (the Anthropic
// Messages API, POST /v1/messages) into an OpenAI Chat Completions-compatible
// one. It lets a caller use the OpenAI SDK — or any OpenAI HTTP client — to talk
// to providers that only expose an Anthropic-compatible endpoint (Z.ai/GLM,
// Kimi, DeepSeek, and Claude itself).
//
// The name encodes the direction: anth2oai = "expose an Anthropic endpoint
// through the OpenAI interface".
//
// # Layers
//
//	L1  pure translators   OpenAI ⇄ Anthropic (request, response, SSE stream)
//	L2  http.RoundTripper  plugs the translation into any *http.Client / the SDK
//	L3  New(...)           a *openai.Client preloaded with the L2 transport
//	L4  NewHandler(...)    an OpenAI-compatible POST /v1/chat/completions proxy
//
// # Usage
//
//	client := anth2oai.New("https://api.z.ai/api/anthropic", token)
//	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
//	    Model:    openai.F("glm-4.5-air"),
//	    Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
//	        openai.UserMessage("Reply with exactly: OK"),
//	    }),
//	})
//
// Streaming works through the same client via Chat.Completions.NewStreaming.
//
// # Auth
//
// By default the caller's key is forwarded as "Authorization: Bearer <key>"
// (validated against Z.ai/GLM) plus an "anthropic-version: 2023-06-01" header.
// Use WithAuthHeader(AuthXAPIKey) for endpoints that require the native
// Anthropic "x-api-key" scheme.
//
// # Supported OpenAI fields
//
//	model, messages (system/developer, user, assistant, tool),
//	max_tokens / max_completion_tokens, temperature (clamped to [0,1]),
//	top_p, stop, tools + tool_choice, stream + stream_options.include_usage.
//	Image parts (image_url) are translated best-effort (data: URIs become
//	base64 image blocks, http(s) URLs become URL image blocks) and passed
//	through; upstream support varies.
//
// # Unsupported OpenAI fields (ignored in v1)
//
//	n > 1 (treated as 1), logit_bias, presence_penalty, frequency_penalty,
//	seed, response_format, logprobs. No model-name aliasing: the upstream model
//	id is passed through verbatim. Only Chat Completions is proxied — not
//	/v1/models, embeddings, or other OpenAI endpoints.
//
// # Note on types
//
// The translators operate on plain JSON wire structs rather than the SDK
// "params" types: those wrap fields in helpers that marshal but do not
// unmarshal, so an inbound OpenAI body cannot be decoded back into them. The
// official SDK types remain the public boundary (New returns *openai.Client).
package anth2oai
