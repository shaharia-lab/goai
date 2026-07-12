package anth2oai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_NonStreaming(t *testing.T) {
	stub := &stubTransport{
		respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
			return newResp(200, "application/json", `{
				"id":"msg_h","type":"message","role":"assistant","model":"m",
				"content":[{"type":"text","text":"hello"}],
				"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}
			}`), nil
		},
	}
	h := NewHandler("https://up.example", "k", WithBaseTransport(stub))
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	var out oaiResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.NotNil(t, out.Choices[0].Message.Content)
	assert.Equal(t, "hello", *out.Choices[0].Message.Content)
}

func TestHandler_Streaming(t *testing.T) {
	sse, err := os.ReadFile("testdata/text_stream.sse")
	require.NoError(t, err)
	stub := &stubTransport{
		respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
			return newResp(200, "text/event-stream", string(sse)), nil
		},
	}
	h := NewHandler("https://up.example", "k", WithBaseTransport(stub))
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	out := string(body)
	assert.Contains(t, out, `"content":"O"`)
	assert.True(t, strings.HasSuffix(strings.TrimSpace(out), "data: [DONE]"))
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	h := NewHandler("https://up.example", "k", WithBaseTransport(&stubTransport{
		respond: func(_ *http.Request, _ []byte) (*http.Response, error) { return nil, nil },
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/chat/completions")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}
