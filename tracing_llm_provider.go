package goai

import (
	"context"
	"github.com/shaharia-lab/goai/observability"
	"go.opentelemetry.io/otel/attribute"
	"time"
)

// TracingLLMProvider implements the decorator pattern for tracing
type TracingLLMProvider struct {
	provider LLMProvider
}

// NewTracingLLMProvider creates a new tracing decorator for any LLMProvider
func NewTracingLLMProvider(provider LLMProvider) *TracingLLMProvider {
	return &TracingLLMProvider{
		provider: provider,
	}
}

// GetResponse implements LLMProvider interface with added tracing
func (t *TracingLLMProvider) GetResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error) {
	ctx, span := observability.StartSpan(ctx, "LLMProvider.GetResponse")
	defer span.End()

	startTime := time.Now()

	// Call the underlying provider
	response, err := t.provider.GetResponse(ctx, messages, config)

	if err != nil {
		span.RecordError(err)
		return LLMResponse{}, err
	}

	span.SetAttributes(
		attribute.Int("total_input_token", response.TotalInputToken),
		attribute.Int("total_output_token", response.TotalOutputToken),
		attribute.Int("message_count", len(messages)),
		attribute.Float64("completion_time", time.Since(startTime).Seconds()),
		attribute.Int64("max_token", config.MaxToken),
		attribute.Float64("temperature", config.Temperature),
		attribute.Float64("top_p", config.TopP),
		attribute.Int64("top_k", config.TopK),
	)

	return response, nil
}

// GetStreamingResponse implements LLMProvider interface with added tracing
func (t *TracingLLMProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	ctx, span := observability.StartSpan(ctx, "LLMProvider.GetStreamingResponse")

	startTime := time.Now()

	// Get the original stream
	originalStream, err := t.provider.GetStreamingResponse(ctx, messages, config)
	if err != nil {
		span.RecordError(err)
		span.End()
		return nil, err
	}

	tracedStream := make(chan StreamingLLMResponse)

	go func() {
		defer span.End()
		defer close(tracedStream)

		var totalTokens int

		for response := range originalStream {
			if response.Error != nil {
				span.RecordError(response.Error)
				tracedStream <- response
				return
			}

			totalTokens += len(response.Text)
			tracedStream <- response

			if response.Done {
				span.SetAttributes(
					attribute.Int("total_tokens", totalTokens),
					attribute.Float64("total_streaming_time", time.Since(startTime).Seconds()),
					attribute.Int("message_count", len(messages)),
				)
				return
			}
		}
	}()

	return tracedStream, nil
}
