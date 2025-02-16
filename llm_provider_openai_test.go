package goai

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shaharia-lab/goai/mcp"
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
)

// MockOpenAIClient implements OpenAIClientProvider interface for testing
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

func (m *mockTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
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

func TestOpenAILLMProvider_GetResponse_WithTools(t *testing.T) {
	tests := []struct {
		name           string
		messages       []LLMMessage
		config         LLMRequestConfig
		responses      []string
		expectedError  error
		expectedResult LLMResponse
	}{
		{
			name: "successful tool execution",
			messages: []LLMMessage{
				{Role: UserRole, Text: "What's the weather in New York?"},
			},
			config: LLMRequestConfig{
				MaxToken:    100,
				Temperature: 0.7,
				toolsProvider: func() *ToolsProvider {
					provider := NewToolsProvider()
					_ = provider.AddTools([]mcp.Tool{
						{
							Name:        "get_weather",
							Description: "Get weather information",
							InputSchema: json.RawMessage(`{
								"type": "object",
								"properties": {
									"location": {"type": "string"}
								},
								"required": ["location"]
							}`),
							Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
								var input struct {
									Location string `json:"location"`
								}
								json.Unmarshal(params.Arguments, &input)
								return mcp.CallToolResult{
									Content: []mcp.ToolResultContent{{
										Type: "text",
										Text: fmt.Sprintf("The weather in %s is sunny and 25°C", input.Location),
									}},
								}, nil
							},
						},
					})
					return provider
				}(),
			},
			responses: []string{
				`{
					"choices": [{
						"message": {
							"tool_calls": [{
								"id": "call_123",
								"function": {
									"name": "get_weather",
									"arguments": "{\"location\":\"New York\"}"
								}
							}]
						}
					}],
					"usage": {
						"prompt_tokens": 10,
						"completion_tokens": 20
					}
				}`,
				`{
					"choices": [{
						"message": {
							"content": "The weather in New York is sunny and 25°C"
						}
					}],
					"usage": {
						"prompt_tokens": 30,
						"completion_tokens": 40
					}
				}`,
			},
			expectedResult: LLMResponse{
				Text:             "The weather in New York is sunny and 25°C",
				TotalInputToken:  30,
				TotalOutputToken: 40,
			},
		},
		{
			name: "no tool calls needed",
			messages: []LLMMessage{
				{Role: UserRole, Text: "Hello"},
			},
			config: LLMRequestConfig{
				MaxToken:      100,
				Temperature:   0.7,
				toolsProvider: NewToolsProvider(),
			},
			responses: []string{
				`{
					"choices": [{
						"message": {
							"content": "Hi there!"
						}
					}],
					"usage": {
						"prompt_tokens": 5,
						"completion_tokens": 10
					}
				}`,
			},
			expectedResult: LLMResponse{
				Text:             "Hi there!",
				TotalInputToken:  5,
				TotalOutputToken: 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock transport
			responseIndex := 0
			mockTransport := &MockRoundTripper{
				RoundTripFunc: func(req *http.Request) (*http.Response, error) {
					if responseIndex >= len(tt.responses) {
						return nil, fmt.Errorf("no more mock responses")
					}

					resp := &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(tt.responses[responseIndex])),
						Header:     make(http.Header),
					}
					resp.Header.Set("Content-Type", "application/json")
					responseIndex++
					return resp, nil
				},
			}

			// Create mock client with transport
			mockClient := NewMockOpenAIClient(mockTransport)

			// Create provider
			provider := NewOpenAILLMProvider(OpenAIProviderConfig{
				Client: mockClient,
				Model:  "gpt-3.5-turbo",
			})

			// Execute test
			result, err := provider.GetResponse(tt.messages, tt.config)

			// Check error
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
				return
			}

			assert.NoError(t, err)

			// Check result content
			assert.Equal(t, tt.expectedResult.Text, result.Text)
			assert.Equal(t, tt.expectedResult.TotalInputToken, result.TotalInputToken)
			assert.Equal(t, tt.expectedResult.TotalOutputToken, result.TotalOutputToken)
			assert.Greater(t, result.CompletionTime, float64(0))
		})
	}
}

// MockRoundTripper implements http.RoundTripper for testing
type MockRoundTripper struct {
	RoundTripFunc func(*http.Request) (*http.Response, error)
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}
