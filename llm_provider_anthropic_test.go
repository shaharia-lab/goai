package goai

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/shaharia-lab/goai/mcp"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/stretchr/testify/assert"
)

// MockAnthropicClient implements AnthropicClientProvider interface for testing
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

	payload := make(map[string]interface{})
	payload["type"] = event.Type
	payload["index"] = event.Index

	switch event.Type {
	case anthropic.MessageStreamEventTypeMessageStart:
		payload["message"] = event.Message
	case anthropic.MessageStreamEventTypeContentBlockStart:
		payload["content_block"] = event.ContentBlock
	case anthropic.MessageStreamEventTypeContentBlockDelta:
		payload["delta"] = event.Delta
	case anthropic.MessageStreamEventTypeContentBlockStop:
		// No additional fields
	case anthropic.MessageStreamEventTypeMessageDelta:
		payload["delta"] = event.Delta
	case anthropic.MessageStreamEventTypeMessageStop:
		// No additional fields
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
				MaxToken:      100,
				TopP:          0.9,
				Temperature:   0.7,
				toolsProvider: NewToolsProvider(),
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
				createMessageFunc: func(_ context.Context, _ anthropic.MessageNewParams) (*anthropic.Message, error) {
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

			result, err := provider.GetResponse(context.Background(), tt.messages, tt.config)

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

func TestAnthropicLLMProvider_GetResponse_WithTools(t *testing.T) {
	tools := []mcp.Tool{
		{
			Name:        "test_tool",
			Description: "Test tool for unit testing",
			InputSchema: json.RawMessage(`{
        "type": "object",
        "properties": {
            "location": {"type": "string"}
        },
        "required": ["location"]
    }`),
			Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
				var input struct {
					Input string `json:"input"`
				}
				if err := json.Unmarshal(params.Arguments, &input); err != nil {
					return mcp.CallToolResult{}, err
				}

				return mcp.CallToolResult{
					Content: []mcp.ToolResultContent{
						{
							Type: "text",
							Text: "Tool execution completed successfully",
						},
					},
				}, nil
			},
		},
	}

	toolsProvider := NewToolsProvider()
	_ = toolsProvider.AddTools(tools)

	// Helper function to create a tool use block
	createToolUseBlock := func() []byte {
		return []byte(`{
            "type": "tool_use",
            "id": "tool_call_1",
            "name": "test_tool",
            "input": {"input": "test input"}
        }`)
	}

	// Helper function to create a text block
	createTextBlock := func(text string) []byte {
		return []byte(fmt.Sprintf(`{
            "type": "text",
            "text": %q
        }`, text))
	}

	tests := []struct {
		name           string
		messages       []LLMMessage
		config         LLMRequestConfig
		mockResponses  []*anthropic.Message
		expectedResult LLMResponse
		expectError    bool
	}{
		{
			name: "successful tool use and response",
			messages: []LLMMessage{
				{Role: UserRole, Text: "Use the test tool"},
			},
			config: LLMRequestConfig{
				MaxToken:      100,
				TopP:          0.9,
				Temperature:   0.7,
				toolsProvider: toolsProvider,
			},
			mockResponses: []*anthropic.Message{
				{
					Role:  anthropic.MessageRoleAssistant,
					Model: anthropic.ModelClaude_3_5_Sonnet_20240620,
					Content: func() []anthropic.ContentBlock {
						var block anthropic.ContentBlock
						if err := block.UnmarshalJSON(createToolUseBlock()); err != nil {
							t.Fatal(err)
						}
						return []anthropic.ContentBlock{block}
					}(),
					Usage: anthropic.Usage{
						InputTokens:  10,
						OutputTokens: 5,
					},
					StopReason: anthropic.MessageStopReasonToolUse,
				},
				{
					Role:  anthropic.MessageRoleAssistant,
					Model: anthropic.ModelClaude_3_5_Sonnet_20240620,
					Content: func() []anthropic.ContentBlock {
						var block anthropic.ContentBlock
						if err := block.UnmarshalJSON(createTextBlock("Tool execution completed successfully")); err != nil {
							t.Fatal(err)
						}
						return []anthropic.ContentBlock{block}
					}(),
					Usage: anthropic.Usage{
						InputTokens:  15,
						OutputTokens: 8,
					},
					StopReason: anthropic.MessageStopReasonEndTurn,
				},
			},
			expectedResult: LLMResponse{
				Text:             "Tool execution completed successfully",
				TotalInputToken:  25,
				TotalOutputToken: 13,
			},
			expectError: false,
		},
		{
			name: "tool not found error",
			messages: []LLMMessage{
				{Role: UserRole, Text: "Use a tool"},
			},
			config: LLMRequestConfig{
				MaxToken:      100,
				TopP:          0.9,
				Temperature:   0.7,
				toolsProvider: NewToolsProvider(),
			},
			mockResponses: []*anthropic.Message{
				{
					Role:  anthropic.MessageRoleAssistant,
					Model: anthropic.ModelClaude_3_5_Sonnet_20240620,
					Content: func() []anthropic.ContentBlock {
						toolUseBlock := []byte(`{
                            "type": "tool_use",
                            "id": "tool_call_1",
                            "name": "nonexistent_tool",
                            "input": {"input": "test"}
                        }`)
						var block anthropic.ContentBlock
						if err := block.UnmarshalJSON(toolUseBlock); err != nil {
							t.Fatal(err)
						}
						return []anthropic.ContentBlock{block}
					}(),
					Usage: anthropic.Usage{
						InputTokens:  10,
						OutputTokens: 5,
					},
					StopReason: anthropic.MessageStopReasonToolUse,
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responseIndex := 0
			mockClient := &MockAnthropicClient{
				createMessageFunc: func(_ context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
					if responseIndex >= len(tt.mockResponses) {
						t.Fatal("more responses requested than provided in test case")
					}
					response := tt.mockResponses[responseIndex]
					responseIndex++
					return response, nil
				},
			}

			provider := NewAnthropicLLMProvider(AnthropicProviderConfig{
				Client: mockClient,
				Model:  anthropic.ModelClaude_3_5_Sonnet_20240620,
			})

			result, err := provider.GetResponse(context.Background(), tt.messages, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedResult.Text, result.Text)
			assert.Equal(t, tt.expectedResult.TotalInputToken, result.TotalInputToken)
			assert.Equal(t, tt.expectedResult.TotalOutputToken, result.TotalOutputToken)
			assert.Greater(t, result.CompletionTime, float64(0))
		})
	}
}

func TestAnthropicLLMProvider_GetStreamingResponse_Basic(t *testing.T) {
	mockClient := &MockAnthropicClient{
		createStreamingMessageFunc: func(_ context.Context, _ anthropic.MessageNewParams) *ssestream.Stream[anthropic.MessageStreamEvent] {
			events := []anthropic.MessageStreamEvent{
				{
					Type: anthropic.MessageStreamEventTypeMessageStart,
					Message: anthropic.Message{
						Role:  anthropic.MessageRoleAssistant,
						Model: anthropic.ModelClaude_3_5_Sonnet_20240620,
					},
				},
				{
					Type:         anthropic.MessageStreamEventTypeContentBlockStart,
					Index:        0,
					ContentBlock: anthropic.TextBlock{Type: anthropic.TextBlockTypeText, Text: ""},
				},
				{
					Type:  anthropic.MessageStreamEventTypeContentBlockDelta,
					Index: 0,
					Delta: anthropic.ContentBlockDeltaEventDelta{
						Type: anthropic.ContentBlockDeltaEventDeltaTypeTextDelta,
						Text: "Hello",
					},
				},
				{
					Type:  anthropic.MessageStreamEventTypeContentBlockDelta,
					Index: 0,
					Delta: anthropic.ContentBlockDeltaEventDelta{
						Type: anthropic.ContentBlockDeltaEventDeltaTypeTextDelta,
						Text: " world",
					},
				},
				{
					Type:  anthropic.MessageStreamEventTypeContentBlockDelta,
					Index: 0,
					Delta: anthropic.ContentBlockDeltaEventDelta{
						Type: anthropic.ContentBlockDeltaEventDeltaTypeTextDelta,
						Text: "!",
					},
				},
				{
					Type:  anthropic.MessageStreamEventTypeContentBlockStop,
					Index: 0,
				},
				{
					Type: anthropic.MessageStreamEventTypeMessageStop,
				},
			}

			decoder := &mockDecoder{events: events, index: -1}
			return ssestream.NewStream[anthropic.MessageStreamEvent](decoder, nil)
		},
	}

	provider := NewAnthropicLLMProvider(AnthropicProviderConfig{
		Client: mockClient,
		Model:  anthropic.ModelClaude_3_5_Sonnet_20240620,
	})

	ctx := context.Background()
	stream, err := provider.GetStreamingResponse(ctx, []LLMMessage{
		{Role: UserRole, Text: "Hello"},
	}, LLMRequestConfig{MaxToken: 100, toolsProvider: func() *ToolsProvider {
		return NewToolsProvider()
	}()})
	assert.NoError(t, err)

	var receivedText string
	for chunk := range stream {
		if chunk.Error != nil {
			t.Fatal(chunk.Error)
		}
		receivedText += chunk.Text
	}

	assert.Equal(t, "Hello world!", receivedText)
}

func TestAnthropicLLMProvider_GetStreamingResponse_SingleTool(t *testing.T) {
	var callCount int
	mockClient := &MockAnthropicClient{
		createStreamingMessageFunc: func(_ context.Context, params anthropic.MessageNewParams) *ssestream.Stream[anthropic.MessageStreamEvent] {
			// Debug log the request params to ensure tool setup is correct
			t.Logf("Creating streaming message call #%d with %d tools", callCount+1, len(params.Tools.String()))

			callCount++
			var events []anthropic.MessageStreamEvent

			// Create two different event streams based on call count
			if callCount == 1 {
				// First call: Tool usage
				toolUseBlock := anthropic.ToolUseBlock{
					ID:    "toolu_01",
					Name:  "get_weather",
					Input: json.RawMessage(`{"location":"Berlin"}`),
					Type:  anthropic.ToolUseBlockTypeToolUse,
				}

				// This is the first important point - we need to make sure the Message object
				// contains the tool use block in its Content field
				msg := anthropic.Message{
					Role:       anthropic.MessageRoleAssistant,
					StopReason: anthropic.MessageStopReasonToolUse,
					Content: []anthropic.ContentBlock{
						{
							Type: anthropic.ContentBlockTypeText, // or appropriate type
							Text: func() string {
								s, _ := toolUseBlock.Input.MarshalJSON()
								return string(s)
							}(), // adjust according to your toolUseBlock structure
						},
					},
				}

				events = []anthropic.MessageStreamEvent{
					// MessageStart with properly populated Message field
					{
						Type:    anthropic.MessageStreamEventTypeMessageStart,
						Message: msg,
					},
					// ContentBlockStart for the tool use
					{
						Type:         anthropic.MessageStreamEventTypeContentBlockStart,
						Index:        0,
						ContentBlock: toolUseBlock,
					},
					// ContentBlockStop to end the block
					{
						Type:  anthropic.MessageStreamEventTypeContentBlockStop,
						Index: 0,
					},
					// MessageStop to end the message
					{
						Type: anthropic.MessageStreamEventTypeMessageStop,
					},
				}
			} else {
				// Second call: Text response after tool use
				textBlock := anthropic.TextBlock{
					Type: anthropic.TextBlockTypeText,
					Text: "The weather is Sunny",
				}

				msg := anthropic.Message{
					Role:       anthropic.MessageRoleAssistant,
					StopReason: anthropic.MessageStopReasonEndTurn,
					Content: []anthropic.ContentBlock{
						{
							Type: anthropic.ContentBlockTypeText,
							Text: textBlock.Text, // if textBlock is a string
							// or textBlock.Text if textBlock is a struct with a Text field
						},
					},
				}

				events = []anthropic.MessageStreamEvent{
					// MessageStart with text content
					{
						Type:    anthropic.MessageStreamEventTypeMessageStart,
						Message: msg,
					},
					// ContentBlockStart for the text block
					{
						Type:         anthropic.MessageStreamEventTypeContentBlockStart,
						Index:        0,
						ContentBlock: textBlock,
					},
					// ContentBlockDelta to stream the text content
					{
						Type:  anthropic.MessageStreamEventTypeContentBlockDelta,
						Index: 0,
						Delta: anthropic.ContentBlockDeltaEventDelta{
							Type: anthropic.ContentBlockDeltaEventDeltaTypeTextDelta,
							Text: "The weather is Sunny",
						},
					},
					// ContentBlockStop to end the block
					{
						Type:  anthropic.MessageStreamEventTypeContentBlockStop,
						Index: 0,
					},
					// MessageStop to end the message
					{
						Type: anthropic.MessageStreamEventTypeMessageStop,
					},
				}
			}

			decoder := &mockDecoder{events: events, index: -1}
			return ssestream.NewStream[anthropic.MessageStreamEvent](decoder, nil)
		},
	}

	// Setup mock tools provider
	mockToolsProvider := NewToolsProvider()
	_ = mockToolsProvider.AddTools([]mcp.Tool{
		{
			Name:        "get_weather",
			Description: "Get the weather",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
			Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
				t.Logf("Tool handler called with input: %s", params.Arguments)
				return mcp.CallToolResult{
					Content: []mcp.ToolResultContent{
						{
							Type: "text",
							Text: "The weather is Sunny",
						},
					},
					IsError: false,
				}, nil
			},
		},
	})

	provider := NewAnthropicLLMProvider(AnthropicProviderConfig{
		Client: mockClient,
		Model:  anthropic.ModelClaude_3_5_Sonnet_20240620,
	})

	ctx := context.Background()
	stream, err := provider.GetStreamingResponse(ctx, []LLMMessage{
		{Role: UserRole, Text: "Weather in Berlin?"},
	}, LLMRequestConfig{
		MaxToken:      100,
		toolsProvider: mockToolsProvider,
		AllowedTools:  []string{"get_weather"},
	})
	assert.NoError(t, err)

	var receivedText string
	for chunk := range stream {
		if chunk.Error != nil {
			t.Fatalf("Error in chunk: %v", chunk.Error)
		}
		t.Logf("Received chunk: '%s', Done: %v", chunk.Text, chunk.Done)
		receivedText += chunk.Text
	}

	t.Logf("Final received text: '%s'", receivedText)
	t.Logf("Expected text: '%s'", "The weather is Sunny")

	assert.Equal(t, "The weather is Sunny", receivedText)
}

func TestAnthropicLLMProvider_GetStreamingResponse_MultiTool(t *testing.T) {
	var callCount int
	mockClient := &MockAnthropicClient{
		createStreamingMessageFunc: func(_ context.Context, params anthropic.MessageNewParams) *ssestream.Stream[anthropic.MessageStreamEvent] {
			callCount++
			t.Logf("Streaming call #%d with tools: %v", callCount, params.Tools)

			var events []anthropic.MessageStreamEvent

			switch callCount {
			case 1: // First call: Two tool usages
				weatherTool := anthropic.ToolUseBlock{
					ID:    "toolu_01",
					Name:  "get_weather",
					Input: json.RawMessage(`{"location":"Berlin"}`),
					Type:  anthropic.ToolUseBlockTypeToolUse,
				}

				stockTool := anthropic.ToolUseBlock{
					ID:    "toolu_02",
					Name:  "get_stock_price",
					Input: json.RawMessage(`{"symbol":"GOOGL"}`),
					Type:  anthropic.ToolUseBlockTypeToolUse,
				}

				events = []anthropic.MessageStreamEvent{
					{
						Type: anthropic.MessageStreamEventTypeMessageStart,
						Message: anthropic.Message{
							Role:       anthropic.MessageRoleAssistant,
							StopReason: anthropic.MessageStopReasonToolUse,
							Content: []anthropic.ContentBlock{
								{
									Type:  anthropic.ContentBlockTypeToolUse,
									ID:    weatherTool.ID,
									Name:  weatherTool.Name,
									Input: weatherTool.Input,
								},
								{
									Type:  anthropic.ContentBlockTypeToolUse,
									ID:    stockTool.ID,
									Name:  stockTool.Name,
									Input: stockTool.Input,
								},
							},
						},
					},
					{
						Type:         anthropic.MessageStreamEventTypeContentBlockStart,
						Index:        0,
						ContentBlock: weatherTool,
					},
					{
						Type:  anthropic.MessageStreamEventTypeContentBlockStop,
						Index: 0,
					},
					{
						Type:         anthropic.MessageStreamEventTypeContentBlockStart,
						Index:        1,
						ContentBlock: stockTool,
					},
					{
						Type:  anthropic.MessageStreamEventTypeContentBlockStop,
						Index: 1,
					},
					{
						Type: anthropic.MessageStreamEventTypeMessageStop,
					},
				}
			case 2: // Second call: Response after processing both tools
				events = []anthropic.MessageStreamEvent{
					{
						Type: anthropic.MessageStreamEventTypeMessageStart,
						Message: anthropic.Message{
							Role:       anthropic.MessageRoleAssistant,
							StopReason: anthropic.MessageStopReasonEndTurn,
							Content: []anthropic.ContentBlock{
								{
									Type: anthropic.ContentBlockTypeText,
									Text: "The weather is Sunny and GOOGL is at $150",
								},
							},
						},
					},
					{
						Type:         anthropic.MessageStreamEventTypeContentBlockStart,
						Index:        0,
						ContentBlock: anthropic.TextBlock{Type: anthropic.TextBlockTypeText, Text: ""},
					},
					{
						Type:  anthropic.MessageStreamEventTypeContentBlockDelta,
						Index: 0,
						Delta: anthropic.ContentBlockDeltaEventDelta{
							Type: anthropic.ContentBlockDeltaEventDeltaTypeTextDelta,
							Text: "The weather is Sunny",
						},
					},
					{
						Type:  anthropic.MessageStreamEventTypeContentBlockDelta,
						Index: 0,
						Delta: anthropic.ContentBlockDeltaEventDelta{
							Type: anthropic.ContentBlockDeltaEventDeltaTypeTextDelta,
							Text: " and GOOGL is at $150",
						},
					},
					{
						Type:  anthropic.MessageStreamEventTypeContentBlockStop,
						Index: 0,
					},
					{
						Type: anthropic.MessageStreamEventTypeMessageStop,
					},
				}
			}

			decoder := &mockDecoder{events: events, index: -1}
			return ssestream.NewStream[anthropic.MessageStreamEvent](decoder, nil)
		},
	}

	// Setup mock tools provider with two tools
	mockToolsProvider := NewToolsProvider()
	_ = mockToolsProvider.AddTools([]mcp.Tool{
		{
			Name:        "get_weather",
			Description: "Get weather information",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
			Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
				return mcp.CallToolResult{
					Content: []mcp.ToolResultContent{
						{Type: "text", Text: "The weather is Sunny"},
					},
				}, nil
			},
		},
		{
			Name:        "get_stock_price",
			Description: "Get stock price information",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"symbol":{"type":"string"}},"required":["symbol"]}`),
			Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
				return mcp.CallToolResult{
					Content: []mcp.ToolResultContent{
						{Type: "text", Text: "GOOGL is at $150"},
					},
				}, nil
			},
		},
	})

	provider := NewAnthropicLLMProvider(AnthropicProviderConfig{
		Client: mockClient,
		Model:  anthropic.ModelClaude_3_5_Sonnet_20240620,
	})

	ctx := context.Background()
	stream, err := provider.GetStreamingResponse(ctx, []LLMMessage{
		{Role: UserRole, Text: "What's the weather in Berlin and GOOGL stock price?"},
	}, LLMRequestConfig{
		MaxToken:      200,
		toolsProvider: mockToolsProvider,
		AllowedTools:  []string{"get_weather", "get_stock_price"},
	})
	assert.NoError(t, err)

	var receivedText string
	for chunk := range stream {
		if chunk.Error != nil {
			t.Fatalf("Error in chunk: %v", chunk.Error)
		}
		t.Logf("Received chunk: '%s'", chunk.Text)
		receivedText += chunk.Text
	}

	expected := "The weather is Sunny and GOOGL is at $150"
	assert.Equal(t, expected, receivedText)
}
