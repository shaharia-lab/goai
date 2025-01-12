# Text Embeddings

Generate vector embeddings from text using the EmbeddingService interface.

## Basic Usage

```go
// Initialize the service
service := ai.NewEmbeddingService(baseURL, httpClient)

// Single text embedding
response, err := service.GenerateEmbedding(
    context.Background(),
    "Hello world",
    ai.EmbeddingModelAllMiniLML6V2,
)
if err != nil {
    return err
}
fmt.Printf("Generated embedding vector: %v\n", response.Data[0].Embedding)

// Multiple text embeddings
texts := []string{"First text", "Second text"}
response, err := service.GenerateEmbedding(
    context.Background(),
    texts,
    ai.EmbeddingModelAllMiniLML6V2,
)
```

## Response Structure

```go
type EmbeddingResponse struct {
    Object string            `json:"object"`
    Data   []EmbeddingObject `json:"data"`
    Model  EmbeddingModel    `json:"model"`
    Usage  Usage             `json:"usage"`
}

type EmbeddingObject struct {
    Object    string    `json:"object"`
    Embedding []float32 `json:"embedding"`
    Index     int       `json:"index"`
}

type Usage struct {
    PromptTokens int `json:"prompt_tokens"`
    TotalTokens  int `json:"total_tokens"`
}
```

## Available Models

1. **all-MiniLM-L6-v2**

   ```go
   ai.EmbeddingModelAllMiniLML6V2
   ```

   - Lightweight model for general-purpose embedding generation
   - Good balance between performance and quality

2. **all-mpnet-base-v2**

   ```go
   ai.EmbeddingModelAllMpnetBaseV2
   ```

   - Higher quality embeddings
   - More computationally intensive

3. **paraphrase-multilingual-MiniLM-L12-v2**

   ```go
   ai.EmbeddingModelParaphraseMultilingualMiniLML12V2
   ```

   - Specialized for multilingual text
   - Supports embedding generation across multiple languages

## Error Handling

The service returns custom error types for different scenarios:

```go
response, err := service.GenerateEmbedding(ctx, input, model)
if err != nil {
    if llmErr, ok := err.(*ai.LLMError); ok {
        fmt.Printf("LLM error %d: %s\n", llmErr.Code, llmErr.Message)
        return
    }
    fmt.Printf("Unexpected error: %v\n", err)
}
```

## Configuration

When creating a new embedding service:

```go
service := ai.NewEmbeddingService(
    "https://api.example.com",  // Base URL for the embedding API
    &http.Client{              // Optional custom HTTP client
        Timeout: 30 * time.Second,
    },
)

// If no HTTP client is provided, http.DefaultClient will be used
service := ai.NewEmbeddingService("https://api.example.com", nil)
```
