package mcp

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"log"
	"os"
	"testing"
)

func TestListTools(t *testing.T) {
	tools := []Tool{
		{
			Name:        "d_tool",
			Description: "test description",
			InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						}
					},
					"required": ["location"]
				}`),
			Handler: func(ctx context.Context, params CallToolParams) (CallToolResult, error) {
				return CallToolResult{}, nil
			},
		},
		{
			Name:        "a_tool",
			Description: "test description",
			InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						}
					},
					"required": ["location"]
				}`),
			Handler: func(ctx context.Context, params CallToolParams) (CallToolResult, error) {
				return CallToolResult{}, nil
			},
		},
		{
			Name:        "c_tool",
			Description: "test description",
			InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						}
					},
					"required": ["location"]
				}`),
			Handler: func(ctx context.Context, params CallToolParams) (CallToolResult, error) {
				return CallToolResult{}, nil
			},
		},
		{
			Name:        "b_tool",
			Description: "test description",
			InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						}
					},
					"required": ["location"]
				}`),
			Handler: func(ctx context.Context, params CallToolParams) (CallToolResult, error) {
				return CallToolResult{}, nil
			},
		},
	}

	baseServer, _ := NewBaseServer(
		UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)),
	)
	err := baseServer.AddTools(tools...)
	assert.NoError(t, err)

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
			result := baseServer.ListTools(tt.cursor, tt.limit)

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

func TestToolManager_AddTool(t *testing.T) {
	tests := []struct {
		name    string
		tool    Tool
		wantErr bool
	}{
		{
			name: "valid tool",
			tool: Tool{
				Name:        "test-tool",
				Description: "test description",
				InputSchema: json.RawMessage(`{}`),
				Handler: func(ctx context.Context, params CallToolParams) (CallToolResult, error) {
					return CallToolResult{}, nil
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate tool",
			tool: Tool{
				Name:        "test-tool",
				Description: "test description",
				InputSchema: json.RawMessage(`{}`),
				Handler: func(ctx context.Context, params CallToolParams) (CallToolResult, error) {
					return CallToolResult{}, nil
				},
			},
			wantErr: true,
		},
		{
			name: "invalid tool - no description",
			tool: Tool{
				Name:        "invalid-tool",
				Description: "",
				InputSchema: json.RawMessage(`{}`),
				Handler: func(ctx context.Context, params CallToolParams) (CallToolResult, error) {
					return CallToolResult{}, nil
				},
			},
			wantErr: true,
		},
	}

	baseServer, _ := NewBaseServer(
		UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)),
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := baseServer.AddTools(tt.tool)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
