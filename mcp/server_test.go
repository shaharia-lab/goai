package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewMCPServer(t *testing.T) {
	in := strings.NewReader("")
	out := &bytes.Buffer{}

	server := NewMCPServer(in, out)

	if server == nil {
		t.Fatal("NewMCPServer returned nil")
	}

	// Verify initial state
	if server.protocolVersion != "2024-11-05" {
		t.Errorf("Expected protocol version 2024-11-05, got %s", server.protocolVersion)
	}

	if server.ServerInfo.Name != "Resource-Server-Example" {
		t.Errorf("Expected server name Resource-Server-Example, got %s", server.ServerInfo.Name)
	}

	// Verify maps are initialized
	if server.resources == nil {
		t.Error("Resources map was not initialized")
	}

	if server.tools == nil {
		t.Error("Tools map was not initialized")
	}

	if server.prompts == nil {
		t.Error("Prompts map was not initialized")
	}
}

func TestServerCommunication(t *testing.T) {
	in := strings.NewReader("")
	out := &bytes.Buffer{}
	server := NewMCPServer(in, out)

	// Test sending a response
	id := json.RawMessage(`1`)
	result := map[string]string{"status": "ok"}

	server.sendResponse(&id, result, nil)

	// Verify response format
	var response Response
	if err := json.NewDecoder(out).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC 2.0, got %s", response.JSONRPC)
	}

	if string(*response.ID) != "1" {
		t.Errorf("Expected ID 1, got %s", string(*response.ID))
	}
}

func TestServerErrorHandling(t *testing.T) {
	in := strings.NewReader("")
	out := &bytes.Buffer{}
	server := NewMCPServer(in, out)

	// Test sending an error
	id := json.RawMessage(`1`)
	server.sendError(&id, -32600, "Invalid Request", nil)

	var response Response
	if err := json.NewDecoder(out).Decode(&response); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if response.Error == nil {
		t.Fatal("Expected error in response, got nil")
	}

	if response.Error.Code != -32600 {
		t.Errorf("Expected error code -32600, got %d", response.Error.Code)
	}

	if response.Error.Message != "Invalid Request" {
		t.Errorf("Expected error message 'Invalid Request', got %s", response.Error.Message)
	}
}

func TestServerInitialization(t *testing.T) {
	initRequest := `{
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {
                "name": "test-client",
                "version": "1.0.0"
            }
        }}`

	in := strings.NewReader(initRequest)
	out := &bytes.Buffer{}
	server := NewMCPServer(in, out)

	// Process the request through the server
	var request Request
	if err := json.NewDecoder(in).Decode(&request); err != nil {
		t.Fatalf("Failed to unmarshal request: %v", err)
	}

	// Let the server handle the initialize request
	server.handleRequest(&request)

	// Parse the server's response
	var response Response
	if err := json.NewDecoder(out).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify the response
	if response.Error != nil {
		t.Errorf("Unexpected error in response: %v", response.Error)
	}

	// Type assert and verify the result
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Expected result to be a map")
	}

	// Verify the protocol version in the response
	if protocolVersion, ok := result["protocolVersion"].(string); ok {
		if protocolVersion != "2024-11-05" {
			t.Errorf("Expected protocol version 2024-11-05, got %s", protocolVersion)
		}
	} else {
		t.Error("Protocol version missing or invalid type in response")
	}

	// Verify server info in the response
	if serverInfo, ok := result["serverInfo"].(map[string]interface{}); ok {
		if name, ok := serverInfo["name"].(string); ok {
			if name != "Resource-Server-Example" {
				t.Errorf("Expected server name Resource-Server-Example, got %s", name)
			}
		} else {
			t.Error("Server name missing or invalid type in response")
		}
	} else {
		t.Error("Server info missing or invalid type in response")
	}
}

// TestServerRun tests the server's Run method
func TestServerRun(t *testing.T) {
	t.Run("server starts and handles requests", func(t *testing.T) {
		in := &bytes.Buffer{}
		out := &bytes.Buffer{}
		srv := NewMCPServer(in, out)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Write the request before starting the server
		request := Request{
			JSONRPC: "2.0",
			Method:  "initialize",
			Params: json.RawMessage(`{
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "test-client", "version": "1.0.0"}
            }`),
		}
		if err := json.NewEncoder(in).Encode(request); err != nil {
			t.Fatalf("Failed to encode request: %v", err)
		}

		// Run the server and wait for completion
		done := make(chan struct{})
		go func() {
			if err := srv.Run(ctx); err != nil && err != context.Canceled {
				t.Errorf("Server error: %v", err)
			}
			close(done)
		}()

		// Wait for response or timeout
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("Test timed out")
		}
	})
}

// TestServerRequestHandling tests different request handling scenarios
func TestServerRequestHandling(t *testing.T) {
	tests := []struct {
		name         string
		request      Request
		expectError  bool
		expectedCode int
		expectedMsg  string
	}{
		{
			name: "valid initialize request",
			request: Request{
				JSONRPC: "2.0",
				ID:      rawJSON("1"),
				Method:  "initialize",
				Params: json.RawMessage(`{
                    "protocolVersion": "2024-11-05",
                    "capabilities": {},
                    "clientInfo": {"name": "test-client", "version": "1.0.0"}
                }`),
			},
			expectError: false,
		},
		{
			name: "invalid method",
			request: Request{
				JSONRPC: "2.0",
				ID:      rawJSON("2"),
				Method:  "nonexistent",
				Params:  json.RawMessage(`{}`),
			},
			expectError:  true,
			expectedCode: -32601,
			expectedMsg:  "Method not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := &bytes.Buffer{}
			out := &bytes.Buffer{}
			srv := NewMCPServer(in, out)

			// Write request
			if err := json.NewEncoder(in).Encode(tc.request); err != nil {
				t.Fatalf("Failed to encode request: %v", err)
			}

			// Handle the request directly
			srv.handleRequest(&tc.request)

			// Read and verify response
			var resp Response
			if err := json.NewDecoder(out).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Verify error conditions
			if tc.expectError {
				if resp.Error == nil {
					t.Fatal("Expected error but got none")
				}
				if resp.Error.Code != tc.expectedCode {
					t.Errorf("Expected error code %d, got %d", tc.expectedCode, resp.Error.Code)
				}
				if resp.Error.Message != tc.expectedMsg {
					t.Errorf("Expected error message %q, got %q", tc.expectedMsg, resp.Error.Message)
				}
			} else {
				if resp.Error != nil {
					t.Errorf("Expected success but got error: %v", resp.Error)
				}
				if resp.Result == nil {
					t.Error("Expected result but got nil")
				}
			}
		})
	}
}

// Helper function to create json.RawMessage
func rawJSON(s string) *json.RawMessage {
	raw := json.RawMessage(s)
	return &raw
}

// TestServerConcurrency tests concurrent request handling
func TestServerConcurrency(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	srv := NewMCPServer(in, out)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the server in a goroutine
	go func() {
		if err := srv.Run(ctx); err != nil && err != context.Canceled {
			t.Errorf("Server error: %v", err)
		}
	}()

	// Create multiple concurrent requests
	concurrentRequests := 10
	done := make(chan bool, concurrentRequests)

	for i := 0; i < concurrentRequests; i++ {
		go func(id int) {
			request := Request{
				JSONRPC: "2.0",
				Method:  "initialize",
				Params: json.RawMessage(`{
					"protocolVersion": "2024-11-05",
					"capabilities": {},
					"clientInfo": {"name": "test", "version": "1.0"}
				}`),
			}

			if err := json.NewEncoder(in).Encode(request); err != nil {
				t.Errorf("Failed to encode request %d: %v", id, err)
			}
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < concurrentRequests; i++ {
		<-done
	}
}

// TestServerShutdown tests graceful shutdown behavior
func TestServerShutdown(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	server := NewMCPServer(in, out)

	ctx, cancel := context.WithCancel(context.Background())

	// Start server
	errCh := make(chan error)
	go func() {
		errCh <- server.Run(ctx)
	}()

	// Trigger shutdown
	cancel()

	// Check shutdown behavior
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Errorf("Unexpected error during shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Server failed to shut down in time")
	}
}

func TestLogMessage(t *testing.T) {
	tests := []struct {
		name       string
		level      LogLevel
		minLevel   LogLevel
		loggerName string
		data       interface{}
		expectLog  bool
	}{
		{
			name:       "debug message above min level",
			level:      LogLevelDebug,
			minLevel:   LogLevelError,
			loggerName: "test",
			data:       "debug info",
			expectLog:  false,
		},
		{
			name:       "error message at min level",
			level:      LogLevelError,
			minLevel:   LogLevelError,
			loggerName: "test",
			data:       "error occurred",
			expectLog:  true,
		},
		{
			name:       "info message below min level",
			level:      LogLevelInfo,
			minLevel:   LogLevelDebug,
			loggerName: "system",
			data:       map[string]string{"status": "running"},
			expectLog:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			srv := NewMCPServer(strings.NewReader(""), out)
			srv.minLogLevel = tc.minLevel

			// Call LogMessage
			srv.LogMessage(tc.level, tc.loggerName, tc.data)

			// Check if notification was sent
			if tc.expectLog {
				// Try to decode the notification
				var notification struct {
					JSONRPC string           `json:"jsonrpc"`
					Method  string           `json:"method"`
					Params  LogMessageParams `json:"params"`
				}

				if err := json.NewDecoder(out).Decode(&notification); err != nil {
					t.Fatalf("Failed to decode notification: %v", err)
				}

				// Verify notification format
				if notification.JSONRPC != "2.0" {
					t.Errorf("Expected JSONRPC 2.0, got %s", notification.JSONRPC)
				}

				if notification.Method != "notifications/message" {
					t.Errorf("Expected method notifications/message, got %s", notification.Method)
				}

				// Verify params
				if notification.Params.Level != tc.level {
					t.Errorf("Expected level %s, got %s", tc.level, notification.Params.Level)
				}

				if notification.Params.Logger != tc.loggerName {
					t.Errorf("Expected logger %s, got %s", tc.loggerName, notification.Params.Logger)
				}

				// Verify data was included
				if notification.Params.Data == nil {
					t.Error("Expected data in notification, got nil")
				}
			} else {
				// Verify no notification was sent
				if out.Len() > 0 {
					t.Error("Expected no notification, but got output")
				}
			}
		})
	}
}
