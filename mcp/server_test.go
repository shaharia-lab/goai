package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
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
