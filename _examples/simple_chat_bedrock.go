package main

import (
	"context"
	"fmt"
	"log"

	"github.com/shaharia-lab/goai/mcp"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/shaharia-lab/goai"
	mcptools "github.com/shaharia-lab/mcp-tools"
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

	llmProvider := goai.NewBedrockLLMProvider(goai.BedrockProviderConfig{
		Client: bedrockruntime.NewFromConfig(awsConfig),
		Model:  "us.anthropic.claude-3-7-sonnet-20250219-v1:0",
	})

	toolsProvider := goai.NewToolsProvider()
	toolsProvider.AddTools([]mcp.Tool{
		mcptools.GetWeather,
	})

	// Configure LLM Request
	llm := goai.NewLLMRequest(goai.NewRequestConfig(
		goai.WithMaxToken(2000),
		goai.WithTemperature(1),
		goai.WithTopP(0), // for enabling thinking, we need to unset topP
		goai.WithThinkingEnabled(1025),
		goai.UseToolsProvider(toolsProvider),
	), llmProvider)

	// Generate response
	response, err := llm.Generate(ctx, []goai.LLMMessage{
		{Role: goai.UserRole, Text: "What's the weather in Berlin, Germany?"},
	})

	if err != nil {
		panic(err)
	}

	fmt.Printf("Response: %s\n", response.Text)
	fmt.Printf("Input token: %d, Output token: %d", response.TotalInputToken, response.TotalOutputToken)
}
