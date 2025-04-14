package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
	"github.com/shaharia-lab/goai"
)

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

	ctx := context.Background()

	// Generate response
	response, err := llm.Generate(ctx, []goai.LLMMessage{
		{Role: goai.UserRole, Text: "Explain quantum computing"},
	})

	if err != nil {
		panic(err)
	}

	fmt.Printf("Response: %s\n", response.Text)
	fmt.Printf("Input token: %d, Output token: %d", response.TotalInputToken, response.TotalOutputToken)
}
