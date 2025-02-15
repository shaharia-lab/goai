package main

import (
	"github.com/shaharia-lab/goai/mcp"
	"log"
	"os"
	"os/exec"
)

func main() {
	logger := log.New(os.Stderr, "Client: ", log.LstdFlags)

	// Start the server process
	serverCmd := exec.Command("go", "run", "./test") // Path to your server binary

	// Get pipes to the server process
	serverIn, err := serverCmd.StdinPipe()
	if err != nil {
		logger.Fatalf("Failed to get server stdin pipe: %v", err)
	}

	serverOut, err := serverCmd.StdoutPipe()
	if err != nil {
		logger.Fatalf("Failed to get server stdout pipe: %v", err)
	}

	// Start the server
	if err := serverCmd.Start(); err != nil {
		logger.Fatalf("Failed to start server: %v", err)
	}

	// Create and connect client
	clientConfig := mcp.StdIOClientConfig{
		Logger: logger,
		Reader: serverOut,
		Writer: serverIn,
	}

	client := mcp.NewStdIOClient(clientConfig)

	if err := client.Connect(); err != nil {
		logger.Fatalf("Failed to connect client: %v", err)
	}

	// Use the client
	tools, err := client.ListTools()
	if err != nil {
		logger.Fatalf("Failed to list tools: %v", err)
	}
	logger.Printf("Available tools: %+v", tools)

	// Cleanup
	client.Close()
	serverCmd.Process.Kill()
	serverCmd.Wait()
}
