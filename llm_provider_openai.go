// Package ai provides a flexible interface for interacting with various Language Learning Models (LLMs).
package ai

import (
	"context"
	"time"

	"github.com/openai/openai-go"
)

// OpenAILLMProvider implements the LLMProvider interface using OpenAI's official SDK.
type OpenAILLMProvider struct {
	client OpenAIClient
	model  string
}

// OpenAIProviderConfig holds configuration for OpenAI provider.
type OpenAIProviderConfig struct {
	// Client is the OpenAIClient implementation to use
	Client OpenAIClient
	// Model specifies which OpenAI model to use (e.g., "gpt-4", "gpt-3.5-turbo")
	Model string
}

// NewOpenAILLMProvider creates a new OpenAI provider with the specified configuration.
// If no model is specified, it defaults to GPT-3.5-turbo.
//
// Example usage:
//
//	// Create client
//	client := NewRealOpenAIClient("your-api-key")
//
//	// Create provider with default model
//	provider := NewOpenAILLMProvider(OpenAIProviderConfig{
//	    Client: client,
//	})
//
//	// Create provider with specific model
//	provider := NewOpenAILLMProvider(OpenAIProviderConfig{
//	    Client: client,
//	    Model:  "gpt-4",
//	})
func NewOpenAILLMProvider(config OpenAIProviderConfig) *OpenAILLMProvider {
	if config.Model == "" {
		config.Model = string(openai.ChatModelGPT3_5Turbo)
	}

	return &OpenAILLMProvider{
		client: config.Client,
		model:  config.Model,
	}
}

// convertToOpenAIMessages converts internal message format to OpenAI's format
func (p *OpenAILLMProvider) convertToOpenAIMessages(messages []LLMMessage) []openai.ChatCompletionMessageParamUnion {
	var openAIMessages []openai.ChatCompletionMessageParamUnion
	for _, msg := range messages {
		switch msg.Role {
		case UserRole:
			openAIMessages = append(openAIMessages, openai.UserMessage(msg.Text))
		case AssistantRole:
			openAIMessages = append(openAIMessages, openai.AssistantMessage(msg.Text))
		case SystemRole:
			openAIMessages = append(openAIMessages, openai.SystemMessage(msg.Text))
		default:
			openAIMessages = append(openAIMessages, openai.UserMessage(msg.Text))
		}
	}
	return openAIMessages
}

// createCompletionParams creates OpenAI API parameters from request config
func (p *OpenAILLMProvider) createCompletionParams(messages []openai.ChatCompletionMessageParamUnion, config LLMRequestConfig) openai.ChatCompletionNewParams {
	return openai.ChatCompletionNewParams{
		Messages:    openai.F(messages),
		Model:       openai.F(p.model),
		MaxTokens:   openai.Int(config.MaxToken),
		TopP:        openai.Float(config.TopP),
		Temperature: openai.Float(config.Temperature),
	}
}

// GetResponse generates a response using OpenAI's API for the given messages and configuration.
// It supports different message roles (user, assistant, system) and handles them appropriately.
//
// Example usage:
//
//	messages := []ai.LLMMessage{
//	    {Role: "system", Text: "You are a helpful assistant"},
//	    {Role: "user", Text: "What is the capital of France?"},
//	}
//
//	response, err := provider.GetResponse(messages, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Response: %s\n", response.Text)
func (p *OpenAILLMProvider) GetResponse(messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error) {
	startTime := time.Now()

	openAIMessages := p.convertToOpenAIMessages(messages)
	params := p.createCompletionParams(openAIMessages, config)

	completion, err := p.client.CreateCompletion(context.Background(), params)
	if err != nil {
		return LLMResponse{}, err
	}

	if len(completion.Choices) == 0 {
		return LLMResponse{}, &LLMError{Code: 400, Message: "no choices in response"}
	}

	return LLMResponse{
		Text:             completion.Choices[0].Message.Content,
		TotalInputToken:  int(completion.Usage.PromptTokens),
		TotalOutputToken: int(completion.Usage.CompletionTokens),
		CompletionTime:   time.Since(startTime).Seconds(),
	}, nil
}

// GetStreamingResponse generates a streaming response using OpenAI's API.
// It supports streaming tokens as they're generated and handles context cancellation.
//
// Example usage:
//
//	stream, err := provider.GetStreamingResponse(ctx, messages, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for response := range stream {
//	    if response.Error != nil {
//	        log.Printf("Error: %v", response.Error)
//	        break
//	    }
//	    fmt.Print(response.Text)
//	}
func (p *OpenAILLMProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	openAIMessages := p.convertToOpenAIMessages(messages)
	params := p.createCompletionParams(openAIMessages, config)

	stream := p.client.CreateStreamingCompletion(ctx, params)
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
				chunk := stream.Current()
				if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
					responseChan <- StreamingLLMResponse{
						Text:       chunk.Choices[0].Delta.Content,
						TokenCount: 1,
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			responseChan <- StreamingLLMResponse{
				Error: err,
				Done:  true,
			}
			return
		}

		responseChan <- StreamingLLMResponse{Done: true}
	}()

	return responseChan, nil
}
