package main

import (
	"github.com/shaharia-lab/goai/mcp"
	"log"
	"os"
	"time"
)

func main() {
	logger := log.New(os.Stdout, "[SSE Client] ", log.LstdFlags)

	config := mcp.SSEClientConfig{
		URL:           "http://localhost:8080/events",
		RetryDelay:    5 * time.Second,
		MaxRetries:    5,
		ClientName:    "example-client",
		ClientVersion: "1.0.0",
		Logger:        logger,
	}

	client := mcp.NewSSEClient(config)

	defer client.Close()

	// Connect to the server
	if err := client.Connect(); err != nil {
		logger.Fatalf("Failed to connect: %v", err)
	}
	logger.Println("Successfully connected to the server")

	// List available tools
	tools, err := client.ListTools()
	if err != nil {
		logger.Printf("Failed to list tools: %v", err)
	} else {
		logger.Printf("Found %d tools", len(tools))
		for _, tool := range tools {
			logger.Printf("Tool: %s - %s", tool.Name, tool.Description)
		}
	}
}
