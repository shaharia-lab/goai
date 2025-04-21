package goai

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/shaharia-lab/goai/mcp"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestBedrockLLMProvider_GetResponse_BasicConversation(t *testing.T) {
	// Setup mock
	mockClient := new(MockBedrockClient)

	// Create a mock response
	mockResponse := &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{
			Value: types.Message{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: "This is a test response",
					},
				},
			},
		},
		StopReason: types.StopReasonEndTurn,
		Usage: &types.TokenUsage{
			InputTokens:  aws.Int32(10),
			OutputTokens: aws.Int32(5),
		},
	}

	// Setup expectations
	mockClient.On("Converse", mock.Anything, mock.MatchedBy(func(params *bedrockruntime.ConverseInput) bool {
		// Verify the model ID matches what we expect
		return *params.ModelId == "test-model"
	}), mock.Anything).Return(mockResponse, nil)

	// Create the provider with our mock
	provider := NewBedrockLLMProvider(BedrockProviderConfig{
		Client: mockClient,
		Model:  "test-model",
	})

	// Test messages
	messages := []LLMMessage{
		{
			Role: SystemRole,
			Text: "You are a helpful assistant.",
		},
		{
			Role: UserRole,
			Text: "Hello, can you help me?",
		},
	}

	// Create LLM request with configuration
	llm := NewLLMRequest(NewRequestConfig(
		WithMaxToken(1000),
		WithTemperature(0.7),
		WithTopP(0.9),
	), provider)

	// Execute the request
	response, err := llm.Generate(context.Background(), messages)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, "This is a test response", response.Text)
	assert.Equal(t, 10, response.TotalInputToken)
	assert.Equal(t, 5, response.TotalOutputToken)

	// Verify all expectations were met
	mockClient.AssertExpectations(t)
}

func TestBedrockLLMProvider_GetResponse_WithTools(t *testing.T) {
	// Setup mock
	mockClient := new(MockBedrockClient)

	// First response with tool use
	var schemaDoc = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"location": map[string]interface{}{
				"type": "string",
			},
		},
		"required": []interface{}{"location"},
	}
	toolUseResponse := &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{
			Value: types.Message{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: "I'll check that for you.",
					},
					&types.ContentBlockMemberToolUse{
						Value: types.ToolUseBlock{
							ToolUseId: aws.String("tool-call-1"),
							Name:      aws.String("test_tool"),
							Input:     document.NewLazyDocument(schemaDoc),
						},
					},
				},
			},
		},
		StopReason: types.StopReasonToolUse,
		Usage: &types.TokenUsage{
			InputTokens:  aws.Int32(15),
			OutputTokens: aws.Int32(10),
		},
	}

	// Setup expectations for the tool call
	mockClient.On("Converse", mock.Anything, mock.Anything, mock.Anything).Return(toolUseResponse, nil).Once()

	// Second response after tool execution
	finalResponse := &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{
			Value: types.Message{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: "I've processed your request for test location.",
					},
				},
			},
		},
		StopReason: types.StopReasonEndTurn,
		Usage: &types.TokenUsage{
			InputTokens:  aws.Int32(20),
			OutputTokens: aws.Int32(10),
		},
	}

	mockClient.On("Converse", mock.Anything, mock.Anything, mock.Anything).Return(finalResponse, nil).Once()

	// Create tools provider with a test tool
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

	// Create the provider with our mock
	provider := NewBedrockLLMProvider(BedrockProviderConfig{
		Client: mockClient,
		Model:  "test-model",
	})

	// Test messages
	messages := []LLMMessage{
		{
			Role: UserRole,
			Text: "Can you use the test_tool for 'test location'?",
		},
	}

	// Create LLM request with configuration including tools
	llm := NewLLMRequest(NewRequestConfig(
		WithMaxToken(1000),
		WithTemperature(0.7),
		WithTopP(0.9),
		UseToolsProvider(toolsProvider),
		WithAllowedTools([]string{"test_tool"}),
	), provider)

	// Execute the request
	response, err := llm.Generate(context.Background(), messages)

	// Assertions
	assert.NoError(t, err)
	assert.Contains(t, response.Text, "I've processed your request")
	assert.Equal(t, 35, response.TotalInputToken)  // 15 + 20
	assert.Equal(t, 20, response.TotalOutputToken) // 10 + 10

	// Verify all expectations were met
	mockClient.AssertExpectations(t)
}

func TestBedrockLLMProvider_GetResponse_WithMultipleTools(t *testing.T) {
	// Setup mock
	mockClient := new(MockBedrockClient)

	// First response with first tool use
	var weatherSchemaDoc = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"location": map[string]interface{}{
				"type": "string",
			},
		},
		"required": []interface{}{"location"},
	}
	firstToolUseResponse := &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{
			Value: types.Message{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: "I'll check the weather and time for you.",
					},
					&types.ContentBlockMemberToolUse{
						Value: types.ToolUseBlock{
							ToolUseId: aws.String("tool-call-1"),
							Name:      aws.String("weather_tool"),
							Input:     document.NewLazyDocument(weatherSchemaDoc),
						},
					},
				},
			},
		},
		StopReason: types.StopReasonToolUse,
		Usage: &types.TokenUsage{
			InputTokens:  aws.Int32(15),
			OutputTokens: aws.Int32(10),
		},
	}

	// Setup expectations for the first tool call
	mockClient.On("Converse", mock.Anything, mock.Anything, mock.Anything).Return(firstToolUseResponse, nil).Once()

	// Second response with second tool use
	var timeSchemaDoc = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"timezone": map[string]interface{}{
				"type": "string",
			},
		},
		"required": []interface{}{"timezone"},
	}
	secondToolUseResponse := &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{
			Value: types.Message{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: "Now I'll check the time.",
					},
					&types.ContentBlockMemberToolUse{
						Value: types.ToolUseBlock{
							ToolUseId: aws.String("tool-call-2"),
							Name:      aws.String("time_tool"),
							Input:     document.NewLazyDocument(timeSchemaDoc),
						},
					},
				},
			},
		},
		StopReason: types.StopReasonToolUse,
		Usage: &types.TokenUsage{
			InputTokens:  aws.Int32(25),
			OutputTokens: aws.Int32(12),
		},
	}

	// Setup expectations for the second tool call
	mockClient.On("Converse", mock.Anything, mock.Anything, mock.Anything).Return(secondToolUseResponse, nil).Once()

	// Final response after both tool executions
	finalResponse := &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{
			Value: types.Message{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: "In New York, the weather is sunny and the current time is 10:00 AM EDT.",
					},
				},
			},
		},
		StopReason: types.StopReasonEndTurn,
		Usage: &types.TokenUsage{
			InputTokens:  aws.Int32(35),
			OutputTokens: aws.Int32(15),
		},
	}

	// Setup expectations for the final response
	mockClient.On("Converse", mock.Anything, mock.Anything, mock.Anything).Return(finalResponse, nil).Once()

	// Create tools provider with multiple tools
	tools := []mcp.Tool{
		{
			Name:        "weather_tool",
			Description: "Provides weather information for a location",
			InputSchema: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "location": {"type": "string"}
                },
                "required": ["location"]
            }`),
			Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
				return mcp.CallToolResult{
					Content: []mcp.ToolResultContent{
						{
							Type: "text",
							Text: "Weather in New York: Sunny, 75°F",
						},
					},
				}, nil
			},
		},
		{
			Name:        "time_tool",
			Description: "Provides current time for a timezone",
			InputSchema: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "timezone": {"type": "string"}
                },
                "required": ["timezone"]
            }`),
			Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
				return mcp.CallToolResult{
					Content: []mcp.ToolResultContent{
						{
							Type: "text",
							Text: "Current time in EDT: 10:00 AM",
						},
					},
				}, nil
			},
		},
	}

	toolsProvider := NewToolsProvider()
	_ = toolsProvider.AddTools(tools)

	// Create the provider with our mock
	provider := NewBedrockLLMProvider(BedrockProviderConfig{
		Client: mockClient,
		Model:  "test-model",
	})

	// Test messages
	messages := []LLMMessage{
		{
			Role: UserRole,
			Text: "What's the weather and time in New York?",
		},
	}

	// Create LLM request with configuration including tools
	llm := NewLLMRequest(NewRequestConfig(
		WithMaxToken(1000),
		WithTemperature(0.7),
		WithTopP(0.9),
		UseToolsProvider(toolsProvider),
		WithAllowedTools([]string{"weather_tool", "time_tool"}),
	), provider)

	// Execute the request
	response, err := llm.Generate(context.Background(), messages)

	// Assertions
	assert.NoError(t, err)
	assert.Contains(t, response.Text, "the weather is sunny and the current time is 10:00 AM")
	assert.Equal(t, 75, response.TotalInputToken)  // 15 + 25 + 35
	assert.Equal(t, 37, response.TotalOutputToken) // 10 + 12 + 15

	// Verify all expectations were met
	mockClient.AssertExpectations(t)
}

func TestBedrockLLMProvider_GetResponse_Error(t *testing.T) {
	// Setup mock
	mockClient := new(MockBedrockClient)

	// Setup expectations to return an error
	expectedError := errors.New("bedrock service error")
	mockClient.On("Converse", mock.Anything, mock.Anything, mock.Anything).Return((*bedrockruntime.ConverseOutput)(nil), expectedError)

	// Create the provider with our mock
	provider := NewBedrockLLMProvider(BedrockProviderConfig{
		Client: mockClient,
		Model:  "test-model",
	})

	// Test messages
	messages := []LLMMessage{
		{
			Role: UserRole,
			Text: "Hello",
		},
	}

	// Create LLM request with minimal configuration
	llm := NewLLMRequest(NewRequestConfig(), provider)

	// Execute the request
	_, err := llm.Generate(context.Background(), messages)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bedrock service error")

	// Verify all expectations were met
	mockClient.AssertExpectations(t)
}

func TestBedrockLLMProvider_DefaultModel(t *testing.T) {
	// Setup mock
	mockClient := new(MockBedrockClient)

	// Create a simple response
	mockResponse := &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{
			Value: types.Message{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: "Hello",
					},
				},
			},
		},
		Usage: &types.TokenUsage{
			InputTokens:  aws.Int32(5),
			OutputTokens: aws.Int32(1),
		},
	}

	// Setup expectations to check for the default model
	mockClient.On("Converse", mock.Anything, mock.MatchedBy(func(params *bedrockruntime.ConverseInput) bool {
		return *params.ModelId == "anthropic.claude-3-5-sonnet-20240620-v1:0"
	}), mock.Anything).Return(mockResponse, nil)

	// Create the provider with our mock but no specified model
	provider := NewBedrockLLMProvider(BedrockProviderConfig{
		Client: mockClient,
	})

	// Test messages
	messages := []LLMMessage{
		{
			Role: UserRole,
			Text: "Hello",
		},
	}

	// Create LLM request with minimal configuration
	llm := NewLLMRequest(NewRequestConfig(), provider)

	// Execute the request
	_, err := llm.Generate(context.Background(), messages)

	// Assertions
	assert.NoError(t, err)

	// Verify that the default model was used
	mockClient.AssertExpectations(t)
}
