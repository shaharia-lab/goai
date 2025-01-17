package goai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// mockEmbeddingProvider implements EmbeddingProvider for testing
type mockEmbeddingProvider struct {
	response *EmbeddingResponse
	err      error
}

func (m *mockEmbeddingProvider) Generate(ctx context.Context, input interface{}, model EmbeddingModel) (*EmbeddingResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestEmbeddingService_Generate(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		model    EmbeddingModel
		response *EmbeddingResponse
		wantErr  bool
	}{
		{
			name:  "successful single string input",
			input: "test text",
			model: EmbeddingModelAllMiniLML6V2,
			response: &EmbeddingResponse{
				Object: "list",
				Data: []EmbeddingObject{
					{
						Object:    "embedding",
						Embedding: []float32{0.1, 0.2, 0.3},
						Index:     0,
					},
				},
				Model: EmbeddingModelAllMiniLML6V2,
				Usage: Usage{
					PromptTokens: 2,
					TotalTokens:  2,
				},
			},
			wantErr: false,
		},
		{
			name:  "successful multiple string input",
			input: []string{"test1", "test2"},
			model: EmbeddingModelAllMpnetBaseV2,
			response: &EmbeddingResponse{
				Object: "list",
				Data: []EmbeddingObject{
					{
						Object:    "embedding",
						Embedding: []float32{0.1, 0.2, 0.3},
						Index:     0,
					},
					{
						Object:    "embedding",
						Embedding: []float32{0.4, 0.5, 0.6},
						Index:     1,
					},
				},
				Model: EmbeddingModelAllMpnetBaseV2,
				Usage: Usage{
					PromptTokens: 4,
					TotalTokens:  4,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &mockEmbeddingProvider{
				response: tt.response,
			}
			service := NewEmbeddingService(mockProvider)

			gotResp, err := service.Generate(context.Background(), tt.input, tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(gotResp, tt.response) {
				t.Errorf("Generate() = %v, want %v", gotResp, tt.response)
			}
		})
	}
}

func TestOpenAICompatibleEmbeddingProvider_Generate(t *testing.T) {
	tests := []struct {
		name       string
		input      interface{}
		model      EmbeddingModel
		statusCode int
		response   *EmbeddingResponse
		wantErr    bool
	}{
		{
			name:       "successful request",
			input:      "test text",
			model:      EmbeddingModelAllMiniLML6V2,
			statusCode: http.StatusOK,
			response: &EmbeddingResponse{
				Object: "list",
				Data: []EmbeddingObject{
					{
						Object:    "embedding",
						Embedding: []float32{0.1, 0.2, 0.3},
						Index:     0,
					},
				},
				Model: EmbeddingModelAllMiniLML6V2,
				Usage: Usage{
					PromptTokens: 2,
					TotalTokens:  2,
				},
			},
			wantErr: false,
		},
		{
			name:       "server error",
			input:      "test text",
			model:      EmbeddingModelAllMiniLML6V2,
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and headers
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST request, got %s", r.Method)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
				}

				// Verify request body
				var reqBody embeddingRequest
				if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}
				if reqBody.Model != tt.model {
					t.Errorf("Expected model %s, got %s", tt.model, reqBody.Model)
				}
				if reqBody.EncodingFormat != "float" {
					t.Errorf("Expected encoding_format 'float', got %s", reqBody.EncodingFormat)
				}

				// Send response
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					json.NewEncoder(w).Encode(tt.response)
				}
			}))
			defer server.Close()

			provider := NewOpenAICompatibleEmbeddingProvider(server.URL, server.Client())
			gotResp, err := provider.Generate(context.Background(), tt.input, tt.model)

			if (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !reflect.DeepEqual(gotResp, tt.response) {
				t.Errorf("Generate() = %v, want %v", gotResp, tt.response)
			}
		})
	}
}
