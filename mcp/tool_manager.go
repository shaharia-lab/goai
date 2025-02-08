// tool_manager.go
package mcp

import (
	"fmt"
	"sync"
)

type Tool struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	Arguments   []ToolArgument         `json:"arguments"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type ToolArgument struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
}

type ToolManager struct {
	tools    map[string]*Tool
	handlers map[string]ToolHandler
	mu       sync.RWMutex
}

type ToolHandler func(args map[string]interface{}) (interface{}, error)

func NewToolManager() *ToolManager {
	return &ToolManager{
		tools:    make(map[string]*Tool),
		handlers: make(map[string]ToolHandler),
	}
}

func (tm *ToolManager) RegisterTool(tool *Tool, handler ToolHandler) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.tools[tool.ID]; exists {
		return fmt.Errorf("tool %s already registered", tool.ID)
	}

	tm.tools[tool.ID] = tool
	tm.handlers[tool.ID] = handler
	return nil
}

func (tm *ToolManager) ListTools() []*Tool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tools := make([]*Tool, 0, len(tm.tools))
	for _, t := range tm.tools {
		tools = append(tools, t)
	}
	return tools
}

func (tm *ToolManager) ExecuteTool(id string, args map[string]interface{}) (interface{}, error) {
	tm.mu.RLock()
	handler, exists := tm.handlers[id]
	tool := tm.tools[id]
	tm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("tool %s not found", id)
	}

	// Validate arguments
	for _, arg := range tool.Arguments {
		if arg.Required {
			if _, ok := args[arg.Name]; !ok {
				return nil, fmt.Errorf("required argument %s missing", arg.Name)
			}
		}
	}

	return handler(args)
}
