// Command anth2oai_glm demonstrates talking to an Anthropic-format endpoint
// (e.g. GLM/Z.ai) through the OpenAI SDK, using the anth2oai package.
//
// Usage:
//
//	export ANTHROPIC_BASE_URL=https://api.z.ai/api/anthropic
//	export ANTHROPIC_AUTH_TOKEN=<your token>
//	go run ./_examples/anth2oai_glm
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/openai/openai-go"
	"github.com/shaharia-lab/goai/anth2oai"
)

const model = "glm-4.5-air"

func main() {
	base := os.Getenv("ANTHROPIC_BASE_URL")
	token := os.Getenv("ANTHROPIC_AUTH_TOKEN")
	if base == "" || token == "" {
		log.Fatal("set ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN")
	}

	// A normal *openai.Client, wired to the Anthropic endpoint.
	client := anth2oai.New(base, token)
	ctx := context.Background()

	// --- Non-streaming ---
	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.F(model),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You are terse."),
			openai.UserMessage("Reply with exactly: OK"),
		}),
	})
	if err != nil {
		log.Fatalf("non-streaming request failed: %v", err)
	}
	fmt.Printf("non-streaming: %q (tokens: %d)\n",
		resp.Choices[0].Message.Content, resp.Usage.TotalTokens)

	// --- Streaming ---
	fmt.Print("streaming:     ")
	stream := client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Model: openai.F(model),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Count from one to five, comma separated."),
		}),
	})
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 {
			fmt.Print(chunk.Choices[0].Delta.Content)
		}
	}
	if err := stream.Err(); err != nil {
		log.Fatalf("\nstreaming request failed: %v", err)
	}
	fmt.Println()
}
