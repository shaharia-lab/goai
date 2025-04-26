package main

import (
	"context"
	"fmt"
	"os"

	"github.com/shaharia-lab/goai"

	"github.com/anthropics/anthropic-sdk-go"

	mcpTools "github.com/shaharia-lab/mcp-tools"
)

// This example demonstrates how to use streaming response from a supported LLM provider
// Run this example with `go run . simple_chat_with_streaming.go`
func main() {
	// Create OpenAI LLM Provider
	llmProvider := NewAnthropicLLMProvider(AnthropicProviderConfig{
		Client: NewAnthropicClient(os.Getenv("ANTHROPIC_API_KEY")),
		Model:  anthropic.ModelClaude3_5Sonnet20241022,
	}, goai.NewDefaultLogger())

	tools := []Tool{
		mcpTools.GetWeather,
	}

	toolsProvider := NewToolsProvider()
	toolsProvider.AddTools(tools)

	// Configure LLM Request
	llm := NewLLMRequest(NewRequestConfig(
		WithMaxToken(100),
		WithTemperature(0.7),
		UseToolsProvider(toolsProvider),
		WithAllowedTools([]string{"get_weather"}),
	), llmProvider)

	stream, err := llm.GenerateStream(context.Background(), []LLMMessage{
		{Role: UserRole, Text: "What's the weather in Berlin, Germany?"},
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
