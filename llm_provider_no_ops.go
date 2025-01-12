// Package goai provides utilities for text chunking and AI-powered text processing.
package goai

import "context"

// NoOpsLLMProvider implements LLMProvider interface for testing purposes.
// It provides default responses for both regular and streaming requests.
type NoOpsLLMProvider struct {
	response       LLMResponse
	streamResponse StreamingLLMResponse
	streaming      bool
}

// NoOpsOption defines the function signature for option pattern.
type NoOpsOption func(*NoOpsLLMProvider)

// WithResponse sets a custom LLMResponse for the NoOpsProvider.
func WithResponse(response LLMResponse) NoOpsOption {
	return func(n *NoOpsLLMProvider) {
		n.response = response
	}
}

// WithStreamingResponse sets a custom StreamingLLMResponse for the NoOpsProvider.
func WithStreamingResponse(response StreamingLLMResponse) NoOpsOption {
	return func(n *NoOpsLLMProvider) {
		n.streamResponse = response
		n.streaming = true
	}
}

// NewNoOpsLLMProvider creates a new NoOpsLLMProvider with optional configurations.
func NewNoOpsLLMProvider(opts ...NoOpsOption) *NoOpsLLMProvider {
	provider := &NoOpsLLMProvider{
		response: LLMResponse{
			Text:             "Default NoOps response",
			TotalInputToken:  10,
			TotalOutputToken: 3,
			CompletionTime:   0.1,
		},
		streamResponse: StreamingLLMResponse{
			Text:       "Default NoOps streaming response",
			Done:       true,
			TokenCount: 4,
		},
	}

	for _, opt := range opts {
		opt(provider)
	}

	return provider
}

// GetResponse implements the LLMProvider interface.
func (n *NoOpsLLMProvider) GetResponse(_ []LLMMessage, _ LLMRequestConfig) (LLMResponse, error) {
	return n.response, nil
}

// GetStreamingResponse implements the LLMProvider interface.
func (n *NoOpsLLMProvider) GetStreamingResponse(ctx context.Context, _ []LLMMessage, _ LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	responseChan := make(chan StreamingLLMResponse)

	go func() {
		defer close(responseChan)

		select {
		case <-ctx.Done():
			responseChan <- StreamingLLMResponse{
				Error: ctx.Err(),
				Done:  true,
			}
		default:
			responseChan <- n.streamResponse
		}
	}()

	return responseChan, nil
}
