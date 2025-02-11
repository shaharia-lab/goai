package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

// ToolManager handles tool-related operations.
type ToolManager struct {
	tools               []ToolHandler
	toolImplementations map[string]ToolImplementation
}

// NewToolManager creates a new ToolManager instance.
func NewToolManager(tools []ToolHandler) ToolManager {
	return ToolManager{
		tools:               tools,
		toolImplementations: make(map[string]ToolImplementation),
	}
}

// ListTools returns a list of all available tools, with optional pagination.
func (tm *ToolManager) ListTools(cursor string, limit int) ListToolsResult {
	if limit <= 0 {
		limit = 50
	}

	// Create a map for easier tool lookup
	toolMap := make(map[string]Tool)
	var names []string
	for _, t := range tm.tools {
		name := t.GetName()
		names = append(names, name)
		toolMap[name] = Tool{
			Name:        t.GetName(),
			Description: t.GetDescription(),
			InputSchema: t.GetInputSchema(),
		}
	}
	sort.Strings(names)

	startIdx := 0
	if cursor != "" {
		// Find the cursor position
		for i, name := range names {
			if name == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + limit
	if endIdx > len(names) {
		endIdx = len(names)
	}

	// Get the page of tools
	pageTools := make([]Tool, 0)
	for i := startIdx; i < endIdx; i++ {
		if tool, exists := toolMap[names[i]]; exists {
			pageTools = append(pageTools, tool)
		}
	}

	var nextCursor string
	if endIdx < len(names) {
		nextCursor = names[endIdx] // Use the next item's name as cursor
	}

	return ListToolsResult{
		Tools:      pageTools,
		NextCursor: nextCursor,
	}
}

// CallTool executes a tool with the given parameters.
func (tm *ToolManager) CallTool(params CallToolParams) (CallToolResult, error) {
	var targetTool ToolHandler
	found := false

	for _, tool := range tm.tools {
		if tool.GetName() == params.Name {
			targetTool = tool
			found = true
			break
		}
	}

	if !found {
		return CallToolResult{}, fmt.Errorf("tool metadata not found: %s", params.Name)
	}

	if targetTool.GetInputSchema() != nil && len(params.Arguments) > 0 {
		schemaLoader := gojsonschema.NewStringLoader(string(targetTool.GetInputSchema()))

		argsJSON, err := json.Marshal(params.Arguments)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("failed to marshal arguments: %v", err)
		}

		documentLoader := gojsonschema.NewStringLoader(string(argsJSON))

		result, err := gojsonschema.Validate(schemaLoader, documentLoader)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("validation error: %v", err)
		}

		if !result.Valid() {
			var errorMessages []string
			for _, desc := range result.Errors() {
				errorMessages = append(errorMessages, desc.String())
			}

			return CallToolResult{
				IsError: true,
				Content: []ToolResultContent{{
					Type: "text",
					Text: fmt.Sprintf("Schema validation failed: %s", strings.Join(errorMessages, "; ")),
				}},
			}, nil
		}
	}

	result, err := targetTool.Handler(params)
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
func (tm *ToolManager) GetTool(name string) (ToolHandler, error) {
	for _, tool := range tm.tools {
		if tool.GetName() == name {
			return tool, nil
		}
	}

	return nil, fmt.Errorf("tool not found: %s", name)
}
