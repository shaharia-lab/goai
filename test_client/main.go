package main

import (
	"fmt"
	"github.com/shaharia-lab/goai/mcp"
	"log"
	"os"
	"os/exec"
)

/*func main() {
	// --- SSE Example ---
	sseConfig := mcp.ClientConfig{
		ClientName:    "MySSEClient",
		ClientVersion: "1.0.0",
		Logger:        log.New(os.Stdout, "[SSE] ", log.LstdFlags),
		RetryDelay:    5 * time.Second,
		MaxRetries:    3,
		SSE: mcp.SSEConfig{
			URL: "http://localhost:8080/events", // Replace with your SSE endpoint
		},
	}

	sseTransport := mcp.NewSSETransport()
	sseClient := mcp.NewClient(sseTransport, sseConfig)

	if err := sseClient.Connect(); err != nil {
		log.Fatalf("SSE Client failed to connect: %v", err)
	}
	defer sseClient.Close()

	tools, err := sseClient.ListTools()
	if err != nil {
		log.Fatalf("Failed to list tools (SSE): %v", err)
	}
	fmt.Printf("SSE Tools: %+v\n", tools)
}*/

func main() {
	// --- StdIO Example ---

	// Create a command to run your server.  Replace "go run ./yourserver"
	// with the actual command to start your MCP server.  This assumes
	// your server uses standard input/output.
	serverCmd := exec.Command("go", "run", "./test") // IMPORTANT: Change this!

	// Get pipes to the server process
	serverIn, err := serverCmd.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to get server stdin pipe: %v", err)
	}

	serverOut, err := serverCmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get server stdout pipe: %v", err)
	}

	// Start the server process.
	if err := serverCmd.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Ensure the server is stopped when this client exits.
	defer func() {
		if serverCmd.Process != nil {
			_ = serverCmd.Process.Kill() // Best effort kill.
		}
		_ = serverCmd.Wait() // Ensure process resources are cleaned up.
	}()

	stdIOConfig := mcp.ClientConfig{
		ClientName:    "MyStdIOClient",
		ClientVersion: "1.0.0",
		Logger:        log.New(os.Stdout, "[StdIO] ", log.LstdFlags),
		StdIO: mcp.StdIOConfig{
			Reader: serverOut, // Read from server's stdout
			Writer: serverIn,  // Write to server's stdin
		},
	}

	stdIOTransport := mcp.NewStdIOTransport()
	stdIOClient := mcp.NewClient(stdIOTransport, stdIOConfig)

	if err := stdIOClient.Connect(); err != nil {
		log.Fatalf("StdIO Client failed to connect: %v", err)
	}
	defer stdIOClient.Close()

	tools2, err := stdIOClient.ListTools()
	if err != nil {
		log.Fatalf("Failed to list tools (StdIO): %v", err)
	}

	fmt.Printf("StdIO Tools: %+v\n", tools2)

	fmt.Println("Press Enter to exit.")
	fmt.Scanln()
}
