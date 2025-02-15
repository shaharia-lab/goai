package goai

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

// AnthropicClientProvider defines the interface for interacting with Anthropic's API.
// This interface abstracts the essential message-related operations used by AnthropicLLMProvider.
type AnthropicClientProvider interface {
	// CreateMessage creates a new message using Anthropic's API.
	// The method takes a context and MessageNewParams and returns a Message response or an error.
	CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error)

	// CreateStreamingMessage creates a streaming message using Anthropic's API.
	// It returns a stream that can be used to receive message chunks as they're generated.
	CreateStreamingMessage(ctx context.Context, params anthropic.MessageNewParams) *ssestream.Stream[anthropic.MessageStreamEvent]
}

// AnthropicClient implements the AnthropicClientProvider interface using Anthropic's official SDK.
type AnthropicClient struct {
	messages *anthropic.MessageService
}

// NewAnthropicClient creates a new instance of AnthropicClient with the provided API key.
//
// Example usage:
//
//	// Regular message generation
//	client := NewAnthropicClient("your-api-key")
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
func NewAnthropicClient(apiKey string) *AnthropicClient {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &AnthropicClient{
		messages: client.Messages,
	}
}

// CreateMessage implements the AnthropicClientProvider interface using the Anthropic client.
func (c *AnthropicClient) CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	return c.messages.New(ctx, params)
}

// CreateStreamingMessage implements the streaming support for the AnthropicClientProvider interface.
func (c *AnthropicClient) CreateStreamingMessage(ctx context.Context, params anthropic.MessageNewParams) *ssestream.Stream[anthropic.MessageStreamEvent] {
	return c.messages.NewStreaming(ctx, params)
}
