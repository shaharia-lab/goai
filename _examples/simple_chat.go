package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
)

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

	ctx := context.Background()

	// Generate response
	response, err := llm.Generate(ctx, []LLMMessage{
		{Role: UserRole, Text: "Explain quantum computing"},
	})

	if err != nil {
		panic(err)
	}

	fmt.Printf("Response: %s\n", response.Text)
	fmt.Printf("Input token: %d, Output token: %d", response.TotalInputToken, response.TotalOutputToken)
}
