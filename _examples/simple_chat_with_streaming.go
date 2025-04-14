package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
	"github.com/shaharia-lab/goai"
)

// This example demonstrates how to use streaming response from a supported LLM provider
// Run this example with `go run . simple_chat_with_streaming.go`
func main() {
	// Create OpenAI LLM Provider
	llmProvider := goai.NewOpenAILLMProvider(goai.OpenAIProviderConfig{
		Client: goai.NewOpenAIClient(os.Getenv("OPENAI_API_KEY")),
		Model:  openai.ChatModelGPT3_5Turbo,
	})

	// Configure LLM Request
	llm := goai.NewLLMRequest(goai.NewRequestConfig(
		goai.WithMaxToken(100),
		goai.WithTemperature(0.7),
	), llmProvider)

	stream, err := llm.GenerateStream(context.Background(), []goai.LLMMessage{
		{Role: goai.UserRole, Text: "Explain quantum computing"},
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
