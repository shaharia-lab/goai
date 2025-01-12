// Package ai provides a flexible interface for interacting with various Language Learning Models (LLMs).
package ai

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
)

// OpenAIClient defines the interface for interacting with OpenAI's API.
// This interface abstracts the essential operations used by OpenAILLMProvider.
type OpenAIClient interface {
	// CreateCompletion creates a new chat completion using OpenAI's API.
	CreateCompletion(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)

	// CreateStreamingCompletion creates a streaming chat completion using OpenAI's API.
	CreateStreamingCompletion(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk]
}

// RealOpenAIClient implements the OpenAIClient interface using OpenAI's official SDK.
type RealOpenAIClient struct {
	client *openai.Client
}

// NewRealOpenAIClient creates a new instance of RealOpenAIClient with the provided API key
// and optional client options.
//
// Example usage:
//
//	// Basic usage with API key
//	client := NewRealOpenAIClient("your-api-key")
//
//	// Usage with custom HTTP client
//	httpClient := &http.Client{Timeout: 30 * time.Second}
//	client := NewRealOpenAIClient(
//	    "your-api-key",
//	    option.WithHTTPClient(httpClient),
//	)
func NewRealOpenAIClient(apiKey string, opts ...option.RequestOption) *RealOpenAIClient {
	opts = append(opts, option.WithAPIKey(apiKey))
	return &RealOpenAIClient{
		client: openai.NewClient(opts...),
	}
}

// CreateCompletion implements the OpenAIClient interface using the real OpenAI client.
func (c *RealOpenAIClient) CreateCompletion(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return c.client.Chat.Completions.New(ctx, params)
}

// CreateStreamingCompletion implements the streaming support for the OpenAIClient interface.
func (c *RealOpenAIClient) CreateStreamingCompletion(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	return c.client.Chat.Completions.NewStreaming(ctx, params)
}
