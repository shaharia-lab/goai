package goai_test

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
	"github.com/shaharia-lab/goai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockBedrockClient is a mock implementation of the BedrockClient interface
type MockBedrockClient struct {
	mock.Mock
}

func (m *MockBedrockClient) Converse(ctx context.Context, params *bedrockruntime.ConverseInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*bedrockruntime.ConverseOutput), args.Error(1)
}

func (m *MockBedrockClient) ConverseStream(ctx context.Context, params *bedrockruntime.ConverseStreamInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseStreamOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*bedrockruntime.ConverseStreamOutput), args.Error(1)
}

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
	provider := goai.NewBedrockLLMProvider(goai.BedrockProviderConfig{
		Client: mockClient,
		Model:  "test-model",
	})

	// Test messages
	messages := []goai.LLMMessage{
		{
			Role: goai.SystemRole,
			Text: "You are a helpful assistant.",
		},
		{
			Role: goai.UserRole,
			Text: "Hello, can you help me?",
		},
	}

	// Create LLM request with configuration
	llm := goai.NewLLMRequest(goai.NewRequestConfig(
		goai.WithMaxToken(1000),
		goai.WithTemperature(0.7),
		goai.WithTopP(0.9),
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

	toolsProvider := goai.NewToolsProvider()
	_ = toolsProvider.AddTools(tools)

	// Create the provider with our mock
	provider := goai.NewBedrockLLMProvider(goai.BedrockProviderConfig{
		Client: mockClient,
		Model:  "test-model",
	})

	// Test messages
	messages := []goai.LLMMessage{
		{
			Role: goai.UserRole,
			Text: "Can you use the test_tool for 'test location'?",
		},
	}

	// Create LLM request with configuration including tools
	llm := goai.NewLLMRequest(goai.NewRequestConfig(
		goai.WithMaxToken(1000),
		goai.WithTemperature(0.7),
		goai.WithTopP(0.9),
		goai.UseToolsProvider(toolsProvider),
		goai.WithAllowedTools([]string{"test_tool"}),
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

func TestBedrockLLMProvider_GetResponse_Error(t *testing.T) {
	// Setup mock
	mockClient := new(MockBedrockClient)

	// Setup expectations to return an error
	expectedError := errors.New("bedrock service error")
	mockClient.On("Converse", mock.Anything, mock.Anything, mock.Anything).Return((*bedrockruntime.ConverseOutput)(nil), expectedError)

	// Create the provider with our mock
	provider := goai.NewBedrockLLMProvider(goai.BedrockProviderConfig{
		Client: mockClient,
		Model:  "test-model",
	})

	// Test messages
	messages := []goai.LLMMessage{
		{
			Role: goai.UserRole,
			Text: "Hello",
		},
	}

	// Create LLM request with minimal configuration
	llm := goai.NewLLMRequest(goai.NewRequestConfig(), provider)

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
	provider := goai.NewBedrockLLMProvider(goai.BedrockProviderConfig{
		Client: mockClient,
	})

	// Test messages
	messages := []goai.LLMMessage{
		{
			Role: goai.UserRole,
			Text: "Hello",
		},
	}

	// Create LLM request with minimal configuration
	llm := goai.NewLLMRequest(goai.NewRequestConfig(), provider)

	// Execute the request
	_, err := llm.Generate(context.Background(), messages)

	// Assertions
	assert.NoError(t, err)

	// Verify that the default model was used
	mockClient.AssertExpectations(t)
}
