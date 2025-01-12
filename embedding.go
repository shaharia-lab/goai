// Package ai provides artificial intelligence utilities including embedding generation capabilities.
// It offers a flexible interface for generating text embeddings using various models and providers.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// EmbeddingModel represents the type of embedding model to be used for generating embeddings.
type EmbeddingModel string

// Available embedding models that can be used with the EmbeddingService.
// These models provide different trade-offs between performance and accuracy.
const (
	// EmbeddingModelAllMiniLML6V2 is a lightweight model suitable for general-purpose embedding generation.
	// It provides a good balance between performance and quality.
	EmbeddingModelAllMiniLML6V2 EmbeddingModel = "all-MiniLM-L6-v2"

	// EmbeddingModelAllMpnetBaseV2 is a more powerful model that provides higher quality embeddings
	// at the cost of increased computation time.
	EmbeddingModelAllMpnetBaseV2 EmbeddingModel = "all-mpnet-base-v2"

	// EmbeddingModelParaphraseMultilingualMiniLML12V2 is specialized for multilingual text,
	// supporting embedding generation across multiple languages while maintaining semantic meaning.
	EmbeddingModelParaphraseMultilingualMiniLML12V2 EmbeddingModel = "paraphrase-multilingual-MiniLM-L12-v2"
)

// EmbeddingProvider defines the interface for services that can generate embeddings from text.
// Implementations of this interface can connect to different embedding services or models.
type EmbeddingProvider interface {
	// GenerateEmbedding creates an embedding vector from the provided input using the specified model.
	// The input can be a string or array of strings, and the response includes the embedding vectors
	// along with usage statistics.
	GenerateEmbedding(ctx context.Context, input interface{}, model string) (*EmbeddingResponse, error)
}

// EmbeddingObject represents a single embedding result containing the generated vector
// and metadata about the embedding.
type EmbeddingObject struct {
	// Object identifies the type of the response object
	Object string `json:"object"`
	// Embedding is the generated vector representation of the input text
	Embedding []float32 `json:"embedding"`
	// Index is the position of this embedding in the response array
	Index int `json:"index"`
}

// Usage represents token usage information for the embedding generation request.
type Usage struct {
	// PromptTokens is the number of tokens in the input text
	PromptTokens int `json:"prompt_tokens"`
	// TotalTokens is the total number of tokens processed
	TotalTokens int `json:"total_tokens"`
}

// EmbeddingResponse represents the complete response from the embedding API.
// It includes the generated embeddings and usage statistics.
type EmbeddingResponse struct {
	// Object identifies the type of the response
	Object string `json:"object"`
	// Data contains the array of embedding results
	Data []EmbeddingObject `json:"data"`
	// Model identifies which embedding model was used
	Model EmbeddingModel `json:"model"`
	// Usage provides token usage statistics for the request
	Usage Usage `json:"usage"`
}

// EmbeddingService implements the EmbeddingProvider interface for generating embeddings
// using a REST API endpoint.
type EmbeddingService struct {
	// BaseURL is the base URL of the embedding API
	BaseURL string
	// HTTPClient is the HTTP client used for making requests
	HTTPClient *http.Client
}

// NewEmbeddingService creates a new EmbeddingService with the specified base URL and HTTP client.
// If httpClient is nil, it uses http.DefaultClient.
//
// Example usage:
//
//	client := NewEmbeddingService("https://api.example.com", nil)
//	resp, err := client.GenerateEmbedding(
//	    context.Background(),
//	    "Hello, world!",
//	    EmbeddingModelAllMiniLML6V2,
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Generated embedding vector: %v\n", resp.Data[0].Embedding)
func NewEmbeddingService(baseURL string, httpClient *http.Client) *EmbeddingService {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &EmbeddingService{
		BaseURL:    baseURL,
		HTTPClient: httpClient,
	}
}

// embeddingRequest represents the request body sent to the embedding API.
type embeddingRequest struct {
	// Input is the text to generate embeddings for (string or []string)
	Input interface{} `json:"input"`
	// Model specifies which embedding model to use
	Model EmbeddingModel `json:"model"`
	// EncodingFormat specifies the format of the output vectors
	EncodingFormat string `json:"encoding_format"`
}

// GenerateEmbedding generates embedding vectors for the provided input using the specified model.
// The input can be a single string or an array of strings. The method returns the embedding
// vectors along with usage statistics.
//
// Example usage:
//
//	service := NewEmbeddingService("https://api.example.com", nil)
//
//	// Generate embedding for a single string
//	resp, err := service.GenerateEmbedding(
//	    context.Background(),
//	    "Hello, world!",
//	    EmbeddingModelAllMiniLML6V2,
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Generate embeddings for multiple strings
//	texts := []string{"Hello", "World"}
//	resp, err = service.GenerateEmbedding(
//	    context.Background(),
//	    texts,
//	    EmbeddingModelAllMpnetBaseV2,
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// The method returns an error if:
//   - The request cannot be created or sent
//   - The server returns a non-200 status code
//   - The response cannot be decoded
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, input interface{}, model EmbeddingModel) (*EmbeddingResponse, error) {
	reqBody := embeddingRequest{
		Input:          input,
		Model:          model,
		EncodingFormat: "float",
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.BaseURL+"/v1/embeddings", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var embResp EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &embResp, nil
}
