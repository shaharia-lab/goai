// Package ai provides a flexible interface for interacting with various Language Learning Models (LLMs).
package ai

import "context"

// LLMRequest handles the configuration and execution of LLM requests.
// It provides a consistent interface for interacting with different LLM providers.
type LLMRequest struct {
	requestConfig LLMRequestConfig
	provider      LLMProvider
}

// NewLLMRequest creates a new LLMRequest with the specified configuration and provider.
// The provider parameter allows injecting different LLM implementations (OpenAI, Anthropic, etc.).
//
// Example usage:
//
//	// Create provider
//	provider := ai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
//	    APIKey: "your-api-key",
//	    Model:  "gpt-3.5-turbo",
//	})
//
//	// Configure request options
//	config := ai.NewRequestConfig(
//	    ai.WithMaxToken(2000),
//	    ai.WithTemperature(0.7),
//	)
//
//	// Create LLM request client
//	llm := ai.NewLLMRequest(config, provider)
func NewLLMRequest(config LLMRequestConfig, provider LLMProvider) *LLMRequest {
	return &LLMRequest{
		requestConfig: config,
		provider:      provider,
	}
}

// Generate sends messages to the configured LLM provider and returns the response.
// It uses the provider and configuration specified during initialization.
//
// Example usage:
//
//	messages := []ai.LLMMessage{
//	    {Role: ai.SystemRole, Text: "You are a helpful assistant"},
//	    {Role: ai.UserRole, Text: "What is the capital of France?"},
//	}
//
//	response, err := llm.Generate(messages)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Response: %s\n", response.Text)
//	fmt.Printf("Tokens used: %d\n", response.TotalOutputToken)
//
// The method returns LLMResponse containing:
//   - Generated text
//   - Token usage statistics
//   - Completion time
//   - Other provider-specific metadata
func (r *LLMRequest) Generate(messages []LLMMessage) (LLMResponse, error) {
	return r.provider.GetResponse(messages, r.requestConfig)
}

// GenerateStream creates a streaming response channel for the given messages.
// It returns a channel that receives StreamingLLMResponse chunks and an error if initialization fails.
//
// Example usage:
//
//	request := NewLLMRequest(config)
//	stream, err := request.GenerateStream(context.Background(), []LLMMessage{
//	    {Role: UserRole, Text: "Tell me a story"},
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for response := range stream {
//	    if response.Error != nil {
//	        log.Printf("Error: %v", response.Error)
//	        break
//	    }
//	    if response.Done {
//	        break
//	    }
//	    fmt.Print(response.Text)
//	}
func (r *LLMRequest) GenerateStream(ctx context.Context, messages []LLMMessage) (<-chan StreamingLLMResponse, error) {
	return r.provider.GetStreamingResponse(ctx, messages, r.requestConfig)
}
