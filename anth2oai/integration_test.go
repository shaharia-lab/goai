//go:build integration

// Package anth2oai live integration tests. These run against a real
// Anthropic-format endpoint (validated with GLM/Z.ai) and are skipped unless
// both ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN are set.
//
// Run locally after exporting your credentials, e.g.:
//
//	export ANTHROPIC_BASE_URL=https://api.z.ai/api/anthropic
//	export ANTHROPIC_AUTH_TOKEN=<your token>
//	go test -tags=integration ./anth2oai/...
//
// The test never reads any settings file — credentials come only from the
// environment.
package anth2oai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const integrationModel = "glm-4.5-air"

func integrationCreds(t *testing.T) (base, token string) {
	t.Helper()
	base = os.Getenv("ANTHROPIC_BASE_URL")
	token = os.Getenv("ANTHROPIC_AUTH_TOKEN")
	if base == "" || token == "" {
		t.Skip("set ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN to run integration tests")
	}
	return base, token
}

func TestIntegration_NonStreaming(t *testing.T) {
	base, token := integrationCreds(t)
	client := New(base, token)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.F(integrationModel),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Reply with exactly: OK"),
		}),
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.NotEmpty(t, resp.Choices[0].Message.Content)
	assert.NotZero(t, resp.Usage.TotalTokens)
}

func TestIntegration_Streaming(t *testing.T) {
	base, token := integrationCreds(t)
	client := New(base, token)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stream := client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Model: openai.F(integrationModel),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Count to three."),
		}),
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
	assert.NotEmpty(t, content)
	assert.NotEmpty(t, finish)
}

func TestIntegration_ToolCall(t *testing.T) {
	base, token := integrationCreds(t)
	client := New(base, token)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.F(integrationModel),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("What is the weather in Paris? Use the tool."),
		}),
		Tools: openai.F([]openai.ChatCompletionToolParam{{
			Type: openai.F(openai.ChatCompletionToolTypeFunction),
			Function: openai.F(openai.FunctionDefinitionParam{
				Name:        openai.F("get_weather"),
				Description: openai.F("Get the current weather for a location"),
				Parameters: openai.F(openai.FunctionParameters{
					"type":       "object",
					"properties": map[string]any{"location": map[string]any{"type": "string"}},
					"required":   []string{"location"},
				}),
			}),
		}}),
		ToolChoice: openai.F[openai.ChatCompletionToolChoiceOptionUnionParam](
			openai.ChatCompletionToolChoiceOptionAutoRequired),
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	require.NotEmpty(t, resp.Choices[0].Message.ToolCalls)
	call := resp.Choices[0].Message.ToolCalls[0]
	assert.Equal(t, "get_weather", call.Function.Name)
	var args map[string]any
	assert.NoError(t, json.Unmarshal([]byte(call.Function.Arguments), &args), "arguments must be valid JSON")
}

func TestIntegration_Handler(t *testing.T) {
	base, token := integrationCreds(t)
	srv := httptest.NewServer(NewHandler(base, token))
	defer srv.Close()

	// Non-streaming through the proxy handler.
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{
		"model":"`+integrationModel+`",
		"messages":[{"role":"user","content":"Reply with exactly: OK"}]
	}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Equal(t, "chat.completion", out["object"])

	// Streaming through the proxy handler.
	sresp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{
		"model":"`+integrationModel+`",
		"messages":[{"role":"user","content":"Count to three."}],
		"stream":true
	}`))
	require.NoError(t, err)
	defer sresp.Body.Close()
	body, err := io.ReadAll(sresp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "chat.completion.chunk")
	assert.Contains(t, string(body), "data: [DONE]")
}
