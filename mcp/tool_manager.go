package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolExecutor defines the interface for tool execution
type ToolExecutor interface {
	Execute(ctx context.Context, params json.RawMessage) (interface{}, error)
	GetMetadata() ToolMetadata
}

// ToolMetadata contains information about a tool
type ToolMetadata struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
}

// ToolManager manages tool registration and execution
type ToolManager struct {
	tools map[string]ToolExecutor
	mu    sync.RWMutex
}

// NewToolManager creates a new tool manager
func NewToolManager() *ToolManager {
	return &ToolManager{
		tools: make(map[string]ToolExecutor),
	}
}

// RegisterTool registers a new tool
func (tm *ToolManager) RegisterTool(tool ToolExecutor) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	metadata := tool.GetMetadata()
	if _, exists := tm.tools[metadata.Name]; exists {
		return fmt.Errorf("tool %s already registered", metadata.Name)
	}

	tm.tools[metadata.Name] = tool
	return nil
}

// ExecuteTool executes a tool with the given parameters
func (tm *ToolManager) ExecuteTool(ctx context.Context, name string, params json.RawMessage) (interface{}, error) {
	tm.mu.RLock()
	tool, exists := tm.tools[name]
	tm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	return tool.Execute(ctx, params)
}

// ListTools returns metadata for all registered tools
func (tm *ToolManager) ListTools() []ToolMetadata {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tools := make([]ToolMetadata, 0, len(tm.tools))
	for _, tool := range tm.tools {
		tools = append(tools, tool.GetMetadata())
	}
	return tools
}
