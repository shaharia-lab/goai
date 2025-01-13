// Package goai provides a flexible interface for interacting with various Language Learning Models (LLMs).
package goai

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
)

// OpenAIClientProvider defines the interface for interacting with OpenAI's API.
// This interface abstracts the essential operations used by OpenAILLMProvider.
type OpenAIClientProvider interface {
	// CreateCompletion creates a new chat completion using OpenAI's API.
	CreateCompletion(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)

	// CreateStreamingCompletion creates a streaming chat completion using OpenAI's API.
	CreateStreamingCompletion(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk]
}

// OpenAIClient implements the OpenAIClientProvider interface using OpenAI's official SDK.
type OpenAIClient struct {
	client *openai.Client
}

// NewOpenAIClient creates a new instance of OpenAIClient with the provided API key
// and optional client options.
//
// Example usage:
//
//	// Basic usage with API key
//	client := NewOpenAIClient("your-api-key")
//
//	// Usage with custom HTTP client
//	httpClient := &http.Client{Timeout: 30 * time.Second}
//	client := NewOpenAIClient(
//	    "your-api-key",
//	    option.WithHTTPClient(httpClient),
//	)
func NewOpenAIClient(apiKey string, opts ...option.RequestOption) *OpenAIClient {
	opts = append(opts, option.WithAPIKey(apiKey))
	return &OpenAIClient{
		client: openai.NewClient(opts...),
	}
}

// CreateCompletion implements the OpenAIClientProvider interface using the OpenAI client.
func (c *OpenAIClient) CreateCompletion(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return c.client.Chat.Completions.New(ctx, params)
}

// CreateStreamingCompletion implements the streaming support for the OpenAIClientProvider interface.
func (c *OpenAIClient) CreateStreamingCompletion(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	return c.client.Chat.Completions.NewStreaming(ctx, params)
}
