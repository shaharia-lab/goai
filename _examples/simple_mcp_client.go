package main

import (
	"context"
	"fmt"
	"github.com/shaharia-lab/goai"
	"log"
	"time"
)

func main() {
	sseConfig := goai.ClientConfig{
		ClientName:    "MySSEClient",
		ClientVersion: "1.0.0",
		Logger:        goai.NewDefaultLogger(),
		RetryDelay:    5 * time.Second,
		MaxRetries:    3,
		SSE: goai.SSEConfig{
			URL: "http://localhost:8080/events", // Replace with your SSE endpoint
		},
	}
	ctx := context.Background()
	sseTransport := goai.NewSSETransport(goai.NewDefaultLogger())
	sseClient := goai.NewClient(sseTransport, sseConfig)

	if err := sseClient.Connect(ctx); err != nil {
		log.Fatalf("SSE Client failed to connect: %v", err)
	}
	defer sseClient.Close(ctx)

	tools, err := sseClient.ListTools(ctx)
	if err != nil {
		log.Fatalf("Failed to list tools (SSE): %v", err)
	}
	fmt.Printf("SSE Tools: %+v\n", tools)
}
