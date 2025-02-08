/*
Package mcp implements the Modular Content Protocol, a JSON-RPC based protocol
for interacting with content and tools in a modular way.

The protocol supports various features including:
  - Resource management
  - Tool execution
  - Logging
  - Prompt handling

Example usage:

	import "github.com/yourusername/mcp"

	// Initialize the protocol
	params := &mcp.InitializeParams{
		ProtocolVersion: "1.0",
		Capabilities: map[string]any{
			"resources": true,
			"tools": true,
		},
		ClientInfo: struct {
			Name    string
			Version string
		}{
			Name:    "TestClient",
			Version: "1.0",
		},
	}
*/
package mcp
