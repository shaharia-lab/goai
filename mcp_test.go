package goai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockTool implements MCPToolExecutor for testing
type MockTool struct {
	name        string
	description string
	version     string
	execFunc    func(ctx context.Context, input json.RawMessage) (MCPToolResponse, error)
}

func (t *MockTool) GetDefinition() MCPTool {
	return MCPTool{
		Name:        t.name,
		Description: t.description,
		Version:     t.version,
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"input": {"type": "string"}
			}
		}`),
	}
}

func (t *MockTool) Execute(ctx context.Context, input json.RawMessage) (MCPToolResponse, error) {
	if t.execFunc != nil {
		return t.execFunc(ctx, input)
	}
	return MCPToolResponse{
		Content: []MCPContentItem{{
			Type: "text",
			Text: "mock response",
		}},
	}, nil
}

func TestMCPToolRegistry_Register(t *testing.T) {
	tests := []struct {
		name      string
		tool      MCPToolExecutor
		wantError bool
	}{
		{
			name: "register new tool",
			tool: &MockTool{
				name:        "test_tool",
				description: "Test tool",
				version:     "1.0",
			},
			wantError: false,
		},
		{
			name: "register duplicate tool",
			tool: &MockTool{
				name:        "duplicate_tool",
				description: "Duplicate tool",
				version:     "1.0",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewMCPToolRegistry()
			err := registry.Register(tt.tool)

			if tt.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Verify tool was registered
			tool, err := registry.Get(context.Background(), tt.tool.GetDefinition().Name)
			assert.NoError(t, err)
			assert.Equal(t, tt.tool.GetDefinition(), tool.GetDefinition())

			// Try registering same tool again
			err = registry.Register(tt.tool)
			assert.Error(t, err, "should error on duplicate registration")
		})
	}
}

func TestMCPToolRegistry_Get(t *testing.T) {
	registry := NewMCPToolRegistry()
	mockTool := &MockTool{
		name:        "test_tool",
		description: "Test tool",
		version:     "1.0",
	}

	err := registry.Register(mockTool)
	require.NoError(t, err)

	tests := []struct {
		name      string
		toolName  string
		wantError bool
	}{
		{
			name:      "get existing tool",
			toolName:  "test_tool",
			wantError: false,
		},
		{
			name:      "get non-existent tool",
			toolName:  "missing_tool",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, err := registry.Get(context.Background(), tt.toolName)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, tool)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, tool)
			assert.Equal(t, tt.toolName, tool.GetDefinition().Name)
		})
	}
}

func TestMCPToolRegistry_ListTools(t *testing.T) {
	registry := NewMCPToolRegistry()

	// Register multiple tools
	tools := []*MockTool{
		{name: "tool1", description: "Tool 1", version: "1.0"},
		{name: "tool2", description: "Tool 2", version: "1.0"},
		{name: "tool3", description: "Tool 3", version: "1.0"},
	}

	for _, tool := range tools {
		err := registry.Register(tool)
		require.NoError(t, err)
	}

	// Get list of tools
	listedTools := registry.ListTools(context.Background())

	// Verify all tools are listed
	assert.Equal(t, len(tools), len(listedTools))

	// Verify tool details
	for i, tool := range tools {
		found := false
		for _, listed := range listedTools {
			if listed.Name == tool.name {
				found = true
				assert.Equal(t, tool.description, listed.Description)
				assert.Equal(t, tool.version, listed.Version)
				break
			}
		}
		assert.True(t, found, "Tool %d not found in listed tools", i)
	}
}

func TestMCPServer_HandleMCPListTools(t *testing.T) {
	registry := NewMCPToolRegistry()
	mockTool := &MockTool{
		name:        "test_tool",
		description: "Test tool",
		version:     "1.0",
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	server := NewMCPServer(registry, DefaultMCPServerConfig())

	// Create test request
	req := httptest.NewRequest("GET", "/tools/list", nil)
	w := httptest.NewRecorder()

	// Handle request
	server.HandleMCPListTools(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Parse response
	var response struct {
		Tools []MCPTool `json:"tools"`
	}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify response content
	assert.Len(t, response.Tools, 1)
	assert.Equal(t, mockTool.name, response.Tools[0].Name)
	assert.Equal(t, mockTool.description, response.Tools[0].Description)
	assert.Equal(t, mockTool.version, response.Tools[0].Version)
}

func TestMCPServer_HandleMCPToolCall(t *testing.T) {
	registry := NewMCPToolRegistry()
	mockTool := &MockTool{
		name:        "test_tool",
		description: "Test tool",
		version:     "1.0",
		execFunc: func(ctx context.Context, input json.RawMessage) (MCPToolResponse, error) {
			return MCPToolResponse{
				Content: []MCPContentItem{{
					Type: "text",
					Text: "Hello, MCP!",
				}},
			}, nil
		},
	}
	err := registry.Register(mockTool)
	require.NoError(t, err)

	server := NewMCPServer(registry, DefaultMCPServerConfig())

	tests := []struct {
		name       string
		input      string
		wantStatus int
		wantText   string
	}{
		{
			name: "valid tool call",
			input: `{
				"name": "test_tool",
				"arguments": {"input": "test"}
			}`,
			wantStatus: http.StatusOK,
			wantText:   "Hello, MCP!",
		},
		{
			name: "invalid tool name",
			input: `{
				"name": "missing_tool",
				"arguments": {"input": "test"}
			}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid json",
			input:      `{invalid json}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/tools/call", strings.NewReader(tt.input))
			w := httptest.NewRecorder()

			server.HandleMCPToolCall(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantStatus == http.StatusOK {
				var response MCPToolResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				assert.Len(t, response.Content, 1)
				assert.Equal(t, tt.wantText, response.Content[0].Text)
			}
		})
	}
}

func TestMCPServer_Shutdown(t *testing.T) {
	registry := NewMCPToolRegistry()
	server := NewMCPServer(registry, DefaultMCPServerConfig())

	// Create a test HTTP server
	httpServer := &http.Server{
		Addr:    ":0", // Use any available port
		Handler: http.NewServeMux(),
	}
	server.WithHTTPServer(httpServer)

	// Start server in goroutine
	go func() {
		_ = httpServer.ListenAndServe()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestMCPServer_GetMCPHandlers(t *testing.T) {
	registry := NewMCPToolRegistry()
	server := NewMCPServer(registry, DefaultMCPServerConfig())

	handlers := server.GetMCPHandlers()

	assert.NotNil(t, handlers.ListTools)
	assert.NotNil(t, handlers.ToolCall)
}

func TestMCPServer_AddMCPHandlers(t *testing.T) {
	registry := NewMCPToolRegistry()
	server := NewMCPServer(registry, DefaultMCPServerConfig())

	mux := http.NewServeMux()
	server.AddMCPHandlers(context.Background(), mux)

	// Test list tools endpoint
	req := httptest.NewRequest("GET", "/tools/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test tool call endpoint
	req = httptest.NewRequest("POST", "/tools/call", strings.NewReader(`{
		"name": "test_tool",
		"arguments": {"input": "test"}
	}`))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Should be 404 since no tools are registered
	assert.Equal(t, http.StatusNotFound, w.Code)
}
