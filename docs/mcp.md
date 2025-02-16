# Model Context Protocol (MCP)

The Model Context Protocol (MCP) enables AI models to interact with external tools
and data sources in a standardized way. This document covers the Go implementation of an MCP server.

You can learn more about [Model Context Procol (MCP)](https://modelcontextprotocol.io/) from the official website.

## Features

- Server Types: SSE (for web applications) and StdIO (for CLI applications)
- Tools: Create custom handlers with JSON schema validation
- Prompts: Template system with argument substitution
- Resources: Support for various MIME types and URIs
- Logging: Configurable logging levels (debug to emergency)
- Protocol: JSON-RPC based communication
- Validation: Input schema validation using gojsonschema
- Pagination: Built-in cursor-based pagination for listing resources/tools
- Notifications: Support for server-to-client notifications
- Error Handling: Standardized error responses and codes

## Installation

```bash
go get github.com/shaharia-lab/goai/mcp
```

## Core Components

### Tools

Tools are custom functions that can be called through the MCP protocol. Each tool has a name, description, input schema,
and a handler function. Tools are validated against their input schema before execution.

### Prompts

Prompts are templates with argument substitution capabilities. They support dynamic message generation based on provided
arguments, useful for creating reusable message templates.

### Resources

Resources represent content accessible via URIs. They support various MIME types and can be used to serve both text and
binary content. Resources can be listed and read through the protocol.

### MCP Server Compatibility Matrix

<!-- markdownlint-disable -->
| Feature                          | Status             | Notes                                                                           |
|----------------------------------|--------------------|---------------------------------------------------------------------------------|
| **Base Protocol**                | ✅ Fully Compatible | Implements JSON-RPC 2.0, stateful connections, and capability negotiation.      |
| **Lifecycle Management**         | ✅ Fully Compatible | Supports initialization, operation, shutdown, version & capability negotiation. |
| **Resources**                    | ✅ Fully Compatible | Supports listing, reading resources, and resource list change notifications.    |
| **Prompts**                      | ✅ Fully Compatible | Supports listing and getting prompts and prompt list change notifications.      |
| **Tools**                        | ✅ Fully Compatible | Supports listing and calling tools, and tool list change notifications.         |
| **Sampling**                     | 🚫 Not Implemented | No implementation for server-initiated sampling. Sampling is a client feature.  |
| **Logging**                      | ✅ Fully Compatible | Supports structured log messages and setting log levels                         |
| **Ping**                         | ✅ Fully Compatible | Implemented                                                                     |
| **Cancellation**                 | 🔲 Partial         | Supports cancellation notifications but ignores them.                           |
| **Progress Tracking**            | 🔲 Partial         | Mentioned in spec, but no implementation in the Go server code.                 |
| **Pagination**                   | ✅ Fully Compatible | Implemented for listing resources, prompts, and tools.                          |
| **Completion**                   | 🚫 Not Implemented | No implementation in the code                                                   |
| **StdIO Transport**              | ✅ Fully Compatible | Implemented                                                                     |
| **SSE Transport**                | ✅ Fully Compatible | Implemented                                                                     |
| **Custom Transports**            | 🔲 Partial         | Protocol allows for custom transports, but no implementation in sources.        |
| **Authentication/Authorization** | 🚫 Not Implemented | Not implemented in core protocol, custom strategies can be negotiated.          |
| **Roots**                        | 🚫 Not Implemented | Not a server feature                                                            |

<!-- markdownlint-enable -->

**Notes:**

- ✅ Fully Compatible - Feature is fully implemented and conforms to the specification.
- 🔲 Partial - Feature is partially implemented or has some limitations.
- 🚫 Not Implemented - Feature is not implemented.

## Server

### SSE (Server-Sent Events) Server

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

### Integrate Custom Tool

```go
tool := mcp.Tool{
    Name:        "get_weather",
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
    Name:        "greet",
    Description: "Greeting prompt",
    Arguments: []mcp.PromptArgument{{
        Name:     "name",
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

### Complete Example for Server

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

## Client

### SSE (Server-Sent Events) Client

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/shaharia-lab/goai/mcp"
)

func main() {
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
	ctx := context.Background()
	sseTransport := mcp.NewSSETransport()
	sseClient := mcp.NewClient(sseTransport, sseConfig)

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
```

### StdIO Client

<!-- markdownlint-disable -->

```go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/shaharia-lab/goai/mcp"
)

func main() {
	serverCmd := exec.Command("go", "run", "./test") // IMPORTANT: Change this!

	serverIn, err := serverCmd.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to get server stdin pipe: %v", err)
	}

	serverOut, err := serverCmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get server stdout pipe: %v", err)
	}

	if err := serverCmd.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	defer func() {
		if serverCmd.Process != nil {
			_ = serverCmd.Process.Kill()
		}
		_ = serverCmd.Wait()
	}()

	stdIOConfig := mcp.ClientConfig{
		ClientName:    "MyStdIOClient",
		ClientVersion: "1.0.0",
		Logger:        log.New(os.Stdout, "[StdIO] ", log.LstdFlags),
		StdIO: mcp.StdIOConfig{
			Reader: serverOut,
			Writer: serverIn,
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
```
<!-- markdownlint-enable -->
