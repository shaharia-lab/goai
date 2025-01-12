package ai

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
)

// MockOpenAIClient implements OpenAIClient interface for testing
type MockOpenAIClient struct {
	client *openai.Client
}

func NewMockOpenAIClient(transport http.RoundTripper) *MockOpenAIClient {
	return &MockOpenAIClient{
		client: openai.NewClient(
			option.WithHTTPClient(&http.Client{Transport: transport}),
		),
	}
}

func (m *MockOpenAIClient) CreateCompletion(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return m.client.Chat.Completions.New(ctx, params)
}

func (m *MockOpenAIClient) CreateStreamingCompletion(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	return m.client.Chat.Completions.NewStreaming(ctx, params)
}

type mockTransport struct {
	responses []string
	delay     time.Duration
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for _, resp := range m.responses {
			time.Sleep(10 * time.Millisecond) // Simulate streaming delay
			pw.Write([]byte(resp + "\n"))
		}
	}()

	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: pr,
	}, nil
}

func TestOpenAILLMProvider_NewOpenAILLMProvider(t *testing.T) {
	mockClient := NewMockOpenAIClient(http.DefaultTransport)

	tests := []struct {
		name          string
		config        OpenAIProviderConfig
		expectedModel string
	}{
		{
			name: "with specified model",
			config: OpenAIProviderConfig{
				Client: mockClient,
				Model:  "gpt-4",
			},
			expectedModel: "gpt-4",
		},
		{
			name: "with default model",
			config: OpenAIProviderConfig{
				Client: mockClient,
			},
			expectedModel: string(openai.ChatModelGPT3_5Turbo),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewOpenAILLMProvider(tt.config)

			if provider.model != tt.expectedModel {
				t.Errorf("expected model %q, got %q", tt.expectedModel, provider.model)
			}
			if provider.client == nil {
				t.Error("expected client to be initialized")
			}
		})
	}
}

func TestOpenAILLMProvider_GetStreamingResponse(t *testing.T) {
	tests := []struct {
		name     string
		messages []LLMMessage
		timeout  time.Duration
		delay    time.Duration
		wantErr  bool
	}{
		{
			name:    "successful streaming",
			timeout: 100 * time.Millisecond,
			delay:   0,
		},
		{
			name:    "context cancellation",
			timeout: 5 * time.Millisecond,
			delay:   50 * time.Millisecond,
		},
	}

	responses := []string{
		`data: {"id":"123","choices":[{"delta":{"content":"Hello"}}]}`,
		`data: {"id":"123","choices":[{"delta":{"content":" world"}}]}`,
		`data: [DONE]`,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client with custom transport
			mockClient := NewMockOpenAIClient(&mockTransport{
				responses: responses,
				delay:     tt.delay,
			})

			provider := NewOpenAILLMProvider(OpenAIProviderConfig{
				Client: mockClient,
				Model:  "gpt-4",
			})

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			stream, err := provider.GetStreamingResponse(ctx, []LLMMessage{{Role: UserRole, Text: "test"}}, LLMRequestConfig{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var gotCancel bool
			for resp := range stream {
				if resp.Error != nil {
					gotCancel = true
					break
				}
			}

			if tt.delay > tt.timeout && !gotCancel {
				t.Error("expected context cancellation")
			}
		})
	}
}
