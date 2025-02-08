package mcp

import (
	"encoding/json"
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

	// Register test tools in specific order
	tools := []Tool{
		{Name: "tool1"},
		{Name: "tool2"},
		{Name: "tool3"},
		{Name: "tool4"},
	}

	// Register tools in order
	for _, tool := range tools {
		_ = tm.RegisterTool(tool, func(args json.RawMessage) (CallToolResult, error) {
			return CallToolResult{}, nil
		})
	}

	tests := []struct {
		name       string
		cursor     string
		limit      int
		wantTools  []string // Expected tool names in order
		wantCursor string
	}{
		{
			name:       "no cursor, default limit",
			cursor:     "",
			limit:      0,
			wantTools:  []string{"tool1", "tool2", "tool3", "tool4"},
			wantCursor: "",
		},
		{
			name:       "with cursor",
			cursor:     "tool2",
			limit:      2,
			wantTools:  []string{"tool3", "tool4"},
			wantCursor: "",
		},
		{
			name:       "limit larger than remaining items",
			cursor:     "tool3",
			limit:      10,
			wantTools:  []string{"tool4"},
			wantCursor: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tm.ListTools(tt.cursor, tt.limit)

			// Check length
			if len(result.Tools) != len(tt.wantTools) {
				t.Errorf("ListTools() returned %d tools, want %d",
					len(result.Tools), len(tt.wantTools))
			}

			// Check tool names in order
			for i, tool := range result.Tools {
				if i >= len(tt.wantTools) {
					break
				}
				if tool.Name != tt.wantTools[i] {
					t.Errorf("Tool at position %d: got %s, want %s",
						i, tool.Name, tt.wantTools[i])
				}
			}

			// Check cursor
			if result.NextCursor != tt.wantCursor {
				t.Errorf("ListTools() NextCursor = %v, want %v",
					result.NextCursor, tt.wantCursor)
			}
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
