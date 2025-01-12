// Package ai provides a flexible interface for interacting with various Language Learning Models (LLMs).
package ai

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

// AnthropicClient defines the interface for interacting with Anthropic's API.
// This interface abstracts the essential message-related operations used by AnthropicLLMProvider.
type AnthropicClient interface {
	// CreateMessage creates a new message using Anthropic's API.
	// The method takes a context and MessageNewParams and returns a Message response or an error.
	CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error)

	// CreateStreamingMessage creates a streaming message using Anthropic's API.
	// It returns a stream that can be used to receive message chunks as they're generated.
	CreateStreamingMessage(ctx context.Context, params anthropic.MessageNewParams) *ssestream.Stream[anthropic.MessageStreamEvent]
}

// RealAnthropicClient implements the AnthropicClient interface using Anthropic's official SDK.
type RealAnthropicClient struct {
	messages *anthropic.MessageService
}

// NewRealAnthropicClient creates a new instance of RealAnthropicClient with the provided API key.
//
// Example usage:
//
//	// Regular message generation
//	client := NewRealAnthropicClient("your-api-key")
//	provider := NewAnthropicLLMProvider(AnthropicProviderConfig{
//	    Client: client,
//	    Model:  "claude-3-sonnet-20240229",
//	})
//
//	// Streaming message generation
//	streamingResp, err := provider.GetStreamingResponse(ctx, messages, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for chunk := range streamingResp {
//	    fmt.Print(chunk.Text)
//	}
func NewRealAnthropicClient(apiKey string) *RealAnthropicClient {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &RealAnthropicClient{
		messages: client.Messages,
	}
}

// CreateMessage implements the AnthropicClient interface using the real Anthropic client.
func (c *RealAnthropicClient) CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	return c.messages.New(ctx, params)
}

// CreateStreamingMessage implements the streaming support for the AnthropicClient interface.
func (c *RealAnthropicClient) CreateStreamingMessage(ctx context.Context, params anthropic.MessageNewParams) *ssestream.Stream[anthropic.MessageStreamEvent] {
	return c.messages.NewStreaming(ctx, params)
}
