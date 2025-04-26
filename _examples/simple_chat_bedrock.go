package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	ools "github.com/shaharia-lab/tools"
)

// This example demonstrates how to use the AWS Bedrock LLM provider with the GoAI library.
// Run this using "go run ./_examples/simple_chat_bedrock.go"
func main() {
	ctx := context.Background()
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Printf("failed to load AWS config: %w", err)
		return
	}

	llmProvider := NewBedrockLLMProvider(BedrockProviderConfig{
		Client: bedrockruntime.NewFromConfig(awsConfig),
		Model:  "us.anthropic.claude-3-7-sonnet-20250219-v1:0",
	})

	toolsProvider := NewToolsProvider()
	toolsProvider.AddTools([]Tool{
		ools.GetWeather,
	})

	// Configure LLM Request
	llm := NewLLMRequest(NewRequestConfig(
		WithMaxToken(2000),
		WithTemperature(1),
		WithTopP(0), // for enabling thinking, we need to unset topP
		WithThinkingEnabled(1025),
		UseToolsProvider(toolsProvider),
	), llmProvider)

	// Generate response
	response, err := llm.Generate(ctx, []LLMMessage{
		{Role: UserRole, Text: "What's the weather in Berlin, Germany?"},
	})

	if err != nil {
		panic(err)
	}

	fmt.Printf("Response: %s\n", response.Text)
	fmt.Printf("Input token: %d, Output token: %d", response.TotalInputToken, response.TotalOutputToken)
}
