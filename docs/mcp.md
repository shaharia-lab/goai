# MCP Server Implementation Guide

## Table of Contents
- [Overview](#overview)
- [Server Setup](#server-setup)
- [Endpoints](#endpoints)
- [Transport Options](#transport-options)
- [Custom Tools](#custom-tools)
- [Testing](#testing)
- [Advanced Features](#advanced-features)

## Overview

The MCP Server implementation provides a Go-based server following the Model Context Protocol specification. It supports:

- Multiple transport options (stdio, WebSocket)
- Resource management
- Tool execution
- Progress tracking
- Cancellation support

## Server Setup

### Basic Configuration

```go
package main

import (
    "context"
    "net/http"
    "github.com/shaharia-lab/goai"
    "github.com/sirupsen/logrus"
)

func main() {
    config := goai.MCPServerConfig{
        Logger:               logrus.New(),
        MaxToolExecutionTime: 30 * time.Second,
        MaxRequestSize:       1024 * 1024,
        AllowedOrigins:      []string{"*"},
        EnableWebSocket:     true,
    }
    
    server := goai.NewMCPServer(config)
    
    // Start HTTP server for WebSocket transport
    http.HandleFunc("/mcp", server.HandleWebSocket)
    go http.ListenAndServe(":8080", nil)
    
    // Start MCP server
    ctx := context.Background()
    if err := server.Start(ctx); err != nil {
        panic(err)
    }
}
```

## Endpoints

The MCP server exposes endpoints based on the selected transport options.

### WebSocket Endpoint

- **URL**: `/mcp`
- **Protocol**: WebSocket
- **Headers**:
    - `Upgrade: websocket`
    - `Connection: Upgrade`

### JSON-RPC Methods

All communication follows the JSON-RPC 2.0 specification.

#### Initialize

Request:
```json
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
        "protocolVersion": "2024-11-05",
        "capabilities": {
            "resources": {
                "subscribe": true,
                "listChanged": true
            }
        },
        "clientInfo": {
            "name": "test-client",
            "version": "1.0.0"
        }
    }
}
```

Response:
```json
{
    "jsonrpc": "2.0",
    "id": 1,
    "result": {
        "protocolVersion": "2024-11-05",
        "capabilities": {
            "resources": {
                "subscribe": true,
                "listChanged": true
            },
            "tools": {
                "listChanged": true
            },
            "logging": {}
        },
        "serverInfo": {
            "name": "MCP Go Server",
            "version": "1.0.0"
        }
    }
}
```

#### Execute Tool

Request:
```json
{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/call",
    "params": {
        "name": "get_weather",
        "arguments": {
            "location": "New York"
        }
    }
}
```

Response:
```json
{
    "jsonrpc": "2.0",
    "id": 2,
    "result": {
        "content": [
            {
                "type": "text",
                "text": "Current weather in New York: Sunny, 22°C"
            }
        ],
        "isError": false
    }
}
```

#### Resource Operations

List Resources:
```json
{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "resources/list"
}
```

Subscribe to Resource:
```json
{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "resources/subscribe",
    "params": {
        "uri": "file:///example/test.txt"
    }
}
```

## Testing

### Using curl with WebSocket

Test the WebSocket connection using wscat:

```bash
# Install wscat
npm install -g wscat

# Connect to server
wscat -c ws://localhost:8080/mcp

# Initialize connection
> {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{}}}
```

### Unit Testing Tools

```go
package main

import (
    "context"
    "encoding/json"
    "testing"
    "github.com/shaharia-lab/goai"
)

func TestWeatherTool(t *testing.T) {
    tool := &WeatherTool{}
    
    // Test tool definition
    def := tool.GetDefinition()
    if def.Name != "get_weather" {
        t.Errorf("expected tool name 'get_weather', got %s", def.Name)
    }
    
    // Test execution
    input := json.RawMessage(`{"location":"New York"}`)
    resp, err := tool.Execute(context.Background(), input)
    if err != nil {
        t.Fatal(err)
    }
    
    if len(resp.Content) != 1 {
        t.Fatal("expected 1 content item")
    }
}
```

### Integration Testing

```go
func TestMCPServer(t *testing.T) {
    // Create test server
    config := goai.DefaultMCPServerConfig()
    server := goai.NewMCPServer(config)
    
    // Register test tool
    tool := &WeatherTool{}
    if err := server.RegisterTool(tool); err != nil {
        t.Fatal(err)
    }
    
    // Start server
    ctx := context.Background()
    if err := server.Start(ctx); err != nil {
        t.Fatal(err)
    }
    
    // Create test client connection
    conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/mcp", nil)
    if err != nil {
        t.Fatal(err)
    }
    defer conn.Close()
    
    // Send initialize request
    initRequest := goai.MCPMessage{
        JsonRPC: "2.0",
        ID:      1,
        Method:  "initialize",
        Params:  json.RawMessage(`{"protocolVersion":"2024-11-05"}`),
    }
    
    if err := conn.WriteJSON(initRequest); err != nil {
        t.Fatal(err)
    }
    
    // Read response
    var response goai.MCPMessage
    if err := conn.ReadJSON(&response); err != nil {
        t.Fatal(err)
    }
    
    // Verify response
    if response.Error != nil {
        t.Fatalf("initialization failed: %v", response.Error)
    }
}
```

## Custom Tools

### Example Tool Implementation

```go
type SearchTool struct{}

type SearchParams struct {
    Query string `json:"query"`
    Limit int    `json:"limit"`
}

func (t *SearchTool) GetDefinition() goai.MCPTool {
    return goai.MCPTool{
        Name:        "search",
        Description: "Search content",
        InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "Search query"
                },
                "limit": {
                    "type": "integer",
                    "default": 10
                }
            },
            "required": ["query"]
        }`),
        Version:      "1.0.0",
        Capabilities: []string{"text"},
    }
}

func (t *SearchTool) Execute(ctx context.Context, input json.RawMessage) (goai.MCPToolResponse, error) {
    var params SearchParams
    if err := json.Unmarshal(input, &params); err != nil {
        return goai.MCPToolResponse{}, err
    }
    
    // Report progress
    if execCtx, ok := ctx.Value("execContext").(*goai.MCPToolExecutionContext); ok {
        execCtx.Progress(goai.MCPProgress{
            Percentage: 50,
            Message:   "Searching...",
        })
    }
    
    // Implement search logic
    results := performSearch(params.Query, params.Limit)
    
    return goai.MCPToolResponse{
        Content: []goai.MCPContentItem{
            {
                Type: "text",
                Text: fmt.Sprintf("Found %d results", len(results)),
            },
        },
        Metadata: json.RawMessage(results),
    }, nil
}
```

### Tool Registration

```go
func main() {
    server := goai.NewMCPServer(goai.DefaultMCPServerConfig())
    
    // Register multiple tools
    tools := []goai.MCPToolExecutor{
        &SearchTool{},
        &WeatherTool{},
    }
    
    for _, tool := range tools {
        if err := server.RegisterTool(tool); err != nil {
            log.Fatalf("Failed to register tool: %v", err)
        }
    }
}
```

## Advanced Features

### Resource Subscriptions

```go
// Subscribe to resource changes
notification := goai.MCPMessage{
    JsonRPC: "2.0",
    Method:  "resources/subscribe",
    Params:  json.RawMessage(`{"uri":"file:///example.txt"}`),
}

// Resource change notification format
{
    "jsonrpc": "2.0",
    "method": "notifications/resources/updated",
    "params": {
        "uri": "file:///example.txt",
        "content": "Updated content",
        "version": "2"
    }
}
```

### Progress Tracking

```go
// Progress notification format
{
    "jsonrpc": "2.0",
    "method": "$/progress",
    "params": {
        "percentage": 75,
        "message": "Processing data...",
        "details": {
            "currentStep": "validation",
            "remainingSteps": 2
        }
    }
}
```

### Cancellation

```go
// Cancellation request
{
    "jsonrpc": "2.0",
    "method": "$/cancel",
    "params": {
        "id": "request-123",
        "reason": "User requested cancellation"
    }
}
```

### Error Handling

Standard error codes:
- `-32700`: Parse error
- `-32600`: Invalid request
- `-32601`: Method not found
- `-32602`: Invalid params
- `-32603`: Internal error

Custom error response example:
```json
{
    "jsonrpc": "2.0",
    "id": 5,
    "error": {
        "code": -32602,
        "message": "Invalid parameters",
        "data": {
            "missing": ["location"],
            "invalid": {"temperature": "must be numeric"}
        }
    }
}
```

This documentation provides implementation details for the most common use cases. For additional features or customization, refer to the MCP specification or the package documentation.