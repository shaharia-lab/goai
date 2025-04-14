package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/shaharia-lab/goai"

	"github.com/shaharia-lab/goai/chat_history"
)

// This example demonstrates how to use the InMemoryChatHistoryStorage
// to store chat history in memory. It allows you to have a conversation
// with the AI model and keeps track of the messages exchanged.
//
// You can use any other available storage implementation by
// replacing the InMemoryChatHistoryStorage with your desired storage
func main() {
	llmProvider := goai.NewOpenAILLMProvider(goai.OpenAIProviderConfig{
		Client: goai.NewOpenAIClient(os.Getenv("OPENAI_API_KEY")),
		Model:  openai.ChatModelGPT3_5Turbo,
	})

	llm := goai.NewLLMRequest(goai.NewRequestConfig(
		goai.WithMaxToken(1000),
		goai.WithTemperature(0.7),
		goai.WithMaxIterations(10),
	), llmProvider)

	storage := chat_history.NewInMemoryChatHistoryStorage()

	ctx := context.Background()
	chatHistory, err := storage.CreateChat(ctx)
	if err != nil {
		panic(fmt.Sprintf("Failed to create chat: %v", err))
	}

	fmt.Println("Chat session started. Type 'exit' to quit.")
	fmt.Printf("Session ID: %s\n\n", chatHistory.SessionID)

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("You: ")
		if !scanner.Scan() {
			break
		}

		userInput := scanner.Text()
		if strings.ToLower(userInput) == "exit" {
			break
		}

		userMessage := chat_history.ChatHistoryMessage{
			LLMMessage: goai.LLMMessage{
				Role: goai.UserRole,
				Text: userInput,
			},
			GeneratedAt: time.Time{},
		}
		if err := storage.AddMessage(ctx, chatHistory.SessionID, userMessage); err != nil {
			fmt.Printf("Error saving message: %v\n", err)
			continue
		}

		updatedHistory, err := storage.GetChat(ctx, chatHistory.SessionID)
		if err != nil {
			fmt.Printf("Error retrieving chat history: %v\n", err)
			continue
		}

		var messages []goai.LLMMessage
		for _, msg := range updatedHistory.Messages {
			messages = append(messages, goai.LLMMessage{
				Role: msg.Role,
				Text: msg.Text,
			})
		}

		response, err := llm.Generate(ctx, messages)
		if err != nil {
			fmt.Printf("Error generating response: %v\n", err)
			continue
		}

		fmt.Printf("AI: %s\n\n", response.Text)

		assistantMessage := chat_history.ChatHistoryMessage{
			LLMMessage: goai.LLMMessage{
				Role: goai.AssistantRole,
				Text: response.Text,
			},
			GeneratedAt: time.Now().UTC(),
		}

		if err := storage.AddMessage(ctx, chatHistory.SessionID, assistantMessage); err != nil {
			fmt.Printf("Error saving AI response: %v\n", err)
		}

		fmt.Printf("Tokens - Input: %d, Output: %d\n\n",
			response.TotalInputToken, response.TotalOutputToken)
	}

	fmt.Println("Chat session ended.")
}
