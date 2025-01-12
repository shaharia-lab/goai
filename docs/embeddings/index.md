# Text Embeddings

Generate vector embeddings from text for semantic search, similarity matching, and other NLP tasks.

## Basic Usage
```go
service := ai.NewEmbeddingService(
    "http://api.example.com",
    &http.Client{Timeout: 30 * time.Second},
)

embedding, err := service.GenerateEmbedding(
    context.Background(),
    "Hello world",
    ai.EmbeddingModelAllMiniLML6V2,
)
```

See [Available Models](models.md) for detailed model information.