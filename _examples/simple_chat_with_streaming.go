package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
)

// This example demonstrates how to use streaming response from a supported LLM provider
// Run this example with `go run . simple_chat_with_streaming.go`
func main() {
	// Create OpenAI LLM Provider
	llmProvider := NewOpenAILLMProvider(OpenAIProviderConfig{
		Client: NewOpenAIClient(os.Getenv("OPENAI_API_KEY")),
		Model:  openai.ChatModelGPT3_5Turbo,
	})

	// Configure LLM Request
	llm := NewLLMRequest(NewRequestConfig(
		WithMaxToken(100),
		WithTemperature(0.7),
	), llmProvider)

	stream, err := llm.GenerateStream(context.Background(), []LLMMessage{
		{Role: UserRole, Text: "Explain quantum computing"},
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
