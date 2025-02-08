package main

import (
	"github.com/shaharia-lab/goai/mcp"
	"os"
)

func main() {
	server := mcp.NewMCPServer(os.Stdin, os.Stdout)

	// Example usage of logging:
	server.LogMessage(mcp.LogLevelInfo, "main", "Starting MCP server...")

	server.Run() // This will block and run the server.

	server.LogMessage(mcp.LogLevelInfo, "main", "MCP server shutting down.")
}
