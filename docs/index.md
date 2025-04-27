# Getting Started with GoAI

## Prerequisites

- Go 1.23 or later
- PostgreSQL 11+ with [pgvector](https://github.com/pgvector/pgvector) extension (for embedding vector storage only)

## Installation

```bash
go get github.com/shaharia-lab/goai
```

## Quick Start

- [LLM integreation](llm.md)
- [Embedding Vector Generation](embeddings.md)
- [Prompt-based LLM Generation](prompt-template.md)

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
err = storage.CreateCollection(ctx, &goai.VectorCollectionConfig{
    Name:         "documents",
    Dimension:    384,
    IndexType:    goai.IndexTypeHNSW,
    DistanceType: goai.DistanceTypeCosine,
})

// Store document
doc := &goai.VectorDocument{
    ID:      "doc1",
    Vector:  embedding.Data[0].Embedding,
    Content: "Document content",
}
err = storage.UpsertDocument(ctx, "documents", doc)

// Search
results, err := storage.SearchByVector(ctx, "documents", queryVector, &goai.VectorSearchOptions{
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
