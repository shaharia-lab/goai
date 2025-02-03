// Package goai provides artificial intelligence utilities including embedding generation capabilities.
package goai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	if err := validateInput(input, model); err != nil {
		return nil, err
	}

	resp, err := s.provider.Generate(ctx, input, model)
	if err != nil {
		return nil, err
	}

	if err := validateEmbeddingResponse(resp); err != nil {
		return nil, err
	}

	return resp, nil
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
	if err := validateInput(input, model); err != nil {
		return nil, err
	}

	reqBody := embeddingRequest{
		Input:          input,
		Model:          model,
		EncodingFormat: "float",
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/embeddings", bytes.NewBuffer(jsonBody))
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
		switch resp.StatusCode {
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("rate limit exceeded: %d", resp.StatusCode)
		case http.StatusInternalServerError:
			return nil, fmt.Errorf("server error: %d", resp.StatusCode)
		default:
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	}

	var embResp EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if err := validateEmbeddingResponse(&embResp); err != nil {
		return nil, err
	}

	return &embResp, nil
}

func validateInput(input interface{}, model EmbeddingModel) error {
	if input == nil {
		return fmt.Errorf("input cannot be nil")
	}

	if model == "" {
		return fmt.Errorf("model cannot be empty")
	}

	switch v := input.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("input cannot be empty")
		}
	case []string:
		if len(v) == 0 {
			return fmt.Errorf("input array cannot be empty")
		}
		for i, s := range v {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("input strings cannot be empty at index %d", i)
			}
		}
	default:
		return fmt.Errorf("unsupported input type: %T", input)
	}

	return nil
}

// ValidateEmbeddingResponse validates that an embedding response meets all requirements:
// - Response must not be nil
// - Must contain at least one embedding
// - All embedding vectors must be non-nil and non-empty
// - Indices must be sequential and match array position
func validateEmbeddingResponse(resp *EmbeddingResponse) error {
	if resp == nil {
		return fmt.Errorf("response cannot be nil")
	}
	if len(resp.Data) == 0 {
		return fmt.Errorf("response must contain at least one embedding")
	}

	dimension := -1
	for i, obj := range resp.Data {
		if obj.Embedding == nil || len(obj.Embedding) == 0 {
			return fmt.Errorf("embedding vector at index %d cannot be nil or empty", i)
		}

		// Verify consistent dimensions
		if dimension == -1 {
			dimension = len(obj.Embedding)
		} else if len(obj.Embedding) != dimension {
			return fmt.Errorf("inconsistent embedding dimensions: got %d, want %d at index %d",
				len(obj.Embedding), dimension, i)
		}

		if obj.Index != i {
			return fmt.Errorf("invalid embedding index: got %d, want %d", obj.Index, i)
		}
	}
	return nil
}
