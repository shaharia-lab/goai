package mcp

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewToolManager(t *testing.T) {
	tm := NewToolManager()
	if tm == nil {
		t.Error("NewToolManager returned nil")
	}
	if tm.tools == nil {
		t.Error("tools map was not initialized")
	}
	if tm.toolImplementations == nil {
		t.Error("toolImplementations map was not initialized")
	}
}

func TestRegisterTool(t *testing.T) {
	tm := NewToolManager()

	tests := []struct {
		name           string
		tool           Tool
		implementation ToolImplementation
		wantErr        bool
	}{
		{
			name: "valid registration",
			tool: Tool{Name: "test-tool"},
			implementation: func(args json.RawMessage) (CallToolResult, error) {
				return CallToolResult{}, nil
			},
			wantErr: false,
		},
		{
			name: "empty tool name",
			tool: Tool{Name: ""},
			implementation: func(args json.RawMessage) (CallToolResult, error) {
				return CallToolResult{}, nil
			},
			wantErr: true,
		},
		{
			name:           "nil implementation",
			tool:           Tool{Name: "test-tool"},
			implementation: nil,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tm.RegisterTool(tt.tool, tt.implementation)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterTool() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				// Verify tool was registered
				if _, exists := tm.tools[tt.tool.Name]; !exists {
					t.Error("tool was not added to tools map")
				}
				if _, exists := tm.toolImplementations[tt.tool.Name]; !exists {
					t.Error("implementation was not added to toolImplementations map")
				}
			}
		})
	}
}

func TestListTools(t *testing.T) {
	tm := NewToolManager()

	// Register test tools in a way that would expose ordering issues
	tools := []Tool{
		{Name: "d_tool"},
		{Name: "a_tool"},
		{Name: "c_tool"},
		{Name: "b_tool"},
	}

	// Register tools
	for _, tool := range tools {
		_ = tm.RegisterTool(tool, func(args json.RawMessage) (CallToolResult, error) {
			return CallToolResult{}, nil
		})
	}

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
			wantCursor: "c_tool",
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

			// Check length
			assert.Len(t, result.Tools, len(tt.wantTools),
				"Expected %d tools, got %d", len(tt.wantTools), len(result.Tools))

			// Check tool names in order
			for i, tool := range result.Tools {
				assert.Equal(t, tt.wantTools[i], tool.Name,
					"Tool at position %d: expected %s, got %s",
					i, tt.wantTools[i], tool.Name)
			}

			// Check cursor
			assert.Equal(t, tt.wantCursor, result.NextCursor,
				"Expected next cursor to be %q, got %q",
				tt.wantCursor, result.NextCursor)
		})
	}
}

func TestCallTool(t *testing.T) {
	tm := NewToolManager()

	// Register a test tool
	testTool := Tool{Name: "test-tool"}
	testImpl := func(args json.RawMessage) (CallToolResult, error) {
		return CallToolResult{
			Content: []ToolResultContent{{
				Type: "text",
				Text: "success",
			}},
		}, nil
	}

	_ = tm.RegisterTool(testTool, testImpl)

	tests := []struct {
		name    string
		params  CallToolParams
		want    string
		wantErr bool
	}{
		{
			name: "valid tool call",
			params: CallToolParams{
				Name:      "test-tool",
				Arguments: json.RawMessage(`{}`),
			},
			want:    "success",
			wantErr: false,
		},
		{
			name: "non-existent tool",
			params: CallToolParams{
				Name:      "invalid-tool",
				Arguments: json.RawMessage(`{}`),
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tm.CallTool(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("CallTool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(result.Content) > 0 && result.Content[0].Text != tt.want {
				t.Errorf("CallTool() result = %v, want %v", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestGetTool(t *testing.T) {
	tm := NewToolManager()

	testTool := Tool{Name: "test-tool"}
	_ = tm.RegisterTool(testTool, func(args json.RawMessage) (CallToolResult, error) {
		return CallToolResult{}, nil
	})

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
			toolName: "invalid-tool",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, err := tm.GetTool(tt.toolName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tool.Name != tt.toolName {
				t.Errorf("GetTool() returned tool name = %v, want %v", tool.Name, tt.toolName)
			}
		})
	}
}
