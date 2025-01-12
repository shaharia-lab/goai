// Package ai provides a flexible interface for interacting with various Language Learning Models (LLMs).
package ai

import (
	"context"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// AnthropicLLMProvider implements the LLMProvider interface using Anthropic's official Go SDK.
// It provides access to Claude models through Anthropic's API.
type AnthropicLLMProvider struct {
	client AnthropicClient
	model  anthropic.Model
}

// AnthropicProviderConfig holds the configuration options for creating an Anthropic provider.
type AnthropicProviderConfig struct {
	// Client is the AnthropicClient implementation to use
	Client AnthropicClient

	// Model specifies which Anthropic model to use (e.g., "claude-3-opus-20240229", "claude-3-sonnet-20240229")
	Model anthropic.Model
}

// NewAnthropicLLMProvider creates a new Anthropic provider with the specified configuration.
// If no model is specified, it defaults to Claude 3.5 Sonnet.
//
// Example usage:
//
//	client := NewRealAnthropicClient("your-api-key")
//	provider := NewAnthropicLLMProvider(AnthropicProviderConfig{
//	    Client: client,
//	    Model:  anthropic.ModelClaude_3_5_Sonnet_20240620,
//	})
//
//	response, err := provider.GetResponse(messages, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
func NewAnthropicLLMProvider(config AnthropicProviderConfig) *AnthropicLLMProvider {
	if config.Model == "" {
		config.Model = anthropic.ModelClaude_3_5_Sonnet_20240620
	}

	return &AnthropicLLMProvider{
		client: config.Client,
		model:  config.Model,
	}
}

// prepareMessageParams creates the Anthropic message parameters from LLM messages and config.
// This is an internal helper function to reduce code duplication.
func (p *AnthropicLLMProvider) prepareMessageParams(messages []LLMMessage, config LLMRequestConfig) anthropic.MessageNewParams {
	var anthropicMessages []anthropic.MessageParam
	var systemMessage string

	// Process messages based on their role
	for _, msg := range messages {
		switch msg.Role {
		case SystemRole:
			systemMessage = msg.Text
		case UserRole:
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text)))
		case AssistantRole:
			anthropicMessages = append(anthropicMessages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Text)))
		default:
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text)))
		}
	}

	params := anthropic.MessageNewParams{
		Model:       anthropic.F(p.model),
		Messages:    anthropic.F(anthropicMessages),
		MaxTokens:   anthropic.F(config.MaxToken),
		TopP:        anthropic.Float(config.TopP),
		Temperature: anthropic.Float(config.Temperature),
	}

	// Add system message if present
	if systemMessage != "" {
		params.System = anthropic.F([]anthropic.TextBlockParam{
			anthropic.NewTextBlock(systemMessage),
		})
	}

	return params
}

// GetResponse generates a response using Anthropic's API for the given messages and configuration.
// It supports different message roles (user, assistant, system) and handles them appropriately.
// System messages are handled separately through Anthropic's system parameter.
func (p *AnthropicLLMProvider) GetResponse(messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error) {
	startTime := time.Now()

	params := p.prepareMessageParams(messages, config)
	message, err := p.client.CreateMessage(context.Background(), params)
	if err != nil {
		return LLMResponse{}, err
	}

	var responseText string
	for _, block := range message.Content {
		switch block := block.AsUnion().(type) {
		case anthropic.TextBlock:
			responseText += block.Text
		}
	}

	return LLMResponse{
		Text:             responseText,
		TotalInputToken:  int(message.Usage.InputTokens),
		TotalOutputToken: int(message.Usage.OutputTokens),
		CompletionTime:   time.Since(startTime).Seconds(),
	}, nil
}

// GetStreamingResponse generates a streaming response using Anthropic's API.
// It returns a channel that receives chunks of the response as they're generated.
//
// Example usage:
//
//	client := NewRealAnthropicClient("your-api-key")
//	provider := NewAnthropicLLMProvider(AnthropicProviderConfig{
//	    Client: client,
//	    Model:  anthropic.ModelClaude_3_5_Sonnet_20240620,
//	})
//
//	streamingResp, err := provider.GetStreamingResponse(ctx, messages, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for chunk := range streamingResp {
//	    if chunk.Error != nil {
//	        log.Printf("Error: %v", chunk.Error)
//	        break
//	    }
//	    fmt.Print(chunk.Text)
//	}
func (p *AnthropicLLMProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	params := p.prepareMessageParams(messages, config)
	stream := p.client.CreateStreamingMessage(ctx, params)
	responseChan := make(chan StreamingLLMResponse, 100)

	go func() {
		defer close(responseChan)

		for stream.Next() {
			select {
			case <-ctx.Done():
				responseChan <- StreamingLLMResponse{
					Error: ctx.Err(),
					Done:  true,
				}
				return
			default:
				event := stream.Current()

				switch event.Type {
				case anthropic.MessageStreamEventTypeContentBlockDelta:
					delta, ok := event.Delta.(anthropic.ContentBlockDeltaEventDelta)
					if !ok {
						continue
					}

					if delta.Type == anthropic.ContentBlockDeltaEventDeltaTypeTextDelta && delta.Text != "" {
						responseChan <- StreamingLLMResponse{
							Text:       delta.Text,
							TokenCount: 1,
						}
					}
				case anthropic.MessageStreamEventTypeMessageStop:
					responseChan <- StreamingLLMResponse{Done: true}
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			responseChan <- StreamingLLMResponse{
				Error: err,
				Done:  true,
			}
		}
	}()

	return responseChan, nil
}
