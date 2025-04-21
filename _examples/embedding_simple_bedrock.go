package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/shaharia-lab/goai"
)

func main() {
	ctx := context.Background()

	awsRegion := "us-east-1" // Replace with your desired AWS region
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(awsRegion))
	if err != nil {
		log.Fatalf("Failed to load AWS configuration: %v", err)
	}

	// Create the Bedrock Runtime client
	bedrockClient := bedrockruntime.NewFromConfig(cfg)

	// Create a new Bedrock embedding provider
	// Ensure you have the AWS credentials configured in your environment
	// or use any other provider that implements the goai.EmbeddingProvider interface
	provider, err := goai.NewBedrockEmbeddingProvider(bedrockClient)
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
