package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/stretchr/testify/assert"
)

// MockAnthropicClient implements AnthropicClient interface for testing
type MockAnthropicClient struct {
	createMessageFunc          func(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error)
	createStreamingMessageFunc func(ctx context.Context, params anthropic.MessageNewParams) *ssestream.Stream[anthropic.MessageStreamEvent]
}

func (m *MockAnthropicClient) CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	if m.createMessageFunc != nil {
		return m.createMessageFunc(ctx, params)
	}
	return nil, nil
}

func (m *MockAnthropicClient) CreateStreamingMessage(ctx context.Context, params anthropic.MessageNewParams) *ssestream.Stream[anthropic.MessageStreamEvent] {
	if m.createStreamingMessageFunc != nil {
		return m.createStreamingMessageFunc(ctx, params)
	}
	return nil
}

type mockEventStream struct {
	events []anthropic.MessageStreamEvent
	index  int
}

// Implement ssestream.Runner interface
func (m *mockEventStream) Run() error {
	return nil
}

type mockDecoder struct {
	events []anthropic.MessageStreamEvent
	index  int
}

func (d *mockDecoder) Event() ssestream.Event {
	if d.index < 0 || d.index >= len(d.events) {
		return ssestream.Event{}
	}

	event := d.events[d.index]

	// Create a custom payload that can be unmarshaled correctly
	payload := map[string]interface{}{
		"type":  event.Type,
		"delta": event.Delta,
		"index": event.Index,
	}

	if event.Type == anthropic.MessageStreamEventTypeMessageStart {
		payload["message"] = event.Message
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return ssestream.Event{}
	}

	return ssestream.Event{
		Type: string(event.Type),
		Data: data,
	}
}

func (d *mockDecoder) Next() bool {
	d.index++
	return d.index < len(d.events)
}

func (d *mockDecoder) Err() error {
	return nil
}

func (d *mockDecoder) Close() error {
	return nil
}

func TestAnthropicLLMProvider_NewAnthropicLLMProvider(t *testing.T) {
	tests := []struct {
		name          string
		config        AnthropicProviderConfig
		expectedModel anthropic.Model
	}{
		{
			name: "with specified model",
			config: AnthropicProviderConfig{
				Client: &MockAnthropicClient{},
				Model:  "claude-3-opus-20240229",
			},
			expectedModel: "claude-3-opus-20240229",
		},
		{
			name: "with default model",
			config: AnthropicProviderConfig{
				Client: &MockAnthropicClient{},
			},
			expectedModel: anthropic.ModelClaude_3_5_Sonnet_20240620,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewAnthropicLLMProvider(tt.config)

			assert.Equal(t, tt.expectedModel, provider.model, "unexpected model")
			assert.NotNil(t, provider.client, "expected client to be initialized")
		})
	}
}

func TestAnthropicLLMProvider_GetResponse(t *testing.T) {
	tests := []struct {
		name           string
		messages       []LLMMessage
		config         LLMRequestConfig
		expectedResult LLMResponse
		expectError    bool
	}{
		{
			name: "successful response with all message types",
			messages: []LLMMessage{
				{Role: SystemRole, Text: "You are a helpful assistant"},
				{Role: UserRole, Text: "Hello"},
				{Role: AssistantRole, Text: "Hi there"},
			},
			config: LLMRequestConfig{
				MaxToken:    100,
				TopP:        0.9,
				Temperature: 0.7,
			},
			expectedResult: LLMResponse{
				Text:             "Test response",
				TotalInputToken:  10,
				TotalOutputToken: 5,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockAnthropicClient{
				createMessageFunc: func(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
					message := &anthropic.Message{
						Role:  anthropic.MessageRoleAssistant,
						Model: anthropic.ModelClaude_3_5_Sonnet_20240620,
						Usage: anthropic.Usage{
							InputTokens:  10,
							OutputTokens: 5,
						},
						Type: anthropic.MessageTypeMessage,
					}

					block := anthropic.ContentBlock{}
					if err := block.UnmarshalJSON([]byte(`{
						"type": "text",
						"text": "Test response"
					}`)); err != nil {
						t.Fatal(err)
					}

					message.Content = []anthropic.ContentBlock{block}
					return message, nil
				},
			}

			provider := NewAnthropicLLMProvider(AnthropicProviderConfig{
				Client: mockClient,
				Model:  anthropic.ModelClaude_3_5_Sonnet_20240620,
			})

			result, err := provider.GetResponse(tt.messages, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedResult.Text, result.Text)
			assert.Equal(t, tt.expectedResult.TotalInputToken, result.TotalInputToken)
			assert.Equal(t, tt.expectedResult.TotalOutputToken, result.TotalOutputToken)
			assert.Greater(t, result.CompletionTime, float64(0), "completion time should be greater than 0")
		})
	}
}

func TestAnthropicLLMProvider_GetStreamingResponse(t *testing.T) {
	tests := []struct {
		name        string
		messages    []LLMMessage
		config      LLMRequestConfig
		streamText  []string
		expectError bool
	}{
		{
			name: "successful streaming response",
			messages: []LLMMessage{
				{Role: UserRole, Text: "Hello"},
			},
			config: LLMRequestConfig{
				MaxToken:    100,
				TopP:        0.9,
				Temperature: 0.7,
			},
			streamText: []string{"Hello", " world", "!"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockAnthropicClient{
				createStreamingMessageFunc: func(ctx context.Context, params anthropic.MessageNewParams) *ssestream.Stream[anthropic.MessageStreamEvent] {
					var events []anthropic.MessageStreamEvent

					// Create start event
					events = append(events, anthropic.MessageStreamEvent{
						Type: anthropic.MessageStreamEventTypeMessageStart,
						Message: anthropic.Message{
							Role:  anthropic.MessageRoleAssistant,
							Model: anthropic.ModelClaude_3_5_Sonnet_20240620,
						},
					})

					// Create content block delta events
					for i, text := range tt.streamText {
						t.Logf("Adding delta event %d with text: %q", i, text)
						events = append(events, anthropic.MessageStreamEvent{
							Type:  anthropic.MessageStreamEventTypeContentBlockDelta,
							Index: int64(i),
							Delta: anthropic.ContentBlockDeltaEventDelta{
								Type: anthropic.ContentBlockDeltaEventDeltaTypeTextDelta,
								Text: text,
							},
						})
					}

					// Add stop event
					events = append(events, anthropic.MessageStreamEvent{
						Type: anthropic.MessageStreamEventTypeMessageStop,
					})

					decoder := &mockDecoder{
						events: events,
						index:  -1,
					}

					stream := ssestream.NewStream[anthropic.MessageStreamEvent](decoder, nil)
					return stream
				},
			}

			provider := NewAnthropicLLMProvider(AnthropicProviderConfig{
				Client: mockClient,
				Model:  anthropic.ModelClaude_3_5_Sonnet_20240620,
			})

			ctx := context.Background()
			stream, err := provider.GetStreamingResponse(ctx, tt.messages, tt.config)
			assert.NoError(t, err)

			var receivedText string
			for chunk := range stream {
				t.Logf("Received streaming chunk: %+v", chunk)
				if chunk.Error != nil {
					t.Fatalf("Unexpected error: %v", chunk.Error)
				}
				if !chunk.Done {
					receivedText += chunk.Text
					t.Logf("Current accumulated text: %q", receivedText)
				}
			}

			t.Logf("Final text: %q", receivedText)
			assert.Equal(t, strings.Join(tt.streamText, ""), receivedText)
		})
	}
}

func createStreamEvent(eventType string, index int64, text string) anthropic.MessageStreamEvent {
	var event anthropic.MessageStreamEvent

	switch eventType {
	case "message_start":
		event = anthropic.MessageStreamEvent{
			Type: anthropic.MessageStreamEventTypeMessageStart,
			Message: anthropic.Message{
				Role:  anthropic.MessageRoleAssistant,
				Model: anthropic.ModelClaude_3_5_Sonnet_20240620,
			},
		}
	case "content_block_delta":
		/*textDelta := anthropic.TextDelta{
			Type: anthropic.TextDeltaTypeTextDelta,
			Text: text,
		}*/
		event = anthropic.MessageStreamEvent{
			Type:  anthropic.MessageStreamEventTypeContentBlockDelta,
			Index: index,
			Delta: anthropic.ContentBlockDeltaEventDelta{
				Type: anthropic.ContentBlockDeltaEventDeltaTypeTextDelta,
				Text: text,
			},
		}
	case "content_block_stop":
		event = anthropic.MessageStreamEvent{
			Type:  anthropic.MessageStreamEventTypeContentBlockStop,
			Index: index,
		}
	case "message_stop":
		event = anthropic.MessageStreamEvent{
			Type: anthropic.MessageStreamEventTypeMessageStop,
		}
	}

	return event
}
