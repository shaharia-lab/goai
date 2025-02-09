package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// Test helper to wait for and parse response
func waitForResponse(t *testing.T, out *bytes.Buffer, timeout time.Duration) *Response {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if out.Len() > 0 {
			var response Response
			if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &response); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}
			return &response
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Timeout waiting for response")
	return nil
}

func TestNewStdIOServer(t *testing.T) {
	in := strings.NewReader("")
	out := &bytes.Buffer{}

	server := NewStdIOServer(in, out)

	if server == nil {
		t.Fatal("Expected non-nil server")
	}
	if server.in == nil {
		t.Error("Expected non-nil input reader")
	}
	if server.out == nil {
		t.Error("Expected non-nil output writer")
	}

	// Verify initial resources were added
	if len(server.resources) != 2 {
		t.Errorf("Expected 2 initial resources, got %d", len(server.resources))
	}

	// Verify initial tools were added
	if len(server.tools) != 1 {
		t.Errorf("Expected 1 initial tool, got %d", len(server.tools))
	}

	// Verify initial prompts were added
	if len(server.prompts) != 2 {
		t.Errorf("Expected 2 initial prompts, got %d", len(server.prompts))
	}
}

func TestSendResponse(t *testing.T) {
	out := &bytes.Buffer{}
	server := NewStdIOServer(strings.NewReader(""), out)

	testID := json.RawMessage(`1`)
	testResult := map[string]string{"status": "ok"}

	server.sendResponse("", &testID, testResult, nil)

	var response Response
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC 2.0, got %s", response.JSONRPC)
	}

	if string(*response.ID) != string(testID) {
		t.Errorf("Expected ID %s, got %s", string(testID), string(*response.ID))
	}

	resultMap, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Expected map result")
	}
	if resultMap["status"] != "ok" {
		t.Errorf("Expected status ok, got %v", resultMap["status"])
	}
}

func TestSendError(t *testing.T) {
	out := &bytes.Buffer{}
	server := NewStdIOServer(strings.NewReader(""), out)

	testID := json.RawMessage(`1`)
	server.sendError("", &testID, -32600, "Invalid Request", nil)

	var response Response
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error == nil {
		t.Fatal("Expected error in response")
	}
	if response.Error.Code != -32600 {
		t.Errorf("Expected error code -32600, got %d", response.Error.Code)
	}
	if response.Error.Message != "Invalid Request" {
		t.Errorf("Expected error message 'Invalid Request', got %s", response.Error.Message)
	}
}

func TestSendNotification(t *testing.T) {
	out := &bytes.Buffer{}
	server := NewStdIOServer(strings.NewReader(""), out)

	testMethod := "test/notification"
	testParams := map[string]string{"message": "test"}

	server.sendNotification("", testMethod, testParams)

	var notification Notification
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &notification); err != nil {
		t.Fatalf("Failed to unmarshal notification: %v", err)
	}

	if notification.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC 2.0, got %s", notification.JSONRPC)
	}
	if notification.Method != testMethod {
		t.Errorf("Expected method %s, got %s", testMethod, notification.Method)
	}

	var params map[string]string
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("Failed to unmarshal params: %v", err)
	}
	if params["message"] != "test" {
		t.Errorf("Expected message 'test', got %s", params["message"])
	}
}

func TestRun(t *testing.T) {
	// Test initialization sequence
	initMessage := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	initNotification := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	pingMessage := `{"jsonrpc":"2.0","id":2,"method":"ping"}`

	messages := []string{
		initMessage + "\n",
		initNotification + "\n",
		pingMessage + "\n",
	}

	in := newMockReader(messages)
	out := &bytes.Buffer{}
	server := NewStdIOServer(in, out)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Run(ctx)
	}()

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop server
	cancel()

	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled {
			t.Errorf("Unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Server did not shut down in time")
	}

	// Verify responses
	responses := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(responses) < 2 {
		t.Fatalf("Expected at least 2 responses, got %d", len(responses))
	}

	// Check initialization response
	var initResponse Response
	if err := json.Unmarshal([]byte(responses[0]), &initResponse); err != nil {
		t.Fatalf("Failed to unmarshal init response: %v", err)
	}
	if initResponse.Error != nil {
		t.Errorf("Unexpected error in init response: %v", initResponse.Error)
	}

	// Check ping response
	var pingResponse Response
	if err := json.Unmarshal([]byte(responses[1]), &pingResponse); err != nil {
		t.Fatalf("Failed to unmarshal ping response: %v", err)
	}
	if pingResponse.Error != nil {
		t.Errorf("Unexpected error in ping response: %v", pingResponse.Error)
	}
}

func newMockReader(messages []string) io.Reader {
	return &mockReader{
		messages: messages,
		current:  0,
	}
}

type mockReader struct {
	messages []string
	current  int
}

func (m *mockReader) Read(p []byte) (n int, err error) {
	if m.current >= len(m.messages) {
		return 0, io.EOF
	}

	message := m.messages[m.current]
	m.current++

	copy(p, []byte(message))
	return len(message), nil
}

func TestErrorHandling(t *testing.T) {
	// Test various error conditions
	tests := []struct {
		name     string
		messages []string
		expected int // expected error code
	}{
		{
			name:     "Invalid JSON",
			messages: []string{"invalid json\n"},
			expected: -32700, // Parse error
		},
		{
			name: "Missing Method",
			messages: []string{
				`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}` + "\n",
				`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n",
				`{"jsonrpc":"2.0","id":2}` + "\n",
			},
			expected: -32600, // Invalid Request
		},
		{
			name: "Uninitialized Request",
			messages: []string{
				`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n",
			},
			expected: -32000, // Server not initialized
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new server for each test
			in := strings.NewReader(strings.Join(tt.messages, ""))
			out := &bytes.Buffer{}
			server := NewStdIOServer(in, out)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			errChan := make(chan error, 1)
			go func() {
				errChan <- server.Run(ctx)
			}()

			// Wait for processing
			time.Sleep(100 * time.Millisecond)

			// Cancel context to stop server
			cancel()

			select {
			case err := <-errChan:
				if err != nil && err != context.Canceled {
					t.Errorf("Unexpected error: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Server did not shut down in time")
			}

			// Get the last response
			responses := strings.Split(strings.TrimSpace(out.String()), "\n")
			if len(responses) == 0 {
				t.Fatal("No response received")
			}

			lastResponse := responses[len(responses)-1]
			var response Response
			if err := json.Unmarshal([]byte(lastResponse), &response); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if response.Error == nil {
				t.Fatal("Expected error response")
			}
			if response.Error.Code != tt.expected {
				t.Errorf("Expected error code %d, got %d", tt.expected, response.Error.Code)
			}
		})
	}
}

func TestResourceHandling(t *testing.T) {
	out := &bytes.Buffer{}
	server := NewStdIOServer(strings.NewReader(""), out)

	// Test listing resources
	listRequest := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	listRequest += "\n"
	listRequest += `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
	listRequest += `{"jsonrpc":"2.0","id":2,"method":"resources/list"}` + "\n"

	in := strings.NewReader(listRequest)
	server.in = in

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Run(ctx)
	}()

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop server
	cancel()

	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled {
			t.Errorf("Unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not shut down in time")
	}

	// Get the last response (resources/list)
	responses := strings.Split(strings.TrimSpace(out.String()), "\n")
	var listResponse Response
	if err := json.Unmarshal([]byte(responses[len(responses)-1]), &listResponse); err != nil {
		t.Fatalf("Failed to unmarshal list response: %v", err)
	}

	// Verify resource list
	result, ok := listResponse.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Expected map result for resource list")
	}

	resources, ok := result["resources"].([]interface{})
	if !ok {
		t.Fatal("Expected array of resources")
	}

	if len(resources) != 2 {
		t.Errorf("Expected 2 resources, got %d", len(resources))
	}
}
