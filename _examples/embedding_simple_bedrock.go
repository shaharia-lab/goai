package main

import (
	"context"
	"fmt"
	"github.com/shaharia-lab/goai"
	"log"
)

func main() {
	ctx := context.Background()

	// Create a new Bedrock embedding provider
	// Replace "us-east-1" with your desired AWS region
	// Ensure you have the AWS credentials configured in your environment
	// or use any other provider that implements the goai.EmbeddingProvider interface
	provider, err := goai.NewBedrockEmbeddingProvider(ctx, "us-east-1")
	if err != nil {
		log.Fatalf("Failed to create Bedrock embedding provider: %v", err)
	}

	embeddingService := goai.NewEmbeddingService(provider)

	input := "Hello, world! This is a test sentence for embedding generation."

	resp, err := embeddingService.Generate(
		ctx,
		input,
		"amazon.titan-embed-text-v1",
	)
	if err != nil {
		log.Fatalf("Failed to generate embedding: %v", err)
	}

	fmt.Printf("Single text embedding:\n")
	fmt.Printf("- Input text: %s\n", input)
	fmt.Printf("- Embedding dimension: %d\n", len(resp.Data[0].Embedding))
	fmt.Printf("- Used tokens: %d\n\n", resp.Usage.TotalTokens)
	fmt.Printf("Embedding: %v\n", resp.Data[0].Embedding)
}
