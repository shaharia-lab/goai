# Model Context Protocol (MCP)

The Model Context Protocol (MCP) enables AI models to interact with external tools
and data sources in a standardized way. This document covers the Go implementation of an MCP server.

You can learn more about [Model Context Procol (MCP)](https://modelcontextprotocol.io/) from the official website.

## Features

- **Server Types**: SSE (for web applications) and StdIO (for CLI applications)
- **Tools**: Create custom handlers with JSON schema validation
- **Prompts**: Template system with argument substitution
- **Resources**: Support for various MIME types and URIs
- **Logging**: Configurable logging levels (debug to emergency)
- **Protocol**: JSON-RPC based communication
- **Validation**: Input schema validation using gojsonschema
- **Pagination**: Built-in cursor-based pagination for listing resources/tools
- **Notifications**: Support for server-to-client notifications
- **Error Handling**: Standardized error responses and codes

## Installation

```bash
go get github.com/shaharia-lab/goai/mcp
```

## Core Components

### Tools
Tools are custom functions that can be called through the MCP protocol. Each tool has a name, description, input schema, and a handler function. Tools are validated against their input schema before execution.

### Prompts
Prompts are templates with argument substitution capabilities. They support dynamic message generation based on provided arguments, useful for creating reusable message templates.

### Resources
Resources represent content accessible via URIs. They support various MIME types and can be used to serve both text and binary content. Resources can be listed and read through the protocol.

### MCP Server Compatibility Matrix

Okay, here's the feature compatibility matrix in markdown, as requested:

| Feature                          | Status             | Notes                                                                                                |
|----------------------------------|--------------------|------------------------------------------------------------------------------------------------------|
| **Base Protocol**                | ✅ Fully Compatible | Implements JSON-RPC 2.0, stateful connections, and capability negotiation.                           |
| **Lifecycle Management**         | ✅ Fully Compatible | Supports initialization, operation, shutdown, version & capability negotiation.                      |
| **Resources**                    | ✅ Fully Compatible | Supports listing, reading resources, and resource list change notifications.                         |
| **Prompts**                      | ✅ Fully Compatible | Supports listing and getting prompts and prompt list change notifications.                           |
| **Tools**                        | ✅ Fully Compatible | Supports listing and calling tools, and tool list change notifications.                              |
| **Sampling**                     | 🚫 Not Implemented | No implementation for server-initiated sampling.  Sampling is a client feature.                      |
| **Logging**                      | ✅ Fully Compatible | Supports structured log messages and setting log levels                                              |
| **Ping**                         | ✅ Fully Compatible | Implemented                                                                                          |
| **Cancellation**                 | 🔲 Partial         | Placeholder exists, but not fully implemented. Supports cancellation notifications but ignores them. |
| **Progress Tracking**            | 🔲 Partial         | Mentioned in spec, but no implementation in the Go server code.                                      |
| **Pagination**                   | ✅ Fully Compatible | Implemented for listing resources, prompts, and tools.                                               |
| **Completion**                   | 🚫 Not Implemented | No implementation in the code                                                                        |
| **StdIO Transport**              | ✅ Fully Compatible | Implemented                                                                                          |
| **SSE Transport**                | ✅ Fully Compatible | Implemented                                                                                          |
| **Custom Transports**            | 🔲 Partial         | Protocol allows for custom transports, but no implementation is in the sources.                      |
| **Authentication/Authorization** | 🚫 Not Implemented | Not implemented in the core protocol, custom strategies can be negotiated.                           |
| **Roots**                        | 🚫 Not Implemented | Not a server feature                                                                                 |

**Notes:**

*   ✅ Fully Compatible - Feature is fully implemented and conforms to the specification.
*   🔲 Partial - Feature is partially implemented or has some limitations.
*   🚫 Not Implemented - Feature is not implemented.


## Server Setup

### SSE Server
```go
baseServer, err := mcp.NewBaseServer(
    mcp.UseLogger(log.New(os.Stderr, "[MCP] ", log.LstdFlags)),
)
if err != nil {
    panic(err)
}

server := mcp.NewSSEServer(baseServer)
server.SetAddress(":8080")
ctx := context.Background()
server.Run(ctx)
```

### StdIO Server
```go
baseServer, err := mcp.NewBaseServer(
    mcp.UseLogger(log.New(os.Stderr, "[MCP] ", log.LstdFlags)),
)
if err != nil {
    panic(err)
}

server := mcp.NewStdIOServer(baseServer, os.Stdin, os.Stdout)
ctx := context.Background()
server.Run(ctx)
```

## Adding Components

### Custom Tool
```go
tool := mcp.Tool{
    Name: "get_weather",
    Description: "Get weather for location",
    InputSchema: json.RawMessage(`{
        "type": "object",
        "properties": {
            "location": {"type": "string"}
        },
        "required": ["location"]
    }`),
    Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
        var input struct {
            Location string `json:"location"`
        }
        json.Unmarshal(params.Arguments, &input)
        return mcp.CallToolResult{
            Content: []mcp.ToolResultContent{{
                Type: "text",
                Text: fmt.Sprintf("Weather in %s: Sunny", input.Location),
            }},
        }, nil
    },
}
baseServer.AddTools(tool)
```

### Prompt
```go
prompt := mcp.Prompt{
    Name: "greet",
    Description: "Greeting prompt",
    Arguments: []mcp.PromptArgument{{
        Name: "name",
        Required: true,
    }},
    Messages: []mcp.PromptMessage{{
        Role: "user",
        Content: mcp.PromptContent{
            Type: "text",
            Text: "Hello {{name}}!",
        },
    }},
}
baseServer.AddPrompts(prompt)
```

### Resource
```go
resource := mcp.Resource{
    URI: "file:///example.txt",
    Name: "Example",
    MimeType: "text/plain",
    TextContent: "Sample content",
}
baseServer.AddResources(resource)
```

## Complete Example

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"github.com/shaharia-lab/goai/mcp"
)

func main() {
	// Create base server
	baseServer, err := mcp.NewBaseServer(
		mcp.UseLogger(log.New(os.Stderr, "[MCP] ", log.LstdFlags)),
	)
	if err != nil {
		panic(err)
	}

	// Add tool
	tool := mcp.Tool{
		Name: "greet",
		Description: "Greet user",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "name": {"type": "string"}
            },
            "required": ["name"]
        }`),
		Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
			var input struct {
				Name string `json:"name"`
			}
			json.Unmarshal(params.Arguments, &input)
			return mcp.CallToolResult{
				Content: []mcp.ToolResultContent{{
					Type: "text",
					Text: fmt.Sprintf("Hello, %s!", input.Name),
				}},
			}, nil
		},
	}
	baseServer.AddTools(tool)

	// Create and run SSE server
	server := mcp.NewSSEServer(baseServer)
	server.SetAddress(":8080")

	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		panic(err)
	}
}
```