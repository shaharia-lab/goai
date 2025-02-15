package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"testing"
)

type mockServer struct {
	in     io.Reader
	out    io.Writer
	server *StdIOServer
}

func setupTestClient(t *testing.T) (*StdIOClient, *mockServer, func()) {
	// Create pipes for bidirectional communication
	clientRead, serverWrite := io.Pipe()
	serverRead, clientWrite := io.Pipe()

	// Create a buffer for logs
	logBuffer := &bytes.Buffer{}
	logger := log.New(logBuffer, "Test: ", log.LstdFlags)

	// Create client config
	clientConfig := StdIOClientConfig{
		Logger: logger,
		Reader: clientRead,
		Writer: clientWrite,
	}

	// Create client
	client := NewStdIOClient(clientConfig)

	// Create base server
	baseServer, err := NewBaseServer(
		UseLogger(logger),
	)
	if err != nil {
		t.Fatalf("Failed to create base server: %v", err)
	}

	// Create server
	server := NewStdIOServer(baseServer, serverRead, serverWrite)

	mock := &mockServer{
		in:     serverRead,
		out:    serverWrite,
		server: server,
	}

	// Start server in a goroutine
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := server.Run(ctx); err != nil && err != context.Canceled {
			t.Errorf("Server error: %v", err)
		}
	}()

	cleanup := func() {
		cancel()
		clientRead.Close()
		clientWrite.Close()
		serverRead.Close()
		serverWrite.Close()
	}

	return client, mock, cleanup
}

func TestClientInitialization(t *testing.T) {
	client, _, cleanup := setupTestClient(t)
	defer cleanup()

	// Test initialization
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Verify client state
	if !client.initialized {
		t.Error("Client should be initialized")
	}

	if client.state != Connected {
		t.Error("Client should be in Connected state")
	}
}

func TestClientListTools(t *testing.T) {
	client, mock, cleanup := setupTestClient(t)
	defer cleanup()

	// Add a test tool to the server
	testTool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"param": {
					"type": "string"
				}
			}
		}`),
		Handler: func(ctx context.Context, params CallToolParams) (CallToolResult, error) {
			return CallToolResult{
				Content: []ToolResultContent{
					{
						Type: "text",
						Text: "test response",
					},
				},
			}, nil
		},
	}

	err := mock.server.BaseServer.AddTools(testTool)
	if err != nil {
		t.Fatalf("Failed to add test tool: %v", err)
	}

	// Connect client
	if err := client.Connect(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Test listing tools
	tools, err := client.ListTools()
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}

	if tools[0].Name != "test_tool" {
		t.Errorf("Expected tool name 'test_tool', got '%s'", tools[0].Name)
	}

	if tools[0].Description != "A test tool" {
		t.Errorf("Expected tool description 'A test tool', got '%s'", tools[0].Description)
	}
}
