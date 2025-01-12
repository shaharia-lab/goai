# Embedding Models

## Available Models

```go
const (
    EmbeddingModelAllMiniLML6V2 EmbeddingModel = "all-MiniLM-L6-v2"
    EmbeddingModelAllMpnetBaseV2 EmbeddingModel = "all-mpnet-base-v2"
    EmbeddingModelParaphraseMultilingualMiniLML12V2 EmbeddingModel = "paraphrase-multilingual-MiniLM-L12-v2"
)
```

### all-MiniLM-L6-v2

- Lightweight model suitable for general-purpose embedding generation
- Good balance between performance and quality

### all-mpnet-base-v2

- Higher quality embeddings
- More powerful model with increased computation time

### paraphrase-multilingual-MiniLM-L12-v2

- Specialized for multilingual text
- Supports embedding generation across multiple languages
- Maintains semantic meaning across languages

## Usage

```go
response, err := service.GenerateEmbedding(
    ctx,
    input,
    goai.EmbeddingModelAllMiniLML6V2,
)

// Response includes:
type EmbeddingResponse struct {
    Object string            `json:"object"`
    Data   []EmbeddingObject `json:"data"`
    Model  EmbeddingModel    `json:"model"`
    Usage  Usage             `json:"usage"`
}
```
