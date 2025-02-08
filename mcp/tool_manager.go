package mcp

import (
	"fmt"
	"sort"
)

// ToolManager handles tool-related operations.
type ToolManager struct {
	tools               map[string]Tool
	toolImplementations map[string]ToolImplementation
}

// ToolImplementation represents the actual implementation of a tool.

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
		limit = 50
	}

	// Get all tool names and sort them
	var names []string
	for name := range tm.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	// Find starting index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, name := range names {
			if name == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Calculate end index
	endIdx := startIdx + limit
	if endIdx > len(names) {
		endIdx = len(names)
	}

	// Get the slice of tools for this page
	var pageTools []Tool
	for i := startIdx; i < endIdx; i++ {
		pageTools = append(pageTools, tm.tools[names[i]])
	}

	// Set next cursor
	var nextCursor string
	if endIdx < len(names) {
		nextCursor = names[endIdx-1]
	}

	return ListToolsResult{
		Tools:      pageTools,
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
