package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockToolHandler implements ToolHandler interface for testing
// MockToolHandler implements ToolHandler interface for testing
type MockToolHandler struct {
	name        string
	description string
	inputSchema json.RawMessage
	handler     func(params CallToolParams) (CallToolResult, error)
}

func (m MockToolHandler) GetName() string                 { return m.name }
func (m MockToolHandler) GetDescription() string          { return m.description }
func (m MockToolHandler) GetInputSchema() json.RawMessage { return m.inputSchema }
func (m MockToolHandler) Handler(params CallToolParams) (CallToolResult, error) {
	return m.handler(params)
}

func TestNewToolManager(t *testing.T) {
	tools := []ToolHandler{
		MockToolHandler{name: "test-tool", description: "test description", inputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						}
					},
					"required": ["location"]
				}`)},
	}

	tm, err := NewToolManager(tools)
	assert.NoError(t, err, "NewToolManager returned an error")
	assert.NotNil(t, tm, "NewToolManager returned nil")
	assert.NotNil(t, tm.tools, "tools slice was not initialized")
	assert.Len(t, tm.tools, 1, "tools slice should contain one tool")
}

func TestListTools(t *testing.T) {
	tools := []ToolHandler{
		MockToolHandler{name: "d_tool", description: "test description", inputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						}
					},
					"required": ["location"]
				}`)},
		MockToolHandler{name: "a_tool", description: "test description", inputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						}
					},
					"required": ["location"]
				}`)},
		MockToolHandler{name: "c_tool", description: "test description", inputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						}
					},
					"required": ["location"]
				}`)},
		MockToolHandler{name: "b_tool", description: "test description", inputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						}
					},
					"required": ["location"]
				}`)},
	}

	tm, _ := NewToolManager(tools)

	tests := []struct {
		name       string
		cursor     string
		limit      int
		wantTools  []string // Expected tool names in alphabetical order
		wantCursor string
	}{
		{
			name:       "no cursor, default limit",
			cursor:     "",
			limit:      0,
			wantTools:  []string{"a_tool", "b_tool", "c_tool", "d_tool"},
			wantCursor: "",
		},
		{
			name:       "with cursor",
			cursor:     "b_tool",
			limit:      2,
			wantTools:  []string{"c_tool", "d_tool"},
			wantCursor: "",
		},
		{
			name:       "with cursor and limit",
			cursor:     "a_tool",
			limit:      2,
			wantTools:  []string{"b_tool", "c_tool"},
			wantCursor: "d_tool",
		},
		{
			name:       "limit larger than remaining items",
			cursor:     "c_tool",
			limit:      10,
			wantTools:  []string{"d_tool"},
			wantCursor: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tm.ListTools(tt.cursor, tt.limit)

			assert.Len(t, result.Tools, len(tt.wantTools),
				"Expected %d tools, got %d", len(tt.wantTools), len(result.Tools))

			for i, tool := range result.Tools {
				assert.Equal(t, tt.wantTools[i], tool.Name,
					"Tool at position %d: expected %s, got %s",
					i, tt.wantTools[i], tool.Name)
			}

			assert.Equal(t, tt.wantCursor, result.NextCursor,
				"Expected next cursor to be %q, got %q",
				tt.wantCursor, result.NextCursor)
		})
	}
}

func TestCallTool(t *testing.T) {
	successHandler := func(params CallToolParams) (CallToolResult, error) {
		return CallToolResult{
			Content: []ToolResultContent{{
				Type: "text",
				Text: "success",
			}},
		}, nil
	}

	tools := []ToolHandler{
		MockToolHandler{
			name:        "test-tool",
			description: "test description",
			inputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"location": {
						"type": "string",
						"description": "The city and state, e.g. San Francisco, CA"
					}
				},	
				"required": ["location"]	
			}`),
			handler: successHandler,
		},
	}

	tm, err := NewToolManager(tools)
	assert.NoError(t, err)

	tests := []struct {
		name    string
		params  CallToolParams
		want    CallToolResult
		wantErr bool
	}{
		{
			name: "valid tool call",
			params: CallToolParams{
				Name: "test-tool",
			},
			want: CallToolResult{
				Content: []ToolResultContent{{
					Type: "text",
					Text: "success",
				}},
			},
			wantErr: false,
		},
		{
			name: "unknown tool",
			params: CallToolParams{
				Name: "unknown-tool",
			},
			want:    CallToolResult{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tm.CallTool(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestGetTool(t *testing.T) {
	tools := []ToolHandler{
		MockToolHandler{
			name:        "existing-tool",
			description: "test description",
			inputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"location": {
						"type": "string",
						"description": "The city and state, e.g. San Francisco, CA"
					}
				},	
				"required": ["location"]	
			}`),
		},
	}

	tm, _ := NewToolManager(tools)

	tests := []struct {
		name     string
		toolName string
		wantErr  bool
	}{
		{
			name:     "existing tool",
			toolName: "existing-tool",
			wantErr:  false,
		},
		{
			name:     "non-existing tool",
			toolName: "non-existing-tool",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, err := tm.GetTool(tt.toolName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, tool)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tool)
				assert.Equal(t, tt.toolName, tool.GetName())
			}
		})
	}
}

func TestListToolsWithNoTools(t *testing.T) {
	tm, _ := NewToolManager(nil) // or []ToolHandler{}
	result := tm.ListTools("", 10)

	if result.Tools == nil {
		t.Error("Expected empty slice, got nil")
	}
	if len(result.Tools) != 0 {
		t.Error("Expected 0 tools, got", len(result.Tools))
	}
	if result.NextCursor != "" {
		t.Error("Expected empty cursor, got", result.NextCursor)
	}
}

func TestNewToolManagerValidation(t *testing.T) {
	tests := []struct {
		name    string
		tools   []ToolHandler
		wantErr string
	}{
		{
			name: "empty tool name",
			tools: []ToolHandler{
				MockToolHandler{
					name:        "",
					description: "test description",
				},
			},
			wantErr: "invalid tool: tool name cannot be empty",
		},
		{
			name: "empty tool description",
			tools: []ToolHandler{
				MockToolHandler{
					name:        "test-tool",
					description: "",
				},
			},
			wantErr: "invalid tool: tool description cannot be empty",
		},
		{
			name: "invalid json schema",
			tools: []ToolHandler{
				MockToolHandler{
					name:        "test-tool",
					description: "test description",
					inputSchema: json.RawMessage(`{invalid json`),
				},
			},
			wantErr: "invalid tool: invalid input schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm, err := NewToolManager(tt.tools)
			assert.Nil(t, tm)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestToolManager_AddTool(t *testing.T) {
	tm, err := NewToolManager([]ToolHandler{})
	assert.NoError(t, err)

	tests := []struct {
		name    string
		tool    MockToolHandler
		wantErr bool
	}{
		{
			name: "valid tool",
			tool: MockToolHandler{
				name:        "test-tool",
				description: "test description",
				handler: func(params CallToolParams) (CallToolResult, error) {
					return CallToolResult{}, nil
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate tool",
			tool: MockToolHandler{
				name:        "test-tool",
				description: "test description",
				handler: func(params CallToolParams) (CallToolResult, error) {
					return CallToolResult{}, nil
				},
			},
			wantErr: true,
		},
		{
			name: "invalid tool - no description",
			tool: MockToolHandler{
				name: "invalid-tool",
				handler: func(params CallToolParams) (CallToolResult, error) {
					return CallToolResult{}, nil
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tm.AddTool(tt.tool)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify tool was added
				tool, err := tm.GetTool(tt.tool.name)
				assert.NoError(t, err)
				assert.Equal(t, tt.tool.name, tool.GetName())
			}
		})
	}
}

func TestToolManager_RemoveTool(t *testing.T) {
	initialTool := MockToolHandler{
		name:        "test-tool",
		description: "test description",
		handler: func(params CallToolParams) (CallToolResult, error) {
			return CallToolResult{}, nil
		},
	}

	tm, err := NewToolManager([]ToolHandler{initialTool})
	assert.NoError(t, err)

	tests := []struct {
		name     string
		toolName string
		wantErr  bool
	}{
		{
			name:     "existing tool",
			toolName: "test-tool",
			wantErr:  false,
		},
		{
			name:     "non-existent tool",
			toolName: "nonexistent-tool",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tm.RemoveTool(tt.toolName)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify tool was removed
				_, err := tm.GetTool(tt.toolName)
				assert.Error(t, err)
			}
		})
	}
}
