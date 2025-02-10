package mcp

import (
	"encoding/json"
	"fmt"
	"github.com/xeipuuv/gojsonschema"
	"reflect"
	"sort"
	"strings"
)

// ToolManager handles tool-related operations.
type ToolManager struct {
	tools map[string]ToolHandler
}

// NewToolManager creates a new ToolManager instance.
func NewToolManager(tools []ToolHandler) (*ToolManager, error) {
	t := make(map[string]ToolHandler)

	for _, tool := range tools {
		err := validateTool(tool)
		if err != nil {
			return nil, fmt.Errorf("invalid tool: %v", err)
		}

		t[tool.GetName()] = tool
	}

	return &ToolManager{
		tools: t,
	}, nil
}

func validateTool(tool ToolHandler) error {
	if tool.GetName() == "" {
		return fmt.Errorf("tool name cannot be empty")
	}

	if tool.GetDescription() == "" {
		return fmt.Errorf("tool description cannot be empty")
	}

	if tool.GetInputSchema() != nil {
		loader := gojsonschema.NewStringLoader(string(tool.GetInputSchema()))
		_, err := gojsonschema.NewSchema(loader)
		if err != nil {
			return fmt.Errorf("invalid input schema: %v", err)
		}
	}

	if reflect.ValueOf(tool.Handler).IsNil() {
		return fmt.Errorf("tool handler cannot be nil")
	}

	return nil
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
		names = append(names, t.GetName())
		toolMap[t.GetName()] = Tool{
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
	if _, exists := tm.tools[params.Name]; !exists {
		return CallToolResult{}, fmt.Errorf("tool metadata not found: %s", params.Name)
	}

	if tm.tools[params.Name].GetInputSchema() != nil && len(params.Arguments) > 0 {
		schemaLoader := gojsonschema.NewStringLoader(string(tm.tools[params.Name].GetInputSchema()))

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

	result, err := tm.tools[params.Name].Handler(params)
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
	if _, exists := tm.tools[name]; !exists {
		return nil, fmt.Errorf("tool metadata not found: %s", name)
	}

	return tm.tools[name], nil
}

func (tm *ToolManager) AddTool(tool ToolHandler) error {
	if err := validateTool(tool); err != nil {
		return fmt.Errorf("invalid tool: %v", err)
	}

	if _, exists := tm.tools[tool.GetName()]; exists {
		return fmt.Errorf("tool with name '%s' already exists", tool.GetName())
	}

	tm.tools[tool.GetName()] = tool
	return nil
}

func (tm *ToolManager) RemoveTool(name string) error {
	if _, exists := tm.tools[name]; !exists {
		return fmt.Errorf("tool '%s' not found", name)
	}

	delete(tm.tools, name)
	return nil
}
