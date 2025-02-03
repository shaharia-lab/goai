package goai

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mockEmbeddingProvider implements EmbeddingProvider for testing
type mockEmbeddingProvider struct {
	response *EmbeddingResponse
	err      error
}

func (m *mockEmbeddingProvider) Generate(ctx context.Context, input interface{}, model EmbeddingModel) (*EmbeddingResponse, error) {
	if err := validateInput(input, model); err != nil {
		return nil, err
	}

	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func timeoutHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		h.ServeHTTP(w, r)
	})
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

func TestEmbeddingService_Generate_InputValidation(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		model    EmbeddingModel
		wantErr  bool
		errCheck func(error) bool
	}{
		{
			name:    "nil input",
			input:   nil,
			model:   EmbeddingModelAllMiniLML6V2,
			wantErr: true,
			errCheck: func(err error) bool {
				return err != nil && strings.Contains(err.Error(), "input cannot be nil")
			},
		},
		{
			name:    "empty string input",
			input:   "",
			model:   EmbeddingModelAllMiniLML6V2,
			wantErr: true,
			errCheck: func(err error) bool {
				return err != nil && strings.Contains(err.Error(), "input cannot be empty")
			},
		},
		{
			name:    "empty string array",
			input:   []string{},
			model:   EmbeddingModelAllMiniLML6V2,
			wantErr: true,
			errCheck: func(err error) bool {
				return err != nil && strings.Contains(err.Error(), "input array cannot be empty")
			},
		},
		{
			name:    "array with empty strings",
			input:   []string{"valid", "", "also valid"},
			model:   EmbeddingModelAllMiniLML6V2,
			wantErr: true,
			errCheck: func(err error) bool {
				return err != nil && strings.Contains(err.Error(), "input strings cannot be empty")
			},
		},
		{
			name:    "invalid input type",
			input:   123,
			model:   EmbeddingModelAllMiniLML6V2,
			wantErr: true,
			errCheck: func(err error) bool {
				return err != nil && strings.Contains(err.Error(), "unsupported input type")
			},
		},
		{
			name:    "empty model",
			input:   "test",
			model:   "",
			wantErr: true,
			errCheck: func(err error) bool {
				return err != nil && strings.Contains(err.Error(), "model cannot be empty")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &mockEmbeddingProvider{}
			service := NewEmbeddingService(mockProvider)

			_, err := service.Generate(context.Background(), tt.input, tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !tt.errCheck(err) {
				t.Errorf("Generate() error = %v, expected error matching checker", err)
			}
		})
	}
}

func TestOpenAICompatibleEmbeddingProvider_Generate_ErrorHandling(t *testing.T) {
	tests := []struct {
		name      string
		input     interface{}
		model     EmbeddingModel
		setupMock func(w http.ResponseWriter, r *http.Request)
		wantErr   bool
		errCheck  func(error) bool
	}{
		{
			name:  "rate limit error",
			input: "test",
			model: EmbeddingModelAllMiniLML6V2,
			setupMock: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			wantErr: true,
			errCheck: func(err error) bool {
				return strings.Contains(err.Error(), "rate limit exceeded")
			},
		},
		{
			name:  "server error",
			input: "test",
			model: EmbeddingModelAllMiniLML6V2,
			setupMock: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
			errCheck: func(err error) bool {
				return strings.Contains(err.Error(), "server error")
			},
		},
		{
			name:  "timeout error",
			input: "test",
			model: EmbeddingModelAllMiniLML6V2,
			setupMock: func(w http.ResponseWriter, r *http.Request) {
				// Sleep longer than the client timeout
				time.Sleep(2 * time.Second)
				// No need to write anything as the client will timeout before getting here
			},
			wantErr: true,
			errCheck: func(err error) bool {
				// On timeout, we expect either context deadline exceeded or timeout error
				return strings.Contains(err.Error(), "timeout") ||
					strings.Contains(err.Error(), "deadline exceeded") ||
					strings.Contains(err.Error(), "EOF")
			},
		},
		{
			name:  "invalid response format",
			input: "test",
			model: EmbeddingModelAllMiniLML6V2,
			setupMock: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("invalid json"))
			},
			wantErr: true,
			errCheck: func(err error) bool {
				return strings.Contains(err.Error(), "failed to decode response")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.setupMock(w, r)
			}))
			defer server.Close()

			// Create client with a very short timeout for timeout tests
			client := &http.Client{
				Timeout: 100 * time.Millisecond,
			}

			provider := NewOpenAICompatibleEmbeddingProvider(server.URL, client)
			_, err := provider.Generate(context.Background(), tt.input, tt.model)

			if (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && !tt.errCheck(err) {
				t.Errorf("Generate() error = %v, expected error matching checker", err)
			}
		})
	}
}

func TestEmbeddingResponse_Validation(t *testing.T) {
	tests := []struct {
		name     string
		response *EmbeddingResponse
		wantErr  bool
	}{
		{
			name: "valid response",
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
			name: "empty embeddings",
			response: &EmbeddingResponse{
				Object: "list",
				Data:   []EmbeddingObject{},
				Model:  EmbeddingModelAllMiniLML6V2,
			},
			wantErr: true,
		},
		{
			name: "nil embedding vector",
			response: &EmbeddingResponse{
				Object: "list",
				Data: []EmbeddingObject{
					{
						Object:    "embedding",
						Embedding: nil,
						Index:     0,
					},
				},
				Model: EmbeddingModelAllMiniLML6V2,
			},
			wantErr: true,
		},
		{
			name: "invalid index order",
			response: &EmbeddingResponse{
				Object: "list",
				Data: []EmbeddingObject{
					{
						Object:    "embedding",
						Embedding: []float32{0.1, 0.2},
						Index:     1,
					},
					{
						Object:    "embedding",
						Embedding: []float32{0.3, 0.4},
						Index:     0,
					},
				},
				Model: EmbeddingModelAllMiniLML6V2,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEmbeddingResponse(tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEmbeddingResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOpenAICompatibleEmbeddingProvider_Concurrency(t *testing.T) {
	// Track number of concurrent requests
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Increment concurrent request counter
		current := requestCount.Add(1)
		defer requestCount.Add(-1)

		// Add random delay to simulate real-world conditions
		time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)

		// Track maximum concurrent requests
		if current > 5 {
			t.Logf("High concurrency detected: %d simultaneous requests", current)
		}

		// Verify request headers and body
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("missing content-type header")
		}

		var reqBody embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&EmbeddingResponse{
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
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleEmbeddingProvider(server.URL, server.Client())

	// Use errgroup for better error handling in goroutines
	g := new(errgroup.Group)
	const numRequests = 10

	// Create test inputs
	inputs := []string{
		"test text 1",
		"another test text",
		"yet another test",
		"more test content",
		"final test text",
	}

	// Launch concurrent requests
	for i := 0; i < numRequests; i++ {
		input := inputs[i%len(inputs)] // Cycle through test inputs
		g.Go(func() error {
			resp, err := provider.Generate(context.Background(), input, EmbeddingModelAllMiniLML6V2)
			if err != nil {
				return fmt.Errorf("generate failed: %w", err)
			}
			if len(resp.Data) == 0 {
				return fmt.Errorf("empty response data")
			}
			return nil
		})
	}

	// Wait for all requests and check for errors
	if err := g.Wait(); err != nil {
		t.Errorf("concurrent requests failed: %v", err)
	}
}

// Example of when to use concurrent embedding generation in real applications:

func GenerateEmbeddingsBatch(ctx context.Context, provider EmbeddingProvider, texts []string) ([][]float32, error) {
	// For small batches, just use the batch API
	if len(texts) <= 8 {
		resp, err := provider.Generate(ctx, texts, EmbeddingModelAllMiniLML6V2)
		if err != nil {
			return nil, err
		}

		vectors := make([][]float32, len(resp.Data))
		for i, obj := range resp.Data {
			vectors[i] = obj.Embedding
		}
		return vectors, nil
	}

	// For larger batches, split into chunks and process concurrently
	const chunkSize = 8
	chunks := (len(texts) + chunkSize - 1) / chunkSize
	vectors := make([][]float32, len(texts))

	g := new(errgroup.Group)
	g.SetLimit(4) // Limit concurrent API calls

	for i := 0; i < chunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(texts) {
			end = len(texts)
		}

		// Capture loop variables
		i, start, end := i, start, end
		chunk := texts[start:end]

		g.Go(func() error {
			resp, err := provider.Generate(ctx, chunk, EmbeddingModelAllMiniLML6V2)
			if err != nil {
				return fmt.Errorf("chunk %d failed: %w", i, err)
			}

			// Copy results to correct positions
			for j, obj := range resp.Data {
				vectors[start+j] = obj.Embedding
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return vectors, nil
}

// Example usage with rate limiting:

type RateLimitedEmbeddingProvider struct {
	provider EmbeddingProvider
	limiter  *rate.Limiter
}

func NewRateLimitedProvider(provider EmbeddingProvider, rps float64) *RateLimitedEmbeddingProvider {
	return &RateLimitedEmbeddingProvider{
		provider: provider,
		limiter:  rate.NewLimiter(rate.Limit(rps), 1),
	}
}

func (p *RateLimitedEmbeddingProvider) Generate(ctx context.Context, input interface{}, model EmbeddingModel) (*EmbeddingResponse, error) {
	err := p.limiter.Wait(ctx)
	if err != nil {
		return nil, fmt.Errorf("rate limit wait failed: %w", err)
	}
	return p.provider.Generate(ctx, input, model)
}
