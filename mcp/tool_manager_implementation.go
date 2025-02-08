package mcp

import (
	"encoding/json"
	"fmt"
)

// Tool represents a callable tool in the MCP system.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolResultContent represents the content returned by a tool.
type ToolResultContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// CallToolParams represents parameters for calling a tool.
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// CallToolResult represents the result of calling a tool.
type CallToolResult struct {
	Content []ToolResultContent `json:"content"`
	IsError bool                `json:"isError"`
}

// ListToolsResult represents the result of listing available tools.
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// ToolManager handles tool-related operations.
type ToolManager struct {
	tools               map[string]Tool
	toolImplementations map[string]ToolImplementation
}

// ToolImplementation represents the actual implementation of a tool.
type ToolImplementation func(args json.RawMessage) (CallToolResult, error)

// NewToolManager creates a new ToolManager instance.
func NewToolManager() *ToolManager {
	return &ToolManager{
		tools:               make(map[string]Tool),
		toolImplementations: make(map[string]ToolImplementation),
	}
}

// RegisterTool registers a new tool with its implementation.
func (tm *ToolManager) RegisterTool(tool Tool, implementation ToolImplementation) error {
	if tool.Name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}

	if implementation == nil {
		return fmt.Errorf("tool implementation cannot be nil")
	}

	tm.tools[tool.Name] = tool
	tm.toolImplementations[tool.Name] = implementation
	return nil
}

// ListTools returns a list of all available tools, with optional pagination.
func (tm *ToolManager) ListTools(cursor string, limit int) ListToolsResult {
	if limit <= 0 {
		limit = 50 // Default limit
	}

	var nextCursor string

	// Convert map to slice for pagination
	allTools := make([]Tool, 0, len(tm.tools))
	for _, t := range tm.tools {
		allTools = append(allTools, t)
	}

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, t := range allTools {
			if t.Name == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Get tools for current page
	endIdx := startIdx + limit
	if endIdx > len(allTools) {
		endIdx = len(allTools)
	}

	if endIdx < len(allTools) {
		nextCursor = allTools[endIdx-1].Name
	}

	return ListToolsResult{
		Tools:      allTools[startIdx:endIdx],
		NextCursor: nextCursor,
	}
}

// CallTool executes a tool with the given parameters.
func (tm *ToolManager) CallTool(params CallToolParams) (CallToolResult, error) {
	implementation, exists := tm.toolImplementations[params.Name]
	if !exists {
		return CallToolResult{}, fmt.Errorf("tool not found: %s", params.Name)
	}

	// Validate tool exists
	tool, exists := tm.tools[params.Name]
	if !exists {
		return CallToolResult{}, fmt.Errorf("tool metadata not found: %s", params.Name)
	}

	// Validate arguments against schema if provided
	if tool.InputSchema != nil && len(params.Arguments) > 0 {
		// Note: Implement schema validation here if needed
	}

	// Call the tool implementation
	result, err := implementation(params.Arguments)
	if err != nil {
		return CallToolResult{
			IsError: true,
			Content: []ToolResultContent{{
				Type: "text",
				Text: err.Error(),
			}},
		}, nil
	}

	return result, nil
}

// GetTool retrieves a tool by its name.
func (tm *ToolManager) GetTool(name string) (Tool, error) {
	tool, exists := tm.tools[name]
	if !exists {
		return Tool{}, fmt.Errorf("tool not found: %s", name)
	}
	return tool, nil
}
