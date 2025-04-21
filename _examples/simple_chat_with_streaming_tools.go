package main

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/shaharia-lab/goai/mcp"
	mcptools "github.com/shaharia-lab/mcp-tools"

	"github.com/shaharia-lab/goai"
	"github.com/shaharia-lab/mcp-tools"
)

// This example demonstrates how to use streaming response from a supported LLM provider
// Run this example with `go run . simple_chat_with_streaming.go`
func main() {
	// Create OpenAI LLM Provider
	llmProvider := goai.NewAnthropicLLMProvider(goai.AnthropicProviderConfig{
		Client: goai.NewAnthropicClient(os.Getenv("ANTHROPIC_API_KEY")),
		Model:  anthropic.ModelClaude3_5Sonnet20241022,
	})

	tools := []mcp.Tool{
		mcptools.GetWeather,
	}

	toolsProvider := goai.NewToolsProvider()
	toolsProvider.AddTools(tools)

	// Configure LLM Request
	llm := goai.NewLLMRequest(goai.NewRequestConfig(
		goai.WithMaxToken(100),
		goai.WithTemperature(0.7),
		goai.UseToolsProvider(toolsProvider),
		goai.WithAllowedTools([]string{"get_weather"}),
	), llmProvider)

	stream, err := llm.GenerateStream(context.Background(), []goai.LLMMessage{
		{Role: goai.UserRole, Text: "What's the weather in Berlin, Germany?"},
	})

	if err != nil {
		panic(err)
	}

	for resp := range stream {
		if resp.Error != nil {
			fmt.Printf("Error: %v\n", resp.Error)
			break
		}
		if resp.Done {
			break
		}
		fmt.Print(resp.Text)
	}
}
