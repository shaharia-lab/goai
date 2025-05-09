package goai // Or your actual package name

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/stretchr/testify/mock"
)

func createMockInvokeOutput(t *testing.T, body interface{}) *bedrockruntime.InvokeModelOutput {
	t.Helper()
	jsonBody, err := json.Marshal(body)
	assert.NoError(t, err, "Failed to marshal mock response body")
	return &bedrockruntime.InvokeModelOutput{
		Body:        jsonBody,
		ContentType: aws.String("application/json"),
	}
}

func TestBedrockEmbeddingProvider_Generate(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name                 string
		input                interface{}
		model                EmbeddingModel
		mockSetup            func(m *MockBedrockClient, model EmbeddingModel, input interface{}) // Function to setup mock expectations
		expectedResponse     *EmbeddingResponse
		expectedErrSubstring string
		expectedInvokeCalls  int
	}{
		{
			name:  "Success - Single Input - Titan",
			input: "Hello Titan",
			model: EmbeddingModel("amazon.titan-embed-text-v1"),
			mockSetup: func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {
				respBody := map[string]interface{}{
					"embedding":           []float32{0.1, 0.2, 0.3},
					"inputTextTokenCount": 3,
				}
				mockOutput := createMockInvokeOutput(t, respBody)

				m.On("InvokeModel", mock.Anything, mock.MatchedBy(func(params *bedrockruntime.InvokeModelInput) bool {
					return *params.ModelId == string(model) && string(params.Body) == `{"inputText":"Hello Titan"}` // Check body precisely
				}), mock.Anything).Return(mockOutput, nil).Once()
			},
			expectedResponse: &EmbeddingResponse{
				Object: "list",
				Data: []EmbeddingObject{
					{Object: "embedding", Embedding: []float32{0.1, 0.2, 0.3}, Index: 0},
				},
				Model: EmbeddingModel("amazon.titan-embed-text-v1"),
				Usage: Usage{PromptTokens: 3, TotalTokens: 3},
			},
			expectedInvokeCalls: 1,
		},
		{
			name:  "Success - Single Input - Cohere",
			input: "Hello Cohere",
			model: EmbeddingModel("cohere.embed-english-v3"),
			mockSetup: func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {
				respBody := map[string]interface{}{
					"embeddings": [][]float32{{0.4, 0.5, 0.6}},
					"meta": map[string]interface{}{
						"billed_units": map[string]interface{}{
							"input_tokens": 4,
						},
					},
				}
				mockOutput := createMockInvokeOutput(t, respBody)

				m.On("InvokeModel", mock.Anything, mock.MatchedBy(func(params *bedrockruntime.InvokeModelInput) bool {
					var bodyMap map[string]interface{}
					err := json.Unmarshal(params.Body, &bodyMap)
					assert.NoError(t, err)
					texts, ok := bodyMap["texts"].([]interface{})
					return *params.ModelId == string(model) &&
						ok && len(texts) == 1 && texts[0] == "Hello Cohere" &&
						bodyMap["input_type"] == "search_document" // default type used in provider
				}), mock.Anything).Return(mockOutput, nil).Once()
			},
			expectedResponse: &EmbeddingResponse{
				Object: "list",
				Data: []EmbeddingObject{
					{Object: "embedding", Embedding: []float32{0.4, 0.5, 0.6}, Index: 0},
				},
				Model: EmbeddingModel("cohere.embed-english-v3"),
				Usage: Usage{PromptTokens: 4, TotalTokens: 4},
			},
			expectedInvokeCalls: 1,
		},
		{
			name:  "Success - Multiple Inputs - Titan",
			input: []string{"Text 1", "Text 2"},
			model: EmbeddingModel("amazon.titan-embed-text-v1"),
			mockSetup: func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {
				respBody1 := map[string]interface{}{"embedding": []float32{1.1, 1.2}, "inputTextTokenCount": 2}
				mockOutput1 := createMockInvokeOutput(t, respBody1)
				m.On("InvokeModel", mock.Anything, mock.MatchedBy(func(params *bedrockruntime.InvokeModelInput) bool {
					return *params.ModelId == string(model) && string(params.Body) == `{"inputText":"Text 1"}`
				}), mock.Anything).Return(mockOutput1, nil).Once()

				// Call 2
				respBody2 := map[string]interface{}{"embedding": []float32{2.1, 2.2}, "inputTextTokenCount": 3}
				mockOutput2 := createMockInvokeOutput(t, respBody2)
				m.On("InvokeModel", mock.Anything, mock.MatchedBy(func(params *bedrockruntime.InvokeModelInput) bool {
					return *params.ModelId == string(model) && string(params.Body) == `{"inputText":"Text 2"}`
				}), mock.Anything).Return(mockOutput2, nil).Once()
			},
			expectedResponse: &EmbeddingResponse{
				Object: "list",
				Data: []EmbeddingObject{
					{Object: "embedding", Embedding: []float32{1.1, 1.2}, Index: 0},
					{Object: "embedding", Embedding: []float32{2.1, 2.2}, Index: 1},
				},
				Model: EmbeddingModel("amazon.titan-embed-text-v1"),
				Usage: Usage{PromptTokens: 5, TotalTokens: 5},
			},
			expectedInvokeCalls: 2,
		},
		{
			name:  "Success - Multiple Inputs - Cohere",
			input: []string{"Batch 1", "Batch 2"},
			model: EmbeddingModel("cohere.embed-multilingual-v3"),
			mockSetup: func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {
				respBody := map[string]interface{}{
					"embeddings": [][]float32{{1.1, 1.2}, {2.1, 2.2}},
					"meta":       map[string]interface{}{"billed_units": map[string]interface{}{"input_tokens": 6}},
				}
				mockOutput := createMockInvokeOutput(t, respBody)

				m.On("InvokeModel", mock.Anything, mock.MatchedBy(func(params *bedrockruntime.InvokeModelInput) bool {
					var bodyMap map[string]interface{}
					err := json.Unmarshal(params.Body, &bodyMap)
					assert.NoError(t, err)
					texts, ok := bodyMap["texts"].([]interface{})
					return *params.ModelId == string(model) &&
						ok && len(texts) == 2 && texts[0] == "Batch 1" && texts[1] == "Batch 2" &&
						bodyMap["input_type"] == "search_document"
				}), mock.Anything).Return(mockOutput, nil).Once()
			},
			expectedResponse: &EmbeddingResponse{
				Object: "list",
				Data: []EmbeddingObject{
					{Object: "embedding", Embedding: []float32{1.1, 1.2}, Index: 0},
					{Object: "embedding", Embedding: []float32{2.1, 2.2}, Index: 1},
				},
				Model: EmbeddingModel("cohere.embed-multilingual-v3"),
				Usage: Usage{PromptTokens: 6, TotalTokens: 6},
			},
			expectedInvokeCalls: 1,
		},
		{
			name:  "Success - Cohere - No Meta/Usage Info",
			input: []string{"Batch 1", "Batch 2"},
			model: EmbeddingModel("cohere.embed-english-v3"),
			mockSetup: func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {
				respBody := map[string]interface{}{
					"embeddings": [][]float32{{1.1, 1.2}, {2.1, 2.2}},
					"id":         "some_id",
				}
				mockOutput := createMockInvokeOutput(t, respBody)

				m.On("InvokeModel", mock.Anything, mock.AnythingOfType("*bedrockruntime.InvokeModelInput"), mock.Anything).Return(mockOutput, nil).Once()
			},
			expectedResponse: &EmbeddingResponse{
				Object: "list",
				Data: []EmbeddingObject{
					{Object: "embedding", Embedding: []float32{1.1, 1.2}, Index: 0},
					{Object: "embedding", Embedding: []float32{2.1, 2.2}, Index: 1},
				},
				Model: EmbeddingModel("cohere.embed-english-v3"),
				Usage: Usage{PromptTokens: 0, TotalTokens: 0},
			},
			expectedInvokeCalls: 1,
		},
		{
			name:                 "Error - Invalid Input Type",
			input:                12345,
			model:                EmbeddingModel("amazon.titan-embed-text-v1"),
			mockSetup:            func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {},
			expectedErrSubstring: "unsupported input type: int",
			expectedInvokeCalls:  0,
		},
		{
			name:                 "Error - Empty String Input",
			input:                "",
			model:                EmbeddingModel("amazon.titan-embed-text-v1"),
			mockSetup:            func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {},
			expectedErrSubstring: "input string cannot be empty",
			expectedInvokeCalls:  0,
		},
		{
			name:                 "Error - Empty Slice Input",
			input:                []string{},
			model:                EmbeddingModel("amazon.titan-embed-text-v1"),
			mockSetup:            func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {},
			expectedErrSubstring: "input string slice cannot be empty",
			expectedInvokeCalls:  0,
		},
		{
			name:                 "Error - Slice Contains Empty String",
			input:                []string{"hello", ""},
			model:                EmbeddingModel("amazon.titan-embed-text-v1"),
			mockSetup:            func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {},
			expectedErrSubstring: "input slice contains an empty string",
			expectedInvokeCalls:  0,
		},
		{
			name:  "Error - Bedrock InvokeModel Fails",
			input: "Test text",
			model: EmbeddingModel("amazon.titan-embed-text-v1"),
			mockSetup: func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {
				expectedErr := errors.New("AWS Bedrock service error")
				m.On("InvokeModel", mock.Anything, mock.AnythingOfType("*bedrockruntime.InvokeModelInput"), mock.Anything).Return(nil, expectedErr).Once()
			},
			expectedErrSubstring: "AWS Bedrock service error",
			expectedInvokeCalls:  1,
		},
		{
			name:  "Error - Titan Response Unmarshal Fails",
			input: "Test text",
			model: EmbeddingModel("amazon.titan-embed-text-v1"),
			mockSetup: func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {
				invalidOutput := &bedrockruntime.InvokeModelOutput{
					Body:        []byte(`{"embedding": "not-a-slice", "inputTextTokenCount": 5}`),
					ContentType: aws.String("application/json"),
				}
				m.On("InvokeModel", mock.Anything, mock.AnythingOfType("*bedrockruntime.InvokeModelInput"), mock.Anything).Return(invalidOutput, nil).Once()
			},
			expectedErrSubstring: "failed to unmarshal titan response body",
			expectedInvokeCalls:  1,
		},
		{
			name:  "Error - Cohere Response Unmarshal Fails",
			input: []string{"Batch 1"},
			model: EmbeddingModel("cohere.embed-english-v3"),
			mockSetup: func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {
				invalidOutput := &bedrockruntime.InvokeModelOutput{
					Body:        []byte(`{"embeddings": ["not-a-slice-of-slices"]}`),
					ContentType: aws.String("application/json"),
				}
				m.On("InvokeModel", mock.Anything, mock.AnythingOfType("*bedrockruntime.InvokeModelInput"), mock.Anything).Return(invalidOutput, nil).Once()
			},
			expectedErrSubstring: "failed to unmarshal cohere response body",
			expectedInvokeCalls:  1,
		},
		{
			name:  "Error - Cohere Response Mismatched Counts",
			input: []string{"Text 1", "Text 2"},
			model: EmbeddingModel("cohere.embed-english-v3"),
			mockSetup: func(m *MockBedrockClient, model EmbeddingModel, input interface{}) {
				respBody := map[string]interface{}{
					"embeddings": [][]float32{{1.1, 1.2}},
					"meta":       map[string]interface{}{"billed_units": map[string]interface{}{"input_tokens": 6}},
				}
				mockOutput := createMockInvokeOutput(t, respBody)
				m.On("InvokeModel", mock.Anything, mock.AnythingOfType("*bedrockruntime.InvokeModelInput"), mock.Anything).Return(mockOutput, nil).Once()
			},
			expectedErrSubstring: "cohere response embeddings count (1) does not match input texts count (2)",
			expectedInvokeCalls:  1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockClient := new(MockBedrockClient)
			if tc.mockSetup != nil {
				tc.mockSetup(mockClient, tc.model, tc.input)
			}

			provider := NewBedrockEmbeddingProviderWithClient(mockClient)

			resp, err := provider.Generate(ctx, tc.input, tc.model)

			if tc.expectedErrSubstring != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrSubstring)
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.Equal(t, tc.expectedResponse.Object, resp.Object)
				assert.Equal(t, tc.expectedResponse.Model, resp.Model)
				assert.Equal(t, tc.expectedResponse.Usage, resp.Usage)
				assert.Equal(t, len(tc.expectedResponse.Data), len(resp.Data))
				for i := range tc.expectedResponse.Data {
					assert.Equal(t, tc.expectedResponse.Data[i].Object, resp.Data[i].Object)
					assert.Equal(t, tc.expectedResponse.Data[i].Index, resp.Data[i].Index)
					assert.Equal(t, tc.expectedResponse.Data[i].Embedding, resp.Data[i].Embedding) // Compare slice values
				}
			}

			mockClient.AssertExpectations(t)
			mockClient.AssertNumberOfCalls(t, "InvokeModel", tc.expectedInvokeCalls)
		})
	}
}

// Test case for uninitialized client
func TestBedrockEmbeddingProvider_Generate_NilClient(t *testing.T) {
	provider := &BedrockEmbeddingProvider{client: nil}
	_, err := provider.Generate(context.Background(), "test", "amazon.titan-embed-text-v1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bedrock client is not initialized")
}
