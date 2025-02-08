package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
)

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
// ListTools returns a list of all available tools, with optional pagination.
func (tm *ToolManager) ListTools(cursor string, limit int) ListToolsResult {
	if limit <= 0 {
		limit = 50 // Default limit
	}

	// Convert map to slice and sort by name for consistent ordering
	allTools := make([]Tool, 0, len(tm.tools))
	for _, t := range tm.tools {
		allTools = append(allTools, t)
	}
	sort.Slice(allTools, func(i, j int) bool {
		return allTools[i].Name < allTools[j].Name
	})

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

	var nextCursor string
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
