package anth2oai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubTransport returns canned upstream responses and captures the request it
// received, so tests can assert path rewriting, headers, and body translation.
type stubTransport struct {
	lastReq  *http.Request
	lastBody []byte
	respond  func(req *http.Request, body []byte) (*http.Response, error)
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	s.lastReq = req
	s.lastBody = body
	return s.respond(req, body)
}

func newResp(status int, contentType, body string) *http.Response {
	h := make(http.Header)
	h.Set("Content-Type", contentType)
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestRoundTrip_NonStreaming(t *testing.T) {
	stub := &stubTransport{
		respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
			return newResp(200, "application/json", `{
				"id":"msg_1","type":"message","role":"assistant","model":"glm-4.5-air",
				"content":[{"type":"text","text":"OK"}],
				"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}
			}`), nil
		},
	}
	rt := NewRoundTripper("https://api.z.ai/api/anthropic", "secret", WithBaseTransport(stub))

	req := newOAIRequest(t, "https://api.z.ai/api/anthropic/chat/completions", `{
		"model":"glm-4.5-air","messages":[{"role":"user","content":"hi"}]
	}`)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	// Path rewritten to /v1/messages, auth + version headers set.
	assert.Equal(t, "/api/anthropic/v1/messages", stub.lastReq.URL.Path)
	assert.Equal(t, "Bearer secret", stub.lastReq.Header.Get("Authorization"))
	assert.Equal(t, "2023-06-01", stub.lastReq.Header.Get("anthropic-version"))

	// Upstream body is Anthropic-shaped with a default max_tokens.
	var anthBody map[string]any
	require.NoError(t, json.Unmarshal(stub.lastBody, &anthBody))
	assert.Equal(t, float64(4096), anthBody["max_tokens"])

	// Response body is OpenAI-shaped.
	var out oaiResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Equal(t, "chat.completion", out.Object)
	require.NotNil(t, out.Choices[0].Message.Content)
	assert.Equal(t, "OK", *out.Choices[0].Message.Content)
}

func TestRoundTrip_XAPIKeyAuth(t *testing.T) {
	stub := &stubTransport{
		respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
			return newResp(200, "application/json",
				`{"id":"m","model":"m","content":[{"type":"text","text":"x"}],"stop_reason":"end_turn","usage":{}}`), nil
		},
	}
	rt := NewRoundTripper("https://up.example/anthropic", "k", WithBaseTransport(stub), WithAuthHeader(AuthXAPIKey))
	req := newOAIRequest(t, "https://up.example/anthropic/chat/completions",
		`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	_, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, "k", stub.lastReq.Header.Get("x-api-key"))
	assert.Empty(t, stub.lastReq.Header.Get("Authorization"))
}

func TestRoundTrip_UpstreamError(t *testing.T) {
	stub := &stubTransport{
		respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
			return newResp(429, "application/json",
				`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`), nil
		},
	}
	rt := NewRoundTripper("https://up.example", "k", WithBaseTransport(stub))
	req := newOAIRequest(t, "https://up.example/chat/completions",
		`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)

	var env oaiErrorEnvelope
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	assert.Equal(t, "slow down", env.Error.Message)
	assert.Equal(t, "rate_limit_error", env.Error.Type)
}

func TestRoundTrip_Streaming(t *testing.T) {
	sse, err := os.ReadFile("testdata/text_stream.sse")
	require.NoError(t, err)
	stub := &stubTransport{
		respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
			return newResp(200, "text/event-stream", string(sse)), nil
		},
	}
	rt := NewRoundTripper("https://up.example", "k", WithBaseTransport(stub))
	req := newOAIRequest(t, "https://up.example/chat/completions",
		`{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	assert.Equal(t, "text/event-stream", stub.lastReq.Header.Get("Accept"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	out := string(body)
	assert.Contains(t, out, `"role":"assistant"`)
	assert.Contains(t, out, `"content":"O"`)
	assert.Contains(t, out, `"content":"K"`)
	assert.Contains(t, out, `"finish_reason":"stop"`)
	assert.True(t, strings.HasSuffix(strings.TrimSpace(out), "data: [DONE]"))
}

func TestRoundTrip_UnsupportedPath(t *testing.T) {
	rt := NewRoundTripper("https://up.example", "k", WithBaseTransport(&stubTransport{
		respond: func(_ *http.Request, _ []byte) (*http.Response, error) { return nil, nil },
	}))
	req := newOAIRequest(t, "https://up.example/models", `{}`)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

// TestNew_EndToEndWithSDK drives the real OpenAI SDK client against a stub
// upstream, proving the returned *openai.Client deserialises our output.
func TestNew_EndToEndWithSDK(t *testing.T) {
	stub := &stubTransport{
		respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
			return newResp(200, "application/json", `{
				"id":"msg_e2e","type":"message","role":"assistant","model":"glm-4.5-air",
				"content":[{"type":"text","text":"OK"}],
				"stop_reason":"end_turn","usage":{"input_tokens":8,"output_tokens":2}
			}`), nil
		},
	}
	client := New("https://api.z.ai/api/anthropic", "secret", WithBaseTransport(stub))

	resp, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model: openai.F("glm-4.5-air"),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Reply with exactly: OK"),
		}),
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "OK", resp.Choices[0].Message.Content)
	assert.Equal(t, int64(8), resp.Usage.PromptTokens)
	assert.Equal(t, int64(10), resp.Usage.TotalTokens)
}

// TestNewStreaming_EndToEndWithSDK drives client.Chat.Completions.NewStreaming
// against a stub upstream, proving the OpenAI SDK's SSE decoder parses our
// emitted chunks and accumulates them correctly.
func TestNewStreaming_EndToEndWithSDK(t *testing.T) {
	sse, err := os.ReadFile("testdata/text_stream.sse")
	require.NoError(t, err)
	stub := &stubTransport{
		respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
			return newResp(200, "text/event-stream", string(sse)), nil
		},
	}
	client := New("https://up.example", "k", WithBaseTransport(stub))

	stream := client.Chat.Completions.NewStreaming(context.Background(), openai.ChatCompletionNewParams{
		Model:    openai.F("m"),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")}),
	})
	defer stream.Close()

	var content, finish string
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		content += chunk.Choices[0].Delta.Content
		if chunk.Choices[0].FinishReason != "" {
			finish = string(chunk.Choices[0].FinishReason)
		}
	}
	require.NoError(t, stream.Err())
	assert.Equal(t, "OK", content)
	assert.Equal(t, "stop", finish)
}

func newOAIRequest(t *testing.T, url, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}
