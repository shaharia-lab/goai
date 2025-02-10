package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"io"
	"log"
	"os"
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
	baseServer, _ := NewBaseServer(UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)))
	server := NewStdIOServer(
		baseServer,
		strings.NewReader(""),
		&bytes.Buffer{},
	)

	if server == nil {
		t.Fatal("Expected non-nil server")
	}
	if server.in == nil {
		t.Error("Expected non-nil input reader")
	}
	if server.out == nil {
		t.Error("Expected non-nil output writer")
	}
}

func TestSendResponse(t *testing.T) {
	out := &bytes.Buffer{}
	baseServer, _ := NewBaseServer(UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)))
	server := NewStdIOServer(
		baseServer,
		strings.NewReader(""),
		out,
	)

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
	baseServer, _ := NewBaseServer(UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)))
	server := NewStdIOServer(
		baseServer,
		strings.NewReader(""),
		out,
	)

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
	baseServer, _ := NewBaseServer(UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)))
	server := NewStdIOServer(
		baseServer,
		strings.NewReader(""),
		out,
	)

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

	out := &bytes.Buffer{}
	baseServer, _ := NewBaseServer(UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)))
	server := NewStdIOServer(
		baseServer,
		newMockReader(messages),
		out,
	)

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
		if err != nil && !errors.Is(err, context.Canceled) {
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
			out := &bytes.Buffer{}
			baseServer, _ := NewBaseServer(UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)))
			server := NewStdIOServer(
				baseServer,
				strings.NewReader(strings.Join(tt.messages, "")),
				out,
			)

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
	baseServer, _ := NewBaseServer(UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)))
	server := NewStdIOServer(
		baseServer,
		strings.NewReader(""),
		out,
	)

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
		if err != nil && !errors.Is(err, context.Canceled) {
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

	// Convert the result to ListResourcesResult
	resultBytes, err := json.Marshal(listResponse.Result)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	var resourceResult ListResourcesResult
	if err := json.Unmarshal(resultBytes, &resourceResult); err != nil {
		t.Fatalf("Failed to unmarshal to ListResourcesResult: %v", err)
	}

	// Verify the resources
	if len(resourceResult.Resources) != 0 {
		t.Errorf("Expected 0 resources, got %d", len(resourceResult.Resources))
	}
}

func TestStdIOServerRequests(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedOutput string
	}{
		{
			name: "ping request",
			input: `{
				"method": "ping"
			}`,
			expectedOutput: `{"jsonrpc":"2.0","id":null,"result":{}}`,
		},
		{
			name: "tools list request",
			input: `{
				"method": "tools/list",
				"params": {"cursor": "optional-cursor-value"}
			}`,
			expectedOutput: `{"jsonrpc":"2.0","id":null,"result":{"tools":[]}}`,
		},
		{
			name: "prompts list request",
			input: `{
				"method": "prompts/list",
				"params": {}
			}`,
			expectedOutput: `{"jsonrpc":"2.0","id":null,"result":{"prompts":[]}}`,
		},
		{
			name: "resources list request",
			input: `{
				"method": "resources/list",
				"params": {}
			}`,
			expectedOutput: `{"jsonrpc":"2.0","id":null,"result":{"resources":[]}}`,
		},
		{
			name: "method not found request",
			input: `{
				"method": "resources/templates/list",
				"params": {}
			}`,
			expectedOutput: `{"jsonrpc":"2.0","id":null,"error":{"code":-32601,"message":"Method not found"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture output
			var out bytes.Buffer

			baseServer, _ := NewBaseServer(UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)))
			server := NewStdIOServer(
				baseServer,
				strings.NewReader(tt.input+"\n"),
				&out,
			)

			// Process the request
			var request Request
			if err := json.Unmarshal([]byte(tt.input), &request); err != nil {
				t.Fatalf("Failed to unmarshal request: %v", err)
			}

			// Handle the request
			server.handleRequest("test-client", &request)

			// Get output and clean it up (remove newline)
			got := strings.TrimSpace(out.String())

			// Compare with expected output
			if got != tt.expectedOutput {
				t.Errorf("Expected output %s, got %s", tt.expectedOutput, got)
			}
		})
	}
}

func TestStdIOServerRequestsWithToolsMethod(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		tools          []ToolHandler
		expectedOutput string
	}{
		{
			name: "tools list request",
			input: `{
				"method": "tools/list",
				"params": {"cursor": "optional-cursor-value"}
			}`,
			tools: []ToolHandler{
				NewWeatherTool("get_weather", "Get the current weather for a given location.", json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						}
					},
					"required": ["location"]
				}`)),
			},
			expectedOutput: `{"jsonrpc":"2.0","id":null,"result":{"tools":[{"name":"get_weather","description":"Get the current weather for a given location.","inputSchema":{"type":"object","properties":{"location":{"type":"string","description":"The city and state, e.g. San Francisco, CA"}},"required":["location"]}}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			tm, err := NewToolManager(tt.tools)
			require.NoError(t, err, "Failed to create ToolManager")
			require.NotNil(t, tm, "ToolManager should not be nil")

			baseServer, _ := NewBaseServer(
				UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)),
				UseTools(tm),
			)
			server := NewStdIOServer(
				baseServer,
				strings.NewReader(tt.input+"\n"),
				&out,
			)

			// Process the request
			var request Request
			if err := json.Unmarshal([]byte(tt.input), &request); err != nil {
				t.Fatalf("Failed to unmarshal request: %v", err)
			}

			// Handle the request
			server.handleRequest("test-client", &request)

			// Get output and clean it up (remove newline)
			got := strings.TrimSpace(out.String())

			// Compare with expected output
			if got != tt.expectedOutput {
				t.Errorf("Expected output %s, got %s", tt.expectedOutput, got)
			}
		})
	}
}

// An example tool implementation
type WeatherTool struct {
	name        string
	description string
	inputSchema json.RawMessage
}

func NewWeatherTool(name, description string, inputSchema json.RawMessage) *WeatherTool {
	return &WeatherTool{
		name:        name,
		description: description,
		inputSchema: inputSchema,
	}
}

func (wt *WeatherTool) GetName() string {
	return wt.name
}

func (wt *WeatherTool) GetDescription() string {
	return wt.description
}

func (wt *WeatherTool) GetInputSchema() json.RawMessage {
	return wt.inputSchema
}

func (wt *WeatherTool) Handler(params CallToolParams) (CallToolResult, error) {
	// Parse input
	var input struct {
		Location string `json:"location"`
	}
	if err := json.Unmarshal(params.Arguments, &input); err != nil {
		return CallToolResult{}, err
	}

	// Return result
	return CallToolResult{
		Content: []ToolResultContent{
			{
				Type: "text",
				Text: fmt.Sprintf("Weather in %s: Sunny, 72°F", input.Location),
			},
		},
	}, nil
}

func TestHandlePromptGet(t *testing.T) {
	tests := []struct {
		name           string
		initFirst      bool
		input          string
		expectedID     string
		expectedError  *Error
		expectedResult map[string]interface{}
	}{
		{
			name:       "valid prompt get request",
			initFirst:  true,
			input:      `{"jsonrpc": "2.0", "method": "prompts/get", "id": 1, "params": {"name": "code_review", "arguments": {"language": "go", "code": "test code", "focus_areas": "test"}}}`,
			expectedID: "1",
			expectedResult: map[string]interface{}{
				"name": "code_review",
				"messages": []interface{}{
					map[string]interface{}{
						"role": "user",
						"content": map[string]interface{}{
							"type": "text",
							"text": "Please review this code:\nlanguage: go\ncode: test code\nfocus_areas: test\n",
						},
					},
				},
			},
		},
		{
			name:       "request without initialization",
			initFirst:  false,
			input:      `{"jsonrpc": "2.0", "method": "prompts/get", "id": 1, "params": {"id": "test-prompt-1"}}`,
			expectedID: "1",
			expectedError: &Error{
				Code:    -32000,
				Message: "Server not initialized",
			},
		},
		{
			name:      "malformed request",
			initFirst: false,
			input:     `{"jsonrpc": "2.0", "method": "prompts/get", "id": }`,
			expectedError: &Error{
				Code:    -32700,
				Message: "Parse error",
			},
		},
		{
			name:       "get prompt with invalid name",
			initFirst:  true,
			input:      `{"jsonrpc": "2.0", "method": "prompts/get", "id": 1, "params": {"name": "nonexistent_prompt"}}`,
			expectedID: "1",
			expectedError: &Error{
				Code:    -32602,
				Message: "Prompt not found",
			},
		},
		{
			name:       "get prompt without name parameter",
			initFirst:  true,
			input:      `{"jsonrpc": "2.0", "method": "prompts/get", "id": 1, "params": {}}`,
			expectedID: "1",
			expectedError: &Error{
				Code:    -32602,
				Message: "Prompt not found",
			},
		},
		{
			name:       "get prompt with null params",
			initFirst:  true,
			input:      `{"jsonrpc": "2.0", "method": "prompts/get", "id": 1, "params": null}`,
			expectedID: "1",
			expectedError: &Error{
				Code:    -32602,
				Message: "Prompt not found",
			},
		},
		{
			name:       "get prompt with invalid params type",
			initFirst:  true,
			input:      `{"jsonrpc": "2.0", "method": "prompts/get", "id": 1, "params": "code_review"}`,
			expectedID: "1",
			expectedError: &Error{
				Code:    -32602,
				Message: "Invalid params",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inR, inW := io.Pipe()
			out := &bytes.Buffer{}

			codeReviewPrompt := Prompt{
				Name: "code_review",
				Messages: []PromptMessage{
					{
						Role: "user",
						Content: PromptContent{
							Type: "text",
							Text: "Please review this code:",
						},
					},
				},
				Arguments: []PromptArgument{
					{Name: "language", Required: true},
					{Name: "code", Required: true},
					{Name: "focus_areas", Required: true},
				},
			}

			// Initialize PromptManager with the prompt
			pm, _ := NewPromptManager([]Prompt{codeReviewPrompt})

			// Add the PromptManager when creating the server
			baseServer, _ := NewBaseServer(
				UseLogger(log.New(io.Discard, "", 0)),
				UsePrompts(pm),
			)

			server := NewStdIOServer(
				baseServer,
				inR,
				out,
			)

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			go server.Run(ctx)

			if tt.initFirst {
				// Initialize the server
				initRequest := `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`
				_, err := fmt.Fprintln(inW, initRequest)
				require.NoError(t, err)

				_, err = fmt.Fprintln(inW, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
				require.NoError(t, err)

				resp := waitForResponse(t, out, 100*time.Millisecond)
				require.NotNil(t, resp)
				require.Nil(t, resp.Error)
				out.Reset()
			}

			_, err := fmt.Fprintln(inW, tt.input)
			require.NoError(t, err)

			response := waitForResponse(t, out, 100*time.Millisecond)
			require.NotNil(t, response)

			if tt.expectedError != nil {
				require.NotNil(t, response.Error)
				require.Equal(t, tt.expectedError.Code, response.Error.Code)
				require.Equal(t, tt.expectedError.Message, response.Error.Message)
			} else {
				require.Nil(t, response.Error)
				require.Equal(t, "2.0", response.JSONRPC)
				require.Equal(t, tt.expectedID, string(*response.ID))

				result, ok := response.Result.(map[string]interface{})
				require.True(t, ok)
				require.Equal(t, tt.expectedResult, result)
			}

			inW.Close()
		})
	}
}
