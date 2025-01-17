// Package goai provides artificial intelligence utilities including embedding generation capabilities.
package goai

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
const (
	// EmbeddingModelAllMiniLML6V2 is a lightweight model suitable for general-purpose embedding generation.
	EmbeddingModelAllMiniLML6V2 EmbeddingModel = "all-MiniLM-L6-v2"

	// EmbeddingModelAllMpnetBaseV2 is a more powerful model that provides higher quality embeddings.
	EmbeddingModelAllMpnetBaseV2 EmbeddingModel = "all-mpnet-base-v2"

	// EmbeddingModelParaphraseMultilingualMiniLML12V2 is specialized for multilingual text.
	EmbeddingModelParaphraseMultilingualMiniLML12V2 EmbeddingModel = "paraphrase-multilingual-MiniLM-L12-v2"
)

// EmbeddingProvider defines the interface for services that can generate embeddings from text.
type EmbeddingProvider interface {
	// Generate creates embedding vectors from the provided input using the specified model.
	// The input can be a string or array of strings.
	Generate(ctx context.Context, input interface{}, model EmbeddingModel) (*EmbeddingResponse, error)
}

// EmbeddingService provides a high-level interface for generating embeddings using a provider.
type EmbeddingService struct {
	provider EmbeddingProvider
}

// NewEmbeddingService creates a new EmbeddingService with the specified provider.
func NewEmbeddingService(provider EmbeddingProvider) *EmbeddingService {
	return &EmbeddingService{
		provider: provider,
	}
}

// Generate creates embeddings using the configured provider.
func (s *EmbeddingService) Generate(ctx context.Context, input interface{}, model EmbeddingModel) (*EmbeddingResponse, error) {
	return s.provider.Generate(ctx, input, model)
}

// EmbeddingObject represents a single embedding result containing the generated vector.
type EmbeddingObject struct {
	Object    string    `json:"object"`
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

// Usage represents token usage information for the embedding generation request.
type Usage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// EmbeddingResponse represents the complete response from the embedding generation.
type EmbeddingResponse struct {
	Object string            `json:"object"`
	Data   []EmbeddingObject `json:"data"`
	Model  EmbeddingModel    `json:"model"`
	Usage  Usage             `json:"usage"`
}

// OpenAICompatibleEmbeddingProvider implements EmbeddingProvider using an OpenAI-compatible REST API.
type OpenAICompatibleEmbeddingProvider struct {
	baseURL    string
	httpClient *http.Client
}

// NewOpenAICompatibleEmbeddingProvider creates a new provider that works with OpenAI-compatible APIs.
func NewOpenAICompatibleEmbeddingProvider(baseURL string, httpClient *http.Client) *OpenAICompatibleEmbeddingProvider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenAICompatibleEmbeddingProvider{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

type embeddingRequest struct {
	Input          interface{}    `json:"input"`
	Model          EmbeddingModel `json:"model"`
	EncodingFormat string         `json:"encoding_format"`
}

// Generate implements EmbeddingProvider.Generate for OpenAI-compatible APIs.
func (p *OpenAICompatibleEmbeddingProvider) Generate(ctx context.Context, input interface{}, model EmbeddingModel) (*EmbeddingResponse, error) {
	reqBody := embeddingRequest{
		Input:          input,
		Model:          model,
		EncodingFormat: "float",
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/embeddings", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
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
