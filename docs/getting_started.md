# Getting Started with GoAI

## Prerequisites

- Go 1.21 or later
- PostgreSQL 11+ with pgvector extension (for vector storage)

## Installation

```bash
go get github.com/shaharia-lab/goai
```

## LLM Integration

```go
import "github.com/shaharia-lab/goai"

// Using OpenAI
client := goai.NewOpenAIClient("your-api-key")
provider := goai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
})

// Configure request
config := goai.NewRequestConfig(
    goai.WithMaxToken(1000),
    goai.WithTopP(0.9),
    goai.WithTemperature(0.7),
)
llm := goai.NewLLMRequest(config, provider)

// Generate response
response, err := llm.Generate([]goai.LLMMessage{
    {Role: goai.SystemRole, Text: "You are a helpful assistant"},
    {Role: goai.UserRole, Text: "Hello"},
})

// Using Anthropic
client := goai.NewAnthropicClient("your-api-key")
provider := goai.NewAnthropicLLMProvider(ai.AnthropicProviderConfig{
    Client: client,
})

// Using AWS Bedrock
provider := goai.NewBedrockLLMProvider(ai.BedrockProviderConfig{
    Client: bedrockClient,
})
```

## Embeddings

```go
service := goai.NewEmbeddingService(baseURL, nil)
response, err := service.GenerateEmbedding(
    context.Background(),
    "Hello world",
    goai.EmbeddingModelAllMiniLML6V2,
)
```

## Vector Storage

```go
config := goai.PostgresStorageConfig{
    ConnectionString: "postgres://user:pass@localhost:5432/dbname",
    MaxDimension:    384,
}

provider, err := goai.NewPostgresProvider(config)
if err != nil {
    log.Fatal(err)
}

storage, err := goai.NewVectorStorage(context.Background(), provider)
if err != nil {
    log.Fatal(err)
}

// Create collection
err = storage.CreateCollection(ctx, &ai.VectorCollectionConfig{
    Name:         "documents",
    Dimension:    384,
    IndexType:    goai.IndexTypeHNSW,
    DistanceType: goai.DistanceTypeCosine,
})

// Store document
doc := &ai.VectorDocument{
    ID:      "doc1",
    Vector:  embedding.Data[0].Embedding,
    Content: "Document content",
}
err = storage.UpsertDocument(ctx, "documents", doc)

// Search
results, err := storage.SearchByVector(ctx, "documents", queryVector, &ai.VectorSearchOptions{
    Limit: 10,
})
```

## Error Types

```go
// LLM errors
type LLMError struct {
    Code    int
    Message string
}

// Vector storage errors
type VectorError struct {
    Code    int
    Message string
    Err     error
}

// Common error codes
const (
    ErrCodeNotFound           = 404
    ErrCodeInvalidDimension   = 400
    ErrCodeInvalidConfig      = 401
    ErrCodeCollectionExists   = 402
    ErrCodeCollectionNotFound = 403
)
```

## Next Steps

- [Language Models Documentation](llm/index.md)
- [Embeddings Documentation](embeddings/index.md)
- [Vector Storage Documentation](vector-store/index.md)
