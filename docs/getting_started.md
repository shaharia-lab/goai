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
client := ai.NewRealOpenAIClient("your-api-key")
provider := ai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
})

// Configure request
config := ai.NewRequestConfig(
    ai.WithMaxToken(1000),
    ai.WithTopP(0.9),
    ai.WithTemperature(0.7),
)
llm := ai.NewLLMRequest(config, provider)

// Generate response
response, err := llm.Generate([]ai.LLMMessage{
    {Role: ai.SystemRole, Text: "You are a helpful assistant"},
    {Role: ai.UserRole, Text: "Hello"},
})

// Using Anthropic
client := ai.NewRealAnthropicClient("your-api-key")
provider := ai.NewAnthropicLLMProvider(ai.AnthropicProviderConfig{
    Client: client,
})

// Using AWS Bedrock
provider := ai.NewBedrockLLMProvider(ai.BedrockProviderConfig{
    Client: bedrockClient,
})
```

## Embeddings

```go
service := ai.NewEmbeddingService(baseURL, nil)
response, err := service.GenerateEmbedding(
    context.Background(),
    "Hello world",
    ai.EmbeddingModelAllMiniLML6V2,
)
```

## Vector Storage

```go
config := ai.PostgresStorageConfig{
    ConnectionString: "postgres://user:pass@localhost:5432/dbname",
    MaxDimension:    384,
}

provider, err := ai.NewPostgresProvider(config)
if err != nil {
    log.Fatal(err)
}

storage, err := ai.NewVectorStorage(context.Background(), provider)
if err != nil {
    log.Fatal(err)
}

// Create collection
err = storage.CreateCollection(ctx, &ai.VectorCollectionConfig{
    Name:         "documents",
    Dimension:    384,
    IndexType:    ai.IndexTypeHNSW,
    DistanceType: ai.DistanceTypeCosine,
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
