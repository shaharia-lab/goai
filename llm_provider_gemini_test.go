package goai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shaharia-lab/goai/observability"
	"testing"

	"github.com/google/generative-ai-go/genai"
	"github.com/shaharia-lab/goai/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/api/iterator"
)

type MockGeminiModelService struct {
	mock.Mock
}

func (m *MockGeminiModelService) ConfigureModel(config *genai.GenerationConfig, tools []*genai.Tool) error {
	args := m.Called(config, tools)
	return args.Error(0)
}

func (m *MockGeminiModelService) StartChat(initialHistory []*genai.Content) ChatSessionService {
	args := m.Called(initialHistory)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(ChatSessionService)
}

func (m *MockGeminiModelService) GenerateContentStream(ctx context.Context, parts ...genai.Part) (StreamIteratorService, error) {
	args := m.Called(ctx, mock.AnythingOfType("[]genai.Part"))
	var svc StreamIteratorService
	if getSvc := args.Get(0); getSvc != nil {
		svc = getSvc.(StreamIteratorService)
	}
	return svc, args.Error(1)
}

func (m *MockGeminiModelService) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockChatSessionService struct {
	mock.Mock
}

func (m *MockChatSessionService) SendMessage(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error) {
	args := m.Called(ctx, parts)

	var resp *genai.GenerateContentResponse
	if getResp := args.Get(0); getResp != nil {
		resp = getResp.(*genai.GenerateContentResponse)
	}
	return resp, args.Error(1)
}

func (m *MockChatSessionService) AppendHistory(content *genai.Content) {
	m.Called(content)
}

func (m *MockChatSessionService) GetHistory() []*genai.Content {
	args := m.Called()
	var history []*genai.Content
	if getHistory := args.Get(0); getHistory != nil {
		history = getHistory.([]*genai.Content)
	} else {
	}
	return history
}

type MockStreamIteratorService struct {
	mock.Mock
	responses []*genai.GenerateContentResponse
	errors    []error
	idx       int
}

func (m *MockStreamIteratorService) Next() (*genai.GenerateContentResponse, error) {
	m.Called()
	if m.idx < len(m.responses) || m.idx < len(m.errors) {
		var resp *genai.GenerateContentResponse
		var err error
		if m.idx < len(m.responses) {
			resp = m.responses[m.idx]
		}
		if m.idx < len(m.errors) {
			err = m.errors[m.idx]
		}
		m.idx++
		return resp, err
	}
	return nil, iterator.Done
}

func setupTest() (*MockGeminiModelService, observability.Logger, *ToolsProvider) {
	mockService := new(MockGeminiModelService)
	mockLogger := observability.NewNullLogger()

	tools := []mcp.Tool{
		{
			Name:        "get_weather",
			Description: "Fetches the current weather conditions for a specific location.",
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
						Text: fmt.Sprintf("Weather in %s: Sunny", input.Location),
					}},
				}, nil
			},
		},
	}

	toolsProvider := NewToolsProvider()
	toolsProvider.AddTools(tools)
	mockToolsProvider := toolsProvider

	return mockService, mockLogger, mockToolsProvider
}

func TestGeminiProvider_GetResponse_SimpleText(t *testing.T) {
	mockService, mockLogger, _ := setupTest()
	mockChatSession := new(MockChatSessionService)
	provider, err := NewGeminiProvider(mockService, mockLogger)
	assert.NoError(t, err)

	messages := []LLMMessage{{Role: UserRole, Text: "Hello"}}
	config := LLMRequestConfig{}
	expectedInitialHistory := []*genai.Content{}
	expectedInitialParts := []genai.Part{genai.Text("Hello")}
	mockResponse := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content:      &genai.Content{Parts: []genai.Part{genai.Text("Hi there!")}, Role: "model"},
			FinishReason: genai.FinishReasonStop,
		}},
		UsageMetadata: &genai.UsageMetadata{PromptTokenCount: 10, CandidatesTokenCount: 3},
	}

	mockService.On("ConfigureModel", mock.AnythingOfType("*genai.GenerationConfig"), ([]*genai.Tool)(nil)).Return(nil).Once()
	mockService.On("StartChat", expectedInitialHistory).Return(mockChatSession).Once()
	mockChatSession.On("SendMessage", mock.Anything, expectedInitialParts).Return(mockResponse, nil).Once()
	mockChatSession.On("AppendHistory", mockResponse.Candidates[0].Content).Return().Once()
	response, err := provider.GetResponse(context.Background(), messages, config)

	assert.NoError(t, err)
	assert.Equal(t, "Hi there!", response.Text)
	assert.Equal(t, 10, response.TotalInputToken)
	assert.Equal(t, 3, response.TotalOutputToken)
	assert.True(t, response.CompletionTime > 0)
	mockService.AssertExpectations(t)
	mockChatSession.AssertExpectations(t)
}

func TestGeminiProvider_GetResponse_SingleToolCall(t *testing.T) {
	mockService, mockLogger, mockToolsProvider := setupTest()
	mockChatSession := new(MockChatSessionService)

	provider, err := NewGeminiProvider(mockService, mockLogger)
	assert.NoError(t, err)

	messages := []LLMMessage{{Role: UserRole, Text: "Weather Berlin?"}}
	config := LLMRequestConfig{
		AllowedTools:  []string{"get_weather"},
		toolsProvider: mockToolsProvider,
	}

	funcCallArgs := map[string]any{"location": "Berlin"}
	funcCallResponse := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{Parts: []genai.Part{
				genai.FunctionCall{Name: "get_weather", Args: funcCallArgs},
			}, Role: "model"},
			FinishReason: genai.FinishReasonStop,
		}},
		UsageMetadata: &genai.UsageMetadata{PromptTokenCount: 15, CandidatesTokenCount: 10},
	}

	finalTextResponse := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content:      &genai.Content{Parts: []genai.Part{genai.Text("Okay, the weather in Berlin is Sunny and 24C.")}, Role: "model"},
			FinishReason: genai.FinishReasonStop,
		}},
		UsageMetadata: &genai.UsageMetadata{PromptTokenCount: 50, CandidatesTokenCount: 12},
	}

	finalTextResponse = &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content:      &genai.Content{Parts: []genai.Part{genai.Text("Okay, the weather in Berlin is Sunny and 24C.")}, Role: "model"},
			FinishReason: genai.FinishReasonStop,
		}},
		UsageMetadata: &genai.UsageMetadata{PromptTokenCount: 50, CandidatesTokenCount: 12},
	}

	mockService.On("ConfigureModel",
		mock.AnythingOfType("*genai.GenerationConfig"),
		mock.AnythingOfType("[]*genai.Tool"),
	).Return(nil).Once()
	mockService.On("StartChat", []*genai.Content{}).Return(mockChatSession).Once()
	mockChatSession.On("SendMessage", mock.Anything, []genai.Part{genai.Text("Weather Berlin?")}).Return(funcCallResponse, nil).Once()

	mockChatSession.On("AppendHistory", funcCallResponse.Candidates[0].Content).Return().Once()
	mockChatSession.On("AppendHistory", mock.MatchedBy(func(content *genai.Content) bool {
		if content.Role != RoleFunction || len(content.Parts) != 1 {
			return false
		}
		fr, ok := content.Parts[0].(genai.FunctionResponse)
		return ok && fr.Name == "get_weather"
	})).Return().Once()
	mockChatSession.On("SendMessage", mock.Anything, []genai.Part{genai.Text("")}).Return(finalTextResponse, nil).Once()
	mockChatSession.On("AppendHistory", finalTextResponse.Candidates[0].Content).Return().Once()

	response, err := provider.GetResponse(context.Background(), messages, config)

	assert.NoError(t, err)
	assert.Equal(t, "Okay, the weather in Berlin is Sunny and 24C.", response.Text)
	assert.Equal(t, 50, response.TotalInputToken)
	assert.Equal(t, 12, response.TotalOutputToken)
	mockService.AssertExpectations(t)
	mockChatSession.AssertExpectations(t)
}

func TestGeminiProvider_GetResponse_ApiError(t *testing.T) {
	mockService, mockLogger, _ := setupTest()
	mockChatSession := new(MockChatSessionService)
	apiError := errors.New("mock API error 500")

	provider, err := NewGeminiProvider(mockService, mockLogger)
	assert.NoError(t, err)

	messages := []LLMMessage{{Role: UserRole, Text: "Hello"}}
	config := LLMRequestConfig{}

	mockService.On("ConfigureModel", mock.Anything, mock.Anything).Return(nil).Once()
	mockService.On("StartChat", mock.Anything).Return(mockChatSession).Once()
	mockChatSession.On("SendMessage", mock.Anything, mock.Anything).Return((*genai.GenerateContentResponse)(nil), apiError).Once()

	_, err = provider.GetResponse(context.Background(), messages, config)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock API error 500")
	mockService.AssertExpectations(t)
	mockChatSession.AssertExpectations(t)
}

func TestGeminiProvider_GetStreamingResponse_SimpleTextSimulated(t *testing.T) {
	mockService, mockLogger, _ := setupTest()
	mockChatSession := new(MockChatSessionService)

	provider, err := NewGeminiProvider(mockService, mockLogger)
	assert.NoError(t, err)

	messages := []LLMMessage{{Role: UserRole, Text: "Stream test"}}
	config := LLMRequestConfig{}

	mockSyncResponse := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content:      &genai.Content{Parts: []genai.Part{genai.Text("Stream response text")}, Role: "model"},
			FinishReason: genai.FinishReasonStop,
		}},
		UsageMetadata: &genai.UsageMetadata{PromptTokenCount: 8, CandidatesTokenCount: 4},
	}

	mockService.On("ConfigureModel", mock.Anything, mock.Anything).Return(nil).Once()
	mockService.On("StartChat", mock.Anything).Return(mockChatSession).Once()
	mockChatSession.On("SendMessage", mock.Anything, []genai.Part{genai.Text("Stream test")}).Return(mockSyncResponse, nil).Once()
	mockChatSession.On("AppendHistory", mockSyncResponse.Candidates[0].Content).Return().Once()

	streamChan, err := provider.GetStreamingResponse(context.Background(), messages, config)

	assert.NoError(t, err)
	assert.NotNil(t, streamChan)

	receivedChunks := []StreamingLLMResponse{}
	for chunk := range streamChan {
		receivedChunks = append(receivedChunks, chunk)
	}

	assert.Equal(t, 1, len(receivedChunks), "Expected exactly one chunk for simulated stream")
	if len(receivedChunks) == 1 {
		assert.NoError(t, receivedChunks[0].Error)
		assert.True(t, receivedChunks[0].Done)
		assert.Equal(t, "Stream response text", receivedChunks[0].Text)
		assert.Equal(t, 4, receivedChunks[0].TokenCount)
	}

	mockService.AssertExpectations(t)
	mockChatSession.AssertExpectations(t)
}

func TestGeminiProvider_GetStreamingResponse_SingleToolCallSimulated(t *testing.T) {
	// Arrange
	mockService, mockLogger, mockToolsProvider := setupTest()
	mockChatSession := new(MockChatSessionService)

	provider, err := NewGeminiProvider(mockService, mockLogger)
	assert.NoError(t, err)

	messages := []LLMMessage{{Role: UserRole, Text: "Weather London?"}}
	config := LLMRequestConfig{
		AllowedTools:  []string{"get_weather"},
		toolsProvider: mockToolsProvider,
	}

	// Mock Responses for pre-flight loop
	funcCallArgs := map[string]any{"location": "London"}
	funcCallResponse := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content:      &genai.Content{Parts: []genai.Part{genai.FunctionCall{Name: "get_weather", Args: funcCallArgs}}, Role: "model"},
			FinishReason: genai.FinishReasonStop,
		}},
		UsageMetadata: &genai.UsageMetadata{PromptTokenCount: 16, CandidatesTokenCount: 11},
	}

	finalTextContent := "It's likely cloudy in London."
	finalTextResponse := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content:      &genai.Content{Parts: []genai.Part{genai.Text(finalTextContent)}, Role: "model"},
			FinishReason: genai.FinishReasonStop,
		}},
		UsageMetadata: &genai.UsageMetadata{PromptTokenCount: 60, CandidatesTokenCount: 8},
	}

	mockService.On("ConfigureModel", mock.AnythingOfType("*genai.GenerationConfig"), mock.AnythingOfType("[]*genai.Tool")).Return(nil).Once()
	mockService.On("StartChat", []*genai.Content{}).Return(mockChatSession).Once()

	mockChatSession.On("SendMessage", mock.Anything, []genai.Part{genai.Text("Weather London?")}).Return(funcCallResponse, nil).Once()
	mockChatSession.On("AppendHistory", funcCallResponse.Candidates[0].Content).Return().Once()
	mockChatSession.On("AppendHistory", mock.MatchedBy(func(content *genai.Content) bool {
		return content.Role == RoleFunction && len(content.Parts) == 1 && content.Parts[0].(genai.FunctionResponse).Name == "get_weather"
	})).Return().Once()

	mockChatSession.On("SendMessage", mock.Anything, []genai.Part{genai.Text("")}).Return(finalTextResponse, nil).Once()
	mockChatSession.On("AppendHistory", finalTextResponse.Candidates[0].Content).Return().Once() // Expect final text append

	streamChan, err := provider.GetStreamingResponse(context.Background(), messages, config)

	assert.NoError(t, err)
	assert.NotNil(t, streamChan)

	receivedChunks := []StreamingLLMResponse{}
	for chunk := range streamChan {
		receivedChunks = append(receivedChunks, chunk)
	}

	assert.Equal(t, 1, len(receivedChunks), "Expected exactly one chunk for simulated stream after tool call")
	if len(receivedChunks) == 1 {
		assert.NoError(t, receivedChunks[0].Error)
		assert.True(t, receivedChunks[0].Done)
		assert.Equal(t, finalTextContent, receivedChunks[0].Text)
		assert.Equal(t, 8, receivedChunks[0].TokenCount)
	}

	mockService.AssertExpectations(t)
	mockChatSession.AssertExpectations(t)
}
