Here's the complete `mcp.md` documentation:

```markdown
# Model Context Protocol (MCP)

The Model Context Protocol (MCP) enables AI models to interact with external tools 
and data sources in a standardized way. This document covers the Go implementation of an MCP server.

## Build & Run MCP Server

### Installation

First, install the package:
```bash
go get github.com/shaharia-lab/goai
```

### Basic Implementation

Here's a minimal example of setting up an MCP server:

```go
package main

import (
    "context"
    "github.com/shaharia-lab/goai"
    "log"
    "net/http"
)

func main() {
    // Create registry
    registry := goai.NewMCPToolRegistry()

    // Use default configuration
    config := goai.DefaultMCPServerConfig()

    // Create server
    server := goai.NewMCPServer(registry, config)

    // Set up HTTP handlers
    mux := http.NewServeMux()
    server.AddMCPHandlers(context.Background(), mux)

    // Start server
    log.Fatal(http.ListenAndServe(":8080", mux))
}
```

### Server Configuration

The MCP server can be configured in several ways:

#### Basic Configuration (No Logging/Tracing)
```go
config := goai.DefaultMCPServerConfig()
server := goai.NewMCPServer(registry, config)
```

#### With Logging
```go
config := goai.DefaultMCPServerConfig()
config.Logger = logrus.New()
server := goai.NewMCPServer(registry, config)
```

#### With OpenTelemetry Tracing
```go
config := goai.DefaultMCPServerConfig()
config.Tracer = otel.Tracer("my-mcp-server")
server := goai.NewMCPServer(registry, config)
```

#### Full Configuration
```go
config := goai.MCPServerConfig{
    Logger:              logrus.New(),
    Tracer:             otel.Tracer("my-mcp-server"),
    EnableMetrics:      true,
    MaxToolExecutionTime: 30 * time.Second,
}
server := goai.NewMCPServer(registry, config)
```

### Creating Tools

To create a tool, implement the `MCPToolExecutor` interface. Here's an example tool that performs word counting:

```go
type WordCountTool struct{}

func (t *WordCountTool) GetDefinition() goai.MCPTool {
    return goai.MCPTool{
        Name:        "word_count",
        Description: "Counts words in a text",
        Version:     "1.0.0",
        InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "text": {
                    "type": "string",
                    "description": "Text to analyze"
                }
            },
            "required": ["text"]
        }`),
    }
}

func (t *WordCountTool) Execute(ctx context.Context, input json.RawMessage) (goai.MCPToolResponse, error) {
    // Parse input
    var params struct {
        Text string `json:"text"`
    }
    if err := json.Unmarshal(input, &params); err != nil {
        return goai.MCPToolResponse{}, fmt.Errorf("invalid input: %w", err)
    }

    // Count words
    words := len(strings.Fields(params.Text))

    // Return response
    return goai.MCPToolResponse{
        Content: []goai.MCPContentItem{{
            Type: "text",
            Text: fmt.Sprintf("Word count: %d", words),
        }},
    }, nil
}
```

### HTTP Server Integration

The MCP server can be integrated with any HTTP router:

#### Standard HTTP Server
```go
mux := http.NewServeMux()
server.AddMCPHandlers(context.Background(), mux)
log.Fatal(http.ListenAndServe(":8080", mux))
```

#### Chi Router
```go
import "github.com/go-chi/chi/v5"

r := chi.NewRouter()
handlers := server.GetMCPHandlers()
r.Get("/tools/list", handlers.ListTools)
r.Post("/tools/call", handlers.ToolCall)
log.Fatal(http.ListenAndServe(":8080", r))
```

### Graceful Shutdown

The MCP server supports graceful shutdown in two modes:

#### Basic Shutdown
For simple servers, just call Shutdown:
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := server.Shutdown(ctx); err != nil {
    log.Printf("Shutdown error: %v", err)
}
```

#### HTTP Server Shutdown
For servers using `http.Server`, register it for full shutdown handling:
```go
httpServer := &http.Server{
    Addr:    ":8080",
    Handler: mux,
}

// Register HTTP server
server.WithHTTPServer(httpServer)

// Handle shutdown signals
go func() {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh

    // Create shutdown context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Perform graceful shutdown
    if err := server.Shutdown(ctx); err != nil {
        log.Printf("Shutdown error: %v", err)
    }
}()

// Start server
if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
    log.Fatalf("HTTP server error: %v", err)
}
```

### Complete Example

Here's a complete example bringing everything together:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "github.com/shaharia-lab/goai"
    "github.com/sirupsen/logrus"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
)

// Define tool
type WordCountTool struct{}

// ... (WordCountTool implementation from above) ...

func main() {
    // Create registry
    registry := goai.NewMCPToolRegistry()

    // Register tool
    if err := registry.Register(&WordCountTool{}); err != nil {
        log.Fatal(err)
    }

    // Configure server with logging
    config := goai.DefaultMCPServerConfig()
    config.Logger = logrus.New()

    // Create server
    server := goai.NewMCPServer(registry, config)

    // Set up HTTP server
    mux := http.NewServeMux()
    server.AddMCPHandlers(context.Background(), mux)

    // Create HTTP server
    httpServer := &http.Server{
        Addr:    ":8080",
        Handler: mux,
    }

    // Register for shutdown
    server.WithHTTPServer(httpServer)

    // Handle shutdown signals
    go func() {
        sigCh := make(chan os.Signal, 1)
        signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
        <-sigCh

        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        if err := server.Shutdown(ctx); err != nil {
            log.Printf("Shutdown error: %v", err)
        }
    }()

    // Start server
    fmt.Println("Server starting on :8080...")
    if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
        log.Fatalf("HTTP server error: %v", err)
    }
}
```

### Testing Your Server

Once your server is running, you can test it using curl:

```bash
# List available tools
curl http://localhost:8080/tools/list

# Call a tool
curl -X POST http://localhost:8080/tools/call \
  -H "Content-Type: application/json" \
  -d '{
    "name": "word_count",
    "arguments": {
      "text": "Hello, Model Context Protocol!"
    }
  }'
```

### Best Practices

When implementing MCP tools:

1. **Input Validation**: Always validate tool inputs thoroughly
2. **Error Handling**: Return clear, actionable error messages
3. **Context Awareness**: Respect context cancellation for long-running operations
4. **Thread Safety**: Ensure your tool implementation is thread-safe
5. **Documentation**: Provide clear descriptions and input schemas
6. **Versioning**: Include version information in your tool definition
7. **Logging**: Use appropriate log levels when logging is enabled
8. **Tracing**: Add meaningful spans when tracing is enabled
9. **Shutdown**: Implement proper cleanup in your tools if needed
10. **Testing**: Add comprehensive tests for your tools

### Learn More

To learn more about the Model Context Protocol and its capabilities,
visit the official documentation at [https://modelcontextprotocol.io/](https://modelcontextprotocol.io/).