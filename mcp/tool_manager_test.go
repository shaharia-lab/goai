package mcp

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
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
		MockToolHandler{name: "test-tool"},
	}

	tm := NewToolManager(tools)
	assert.NotNil(t, tm, "NewToolManager returned nil")
	assert.NotNil(t, tm.tools, "tools slice was not initialized")
	assert.NotNil(t, tm.toolImplementations, "toolImplementations map was not initialized")
	assert.Len(t, tm.tools, 1, "tools slice should contain one tool")
}

func TestListTools(t *testing.T) {
	tools := []ToolHandler{
		MockToolHandler{name: "d_tool"},
		MockToolHandler{name: "a_tool"},
		MockToolHandler{name: "c_tool"},
		MockToolHandler{name: "b_tool"},
	}

	tm := NewToolManager(tools)

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
			name:    "test-tool",
			handler: successHandler,
		},
	}

	tm := NewToolManager(tools)

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
		MockToolHandler{name: "existing-tool"},
	}

	tm := NewToolManager(tools)

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
	tm := NewToolManager(nil) // or []ToolHandler{}
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
