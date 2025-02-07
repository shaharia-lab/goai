# Model Context Protocol (MCP) Go Implementation

The Model Context Protocol (MCP) enables AI models to interact with external tools and data sources in a standardized way. This document covers the Go implementation of an MCP server that follows the official MCP specification.

## Installation

```bash
go get github.com/shaharia-lab/goai
```

## Server Implementation

### Basic Server Setup

```go
package main

import (
    "context"
    "github.com/shaharia-lab/goai"
    "github.com/sirupsen/logrus"
    "log"
    "net/http"
)

func main() {
    // Create server with default configuration
    config := goai.DefaultMCPServerConfig()
    config.Logger = logrus.New()
    
    server := goai.NewMCPServer(config)
    
    // Register tools
    tool := &MyTool{}
    if err := server.RegisterTool(tool); err != nil {
        log.Fatal(err)
    }
    
    // Set up HTTP handlers
    mux := http.NewServeMux()
    handlers := server.GetMCPHandlers()
    
    // Register required endpoints
    mux.HandleFunc("/initialize", handlers.Initialize)
    mux.HandleFunc("/tools/list", handlers.ListTools)
    mux.HandleFunc("/tools/call", handlers.ToolCall)
    mux.HandleFunc("/resource/get", handlers.GetResource)
    mux.HandleFunc("/resource/watch", handlers.WatchResource)
    
    // Start server
    log.Fatal(http.ListenAndServe(":8080", mux))
}
```

### Server Configuration Options

```go
config := goai.MCPServerConfig{
    Logger:               logrus.New(),                // Optional logging
    Tracer:              otelTracer,                  // Optional OpenTelemetry tracing
    EnableMetrics:       true,                        // Enable metrics collection
    MaxToolExecutionTime: 30 * time.Second,           // Tool execution timeout
    MaxRequestSize:      1024 * 1024,                // Max request size (1MB)
    AllowedOrigins:      []string{"*"},              // CORS settings
}
```

### Implementing Tools

Tools must implement the `MCPToolExecutor` interface:

```go
type MyTool struct{}

func (t *MyTool) GetDefinition() goai.MCPTool {
    return goai.MCPTool{
        Name:        "my_tool",
        Description: "Example tool",
        Version:     "1.0.0",
        InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "input": {"type": "string"}
            },
            "required": ["input"]
        }`),
        Capabilities: []string{"text_processing"},
        Metadata: json.RawMessage(`{
            "author": "Example Author"
        }`),
    }
}

func (t *MyTool) Execute(ctx context.Context, input json.RawMessage) (goai.MCPToolResponse, error) {
    var params struct {
        Input string `json:"input"`
    }
    if err := json.Unmarshal(input, &params); err != nil {
        return goai.MCPToolResponse{}, err
    }
    
    return goai.MCPToolResponse{
        Content: []goai.MCPContentItem{{
            Type: "text",
            Text: "Processed: " + params.Input,
            Metadata: json.RawMessage(`{"timestamp": "2024-01-01T00:00:00Z"}`),
        }},
    }, nil
}
```

### Resource Management

Resources can be managed using the built-in handlers:

```go
// Get a resource
GET /resource/get?id=resource_id

// Watch for resource updates
WebSocket /resource/watch
```

### Error Handling

The server uses standard JSON-RPC 2.0 error codes:

```go
const (
    MCPErrorCodeParseError     = -32700
    MCPErrorCodeInvalidRequest = -32600
    MCPErrorCodeMethodNotFound = -32601
    MCPErrorCodeInvalidParams  = -32602
    MCPErrorCodeInternal      = -32603
)
```

### Graceful Shutdown

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := server.Shutdown(ctx); err != nil {
    log.Printf("Shutdown error: %v", err)
}
```

## Specification Compliance

| Feature                         | Status     | Notes                                               |
|---------------------------------|------------|-----------------------------------------------------|
| **Core Protocol**               |            |                                                     |
| JSON-RPC 2.0 Message Format     | ✅ Full     | Implements all required message structures          |
| Server Capabilities Negotiation | ✅ Full     | Includes version, features, and extensions          |
| Error Handling                  | ✅ Full     | Standard JSON-RPC error codes and custom error data |
| Connection Management           | ✅ Full     | WebSocket support with proper lifecycle             |
| **Resource Management**         |            |                                                     |
| Resource Retrieval              | ✅ Full     | GET endpoint with proper error handling             |
| Resource Watching               | ✅ Full     | WebSocket-based updates                             |
| Resource Versioning             | ✅ Full     | Version tracking and timestamps                     |
| Resource Metadata               | ✅ Full     | Custom metadata support                             |
| **Tool Integration**            |            |                                                     |
| Tool Registration               | ✅ Full     | Dynamic tool registration with validation           |
| Tool Discovery                  | ✅ Full     | List endpoint with filtering                        |
| Tool Execution                  | ✅ Full     | Async execution with timeout control                |
| Input Schema Validation         | ⚠️ Partial | Basic validation, complex schemas need enhancement  |
| Tool Capabilities               | ✅ Full     | Proper capability declaration and checking          |
| **Security**                    |            |                                                     |
| CORS Support                    | ✅ Full     | Configurable allowed origins                        |
| Request Size Limits             | ✅ Full     | Configurable max request size                       |
| Rate Limiting                   | ❌ Missing  | Not implemented yet                                 |
| Authentication                  | ⚠️ Partial | Basic auth support, needs enhancement               |
| **Observability**               |            |                                                     |
| Logging                         | ✅ Full     | Structured logging with levels                      |
| Metrics                         | ⚠️ Partial | Basic metrics, needs more coverage                  |
| Tracing                         | ✅ Full     | OpenTelemetry integration                           |
| **Extensions**                  |            |                                                     |
| Custom Metadata                 | ✅ Full     | Supports arbitrary JSON metadata                    |
| Protocol Extensions             | ⚠️ Partial | Basic extension support, needs documentation        |
| Custom Content Types            | ✅ Full     | Flexible content type handling                      |

### Legend
- ✅ Full: Feature is fully implemented and compliant
- ⚠️ Partial: Feature is partially implemented or needs enhancement
- ❌ Missing: Feature is not implemented yet

### Potential Improvements
1. Implement rate limiting for better security
2. Enhance input schema validation for complex schemas
3. Strengthen authentication mechanisms
4. Expand metrics coverage for better observability
5. Document and formalize protocol extension mechanisms

### Notes
- The implementation is production-ready for basic use cases
- Security features should be enhanced before using in public-facing environments
- All critical path features are fully implemented
- Some optional features are partially implemented but functional