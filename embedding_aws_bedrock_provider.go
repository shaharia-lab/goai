package goai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings" // Added for model checking

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// BedrockEmbeddingProvider implements the EmbeddingProvider interface using AWS Bedrock.
type BedrockEmbeddingProvider struct {
	client BedrockClient
}

// NewBedrockEmbeddingProvider creates a new BedrockEmbeddingProvider.
func NewBedrockEmbeddingProvider(ctx context.Context, awsRegion string) (*BedrockEmbeddingProvider, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(awsRegion))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	// Create the Bedrock Runtime client
	brClient := bedrockruntime.NewFromConfig(cfg)

	return &BedrockEmbeddingProvider{
		client: brClient,
	}, nil
}

// NewBedrockEmbeddingProviderWithClient creates a new provider with an existing Bedrock client.
func NewBedrockEmbeddingProviderWithClient(client BedrockClient) *BedrockEmbeddingProvider {
	return &BedrockEmbeddingProvider{
		client: client,
	}
}

// Generate creates embedding vectors using AWS Bedrock.
func (b *BedrockEmbeddingProvider) Generate(ctx context.Context, input interface{}, model EmbeddingModel) (*EmbeddingResponse, error) {
	if b.client == nil {
		return nil, errors.New("bedrock client is not initialized")
	}

	var texts []string
	switch v := input.(type) {
	case string:
		if v == "" {
			return nil, errors.New("input string cannot be empty")
		}
		texts = []string{v}
	case []string:
		if len(v) == 0 {
			return nil, errors.New("input string slice cannot be empty")
		}

		for _, s := range v {
			if s == "" {
				return nil, errors.New("input slice contains an empty string")
			}
		}
		texts = v
	default:
		return nil, fmt.Errorf("unsupported input type: %T", input)
	}

	modelStr := string(model)
	if strings.HasPrefix(modelStr, "amazon.titan-embed") {
		return b.generateWithTitan(ctx, texts, model)
	} else if strings.HasPrefix(modelStr, "cohere.embed") {
		return b.generateWithCohere(ctx, texts, model, "search_document")
	} else {
		fmt.Printf("Warning: Unknown model family '%s'. Attempting Titan format.\n", model)
		return b.generateWithTitan(ctx, texts, model)
	}
}

// generateWithTitan handles embedding generation for Amazon Titan models.
// Note: Titan embedding models typically handle one input text per API call.
// This implementation makes sequential calls for multiple texts.
func (b *BedrockEmbeddingProvider) generateWithTitan(ctx context.Context, texts []string, model EmbeddingModel) (*EmbeddingResponse, error) {
	embeddings := make([]EmbeddingObject, 0, len(texts))
	totalTokens := 0

	for i, text := range texts {
		requestBody := map[string]string{
			"inputText": text,
		}
		bodyBytes, err := json.Marshal(requestBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal titan request body: %w", err)
		}

		resp, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
			ModelId:     aws.String(string(model)),
			Body:        bodyBytes,
			ContentType: aws.String("application/json"),
			Accept:      aws.String("application/json"),
		})
		if err != nil {
			return nil, fmt.Errorf("bedrock InvokeModel failed for titan (index %d): %w", i, err)
		}

		var titanResp struct {
			Embedding           []float32 `json:"embedding"`
			InputTextTokenCount int       `json:"inputTextTokenCount"`
		}
		if err := json.Unmarshal(resp.Body, &titanResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal titan response body (index %d): %w", i, err)
		}

		embeddings = append(embeddings, EmbeddingObject{
			Object:    "embedding",
			Embedding: titanResp.Embedding,
			Index:     i,
		})
		totalTokens += titanResp.InputTextTokenCount
	}

	return &EmbeddingResponse{
		Object: "list",
		Data:   embeddings,
		Model:  model,
		Usage: Usage{
			PromptTokens: totalTokens,
			TotalTokens:  totalTokens,
		},
	}, nil
}

// generateWithCohere handles embedding generation for Cohere models.
// Cohere models typically support batching multiple texts in one call.
// inputType examples: "search_document", "search_query", "classification", "clustering"
func (b *BedrockEmbeddingProvider) generateWithCohere(ctx context.Context, texts []string, model EmbeddingModel, inputType string) (*EmbeddingResponse, error) {
	// Request Body for Cohere
	requestBody := map[string]interface{}{
		"texts":      texts,
		"input_type": inputType,
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cohere request body: %w", err)
	}

	resp, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(string(model)),
		Body:        bodyBytes,
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	})
	if err != nil {
		return nil, fmt.Errorf("bedrock InvokeModel failed for cohere: %w", err)
	}

	var cohereResp struct {
		Embeddings [][]float32 `json:"embeddings"`
		Meta       *struct {
			BilledUnits *struct {
				InputTokens int `json:"input_tokens"`
			} `json:"billed_units"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(resp.Body, &cohereResp); err != nil {
		var fallbackResp struct {
			Embeddings [][]float32 `json:"embeddings"`
		}
		if errAlt := json.Unmarshal(resp.Body, &fallbackResp); errAlt == nil {
			cohereResp.Embeddings = fallbackResp.Embeddings
		} else {
			return nil, fmt.Errorf("failed to unmarshal cohere response body (tried primary and fallback structures): %w, fallback error: %w", err, errAlt)
		}
	}

	if len(cohereResp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("cohere response embeddings count (%d) does not match input texts count (%d)", len(cohereResp.Embeddings), len(texts))
	}

	embeddings := make([]EmbeddingObject, len(texts))
	for i, emb := range cohereResp.Embeddings {
		embeddings[i] = EmbeddingObject{
			Object:    "embedding",
			Embedding: emb,
			Index:     i,
		}
	}

	promptTokens := 0
	if cohereResp.Meta != nil && cohereResp.Meta.BilledUnits != nil {
		promptTokens = cohereResp.Meta.BilledUnits.InputTokens
	} else {
		fmt.Printf("Warning: Could not extract token usage info from Cohere model '%s' response.\n", model)
	}

	return &EmbeddingResponse{
		Object: "list",
		Data:   embeddings,
		Model:  model,
		Usage: Usage{
			PromptTokens: promptTokens,
			TotalTokens:  promptTokens,
		},
	}, nil
}
