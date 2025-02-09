# MCP Server in Go: A Comprehensive Guide

This document provides a comprehensive guide to the Go implementation of the Model Context Protocol (MCP) server.
It covers how to start the server, add tools and prompts, and interact with it using the MCP Inspector.

## Overview

This Go package provides a robust and flexible implementation of an MCP server.
It adheres to the [Model Context Protocol specification](https://modelcontextprotocol.io),
enabling seamless integration with LLM applications and providing a standardized way to connect
LLMs with context and tools.  The server supports both `stdio` and `HTTP with Server-Sent Events (SSE)`
transports for communication.

## Getting Started

### Prerequisites

- Go (version 1.18 or later) installed and configured.

### Installation

1.  **Clone the repository:**

    ```bash
    git clone <repository_url>
    cd <repository_directory>
    ```
    (Replace `<repository_url>` and `<repository_directory>` with the actual URL and directory name.)
2. The provided code snippets do not contain the GitHub repository url. This will need to be a real repository to work.

2.  **Get dependencies:**

    ```bash
    go get ./...
    ```

### Running the Server (stdio)

The `StdIOServer` uses standard input and output for communication, 
making it suitable for scenarios where the server runs as a subprocess of the client.

1.  **Create a `main.go` file:**

    ```go
    package main

    import (
        "context"
        "os"

        "github.com/shaharia-lab/goai/mcp"
    )

    func main() {
        server := mcp.NewStdIOServer(os.Stdin, os.Stdout)
        ctx := context.Background()
        if err := server.Run(ctx); err != nil {
            panic(err)
        }
    }
    ```

2.  **Build and run:**

    ```bash
    go build -o mcp-server main.go
    ./mcp-server
    ```

The server is now running and listening for MCP messages on `stdin`.

### Running the Server (HTTP with SSE)

The `SSEServer` provides an HTTP interface with Server-Sent Events,
suitable for web-based clients and scenarios requiring persistent connections.

1.  **Create a `main.go` file (or modify the existing one):**

    ```go
    package main

    import (
        "context"
    	"log"
    	"os"

        "github.com/shaharia-lab/goai/mcp"
    )

    func main() {
    	stdioServer := mcp.NewStdIOServer(os.Stdin, os.Stdout) // Used by the SSE server
    	logger := log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)
        sseServer := mcp.NewSSEServer(stdioServer, ":8080", logger) // Listen on port 8080
        ctx := context.Background()
        if err := sseServer.Run(ctx); err != nil {
            panic(err)
        }
    }
    ```

2.  **Build and run:**

    ```bash
    go build -o mcp-server main.go
    ./mcp-server
    ```

The server is now listening for HTTP connections on port 8080.

## Adding Resources

Resources provide contextual data to the LLM.  The `StdIOServer` includes two sample resources by default.
You can add more resources using the `AddResource` method.

```go
// Inside your main function or initialization logic:
server.AddResource(mcp.Resource{
    URI:         "file:///example/data.csv",
    Name:        "Example CSV Data",
    Description: "Sample CSV data for demonstration.",
    MimeType:    "text/csv",
    TextContent: "col1,col2\nval1,val2\nval3,val4",
})
```
-   **URI:**  A unique identifier for the resource.  The `file://` scheme is commonly used for local files.
-   **Name:** A human-readable name.
-   **Description:** (Optional) A description of the resource.
-   **MimeType:** The MIME type of the resource (e.g., `text/plain`, `application/json`).
-   **TextContent:** The actual content of the resource (for text-based resources).
    For binary resources, this field is used internally to store the content,
    but it's accessed via `Blob` (base64 encoded) in the protocol.

## Adding Tools

Tools allow the LLM to perform actions. The `StdIOServer` comes with a sample `get_weather` tool. 
You can register custom tools and their implementations.

```go
// Define the tool implementation (function that will be executed)
weatherToolImpl := func(args json.RawMessage) (mcp.CallToolResult, error) {
    var input map[string]interface{}
    if err := json.Unmarshal(args, &input); err != nil {
        return mcp.CallToolResult{IsError: true, Content: []mcp.ToolResultContent{{Type: "text", Text: "Invalid arguments."}}}, nil
    }
    location, ok := input["location"].(string)
    if !ok {
        return mcp.CallToolResult{IsError: true, Content: []mcp.ToolResultContent{{Type: "text", Text: "Missing 'location' argument."}}}, nil
    }

    // Simulate fetching weather data (replace with your actual logic)
    weatherData := fmt.Sprintf("Weather in %s: Sunny, 25°C", location)

    return mcp.CallToolResult{
        Content: []mcp.ToolResultContent{{Type: "text", Text: weatherData}},
        IsError: false,
    }, nil
}

// Register the tool with the server
server.AddTool(mcp.Tool{
    Name:        "get_weather",
    Description: "Gets the current weather for a given location.",
    InputSchema: json.RawMessage(`{
        "type": "object",
        "properties": {
            "location": {
                "type": "string",
                "description": "The city and state, e.g. San Francisco, CA"
            }
        },
        "required": ["location"]
    }`),
})

// Create a ToolManager
toolManager := mcp.NewToolManager()

// Register the tool implementation
err := toolManager.RegisterTool(mcp.Tool{
    Name:        "get_weather",
    Description: "Gets the current weather for a given location.",
    InputSchema: json.RawMessage(`{
        "type": "object",
        "properties": {
            "location": {
                "type": "string",
                "description": "The city and state, e.g. San Francisco, CA"
            }
        },
        "required": ["location"]
    }`),
}, weatherToolImpl)

if err != nil {
  panic(err)
}
```
- **Name:**  A unique name for the tool (e.g., `get_weather`, `create_file`).
- **Description:**  A description of what the tool does.
- **InputSchema:**  A JSON Schema defining the expected input arguments. 
  This is crucial for the LLM to understand how to call the tool.
- **ToolImplementation:** A function with the signature `func(args json.RawMessage) (mcp.CallToolResult, error)`.
  This function receives the raw JSON arguments and returns a `CallToolResult`.

## Adding Prompts

Prompts are pre-defined templates or instructions for interacting with language models.
The `StdIOServer` includes sample `code_review` and `summarize_text` prompts.  You can add more:

```go
server.AddPrompt(mcp.Prompt{
    Name:        "write_poem",
    Description: "Generates a short poem on a given topic.",
    Arguments: []mcp.PromptArgument{
        {Name: "topic", Description: "The subject of the poem", Required: true},
        {Name: "style", Description: "Poetic style (e.g., haiku, sonnet)", Required: false},
    },
    Messages: []mcp.PromptMessage{
        {
            Role: "user",
            Content: mcp.PromptContent{
                Type: "text",
                Text: "Write a poem about {topic} in the style of {style}.",
            },
        },
    },
})
```
- **Name:**  A unique name for the prompt.
- **Description:**  A description of the prompt's purpose.
- **Arguments:** An array of `PromptArgument` structs, defining the input parameters.
  Each argument has a `Name`, `Description`, and a `Required` boolean flag.
- **Messages:**  An array of `PromptMessage` structs.  This defines the structure of the
  prompt that will be sent to the LLM.  It includes the `Role` ("user" or "assistant") and `Content`.
  The content can include placeholders for arguments (e.g., `{topic}`).

## Using MCP Inspector

The MCP Inspector is a web-based tool for interacting with and debugging MCP servers.
It allows you to explore available resources, tools, and prompts, and to send requests and view responses.

1.  **Install the MCP Inspector:**

    ```bash
    npm install -g @modelcontextprotocol/inspector
    ```

2.  **Run your MCP server** (as described above).

3.  **Start the Inspector, pointing it to your server:**
    *   **For stdio server:**

        ```bash
        npx @modelcontextprotocol/inspector go run main.go
        ```
        If you have built the server before, then you can use:
        ```bash
        npx @modelcontextprotocol/inspector ./mcp-server
        ```
    * **For http server:**

      First run the server
      ```bash
      go run main.go
      ```
        ```
        npx @modelcontextprotocol/inspector http://localhost:8080/events
        ```

4.  **Open your web browser** and go to the URL displayed by the Inspector (usually `http://localhost:5173`).

You can now use the Inspector's UI to interact with your server.
You'll see a list of available resources, tools, and prompts.
You can click on them to see details, and you can send requests and see the raw JSON messages.

## Feature Matrix

This table summarizes the current level of support for MCP features in the Go `mcp` package,
specifically focusing on the *server-side* implementation:

✅ for "Fully Implemented"
🚧 for "Partially Implemented"
❌ for "Not Yet Implemented"

| Feature Category    | Feature                      | Implementation Status | Notes                                                                                                                |
|:--------------------|:-----------------------------|:----------------------|:---------------------------------------------------------------------------------------------------------------------|
| **Base Protocol**   | JSON-RPC 2.0                 | ✅ Fully Implemented   | All messages use the JSON-RPC 2.0 format.                                                                            |
|                     | Requests                     | ✅ Fully Implemented   | `server_stdio.go` handles requests correctly.                                                                        |
|                     | Responses                    | ✅ Fully Implemented   | `server_stdio.go` sends correctly formatted responses.                                                               |
|                     | Notifications                | ✅ Fully Implemented   | `server_stdio.go` handles and sends notifications.                                                                   |
| **Lifecycle**       | Initialization               | ✅ Fully Implemented   | `server_stdio.go` handles `initialize` request/response, capability negotiation, and protocol version check.         |
|                     | Operation                    | ✅ Fully Implemented   | Server processes requests and sends responses/notifications after initialization.                                    |
|                     | Shutdown                     | ✅ Fully Implemented   | `server_stdio.go`'s `Run` method handles context cancellation.  `stdio` transport shutdown is implicitly supported.  |
| **Server Features** | Resources / List             | ✅ Fully Implemented   | `resource_manager.go` and `server_stdio.go` implement `resources/list` with pagination.                              |
|                     | Resources / Read             | ✅ Fully Implemented   | `resource_manager.go` and `server_stdio.go` implement `resources/read`, handling text and binary content.            |
|                     | Resources / Subscribe        | ❌ Not Yet Implemented | No subscription mechanism is present.                                                                                |
|                     | Resources / List Changed     | ✅ Fully Implemented   | `server_stdio.go` checks for the `listChanged` capability.                                                           |
|                     | Resources / Templates / List | ❌ Not Yet Implemented | No resource template functionality.                                                                                  |
|                     | Prompts / List               | ✅ Fully Implemented   | `prompt_manager.go` and `server_stdio.go` implement `prompts/list` with pagination.                                  |
|                     | Prompts / Get                | ✅ Fully Implemented   | `prompt_manager.go` and `server_stdio.go` implement `prompts/get`, including argument substitution.                  |
|                     | Prompts / List Changed       | ✅ Fully Implemented   | `server_stdio.go` includes `SendPromptListChangedNotification` and capability check;  called by `prompt_manager.go`. |
|                     | Tools / List                 | ✅ Fully Implemented   | `tool_manager.go` and `server_stdio.go` implement `tools/list` with pagination.                                      |
|                     | Tools / Call                 | ✅ Fully Implemented   | `tool_manager.go` and `server_stdio.go` implement `tools/call`, including input schema validation.                   |
|                     | Tools / List Changed         | ✅ Fully Implemented   | `server_stdio.go` includes `SendToolListChangedNotification` and capability check; called by `tool_manager.go`       |
| **Utilities**       | Ping                         | ✅ Fully Implemented   | `server_stdio.go` handles the `ping` request.                                                                        |
|                     | Cancellation                 | ✅ Fully Implemented   | `server_stdio.go` handles `notifications/cancelled`.                                                                 |
|                     | Progress Tracking            | ❌ Not Yet Implemented | No progress tracking.                                                                                                |
|                     | Logging / Set Level          | ✅ Fully Implemented   | `server_stdio.go` handles `logging/setLevel`.                                                                        |
|                     | Logging / Message            | ✅ Fully Implemented   | `server_stdio.go` implements `LogMessage` and sends `notifications/message`.                                         |
|                     | Completion / Complete        | ❌ Not Yet Implemented | No completion functionality.                                                                                         |
|                     | Pagination                   | ✅ Fully Implemented   | Implemented in `resource_manager.go`, `prompt_manager.go`, and `tool_manager.go`.                                    |
| **Transports**      | stdio                        | ✅ Fully Implemented   | `server_stdio.go` provides the `stdio` transport implementation.                                                     |
|                     | HTTP with SSE                | ✅ Fully Implemented   | `server_sse.go` implements the full HTTP+SSE transport.                                                              |

## Key Concepts and Components

-   **StdIOServer:** The core server implementation for stdio communication.
-   **SSEServer:** The core server implementation for HTTP/SSE communication.
-   **ResourceManager:** Manages resources (data provided by the server).
-   **ToolManager:**  Manages tools (functions the LLM can call).
-   **PromptManager:** Manages prompt templates.
-   **LogManager:** Handles logging (used internally by the server and can be used by tool implementations).
-   **Request, Response, Notification:**  The core JSON-RPC message types.
-   **Capabilities:**  Declared by the server during initialization to indicate supported features.

