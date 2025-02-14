package main

import (
	"github.com/shaharia-lab/goai/mcp"
	"log"
)

func main() {
	client := mcp.NewSSEClient(mcp.SSEClientConfig{
		URL: "http://localhost:8080/events",
	})

	// First connect
	if err := client.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	// Then call ListTools()
	tools, err := client.ListTools()
	if err != nil {
		log.Fatalf("Failed to list tools: %v", err)
	}

	// Print the tools
	for _, tool := range tools {
		log.Printf("Tool: %s - %s", tool.Name, tool.Description)
	}

	// Keep the program running to maintain the SSE connection
	select {}
}
