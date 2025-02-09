# MCP (Model Context Protocol) Server in Go

This document describes how to use the Go implementation of the Model Context Protocol (MCP) server, built using the `mcp` package.  This server allows you to expose resources, tools, and prompts to MCP clients, enabling AI-powered features in applications like IDEs, chat interfaces, and more.

## Getting Started

### Prerequisites

*   Go (version 1.18 or later recommended)
*   An understanding of the [Model Context Protocol Specification](https://modelcontextprotocol.io)

### Installation

To use the `mcp` package, install it using `go get`:

```bash
go get github.com/shaharia-lab/goai/mcp
```

### Basic Server Implementation

The following code demonstrates a minimal MCP server setup:

```go
package main

import (
	"os"

	"github.com/shaharia-lab/goai/mcp"
)

func main() {
	server := mcp.NewMCPServer(os.Stdin, os.Stdout)

	// Example usage of logging:
	server.LogMessage(mcp.LogLevelInfo, "main", "Starting MCP server...")

	server.Run() // This will block and run the server.

	server.LogMessage(mcp.LogLevelInfo, "main", "MCP server shutting down.")
}

```

**Explanation:**

1.  **`mcp.NewMCPServer(os.Stdin, os.Stdout)`:**  Creates a new MCP server instance.  It takes an `io.Reader` (for input) and an `io.Writer` (for output) as arguments.  In this example, we're using standard input (`os.Stdin`) and standard output (`os.Stdout`), which is the standard way for MCP servers to communicate when launched by a client.
2.  **`server.LogMessage(...)`:**  Demonstrates how to send log messages to the client.  The server supports various log levels (Debug, Info, Notice, Warning, Error, Critical, Alert, Emergency).
3.  **`server.Run()`:**  Starts the server's main loop.  This function blocks indefinitely, continuously reading from the input stream, processing messages, and writing responses to the output stream.

### Running the Server

1.  **Save:** Save the above code as a `.go` file (e.g., `main.go`).
2.  **Build:**  Build the server using: `go build -o mcp-server main.go`
3.  **Test:** Run the MCP server using MCP inspector: `npx @modelcontextprotocol/inspector ./mcp-server`

### Adding Resources

The example server already includes two pre-added resources.  Let's examine how to add your own resources:

```go
server.AddResource(mcp.Resource{
    URI:         "file:///example/my-resource.txt",
    Name:        "My Custom Resource",
    Description: "A description of my resource.",
    MimeType:    "text/plain",
    TextContent: "This is the content of my resource.",
})
```
-   **`server.AddResource(...)`:**  This method adds a resource to the server's internal resource map.
-   **`mcp.Resource{...}`:**  This defines the resource.
  -   **`URI`:**  A *unique* identifier for the resource.  The `file:///` scheme is common for local files.
  -   **`Name`:** A human-readable name for the resource.
  -   **`Description`:**  (Optional) A longer description.
  -   **`MimeType`:**  The MIME type of the resource (e.g., `text/plain`, `application/json`, `image/png`).
  -   **`TextContent`:** The *string* content of the resource.  This is used for text-based resources.  For binary resources, you would populate the `Blob` field (not shown in this example, but it takes a base64-encoded string) *instead* of `TextContent`.  You *must not* set both.

### Adding Tools
The server includes a `get_weather` tool as example.
```go
server.AddTool(mcp.Tool{
    Name:        "my_tool",
    Description: "A tool that does something.",
    InputSchema: json.RawMessage(`{
        "type": "object",
        "properties": {
            "input_param": {
                "type": "string",
                "description": "An example input parameter."
            }
        },
        "required": ["input_param"]
    }`),
})
```

-   **`server.AddTool(...)`:** Adds a tool to the server.
-   **`mcp.Tool{...}`:** Defines the tool.
  -   **`Name`:** A *unique* name for the tool.
  -   **`Description`:** A human-readable description.
  -   **`InputSchema`:** A JSON Schema defining the tool's input parameters.  This is *crucial* for allowing clients (and LLMs) to understand how to call the tool.

To *handle* tool calls, you'll need to modify the `handleRequest` function within `MCPServer` to include a case for `"tools/call"` that matches your tool's name:

```go
    case "tools/call":
        // ... (existing code for unmarshalling params and validating schema) ...

        if params.Name == "my_tool" {
            // 1. Get arguments from 'input' map (created in the existing code)
            inputParam, _ := input["input_param"].(string)

            // 2. Perform the tool's logic
            resultText := fmt.Sprintf("Tool executed! Input: %s", inputParam)

            // 3. Create the result
            result := mcp.CallToolResult{
                Content: []mcp.ToolResultContent{{
                    Type: "text",
                    Text: resultText,
                }},
                IsError: false,
            }
            s.sendResponse(request.ID, result, nil)
            return // Important: Return after handling the tool call
        }

        // ... (existing code for "get_weather" tool) ...

        s.sendError(request.ID, -32602, "Unknown tool", map[string]string{"tool": params.Name})
```
This added code checks for `"my_tool"`, retrieves the `input_param`, performs a simple action (creating a formatted string), and sends the result back to the client.

### Adding Prompts

The server includes a `code_review` and `summarize_text` prompt as examples.

```go
	s.addPrompt(Prompt{
		Name:        "generate_unit_tests",
		Description: "Generates unit tests for the given code",
		Arguments: []PromptArgument{
			{Name: "code", Description: "Code for generating test", Required: true},
			{Name: "language", Description: "Programming language", Required: true},
		},
		Messages: []PromptMessage{ //Added messages
			{
				Role: "user",
				Content: PromptContent{
					Type: "text",
					Text: "Generate unit tests for the following {language} code:\n{code}",
				},
			},
		},
	})
```

-  `server.AddPrompt(...)`: Adds a prompt to server's internal prompt map.
-  `mcp.Prompt{...}`: Defines the prompt.
  - `Name`: A *unique* name for the prompt.
  - `Description`: Optional human-readable description
  - `Arguments`: Optional list of arguments for customization
  -   **`Messages`**:  The structure of the messages. Contains Role and Content.
  -	`Role`: user or assistant.
  -	`Content`: `Type` can be `text` (for now, later will be implemented for other content types. and `Text` is the prompt value.

### Logging

You can use `server.LogMessage()` to send log messages to the client:

```go
server.LogMessage(mcp.LogLevelError, "my_logger", "An error occurred!")
server.LogMessage(mcp.LogLevelDebug, "my_logger", map[string]interface{}{"some": "data"})

```

-   **`server.LogMessage(level, loggerName, data)`:**
  -   **`level`:**  The log level (use the constants like `mcp.LogLevelError`, `mcp.LogLevelInfo`, etc.).
  -   **`loggerName`:**  (Optional) A string identifying the source of the log message.
  -   **`data`:**  The log message itself. This can be a simple string or a more complex, JSON-serializable object (like a map).

The client can control the minimum log level it receives using the `logging/setLevel` request.

### Handling Client Requests and Notifications

The `MCPServer` struct's `handleRequest` and `handleNotification` methods implement the server's logic for responding to different MCP messages.  You *don't* typically call these directly.  Instead, `server.Run()` reads messages from the input stream and calls the appropriate handler.

The provided code *already* handles many standard MCP requests (initialize, ping, resources/list, resources/read, logging/setLevel, tools/list, tools/call, prompts/list, prompts/get).  You've seen how to add resources and tools.  You would extend `handleRequest` if you added support for *new* MCP methods.

## Feature Matrix

This table summarizes the current level of support for MCP features in the Go `mcp` package:

✅ for "Fully Implemented"
🚧 for "Partially Implemented"
❌ for "Not Yet Implemented"


| Feature Category    | Feature                      | Implementation Status | Notes                                                                                                                                                                                                                |
|:--------------------|:-----------------------------|:----------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Base Protocol**   | JSON-RPC 2.0                 | ✅ Fully Implemented   | All messages use the JSON-RPC 2.0 format.                                                                                                                                                                            |
|                     | Requests                     | ✅ Fully Implemented   | `server_stdio.go` handles requests correctly.                                                                                                                                                                        |
|                     | Responses                    | ✅ Fully Implemented   | `server_stdio.go` sends correctly formatted responses.                                                                                                                                                               |
|                     | Notifications                | ✅ Fully Implemented   | `server_stdio.go` handles and sends notifications.                                                                                                                                                                   |
| **Lifecycle**       | Initialization               | ✅ Fully Implemented   | `server_stdio.go` handles the `initialize` request and response, including capability negotiation and protocol version check.                                                                                        |
|                     | Operation                    | ✅ Fully Implemented   | The server processes requests and sends responses/notifications as expected after initialization.                                                                                                                    |
|                     | Shutdown                     | ✅ Fully Implemented   | `server_stdio.go`'s `Run` method handles context cancellation for graceful shutdown (tested in `TestServerShutdown`). The `stdio` transport's shutdown procedure, as described in the spec, is implicitly supported. |
| **Server Features** | Resources / List             | ✅ Fully Implemented   | `resource_manager.go` and `server_stdio.go` implement `resources/list` with basic pagination support.                                                                                                                |
|                     | Resources / Read             | ✅ Fully Implemented   | `resource_manager.go` and `server_stdio.go` implement `resources/read`, handling text and binary content.                                                                                                            |
|                     | Resources / Subscribe        | ❌ Not Yet Implemented | No subscription mechanism is present in the code.                                                                                                                                                                    |
|                     | Resources / List Changed     | ✅ Fully Implemented   | `server_stdio.go` correctly checks for `listChanged` capability of resource.                                                                                                                                         |
|                     | Resources / Templates / List | ❌ Not Yet Implemented | No resource template functionality is present.                                                                                                                                                                       |
|                     | Prompts / List               | ✅ Fully Implemented   | `prompt_manager.go` and `server_stdio.go` implement `prompts/list` with pagination.                                                                                                                                  |
|                     | Prompts / Get                | ✅ Fully Implemented   | `prompt_manager.go` and `server_stdio.go` implement `prompts/get`, including argument substitution.                                                                                                                  |
|                     | Prompts / List Changed       | ✅ Fully Implemented   | `server_stdio.go` includes `SendPromptListChangedNotification` and capability check.  `prompt_manager.go` calls this function on prompt registration/deletion.                                                       |
|                     | Tools / List                 | ✅ Fully Implemented   | `tool_manager.go` and `server_stdio.go` implement `tools/list` with pagination.                                                                                                                                      |
|                     | Tools / Call                 | ✅ Fully Implemented   | `tool_manager.go` and `server_stdio.go` implement `tools/call`, including input schema validation.  A simple `get_weather` example is included.                                                                      |
|                     | Tools / List Changed         | ✅ Fully Implemented   | `server_stdio.go` includes `SendToolListChangedNotification` and capability check. `tool_manager.go` *does not* call the notification function, but it *is* present in `server_stdio.go`, and the framework exists.  |
| **Client Features** | Roots / List                 | ❌ Not Yet Implemented | No client-side root management is present. This table reflects server-side implementation.                                                                                                                           |
|                     | Roots / List Changed         | ❌ Not Yet Implemented | No client-side root change notification handling is present.                                                                                                                                                         |
|                     | Sampling / Create Message    | ❌ Not Yet Implemented | No sampling functionality is implemented.                                                                                                                                                                            |
| **Utilities**       | Ping                         | ✅ Fully Implemented   | `server_stdio.go` handles the `ping` request.                                                                                                                                                                        |
|                     | Cancellation                 | ✅ Fully Implemented   | `server_stdio.go` handles `notifications/cancelled`.                                                                                                                                                                 |
|                     | Progress Tracking            | ❌ Not Yet Implemented | No progress tracking functionality is present.                                                                                                                                                                       |
|                     | Logging / Set Level          | ✅ Fully Implemented   | `server_stdio.go` handles `logging/setLevel`.                                                                                                                                                                        |
|                     | Logging / Message            | ✅ Fully Implemented   | `server_stdio.go` implements `LogMessage` and sends `notifications/message`.                                                                                                                                         |
|                     | Completion / Complete        | ❌ Not Yet Implemented | No completion functionality is present.                                                                                                                                                                              |
|                     | Pagination                   | ✅ Fully Implemented   | `resource_manager.go`, `prompt_manager.go`, `tool_manager.go` implement cursor based list.                                                                                                                           |
| **Transports**      | stdio                        | ✅ Fully Implemented   | `server_stdio.go` and `server_stdio_test.go` demonstrate stdio communication.                                                                                                                                        |
|                     | HTTP with SSE                | ✅ Fully Implemented   | `server_sse.go` implements the full HTTP+SSE transport, including event handling, client message handling, and connection management.                                                                                |
