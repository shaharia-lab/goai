package mcp

import (
	"encoding/json"
	"fmt"
	"github.com/xeipuuv/gojsonschema"
	"sort"
	"strings"
)

// ToolManager handles tool-related operations.
type ToolManager struct {
	tools               map[string]Tool
	toolImplementations map[string]ToolImplementation
}

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

	var names []string
	for name := range tm.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	startIdx := 0
	if cursor != "" {
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

	var pageTools []Tool
	for i := startIdx; i < endIdx; i++ {
		pageTools = append(pageTools, tm.tools[names[i]])
	}

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

	tool, exists := tm.tools[params.Name]
	if !exists {
		return CallToolResult{}, fmt.Errorf("tool metadata not found: %s", params.Name)
	}

	if tool.InputSchema != nil && len(params.Arguments) > 0 {
		// Create schema loader
		schemaLoader := gojsonschema.NewStringLoader(string(tool.InputSchema))

		// Convert arguments to JSON string
		argsJSON, err := json.Marshal(params.Arguments)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("failed to marshal arguments: %v", err)
		}

		// Create document loader for the arguments
		documentLoader := gojsonschema.NewStringLoader(string(argsJSON))

		// Perform validation
		result, err := gojsonschema.Validate(schemaLoader, documentLoader)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("validation error: %v", err)
		}

		if !result.Valid() {
			// Collect all validation errors
			var errMsgs []string
			for _, desc := range result.Errors() {
				errMsgs = append(errMsgs, desc.String())
			}
			return CallToolResult{
				IsError: true,
				Content: []ToolResultContent{{
					Type: "text",
					Text: fmt.Sprintf("Schema validation failed: %s", strings.Join(errMsgs, "; ")),
				}},
			}, nil
		}
	}

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
