package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestEmbeddingService_GenerateEmbedding(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		model    EmbeddingModel
		response EmbeddingResponse
		wantErr  bool
	}{
		{
			name:  "single string input",
			input: "test text",
			model: "all-MiniLM-L6-v2",
			response: EmbeddingResponse{
				Object: "list",
				Data: []EmbeddingObject{
					{
						Object:    "embedding",
						Embedding: []float32{0.1, 0.2, 0.3},
						Index:     0,
					},
				},
				Model: "all-MiniLM-L6-v2",
				Usage: Usage{
					PromptTokens: 2,
					TotalTokens:  2,
				},
			},
		},
		{
			name:  "multiple string input",
			input: []string{"test1", "test2"},
			model: "all-MiniLM-L6-v2",
			response: EmbeddingResponse{
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
				Model: EmbeddingModelAllMiniLML6V2,
				Usage: Usage{
					PromptTokens: 4,
					TotalTokens:  4,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
				}

				json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()

			service := NewEmbeddingService(server.URL, server.Client())
			gotResp, err := service.GenerateEmbedding(context.Background(), tt.input, tt.model)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateEmbedding() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(gotResp, &tt.response) {
				t.Errorf("GenerateEmbedding() = %v, want %v", gotResp, tt.response)
			}
		})
	}
}
