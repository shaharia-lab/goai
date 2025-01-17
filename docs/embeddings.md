I'll help you add a section about creating new embedding providers. Here's how we can extend the documentation:

```markdown
# Embedding Service

Generate vector embeddings from text using the EmbeddingService interface.

## Example

Here's a complete example using OpenAI compatible embedding service
hosted locally, feel free to deploy this [embedding service](https://github.com/shaharia-lab/embedding-service) locally.

```go
package main

import (
    "context"
    "fmt"
    "github.com/shaharia-lab/goai"
    "log"
    "net/http"
    "time"
)

func main() {
    // Create a custom HTTP client with timeout
    httpClient := &http.Client{
        Timeout: 30 * time.Second,
    }

    // Create the OpenAI-compatible provider
    provider := goai.NewOpenAICompatibleEmbeddingProvider(
        "http://localhost:8000",
        httpClient,
    )

    // Create the embedding service with our provider
    embeddingService := goai.NewEmbeddingService(provider)

    // Example 1: Generate embedding for a single string
    ctx := context.Background()
    input := "Hello, world! This is a test sentence for embedding generation."

    resp, err := embeddingService.Generate(
        ctx,
        input,
        goai.EmbeddingModelAllMiniLML6V2,
    )
    if err != nil {
        log.Fatalf("Failed to generate embedding: %v", err)
    }

    fmt.Printf("Single text embedding:\n")
    fmt.Printf("- Input text: %s\n", input)
    fmt.Printf("- Embedding dimension: %d\n", len(resp.Data[0].Embedding))
    fmt.Printf("- Used tokens: %d\n\n", resp.Usage.TotalTokens)

    // Example 2: Generate embeddings for multiple strings
    texts := []string{
        "The quick brown fox jumps over the lazy dog.",
        "Pack my box with five dozen liquor jugs.",
        "How vexingly quick daft zebras jump!",
    }

    batchResp, err := embeddingService.Generate(
        ctx,
        texts,
        goai.EmbeddingModelAllMiniLML6V2,
    )
    if err != nil {
        log.Fatalf("Failed to generate batch embeddings: %v", err)
    }

    fmt.Printf("Batch embeddings:\n")
    for i, text := range texts {
        fmt.Printf("Text %d:\n", i+1)
        fmt.Printf("- Input: %s\n", text)
        fmt.Printf("- Embedding dimension: %d\n", len(batchResp.Data[i].Embedding))
        fmt.Printf("- Vector preview: [%f, %f, %f, ...]\n\n",
            batchResp.Data[i].Embedding[0],
            batchResp.Data[i].Embedding[1],
            batchResp.Data[i].Embedding[2],
        )
    }
    fmt.Printf("Total tokens used: %d\n", batchResp.Usage.TotalTokens)
}
```

Example output:

```text
Single text embedding:
- Input text: Hello, world! This is a test sentence for embedding generation.
- Embedding dimension: 384
- Used tokens: 17

Batch embeddings:
Text 1:
- Input: The quick brown fox jumps over the lazy dog.
- Embedding dimension: 384
- Vector preview: [0.043934, 0.058934, 0.048178, ...]

Text 2:
- Input: Pack my box with five dozen liquor jugs.
- Embedding dimension: 384
- Vector preview: [0.017886, 0.052399, -0.018172, ...]

Text 3:
- Input: How vexingly quick daft zebras jump!
- Embedding dimension: 384
- Vector preview: [-0.098717, 0.039359, 0.052407, ...]

Total tokens used: 37
```

## Creating Custom Embedding Providers

You can create your own embedding provider by implementing the `EmbeddingProvider` interface.
This allows you to integrate with any embedding service or model of your choice.

### Implementation Steps

`1.` Implement the EmbeddingProvider interface:

```go
type EmbeddingProvider interface {
    Generate(ctx context.Context, input interface{}, model EmbeddingModel) (*EmbeddingResponse, error)
}
```

`2.` Create your provider struct with necessary configuration:

```go
type CustomEmbeddingProvider struct {
    apiKey     string
    baseURL    string
    httpClient *http.Client
}

func NewCustomEmbeddingProvider(apiKey string, baseURL string, httpClient *http.Client) *CustomEmbeddingProvider {
    if httpClient == nil {
        httpClient = http.DefaultClient
    }
    return &CustomEmbeddingProvider{
        apiKey:     apiKey,
        baseURL:    baseURL,
        httpClient: httpClient,
    }
}
```

`3.` Implement the Generate method:

```go
func (p *CustomEmbeddingProvider) Generate(
	ctx context.Context, 
	input interface{}, 
	model EmbeddingModel,
) (*EmbeddingResponse, error) {
    // 1. Prepare your request
    reqBody := map[string]interface{}{
        "input": input,
        "model": model,
        // Add any provider-specific parameters
    }
    
    jsonBody, err := json.Marshal(reqBody)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal request: %w", err)
    }

    // 2. Create and send HTTP request
    req, err := http.NewRequestWithContext(
		ctx, 
		"POST", 
		p.baseURL+"/embeddings", 
		bytes.NewBuffer(jsonBody)
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }
    
    // Add any necessary headers
    req.Header.Set("Authorization", "Bearer "+p.apiKey)
    req.Header.Set("Content-Type", "application/json")

    // 3. Send request and handle response
    resp, err := p.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to send request: %w", err)
    }
    defer resp.Body.Close()

    // 4. Parse response into EmbeddingResponse structure
    var embeddingResp EmbeddingResponse
    if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
        return nil, fmt.Errorf("failed to decode response: %w", err)
    }

    return &embeddingResp, nil
}
```

`4.` Use your custom provider:

```go
func main() {
    // Create your custom provider
    provider := NewCustomEmbeddingProvider(
        "your-api-key",
        "https://api.your-service.com",
        nil,
    )

    // Create embedding service with your provider
    service := goai.NewEmbeddingService(provider)

    // Use the service as normal
    resp, err := service.Generate(
        context.Background(),
        "Hello, world!",
        goai.EmbeddingModelAllMiniLML6V2,
    )
    if err != nil {
        log.Fatal(err)
    }
}
```
