package goai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
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
		UseLogger(NewNullLogger()),
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
			result := baseServer.ListTools(context.Background(), tt.cursor, tt.limit)

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
		UseLogger(NewNullLogger()),
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

func TestValidatePrompt(t *testing.T) {
	t.Run("valid prompt", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Role: "system",
					Content: PromptContent{
						Type: "text",
						Text: "Hello, world!",
					},
				},
			},
			Arguments: []PromptArgument{
				{
					Name:     "arg1",
					Required: true,
				},
			},
		}
		err := validatePrompt(prompt)
		assert.NoError(t, err)
	})

	t.Run("empty prompt name", func(t *testing.T) {
		prompt := Prompt{
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "Hello!",
					},
				},
			},
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt name cannot be empty")
	})

	t.Run("empty messages", func(t *testing.T) {
		prompt := Prompt{
			Name:     "test-prompt",
			Messages: []PromptMessage{},
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt must have at least one message")
	})

	t.Run("unsupported content type", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "image",
						Text: "some text",
					},
				},
			},
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only text type is supported")
	})

	t.Run("empty content text", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "",
					},
				},
			},
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message content text cannot be empty")
	})

	t.Run("empty argument name", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "Hello!",
					},
				},
			},
			Arguments: []PromptArgument{
				{
					Name:     "",
					Required: true,
				},
			},
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "argument name cannot be empty")
	})

	t.Run("multiple messages validation", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "Message 1",
					},
				},
				{
					Content: PromptContent{
						Type: "text",
						Text: "", // Invalid
					},
				},
			},
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message content text cannot be empty")
	})

	t.Run("multiple valid arguments", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "Hello {{name}} and {{greeting}}!",
					},
				},
			},
			Arguments: []PromptArgument{
				{
					Name:     "name",
					Required: true,
				},
				{
					Name:     "greeting",
					Required: false,
				},
			},
		}
		err := validatePrompt(prompt)
		assert.NoError(t, err)
	})

	t.Run("nil messages", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt must have at least one message")
	})
}

func TestListResources(t *testing.T) {
	baseServer, _ := NewBaseServer(
		UseLogger(NewNullLogger()),
	)

	// Add resources in non-sequential order
	resources := []Resource{
		{URI: "d_res", Name: "d_res", MimeType: "text/plain"},
		{URI: "a_res", Name: "a_res", MimeType: "text/plain"},
		{URI: "c_res", Name: "c_res", MimeType: "text/plain"},
		{URI: "b_res", Name: "b_res", MimeType: "text/plain"},
	}

	err := baseServer.AddResources(resources...)
	assert.NoError(t, err)

	t.Run("list_with_cursor", func(t *testing.T) {
		ctx := context.Background()
		// First page
		result := baseServer.ListResources(ctx, "", 2)
		assert.Len(t, result.Resources, 2, "First page should have 2 resources")
		assert.Equal(t, "a_res", result.Resources[0].URI)
		assert.Equal(t, "b_res", result.Resources[1].URI)
		assert.Equal(t, "b_res", result.NextCursor)

		// Second page
		result = baseServer.ListResources(ctx, result.NextCursor, 2)
		assert.Len(t, result.Resources, 2, "Second page should have 2 resources")
		assert.Equal(t, "c_res", result.Resources[0].URI)
		assert.Equal(t, "d_res", result.Resources[1].URI)
		assert.Empty(t, result.NextCursor)
	})
}

func TestReadResource(t *testing.T) {

	textResource := Resource{
		URI:         "file://file.txt",
		Name:        "file.txt",
		MimeType:    "text/plain",
		TextContent: "Hello, World!",
	}

	binaryResource := Resource{
		URI:         "file://file.bin",
		Name:        "file.bin",
		MimeType:    "application/octet-stream",
		TextContent: "binary content",
	}

	baseServer, _ := NewBaseServer(
		UseLogger(NewNullLogger()),
	)
	err := baseServer.AddResources(textResource, binaryResource)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		params      ReadResourceParams
		expectError bool
		checkText   bool
	}{
		{
			name:        "read text resource",
			params:      ReadResourceParams{URI: "file://file.txt"},
			expectError: false,
			checkText:   true,
		},
		{
			name:        "read binary resource",
			params:      ReadResourceParams{URI: "file://file.bin"},
			expectError: false,
			checkText:   false,
		},
		{
			name:        "read non-existent resource",
			params:      ReadResourceParams{URI: "file://nonexistent.txt"},
			expectError: true,
			checkText:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := baseServer.ReadResource(context.Background(), tt.params)

			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
				return
			}

			if !tt.expectError {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				if len(result.Contents) != 1 {
					t.Errorf("expected 1 content, got %d", len(result.Contents))
					return
				}

				content := result.Contents[0]
				if tt.checkText {
					if content.Text == "" {
						t.Error("expected non-empty text content")
					}
				} else {
					if content.Blob == "" {
						t.Error("expected non-empty blob content")
					}
				}
			}
		})
	}
}
